package sweexplore

import (
	"github.com/Salvia95/graphin/internal/bench"
	"github.com/Salvia95/graphin/internal/keyword"
)

// GrepRegions is the Grep -C<context> baseline as a ranked region list
// (docs/phase7-spec.md §3.3): terms come from the same derived queries as
// the graphin policy, files rank by match count (ties: path), and each
// file's merged ±context windows emit in line order.
//
// The matching itself lives in internal/keyword, which is also the engine
// behind the `search_keyword` tool. Sharing it is the point: graphin cannot
// be compared against a grep weaker than its own.
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

	hits, err := keyword.Search(root, keyword.Options{Terms: terms, ContextLines: contextLines})
	if err != nil {
		return nil, err
	}
	var regions []Region
	for _, h := range hits {
		for _, r := range h.Regions {
			if len(regions) >= o.MaxRegions {
				return regions, nil
			}
			regions = append(regions, Region{Path: h.RelPath, Start: r.Start, End: r.End})
		}
	}
	return regions, nil
}
