package parse

import (
	"strings"
	"testing"
)

func longSection(paras, words int) string {
	var b strings.Builder
	b.WriteString("# 서문\n\n짧은 서문.\n\n## 운영 메모\n\n")
	for p := 0; p < paras; p++ {
		for w := 0; w < words; w++ {
			b.WriteString(word(p*100 + w))
			b.WriteByte(' ')
		}
		b.WriteString("\n\n")
	}
	b.WriteString("## 다음\n\n끝.\n")
	return b.String()
}

// A section past maxBodyTokens is split into contiguous chunks after the
// first; a short one is not split. The chunks cover the rest of the section
// exactly, close on paragraph breaks, and know their lines.
func TestSectionChunks(t *testing.T) {
	src := longSection(12, 60) // ~720 tokens in 12 paragraphs
	res, err := File("docs/ops.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var long, short *Node
	for i := range res.Nodes {
		switch res.Nodes[i].ID {
		case "docs/ops.md#운영-메모":
			long = &res.Nodes[i]
		case "docs/ops.md#서문":
			short = &res.Nodes[i]
		}
	}
	if short == nil || long == nil {
		t.Fatal("sections missing")
	}
	if len(short.Chunks) != 0 || short.Excerpt != "짧은 서문." {
		t.Fatalf("short section: %d chunks, excerpt %q", len(short.Chunks), short.Excerpt)
	}
	cs := long.Chunks
	if len(cs) < 2 {
		t.Fatalf("long section split into %d continuation chunks", len(cs))
	}
	for i, c := range cs {
		if c.Ord != i+2 || c.Of != len(cs)+1 {
			t.Errorf("chunk %d: ord %d of %d", i, c.Ord, c.Of)
		}
		if len(c.Tokens) == 0 || len(c.Tokens) > maxBodyTokens {
			t.Errorf("chunk %d: %d tokens", c.Ord, len(c.Tokens))
		}
		if i > 0 && cs[i-1].EndByte != c.StartByte {
			t.Errorf("chunk %d does not start where %d ended", c.Ord, c.Ord-1)
		}
		if want := uint32(1 + strings.Count(src[:c.StartByte], "\n")); c.StartLine != want {
			t.Errorf("chunk %d: line %d, want %d", c.Ord, c.StartLine, want)
		}
		if c.StartByte > 0 && src[c.StartByte-1] != '\n' {
			t.Errorf("chunk %d starts mid-line", c.Ord)
		}
	}
	if cs[len(cs)-1].EndByte != long.EndByte {
		t.Errorf("last chunk ends at %d, section at %d", cs[len(cs)-1].EndByte, long.EndByte)
	}
	// The last word of the section is in a chunk: nothing past the cap is lost.
	if last := cs[len(cs)-1].Tokens; !contains(last, word(11*100+59)) {
		t.Errorf("last word missing from the last chunk")
	}
}

// word spells n in lowercase letters: one word, one token.
func word(n int) string {
	s := "q"
	for {
		s += string(rune('a' + n%26))
		n /= 26
		if n == 0 {
			return s
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
