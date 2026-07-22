package sweexplore

import (
	"regexp"
	"sort"
	"strings"
)

// Deterministic query derivation (docs/phase7-spec.md §3.2): same issue text
// → same query list, no model in the loop. Priority: issue title, backtick
// code spans, then identifier-shaped tokens by frequency.

var (
	backtickRe = regexp.MustCompile("`([^`\n]{2,80})`")
	identRe    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)
)

const maxQueryLen = 120

// DeriveQueries returns up to n queries from an issue body.
func DeriveQueries(issue string, n int) []string {
	if n <= 0 {
		n = 3
	}
	var out []string
	seen := map[string]bool{}
	add := func(q string) bool {
		q = strings.TrimSpace(q)
		if len(q) > maxQueryLen {
			q = q[:maxQueryLen]
		}
		key := strings.ToLower(q)
		if q == "" || seen[key] {
			return len(out) >= n
		}
		seen[key] = true
		out = append(out, q)
		return len(out) >= n
	}

	// 1) Title: first non-empty line, markdown header markers stripped.
	for _, line := range strings.Split(issue, "\n") {
		if t := strings.TrimSpace(strings.TrimLeft(line, "# ")); t != "" {
			if add(t) {
				return out
			}
			break
		}
	}

	// 2) Backtick spans in document order.
	for _, m := range backtickRe.FindAllStringSubmatch(issue, -1) {
		if add(m[1]) {
			return out
		}
	}

	// 3) Identifier-shaped tokens (snake_case / camelCase / dotted refs are
	//    already covered by spans; here plain frequency fills the tail).
	freq := map[string]int{}
	for _, tok := range identRe.FindAllString(issue, -1) {
		if isIdentifierShaped(tok) {
			freq[tok]++
		}
	}
	toks := make([]string, 0, len(freq))
	for t := range freq {
		toks = append(toks, t)
	}
	sort.Slice(toks, func(i, j int) bool {
		if freq[toks[i]] != freq[toks[j]] {
			return freq[toks[i]] > freq[toks[j]]
		}
		return toks[i] < toks[j]
	})
	for _, t := range toks {
		if add(t) {
			return out
		}
	}
	return out
}

// isIdentifierShaped keeps tokens that look like code, not prose: an inner
// underscore, an inner uppercase (camelCase), or a known code suffix.
func isIdentifierShaped(tok string) bool {
	if strings.ContainsRune(tok[1:], '_') {
		return true
	}
	for _, r := range tok[1:] {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}
