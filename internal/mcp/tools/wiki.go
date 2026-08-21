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
		Name: "wiki_propose",
		Description: "Submit a glossary candidate for review. Never writes to the glossary itself — " +
			"it files a proposal and returns the mechanical verdict. A word the code index already " +
			"resolves is rejected outright: that is structure, and search answers it. Cite evidence " +
			"as node ids from at least two different files, or the candidate is one author's coinage.",
		InputSchema: objSchema(map[string]any{
			"canonical":  map[string]any{"type": "string", "description": "The one word the project should use."},
			"definition": map[string]any{"type": "string", "description": "One paragraph. What it means and why the project needs the word."},
			"aliases": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "Variants interchangeable with it in EVERY context here. Partial overlap is a separate term, not an alias.",
			},
			"evidence": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "Node ids where the term is actually used, from at least two different files.",
			},
			"not_to_be_confused_with": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "Near-misses, written as \"other term — why they differ\".",
			},
			"scope": map[string]any{
				"type": "array", "items": map[string]any{"type": "string"},
				"description": "Roles this applies to, or \"all\".",
			},
		}, []string{"canonical", "definition", "evidence"}),
		Handler: wikiProposeHandler(ws),
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

// wsDefiner answers the admission test's first rule from the symbol table.
//
// This is the boundary between graphin and the wiki, expressed as a function
// rather than a rule in a document: if the index already resolves the word,
// it is structure and the wiki has nothing to add that will not drift.
type wsDefiner struct{ ws *workspace.Workspace }

func (d wsDefiner) Defines(word string) (string, string, bool) {
	ids := d.ws.Sym.Lookup(word)
	if len(ids) == 0 {
		return "", "", false
	}
	return ids[0], d.ws.NodeKind(ids[0]), true
}

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

		// Record what was asked and whether anything answered. A miss is the
		// only trigger that grows the wiki — there is no retroactive sweep —
		// so it has to be written down where it happens.
		ev := wiki.FrictionEvent{Kind: wiki.FrictionHit, Task: a.Task, Role: a.Role}
		if sel.Empty() {
			ev.Kind = wiki.FrictionMiss
		}
		for _, s := range sel.Sets {
			ev.Sets = append(ev.Sets, s.Name)
		}
		wiki.AppendFriction(ws.Root, ev)

		var sb strings.Builder
		writeStatusPrefix(&sb, ws)
		fmt.Fprintf(&sb, "<knowledge_manifest sets=\"%d\" terms=\"%d\" token=\"%s\">\n",
			len(man.Sets), len(man.Terms), mcp.EscapeAttr(man.Token))
		if len(man.Sets) == 0 && len(man.Terms) == 0 {
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
		for _, t := range man.Terms {
			fmt.Fprintf(&sb, "  <term canonical=\"%s\"", mcp.EscapeAttr(t.Canonical))
			if len(t.Aliases) > 0 {
				fmt.Fprintf(&sb, " aliases=\"%s\"", mcp.EscapeAttr(strings.Join(t.Aliases, ", ")))
			}
			sb.WriteString(">\n")
			fmt.Fprintf(&sb, "    <definition>%s</definition>\n", mcp.EscapeText(t.Definition))
			for _, c := range t.Confusions {
				fmt.Fprintf(&sb, "    <not_to_be_confused_with term=\"%s\">%s</not_to_be_confused_with>\n",
					mcp.EscapeAttr(c.Term), mcp.EscapeText(c.Why))
			}
			sb.WriteString("  </term>\n")
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
		var a args
		_ = json.Unmarshal(raw, &a)
		st := ws.FSM.Status()

		// Naming nothing is a valid call, not a mistake. A caller whose
		// preflight returned an empty catalogue has nothing to name, and the
		// gate tells them to run this anyway — refusing would make the escape
		// the gate promises impossible to take, which is how a blocked agent
		// ends up with no legal next move.
		//
		// It is answered before the bootstrap check for the same reason:
		// reading no sections needs no index, and failing here would deadlock
		// every workspace whose server has not indexed yet.
		if len(a.Sets) == 0 && len(a.NodeIDs) == 0 {
			var sb strings.Builder
			writeStatusPrefix(&sb, ws)
			sb.WriteString("<knowledge requested=\"0\" returned=\"0\">\n" +
				"  <none>Nothing to load. Proceed.</none>\n" +
				"</knowledge>")
			return sb.String(), false
		}

		if !ws.Bootstrapped() {
			return notBootstrapped(ws), true
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
			names := make([]string, 0, len(res.Sets))
			for _, s := range res.Sets {
				names = append(names, s.Name)
			}
			wiki.AppendFriction(ws.Root, wiki.FrictionEvent{Kind: wiki.FrictionResolve, Sets: names})
		}
		if len(a.NodeIDs) > 0 {
			entries = append(entries, store.ResolveNodes(r, a.NodeIDs)...)
		}
		for _, e := range entries {
			// Noticed here, fixed elsewhere. Writing it down at the point of
			// service is what keeps the re-verification queue from depending
			// on someone remembering.
			if e.Drift == wiki.DriftChanged || e.Drift == wiki.DriftGone {
				wiki.AppendFriction(ws.Root, wiki.FrictionEvent{Kind: wiki.FrictionDrift, Node: e.NodeID})
			}
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

func wikiProposeHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Canonical  string   `json:"canonical"`
		Definition string   `json:"definition"`
		Aliases    []string `json:"aliases"`
		Evidence   []string `json:"evidence"`
		Confusions []string `json:"not_to_be_confused_with"`
		Scope      []string `json:"scope"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		var a args
		_ = json.Unmarshal(raw, &a)
		st := ws.FSM.Status()
		if strings.TrimSpace(a.Canonical) == "" || strings.TrimSpace(a.Definition) == "" {
			return mcp.ErrorXML(mcp.ErrInternal, "canonical and definition are required", &st), true
		}

		store, _, err := loadWiki(ws)
		if err != nil {
			return mcp.ErrorXML(mcp.ErrInternal, "read wiki: "+err.Error(), &st), true
		}

		t := &wiki.Term{
			Canonical: strings.TrimSpace(a.Canonical),
			Aliases:   a.Aliases,
			Scope:     a.Scope,
			Evidence:  a.Evidence,
			Body:      strings.TrimSpace(a.Definition),
			Status:    wiki.StatusDraft,
		}
		for _, c := range a.Confusions {
			t.Confusions = append(t.Confusions, wiki.SplitConfusion(c))
		}

		verdict := store.Judge(t, wsDefiner{ws})
		if verdict.Blocked() {
			// Blocked candidates are not filed. The queue is a review list,
			// and padding it with things a rule already rejected is how a
			// review list stops being read.
			var sb strings.Builder
			writeStatusPrefix(&sb, ws)
			sb.WriteString("<proposal accepted=\"false\">\n")
			for _, f := range verdict.Findings {
				fmt.Fprintf(&sb, "  <rejected rule=\"%s\">%s</rejected>\n",
					mcp.EscapeAttr(string(f.Rule)), mcp.EscapeText(f.Detail))
			}
			sb.WriteString("</proposal>")
			return sb.String(), false
		}

		p, err := store.Propose(t)
		if err != nil {
			return mcp.ErrorXML(mcp.ErrInternal, "file proposal: "+err.Error(), &st), true
		}
		var sb strings.Builder
		writeStatusPrefix(&sb, ws)
		fmt.Fprintf(&sb, "<proposal accepted=\"true\" contexts=\"%d\" seen=\"%d\" file=\"%s\">\n",
			verdict.Contexts, p.Seen, mcp.EscapeAttr(p.File))
		sb.WriteString("  <note>Queued for review. It is not in the glossary until a person moves it there.</note>\n")
		sb.WriteString("</proposal>")
		return sb.String(), false
	}
}
