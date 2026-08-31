// Package keyword is graphin's own literal/regex file search — the fourth
// retriever beside Tier-0, BM25 and the vector index.
//
// It exists because keyword search used to live outside graphin entirely: the
// agent reached for its host's grep, and everything that happened there was
// invisible to graphin's ranking, its response budget and its usage
// instrumentation. A retriever the tool cannot see is a retriever the tool
// cannot complement.
//
// **This package also defines the SWE-Explore grep baseline**
// (`eval/sweexplore.GrepRegions`). That is deliberate — it makes it impossible
// for graphin to win a benchmark by quietly holding a better grep than the one
// it is compared against, and it stops the baseline from drifting weaker than
// the product. The cost is that changing the matcher moves the baseline too:
// a change here needs a benchmark run, not just a unit test.
package keyword

import (
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/scan"
)

// Line is one matching line: where it is and what it says.
type Line struct {
	No   int    // 1-based
	Byte int    // offset of the line start, for resolving the hit to a node
	Text string // trimmed; callers truncate for display
}

// Region is a merged ±context window, in 1-based inclusive lines.
type Region struct {
	Start int
	End   int
}

// Hit is one file that matched.
type Hit struct {
	RelPath string
	Matches int
	Lines   []Line   // capped by Options.MaxLines
	Regions []Region // only when Options.ContextLines > 0
}

// Options selects what to match and how much to bring back.
type Options struct {
	// Terms are lowercased literals; a line matches if it contains any of
	// them. Ignored when Re is set.
	Terms []string
	Re    *regexp.Regexp

	PathContains string // rel-path substring filter; "" matches every file
	ContextLines int    // >0 builds merged windows (the benchmark baseline)
	MaxLines     int    // matched lines kept per file; 0 = none
	MaxFiles     int    // files returned after ranking; 0 = all
}

// Compile turns a user pattern into a matcher. Literal is the default because
// most agent queries are names, not expressions; regex is opt-in so that a
// pattern containing regex metacharacters is not silently reinterpreted.
func Compile(pattern string, asRegex bool) (Options, error) {
	if asRegex {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return Options{}, err
		}
		return Options{Re: re}, nil
	}
	return Options{Terms: []string{strings.ToLower(pattern)}}, nil
}

// Search walks the workspace the way the indexer does — same scan, same ignore
// rules — so a keyword hit and an indexed node can never disagree about which
// files exist. Files rank by match count, ties by path.
func Search(root string, o Options) ([]Hit, error) {
	res, err := scan.Walk(root, obs.Nop())
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for _, f := range res.Files {
		if o.PathContains != "" && !strings.Contains(f.RelPath, o.PathContains) {
			continue
		}
		src, err := os.ReadFile(f.AbsPath)
		if err != nil {
			continue
		}
		if h, ok := searchFile(f.RelPath, string(src), o); ok {
			hits = append(hits, h)
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Matches != hits[j].Matches {
			return hits[i].Matches > hits[j].Matches
		}
		return hits[i].RelPath < hits[j].RelPath
	})
	if o.MaxFiles > 0 && len(hits) > o.MaxFiles {
		hits = hits[:o.MaxFiles]
	}
	return hits, nil
}

func searchFile(rel, src string, o Options) (Hit, bool) {
	lines := strings.Split(src, "\n")
	include := make([]bool, len(lines))
	h := Hit{RelPath: rel}
	offset := 0
	for i, line := range lines {
		lineStart := offset
		offset += len(line) + 1 // +\n; the last line's phantom byte is never read
		if !o.matches(line) {
			continue
		}
		h.Matches++
		if o.MaxLines > 0 && len(h.Lines) < o.MaxLines {
			h.Lines = append(h.Lines, Line{No: i + 1, Byte: lineStart, Text: strings.TrimSpace(line)})
		}
		if o.ContextLines > 0 {
			lo, hi := max(0, i-o.ContextLines), min(len(lines)-1, i+o.ContextLines)
			for j := lo; j <= hi; j++ {
				include[j] = true
			}
		}
	}
	if h.Matches == 0 {
		return Hit{}, false
	}
	h.Regions = mergeWindows(include, len(lines))
	return h, true
}

func (o Options) matches(line string) bool {
	if o.Re != nil {
		return o.Re.MatchString(line)
	}
	low := strings.ToLower(line)
	for _, t := range o.Terms {
		if strings.Contains(low, t) {
			return true
		}
	}
	return false
}

// mergeWindows turns the per-line include mask into 1-based inclusive spans.
func mergeWindows(include []bool, n int) []Region {
	var out []Region
	start := -1
	for i, on := range include {
		switch {
		case on && start < 0:
			start = i
		case !on && start >= 0:
			out = append(out, Region{Start: start + 1, End: i})
			start = -1
		}
	}
	if start >= 0 {
		out = append(out, Region{Start: start + 1, End: n})
	}
	return out
}
