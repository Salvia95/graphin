package sweexplore

import (
	"os"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/bench"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/scan"
)

// GrepRegions is the Grep -C<context> baseline as a ranked region list
// (docs/phase7-spec.md §3.3): terms come from the same derived queries as
// the graphin policy, files rank by match count (ties: path), and each
// file's merged ±context windows emit in line order.
func GrepRegions(root, issue string, o Options, contextLines int) ([]Region, error) {
	var terms []string
	seen := map[string]bool{}
	for _, q := range DeriveQueries(issue, o.Queries) {
		for _, t := range bench.Terms(q) {
			if !seen[t] {
				seen[t] = true
				terms = append(terms, t)
			}
		}
	}

	res, err := scan.Walk(root, obs.Nop())
	if err != nil {
		return nil, err
	}
	type fileHit struct {
		rel     string
		matches int
		regions []Region
	}
	var hits []fileHit
	for _, f := range res.Files {
		src, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(src), "\n")
		include := make([]bool, len(lines))
		matches := 0
		for i, line := range lines {
			low := strings.ToLower(line)
			for _, t := range terms {
				if strings.Contains(low, t) {
					matches++
					lo := max(0, i-contextLines)
					hi := min(len(lines)-1, i+contextLines)
					for j := lo; j <= hi; j++ {
						include[j] = true
					}
					break
				}
			}
		}
		if matches == 0 {
			continue
		}
		h := fileHit{rel: f.RelPath, matches: matches}
		start := -1
		for i, on := range include {
			switch {
			case on && start < 0:
				start = i
			case !on && start >= 0:
				h.regions = append(h.regions, Region{Path: f.RelPath, Start: start + 1, End: i})
				start = -1
			}
		}
		if start >= 0 {
			h.regions = append(h.regions, Region{Path: f.RelPath, Start: start + 1, End: len(lines)})
		}
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].matches != hits[j].matches {
			return hits[i].matches > hits[j].matches
		}
		return hits[i].rel < hits[j].rel
	})

	var regions []Region
	for _, h := range hits {
		for _, r := range h.regions {
			if len(regions) >= o.MaxRegions {
				return regions, nil
			}
			regions = append(regions, r)
		}
	}
	return regions, nil
}
