package mcp

import "fmt"

// Status is embedded in tool responses while the index is not fully ready
// (§3.0 공통 프로토콜 규약).
type Status struct {
	State         string // "not_bootstrapped" | "indexing" | "ready"
	Progress      int    // 0..100
	LexicalReady  bool
	SemanticReady bool
}

// XML renders the <system_status/> block.
func (s Status) XML() string {
	return fmt.Sprintf(`<system_status state="%s" progress="%d" lexical_ready="%t" semantic_ready="%t" />`,
		EscapeAttr(s.State), s.Progress, s.LexicalReady, s.SemanticReady)
}
