// scope.go — index_scope shows what the index is made of and takes things
// out of it.
//
// Two properties shape this tool. Adding a pattern never changes the current
// index: it is written to .graphin/ignore and takes effect at the next
// bootstrap, which keeps the read path free of an exclusion filter and gives
// the change one visible moment instead of a half-applied state. And a write
// needs confirm=true, so the first call is always a preview a person can read
// before anything is decided.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/workspace"
)

const scopeDescription = "Show what the index is made of — files and nodes per top-level " +
	"directory and per extension, plus the exclusion patterns in effect — and edit that scope. " +
	"Call with no arguments to see the composition; markdown is broken out because a .md file " +
	"becomes one node per heading, which is usually what makes a node count jump. " +
	"Pass `add` (gitignore syntax: \"build/\", \"*.txt\") to see what those patterns would take " +
	"out: affected files, nodes, and warnings when they would cut wiki-pinned sections or DB " +
	"schema snapshots. Nothing is written until you repeat the call with confirm=true, and even " +
	"then the current index is untouched — search keeps answering from it until the next " +
	"bootstrap_workspace, which is when the nodes actually go. `remove` takes patterns back out " +
	"so the next bootstrap indexes them again. Use it when the node count looks wrong for the " +
	"project, or when scratch files, vendored copies, build output or a git worktree inside the " +
	"project are being indexed."

func registerScope(reg *mcp.Registry, ws *workspace.Workspace) {
	reg.Register(&mcp.Tool{
		Name:        "index_scope",
		Description: scopeDescription,
		InputSchema: objSchema(map[string]any{
			"add": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Patterns to exclude (gitignore syntax). Previews unless confirm is true.",
			},
			"remove": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Patterns to stop excluding, matched literally against the file's lines.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Why these are excluded. Recorded above the patterns so the file explains itself.",
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Write the change. Without it the call only reports what would happen.",
			},
		}, nil),
		Handler: scopeHandler(ws),
	})
}

func scopeHandler(ws *workspace.Workspace) mcp.ToolHandler {
	type args struct {
		Add     []string `json:"add"`
		Remove  []string `json:"remove"`
		Reason  string   `json:"reason"`
		Confirm bool     `json:"confirm"`
	}
	return func(_ context.Context, raw json.RawMessage) (string, bool) {
		st := ws.FSM.Status()
		var a args
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &a); err != nil {
				return mcp.ErrorXML(mcp.ErrInternal, "invalid arguments: "+err.Error(), &st), true
			}
		}
		a.Add = cleanPatterns(a.Add)
		a.Remove = cleanPatterns(a.Remove)

		var sb strings.Builder
		writeStatusPrefix(&sb, ws)

		switch {
		case len(a.Add) > 0 && len(a.Remove) > 0:
			return mcp.ErrorXML(mcp.ErrInternal,
				"pass add or remove, not both", &st), true

		case len(a.Add) > 0:
			imp, err := ws.ScopeImpactOf(a.Add)
			if err != nil {
				return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
			}
			if !a.Confirm {
				writeImpact(&sb, "preview", imp, "")
				sb.WriteString("  <note>확정하려면 같은 호출에 confirm=true. " +
					"쓰기는 .graphin/ignore만 바꾸고, 노드는 다음 bootstrap_workspace에서 사라집니다.</note>\n")
				sb.WriteString("</scope>\n")
				return sb.String(), true
			}
			if _, err := ws.ScopeAdd(a.Add, a.Reason); err != nil {
				return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
			}
			writeImpact(&sb, "excluded", imp, a.Reason)
			sb.WriteString("  <note>.graphin/ignore에 기록했습니다. 현재 인덱스는 그대로이고, " +
				"다음 bootstrap_workspace가 이 노드들을 제거합니다.</note>\n")
			sb.WriteString("</scope>\n")
			return sb.String(), true

		case len(a.Remove) > 0:
			if !a.Confirm {
				imp, err := ws.ScopeImpactOf(a.Remove)
				if err != nil {
					return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
				}
				writeImpact(&sb, "preview_remove", imp, "")
				sb.WriteString("  <note>확정하려면 confirm=true. 다음 bootstrap_workspace가 " +
					"이 파일들을 다시 색인합니다.</note>\n")
				sb.WriteString("</scope>\n")
				return sb.String(), true
			}
			imp, err := ws.ScopeRemove(a.Remove)
			if err != nil {
				return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
			}
			writeImpact(&sb, "unexcluded", imp, "")
			sb.WriteString("  <note>다음 bootstrap_workspace가 이 파일들을 다시 색인합니다.</note>\n")
			sb.WriteString("</scope>\n")
			return sb.String(), true
		}

		rep, err := ws.Scope()
		if err != nil {
			return mcp.ErrorXML(mcp.ErrInternal, err.Error(), &st), true
		}
		writeReport(&sb, rep)
		// A workspace that has been bootstrapped but is still scanning reports
		// exact counts of an index that is not finished. Without this the
		// numbers read as final and a scan caught early looks like an empty
		// repository.
		if !rep.Estimated && st.State != "ready" {
			sb.WriteString("<note>인덱싱이 진행 중입니다 — 위 수치는 지금까지 색인된 것까지입니다.</note>\n")
		}
		return sb.String(), true
	}
}

func writeReport(sb *strings.Builder, r *workspace.ScopeReport) {
	fmt.Fprintf(sb, "<index_scope files=\"%d\" nodes=\"%d\" headings=\"%d\"", r.Files, r.Nodes, r.Headings)
	if r.Estimated {
		// Said plainly rather than as a flag: a reader who skims the numbers
		// must not take an un-indexed preview for a measured index.
		sb.WriteString(" estimated=\"true\" source=\"scan\"")
	}
	sb.WriteString(">\n")
	if r.Estimated {
		sb.WriteString("  <note>부트스트랩 전이라 스캔으로 셌습니다. 파일 수는 정확하고, " +
			"노드 수는 마크다운 헤딩만 셉니다 — 코드 심볼은 파싱해야 알 수 있습니다.</note>\n")
	}
	for _, e := range r.Dirs {
		writeEntry(sb, "dir", e)
	}
	for _, e := range r.Exts {
		writeEntry(sb, "ext", e)
	}
	if len(r.Excluded) == 0 {
		sb.WriteString("  <excluded patterns=\"0\" />\n")
	} else {
		fmt.Fprintf(sb, "  <excluded patterns=\"%d\">\n", len(r.Excluded))
		for _, p := range r.Excluded {
			fmt.Fprintf(sb, "    <pattern>%s</pattern>\n", mcp.EscapeText(p))
		}
		sb.WriteString("  </excluded>\n")
	}
	sb.WriteString("</index_scope>\n")
}

func writeEntry(sb *strings.Builder, kind string, e workspace.ScopeEntry) {
	fmt.Fprintf(sb, "  <%s name=%q files=\"%d\" nodes=\"%d\"", kind, mcp.EscapeAttr(e.Name), e.Files, e.Nodes)
	if e.MDHeadings > 0 {
		fmt.Fprintf(sb, " md_headings=\"%d\"", e.MDHeadings)
	}
	sb.WriteString(" />\n")
}

func writeImpact(sb *strings.Builder, action string, imp *workspace.ScopeImpact, reason string) {
	fmt.Fprintf(sb, "<scope action=%q files=\"%d\" nodes=\"%d\"",
		mcp.EscapeAttr(action), imp.Files, imp.Nodes)
	if imp.Estimated {
		sb.WriteString(" estimated=\"true\"")
	}
	sb.WriteString(">\n")
	for _, p := range imp.Patterns {
		fmt.Fprintf(sb, "  <pattern>%s</pattern>\n", mcp.EscapeText(p))
	}
	if reason != "" {
		fmt.Fprintf(sb, "  <reason>%s</reason>\n", mcp.EscapeText(reason))
	}
	for _, s := range imp.Sample {
		fmt.Fprintf(sb, "  <file>%s</file>\n", mcp.EscapeText(s))
	}
	if imp.Files > len(imp.Sample) {
		fmt.Fprintf(sb, "  <more files=\"%d\" />\n", imp.Files-len(imp.Sample))
	}
	for _, wmsg := range imp.Warnings {
		fmt.Fprintf(sb, "  <warning>%s</warning>\n", mcp.EscapeText(wmsg))
	}
	if imp.Files == 0 {
		sb.WriteString("  <warning>이 패턴에 걸리는 파일이 없습니다 — 문법이나 경로를 확인하세요.</warning>\n")
	}
}

// cleanPatterns drops blanks so an empty array does not read as "the user
// asked for something" and trip the add/remove branch.
func cleanPatterns(in []string) []string {
	out := in[:0:0]
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
