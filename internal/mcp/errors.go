package mcp

import "strings"

// Tool-level error codes (§3.0). They travel as XML inside a tool result with
// isError=true, not as JSON-RPC protocol errors.
const (
	ErrNodeNotFound     = "NODE_NOT_FOUND"
	ErrNodeGone         = "NODE_GONE"
	ErrNotBootstrapped  = "NOT_BOOTSTRAPPED"
	ErrLockHeld         = "LOCK_HELD"
	ErrModelUnavailable = "MODEL_UNAVAILABLE"
	// ErrInternal covers unexpected failures outside the five spec codes.
	ErrInternal = "INTERNAL"
)

// ErrorXML renders a tool error body, optionally with the current system
// status attached.
func ErrorXML(code, msg string, st *Status) string {
	var sb strings.Builder
	sb.WriteString(`<error code="`)
	sb.WriteString(EscapeAttr(code))
	sb.WriteString(`">`)
	sb.WriteString(EscapeText(msg))
	sb.WriteString(`</error>`)
	if st != nil {
		sb.WriteString("\n")
		sb.WriteString(st.XML())
	}
	return sb.String()
}
