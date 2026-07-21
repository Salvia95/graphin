package mcp

import (
	"fmt"
	"unicode/utf8"
)

// MaxResponseBytes caps a single tool response (§3.0).
const MaxResponseBytes = 12 * 1024

// Truncate enforces the response cap: oversized payloads are cut on a UTF-8
// boundary and suffixed with a <truncated> marker. The returned string never
// exceeds MaxResponseBytes.
func Truncate(s string) string {
	if len(s) <= MaxResponseBytes {
		return s
	}
	marker := fmt.Sprintf("\n<truncated reason=\"size\" total_bytes=\"%d\" />", len(s))
	cut := MaxResponseBytes - len(marker)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + marker
}
