package semantic

import (
	"strings"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/parse"
)

const maxSummaryCalls = 8

// Summarize builds the synthetic passage for one node (§2.1.1: 식별자 분해
// 토큰과 시그니처를 결합한 합성 요약문).
func Summarize(pkg string, n parse.Node) string {
	var sb strings.Builder
	sb.WriteString(n.Kind)
	sb.WriteString(" ")
	sb.WriteString(n.DisplayName)
	if pkg != "" {
		sb.WriteString(" in ")
		sb.WriteString(pkg)
	}
	sb.WriteString(": ")
	sb.WriteString(strings.Join(lexical.SplitIdentifier(n.SimpleName), " "))

	if len(n.Params) > 0 {
		sb.WriteString("; params ")
		sb.WriteString(strings.Join(n.Params, " "))
	}
	if len(n.Supers) > 0 {
		sb.WriteString("; extends ")
		sb.WriteString(strings.Join(n.Supers, " "))
	}
	// File nodes have no signature; their content tokens carry the meaning
	// (§보완 A: config keys, doc headings).
	if n.Kind == nodeid.KindFile && len(n.BodyTokens) > 0 {
		k := min(40, len(n.BodyTokens))
		sb.WriteString("; content ")
		sb.WriteString(strings.Join(n.BodyTokens[:k], " "))
	}
	if len(n.Calls) > 0 {
		seen := map[string]bool{}
		var names []string
		for _, c := range n.Calls {
			if seen[c.Name] || len(names) >= maxSummaryCalls {
				continue
			}
			seen[c.Name] = true
			names = append(names, strings.Join(lexical.SplitIdentifier(c.Name), " "))
		}
		if len(names) > 0 {
			sb.WriteString("; calls ")
			sb.WriteString(strings.Join(names, ", "))
		}
	}
	return sb.String()
}
