package mcp

import (
	"fmt"
	"strings"
)

// Status is embedded in tool responses while the index is not fully ready
// (§3.0 공통 프로토콜 규약).
type Status struct {
	State         string // "not_bootstrapped" | "indexing" | "ready"
	Progress      int    // 0..100
	LexicalReady  bool
	SemanticReady bool

	// graphindb bootstrap heuristic (schema/graphindb.md): RDB traces found
	// in the scanned tree and how many snapshot files back them. DBHint is a
	// one-paragraph guide emitted only when traces exist without a snapshot.
	DBSources   string // comma-joined trace labels ("" = none detected)
	DBSnapshots int
	DBHint      string
	// DBManifestErrors carries graphindb manifest validation failures so the
	// agent can iterate on its routing/mapping declarations.
	DBManifestErrors string
}

// XML renders the <system_status/> block. DB attributes appear only when the
// heuristic found something, so code-only workspaces stay noise-free.
func (s Status) XML() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<system_status state="%s" progress="%d" lexical_ready="%t" semantic_ready="%t"`,
		EscapeAttr(s.State), s.Progress, s.LexicalReady, s.SemanticReady)
	if s.DBSources != "" || s.DBSnapshots > 0 {
		fmt.Fprintf(&sb, ` db_sources_detected="%s" db_snapshots="%d"`,
			EscapeAttr(s.DBSources), s.DBSnapshots)
	}
	if s.DBManifestErrors != "" {
		fmt.Fprintf(&sb, ` db_manifest_errors="%s"`, EscapeAttr(s.DBManifestErrors))
	}
	if s.DBHint == "" {
		sb.WriteString(" />")
		return sb.String()
	}
	sb.WriteString("><hint>")
	sb.WriteString(EscapeAttr(s.DBHint))
	sb.WriteString("</hint></system_status>")
	return sb.String()
}
