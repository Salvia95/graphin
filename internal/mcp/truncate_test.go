package mcp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateCapsAt12KB(t *testing.T) {
	big := strings.Repeat("x", 20*1024)
	got := Truncate(big)
	if len(got) > MaxResponseBytes {
		t.Fatalf("truncated response is %d bytes, cap %d", len(got), MaxResponseBytes)
	}
	if !strings.Contains(got, `<truncated reason="size" total_bytes="20480" />`) {
		t.Fatalf("missing truncated marker: ...%s", got[len(got)-100:])
	}
}

func TestTruncateKeepsUTF8Boundary(t *testing.T) {
	big := strings.Repeat("가", 8*1024) // 24KB of 3-byte runes
	got := Truncate(big)
	if !utf8.ValidString(got) {
		t.Fatal("truncation split a multi-byte rune")
	}
	if len(got) > MaxResponseBytes {
		t.Fatalf("truncated response is %d bytes, cap %d", len(got), MaxResponseBytes)
	}
}

func TestTruncateNoopWhenSmall(t *testing.T) {
	s := "<results />"
	if got := Truncate(s); got != s {
		t.Fatalf("small payload modified: %q", got)
	}
}
