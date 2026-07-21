// Package bench simulates the §3.5 baseline scenarios: how many bytes an
// agent would ingest via grep-style exploration versus graphin's three-step
// navigation. All byte counts are computed locally; no external grep runs.
package bench

import (
	"os"
	"strings"

	"github.com/Salvia95/graphin/internal/lexical"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/scan"
)

// Terms derives case-insensitive match terms from a query: the raw query
// plus its identifier tokens.
func Terms(query string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(t string) {
		t = strings.ToLower(strings.TrimSpace(t))
		if len(t) >= 2 && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	add(query)
	for _, t := range lexical.Tokenize(query) {
		add(t)
	}
	return out
}

// GrepFull sums the entire size of every source file containing any term
// (§3.5 시나리오 1: worst-case baseline).
func GrepFull(root, query string) (bytes, files int, err error) {
	terms := Terms(query)
	res, err := scan.Walk(root, obs.Nop())
	if err != nil {
		return 0, 0, err
	}
	for _, f := range res.Files {
		src, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		low := strings.ToLower(string(src))
		for _, t := range terms {
			if strings.Contains(low, t) {
				bytes += len(src)
				files++
				break
			}
		}
	}
	return bytes, files, nil
}

// GrepContext sums ±context lines around every matching line, with
// overlapping windows merged per file (§3.5 시나리오 2: grep -C 20).
func GrepContext(root, query string, context int) (bytes int, err error) {
	terms := Terms(query)
	res, err := scan.Walk(root, obs.Nop())
	if err != nil {
		return 0, err
	}
	for _, f := range res.Files {
		src, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(src), "\n")
		include := make([]bool, len(lines))
		matched := false
		for i, line := range lines {
			low := strings.ToLower(line)
			for _, t := range terms {
				if strings.Contains(low, t) {
					matched = true
					lo := max(0, i-context)
					hi := min(len(lines)-1, i+context)
					for j := lo; j <= hi; j++ {
						include[j] = true
					}
					break
				}
			}
		}
		if !matched {
			continue
		}
		for i, on := range include {
			if on {
				bytes += len(lines[i]) + 1 // +\n
			}
		}
	}
	return bytes, nil
}
