package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/wiki"
	"github.com/Salvia95/graphin/internal/workspace"
)

// wsReader serves node bodies out of the running index.
//
// It exists so internal/wiki states what it needs rather than importing the
// engine: the wiki must stay usable from a process that never opens the
// index, and this adapter is the one place that assumption is spent.
type wsReader struct{ ws *workspace.Workspace }

func (r wsReader) Read(ids []string) []wiki.Block {
	out := make([]wiki.Block, 0, len(ids))
	for _, id := range ids {
		cb, err := r.ws.ReadCode(id)
		if err != nil {
			// One unreadable entry must not cost the reader the other nine
			// sections they asked for.
			out = append(out, wiki.Block{NodeID: id, Err: err})
			continue
		}
		out = append(out, wiki.Block{
			NodeID:    id,
			RelPath:   cb.RelPath,
			StartLine: cb.StartLine,
			EndLine:   cb.EndLine,
			Text:      cb.Code,
		})
	}
	return out
}

// registerWiki wires the knowledge-layer tools.
func registerWiki(reg *mcp.Registry, ws *workspace.Workspace) {
	reg.Register(&mcp.Tool{
		Name: "wiki_preflight",
		Description: "Before delegating work or starting a change, get the knowledge catalogue for it: " +
			"which curated sets of documentation sections apply, with one line each and no bodies. " +
			"Returns a token that must be included in the delegation prompt. " +
			"An empty catalogue is a normal answer meaning no recorded knowledge applies — it still " +
			"returns a token, so proceed. Use wiki_resolve to load what the catalogue names.",
		InputSchema: objSchema(map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "What the work is, in a sentence. Matched against set names, summaries and entries.",
			},
			"role": map[string]any{
				"type":        "string",
				"description": "The role the work belongs to (e.g. backend). Sets tagged for this role are always included.",
			},
		}, nil),
		Handler: wikiPreflightHandler(ws),
	})

	reg.Register(&mcp.Tool{
		Name: "wiki_resolve",
		Description: "Load the sections a knowledge set names, by set name or by node id. " +
			"Each section carries a drift verdict: absent means it is byte-for-byte what it was when " +
			"the set was written, \"changed-since-registration\" means the section was rewritten and the " +
			"set's one-line summary may no longer hold, \"unpinned\" means nothing was recorded to compare " +
			"against. Running this is also what clears the knowledge gate for the caller.",
		InputSchema: objSchema(map[string]any{
			"sets": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Set names from a manifest. Prerequisites are pulled in automatically.",
			},
			"node_ids": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Specific sections, when the catalogue has already been read and only some entries are wanted.",
			},
		}, nil),
		Handler: wikiResolveHandler(ws),
	})
}

// loadWiki opens the workspace's wiki and its signing secret.
//
// The store is read per call rather than cached. The files are few and small,
// and an author editing a set expects the next preflight to see it — a cache
// here would make the wiki feel broken during exactly the work that builds it.
func loadWiki(ws *workspace.Workspace) (*wiki.Store, []byte, error) {
	store, err := wiki.Load(ws.Root)
	if err != nil {
		return nil, nil, err
	}
	secret, err := wiki.LoadOrCreateSecret(ws.Root)
	if err != nil {
		return nil, nil, err
	}
	store.SetRedirector(wsRedirector{ws})
	return store, secret, nil
}

// wsRedirector lets a set keep resolving across a renamed heading.
type wsRedirector struct{ ws *workspace.Workspace }

func (r wsRedirector) ResolveID(id string) string { return r.ws.ResolveNodeID(id) }

func wikiPreflightHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Task string `json:"task"`
		Role string `json:"role"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		var a args
		_ = json.Unmarshal(raw, &a)

		store, secret, err := loadWiki(ws)
		if err != nil {
			st := ws.FSM.Status()
			return mcp.ErrorXML(mcp.ErrInternal, "read wiki: "+err.Error(), &st), true
		}

		sel := store.Select(a.Role, a.Task)
		man := store.Manifest(sel, secret)

		var sb strings.Builder
		writeStatusPrefix(&sb, ws)
		fmt.Fprintf(&sb, "<knowledge_manifest sets=\"%d\" token=\"%s\">\n", len(man.Sets), mcp.EscapeAttr(man.Token))
		if len(man.Sets) == 0 {
			// Say what to do, not just what was found. A bare empty element
			// reads as a failure and invites a retry that cannot succeed.
			sb.WriteString("  <none>No recorded knowledge applies to this work. " +
				"Include the token in the delegation prompt and proceed.</none>\n")
		}
		for _, s := range man.Sets {
			fmt.Fprintf(&sb, "  <set name=\"%s\" node_id=\"%s\" entries=\"%d\">\n",
				mcp.EscapeAttr(s.Name), mcp.EscapeAttr(s.NodeID), s.Entries)
			if s.Summary != "" {
				fmt.Fprintf(&sb, "    <summary>%s</summary>\n", mcp.EscapeText(s.Summary))
			}
			for _, g := range s.Groups {
				fmt.Fprintf(&sb, "    <group title=\"%s\" node_id=\"%s\" entries=\"%d\" />\n",
					mcp.EscapeAttr(g.Title), mcp.EscapeAttr(g.NodeID), g.Entries)
			}
			sb.WriteString("  </set>\n")
		}
		for _, m := range man.Missing {
			// A prerequisite naming a set that does not exist is an
			// authoring bug. Reporting it beats a silently shorter list.
			fmt.Fprintf(&sb, "  <unknown_set name=\"%s\" />\n", mcp.EscapeAttr(m))
		}
		sb.WriteString("</knowledge_manifest>")
		return sb.String(), false
	}
}

func wikiResolveHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Sets    []string `json:"sets"`
		NodeIDs []string `json:"node_ids"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
		}
		var a args
		_ = json.Unmarshal(raw, &a)
		st := ws.FSM.Status()
		if len(a.Sets) == 0 && len(a.NodeIDs) == 0 {
			return mcp.ErrorXML(mcp.ErrInternal, "sets or node_ids is required", &st), true
		}

		store, _, err := loadWiki(ws)
		if err != nil {
			return mcp.ErrorXML(mcp.ErrInternal, "read wiki: "+err.Error(), &st), true
		}
		r := wsReader{ws}

		var entries []wiki.ResolvedEntry
		var missing []string
		if len(a.Sets) > 0 {
			res := store.Resolve(r, a.Sets)
			missing = res.Missing
			for _, s := range res.Sets {
				entries = append(entries, s.Entries...)
			}
		}
		if len(a.NodeIDs) > 0 {
			entries = append(entries, store.ResolveNodes(r, a.NodeIDs)...)
		}

		return renderResolved(ws, entries, missing), false
	}
}

// renderResolved writes the sections, stopping at the response budget and
// naming every entry that did not fit.
//
// The response is a PREFIX of the request, for the same reason read_code's
// is: fitting smaller later sections into the gap would silently reorder the
// reader's priorities, and cutting inside a section hands back half of
// something they had already decided to read.
func renderResolved(ws *workspace.Workspace, entries []wiki.ResolvedEntry, missing []string) string {
	var head strings.Builder
	writeStatusPrefix(&head, ws)

	rendered := make([]string, len(entries))
	for i, e := range entries {
		if e.Block.Err != nil || e.Block.Text == "" {
			continue
		}
		var b strings.Builder
		id := e.NodeID
		if e.RedirectedTo != "" {
			id = e.RedirectedTo
		}
		fmt.Fprintf(&b, "  <section id=\"%s\" file=\"%s\" lines=\"%d-%d\"",
			mcp.EscapeAttr(id), mcp.EscapeAttr(e.Block.RelPath), e.Block.StartLine, e.Block.EndLine)
		if e.RedirectedTo != "" {
			// Naming the old id is the point: the content is fine, but the
			// set still links to a heading that no longer exists.
			fmt.Fprintf(&b, " redirected_from=\"%s\"", mcp.EscapeAttr(e.NodeID))
		}
		if e.Drift != wiki.DriftNone {
			fmt.Fprintf(&b, " drift=\"%s\"", e.Drift)
		}
		b.WriteString(">\n")
		mcp.WriteCDATA(&b, e.Block.Text)
		b.WriteString("\n  </section>")
		rendered[i] = b.String()
	}

	omitLen := func(rest []wiki.ResolvedEntry) int {
		n := 0
		for _, e := range rest {
			n += omitLineLen(e.NodeID, omitReasonFor(e))
		}
		return n
	}

	budget := mcp.MaxResponseBytes
	used := head.Len() + len("<knowledge requested=\"999\" returned=\"999\">\n") + len("</knowledge>")
	cut := len(entries)
	for i, e := range entries {
		if rendered[i] == "" {
			used += omitLineLen(e.NodeID, omitReasonFor(e))
			continue
		}
		if used+len(rendered[i])+1+omitLen(entries[i+1:]) > budget {
			cut = i
			break
		}
		used += len(rendered[i]) + 1
	}

	returned := 0
	for i := range entries[:cut] {
		if rendered[i] != "" {
			returned++
		}
	}

	var sb strings.Builder
	sb.WriteString(head.String())
	fmt.Fprintf(&sb, "<knowledge requested=\"%d\" returned=\"%d\">\n", len(entries), returned)
	for i, e := range entries[:cut] {
		if rendered[i] == "" {
			writeOmitted(&sb, e.NodeID, omitReasonFor(e))
			continue
		}
		sb.WriteString(rendered[i])
		sb.WriteString("\n")
	}
	for _, e := range entries[cut:] {
		writeOmitted(&sb, e.NodeID, "budget")
	}
	for _, m := range missing {
		fmt.Fprintf(&sb, "  <unknown_set name=\"%s\" />\n", mcp.EscapeAttr(m))
	}
	sb.WriteString("</knowledge>")
	return sb.String()
}

// omitReasonFor names why a section carries no text.
func omitReasonFor(e wiki.ResolvedEntry) string {
	switch {
	case e.Drift == wiki.DriftGone:
		return "gone"
	case e.Block.Err == wiki.ErrPinnedDrift:
		// A pinned set exists for reproducibility, so serving the new text
		// would destroy the only thing the reader came for.
		return "pinned-drift"
	case e.Block.Err != nil:
		return omitReason(e.Block.Err)
	default:
		return "empty"
	}
}
