package mcp

import (
	"strings"
	"testing"
)

func TestWriteCDATASplitsTerminator(t *testing.T) {
	var sb strings.Builder
	WriteCDATA(&sb, "a]]>b")
	want := "<![CDATA[a]]]]><![CDATA[>b]]>"
	if sb.String() != want {
		t.Fatalf("got %q, want %q", sb.String(), want)
	}
}

func TestWriteCDATAPlain(t *testing.T) {
	var sb strings.Builder
	WriteCDATA(&sb, "fun process() {}")
	if sb.String() != "<![CDATA[fun process() {}]]>" {
		t.Fatalf("got %q", sb.String())
	}
}
