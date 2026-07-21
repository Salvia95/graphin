// Package lexical implements the dependency-free BM25 inverted index and the
// Tier-0 symbol table (§2.1.1).
package lexical

import (
	"strings"
	"unicode"
)

// SplitIdentifier breaks an identifier into lowercase parts at camelCase,
// snake_case, kebab-case and letter/digit boundaries.
// "HTTPServer2" → ["http", "server", "2"].
func SplitIdentifier(s string) []string {
	var parts []string
	runes := []rune(s)
	start := 0
	flushTo := func(end int) {
		if end > start {
			parts = append(parts, strings.ToLower(string(runes[start:end])))
		}
		start = end
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flushTo(i)
			start = i + 1
			continue
		}
		if i == start {
			continue
		}
		prev := runes[i-1]
		switch {
		case unicode.IsLower(prev) && unicode.IsUpper(r):
			flushTo(i)
		case unicode.IsLetter(prev) != unicode.IsLetter(r): // letter↔digit boundary
			flushTo(i)
		case unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
			flushTo(i) // end of acronym run: "HTTPServer" → HTTP | Server
		}
	}
	flushTo(len(runes))
	return parts
}

// TokenizeCapped tokenizes text and truncates the result to max tokens —
// used for node-body indexing (§보완 B) to bound index memory.
func TokenizeCapped(text string, max int) []string {
	toks := Tokenize(text)
	if len(toks) > max {
		toks = toks[:max:max]
	}
	return toks
}

// Tokenize produces BM25 terms from free text or code fragments: words are
// split on non-identifier runes, each word contributes its identifier parts,
// and composite words also contribute their joined lowercase form so that
// "CancelPayment", "cancel_payment" and "cancelpayment" all meet.
func Tokenize(text string) []string {
	var out []string
	for _, w := range splitWords(text) {
		parts := SplitIdentifier(w)
		out = append(out, parts...)
		if len(parts) > 1 {
			out = append(out, strings.Join(parts, ""))
		}
	}
	return out
}

func splitWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
}

// BuildDocTokens assembles the indexable token multiset for one node
// (§2.1.1): simple name, node-ID segments, identifier splits, signature
// tokens.
func BuildDocTokens(simpleName, nodeID, signature string) []string {
	var out []string
	out = append(out, Tokenize(simpleName)...)
	out = append(out, strings.ToLower(simpleName))
	segs := strings.FieldsFunc(nodeID, func(r rune) bool {
		return r == '.' || r == '(' || r == ')' || r == ',' || r == ' '
	})
	for _, seg := range segs {
		parts := SplitIdentifier(seg)
		out = append(out, parts...)
		if len(parts) > 1 {
			out = append(out, strings.Join(parts, ""))
		}
	}
	out = append(out, Tokenize(signature)...)
	return out
}
