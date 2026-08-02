package usage

import (
	"sort"
)

// Outcome of one graphin-nav run (spec §4.2).
type outcome int

const (
	outAdoption outcome = iota
	outFallback
	outInconclusive
	outMerged // follow-up was another nav run: same exploration, judged there
)

// GroupMetrics holds the headline counters for one population (all / main /
// subagent). All rates are derived at render time from these counts.
type GroupMetrics struct {
	Windows             int `json:"windows"`
	WindowsWithSearch   int `json:"windows_with_search"`
	WindowsWithGraphin  int `json:"windows_with_graphin"`
	Adoptions           int `json:"adoptions"`
	Fallbacks           int `json:"fallbacks"`
	SameIntentFallbacks int `json:"same_intent_fallbacks"`
	Inconclusive        int `json:"inconclusive"`
	LateSwitches        int `json:"late_switches"`
	DiscoveryFailures   int `json:"discovery_failures"`
	FunnelSearches      int `json:"funnel_searches"` // g_search calls that returned result_ids
	FunnelAdherent      int `json:"funnel_adherent"` // …whose ids were explored/read later in the window
}

// FallbackPair is one graphin-query → grep-pattern instance; same-intent
// pairs are the literal repro cases for index/ranking work (spec §0).
type FallbackPair struct {
	TS         string `json:"ts"`
	Query      string `json:"query"`
	Pattern    string `json:"pattern"`
	SameIntent bool   `json:"same_intent"`
}

// DayTrend is one UTC day's adoption/fallback counts.
type DayTrend struct {
	Date      string `json:"date"`
	Adoptions int    `json:"adoptions"`
	Fallbacks int    `json:"fallbacks"`
}

// Report is the full computation result; Markdown renders it, --json emits it
// verbatim.
type Report struct {
	Events                int                       `json:"events"`
	Sessions              int                       `json:"sessions"`
	SessionsWithGraphin   int                       `json:"sessions_with_graphin"`
	MedianCallsToFirstNav int                       `json:"median_calls_to_first_nav"` // -1: no session used graphin
	Groups                map[string]GroupMetrics   `json:"groups"`                    // keys: all, main, subagent
	FallbackPairs         []FallbackPair            `json:"fallback_pairs"`
	Bigrams               map[string]map[string]int `json:"bigrams"`
	Daily                 []DayTrend                `json:"daily"`
	Problems              []string                  `json:"problems,omitempty"`
}

// Options tunes Compute.
type Options struct {
	TopPairs int // FallbackPairs cap (most recent first); 0 = default 20
}

// Compute turns event streams into the usage report (spec §4).
func Compute(events []Event, problems []string, opts Options) Report {
	if opts.TopPairs <= 0 {
		opts.TopPairs = 20
	}
	streams := BuildStreams(events)

	rep := Report{
		Events:   len(events),
		Groups:   map[string]GroupMetrics{},
		Bigrams:  map[string]map[string]int{},
		Problems: problems,
	}
	groups := map[string]*GroupMetrics{"all": {}, "main": {}, "subagent": {}}
	daily := map[string]*DayTrend{}
	var pairs []FallbackPair

	sessions := map[string]bool{}
	sessionNav := map[string]bool{}
	sessionCalls := map[string]int{}    // total calls per session (append order)
	sessionFirstNav := map[string]int{} // calls before first nav

	for _, ev := range events {
		s := ev.SessionID
		sessions[s] = true
		sessionCalls[s]++
		if Classify(ev.Tool, ev.P).IsGraphinNav() && !sessionNav[s] {
			sessionNav[s] = true
			sessionFirstNav[s] = sessionCalls[s] - 1
		}
	}
	rep.Sessions = len(sessions)
	rep.SessionsWithGraphin = len(sessionNav)
	rep.MedianCallsToFirstNav = median(sessionFirstNav)

	for _, st := range streams {
		grp := "main"
		if st.Agent != "" {
			grp = "subagent"
		}
		addBigrams(rep.Bigrams, st.Elems)
		for _, w := range st.Windows() {
			analyzeWindow(w, []*GroupMetrics{groups["all"], groups[grp]}, daily, &pairs)
		}
	}

	for k, g := range groups {
		rep.Groups[k] = *g
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].TS > pairs[j].TS })
	if len(pairs) > opts.TopPairs {
		pairs = pairs[:opts.TopPairs]
	}
	rep.FallbackPairs = pairs
	for _, d := range daily {
		rep.Daily = append(rep.Daily, *d)
	}
	sort.Slice(rep.Daily, func(i, j int) bool { return rep.Daily[i].Date < rep.Daily[j].Date })
	return rep
}

// analyzeWindow applies the §4.2 definitions to one prompt window, adding
// counts to every metric group in gs (the "all" group plus main/subagent).
func analyzeWindow(w Window, gs []*GroupMetrics, daily map[string]*DayTrend, pairs *[]FallbackPair) {
	searchElems, hasNav := 0, false
	searchBeforeNav := 0
	for _, el := range w.Elems {
		isSearch := el.Has(ClassSearch)
		if isSearch {
			searchElems++
			if !hasNav {
				searchBeforeNav++
			}
		}
		if el.HasGraphinNav() {
			hasNav = true
		}
	}
	for _, g := range gs {
		g.Windows++
		if searchElems > 0 {
			g.WindowsWithSearch++
		}
		if hasNav {
			g.WindowsWithGraphin++
			if searchBeforeNav >= 2 {
				g.LateSwitches++
			}
		}
		if searchElems >= 3 && !hasNav {
			g.DiscoveryFailures++
		}
	}

	// Runs: maximal consecutive stretches of nav-bearing elements.
	for i := 0; i < len(w.Elems); {
		if !w.Elems[i].HasGraphinNav() {
			i++
			continue
		}
		j := i
		for j < len(w.Elems) && w.Elems[j].HasGraphinNav() {
			j++
		}
		out, pair := judgeRun(w.Elems[i:j], w.Elems[j:])
		day := w.Elems[i].Events[0].TS
		if len(day) >= 10 {
			day = day[:10]
		}
		d := daily[day]
		if d == nil {
			d = &DayTrend{Date: day}
			daily[day] = d
		}
		switch out {
		case outAdoption:
			d.Adoptions++
			for _, g := range gs {
				g.Adoptions++
			}
		case outFallback:
			d.Fallbacks++
			for _, g := range gs {
				g.Fallbacks++
				if pair != nil && pair.SameIntent {
					g.SameIntentFallbacks++
				}
			}
			if pair != nil {
				*pairs = append(*pairs, *pair)
			}
		case outInconclusive:
			for _, g := range gs {
				g.Inconclusive++
			}
		case outMerged:
			// judged by the following run
		}
		i = j
	}

	funnel(w, gs)
}

// judgeRun decides one nav run's outcome from the first decisive follow-up
// element (spec §4.2). rest starts right after the run.
func judgeRun(run, rest []Elem) (outcome, *FallbackPair) {
	lastQuery, queryTS := "", ""
	endsInRead := false
	for _, el := range run {
		for _, ev := range el.Events {
			switch Classify(ev.Tool, ev.P) {
			case ClassGSearch:
				if q, ok := ev.P["query"].(string); ok && q != "" {
					lastQuery, queryTS = q, ev.TS
				}
				endsInRead = false
			case ClassGExplore:
				endsInRead = false
			case ClassGRead:
				endsInRead = true
			}
		}
	}

	for _, el := range rest {
		// Pessimistic tie-break: a batch mixing search with read/action still
		// signals unmet need — search wins.
		if el.Has(ClassSearch) {
			pair := fallbackPair(lastQuery, queryTS, el)
			return outFallback, pair
		}
		if el.Has(ClassRead) || el.Has(ClassAction) {
			return outAdoption, nil
		}
		if el.HasGraphinNav() {
			// Same exploration resumed after non-decisive noise; the next
			// run's outcome covers it.
			return outMerged, nil
		}
		// other / g_boot / g_bench: not decisive, keep scanning.
	}
	if endsInRead {
		return outAdoption, nil // funnel completed inside graphin
	}
	return outInconclusive, nil
}

func fallbackPair(query, ts string, el Elem) *FallbackPair {
	if query == "" {
		return nil // run had no search_hybrid: nothing to compare intent with
	}
	qTok := Tokens(query)
	var first string
	for _, ev := range el.Events {
		if Classify(ev.Tool, ev.P) != ClassSearch {
			continue
		}
		pat, _ := ev.P["pattern"].(string)
		if pat == "" {
			continue
		}
		if first == "" {
			first = pat
		}
		if Overlaps(qTok, Tokens(pat)) {
			return &FallbackPair{TS: ts, Query: query, Pattern: pat, SameIntent: true}
		}
	}
	return &FallbackPair{TS: ts, Query: query, Pattern: first, SameIntent: false}
}

// funnel measures §4.3 handoff adherence: search_hybrid results whose node
// ids feed a later explore/read in the same window.
func funnel(w Window, gs []*GroupMetrics) {
	type pending struct{ ids map[string]bool }
	var open []pending
	adherent, total := 0, 0
	for _, el := range w.Elems {
		for _, ev := range el.Events {
			switch Classify(ev.Tool, ev.P) {
			case ClassGSearch:
				raw, ok := ev.P["result_ids"].([]any)
				if !ok || len(raw) == 0 {
					continue
				}
				ids := map[string]bool{}
				for _, id := range raw {
					if s, ok := id.(string); ok {
						ids[s] = true
					}
				}
				total++
				open = append(open, pending{ids: ids})
			case ClassGExplore, ClassGRead:
				id, _ := ev.P["node_id"].(string)
				if id == "" {
					continue
				}
				for k := len(open) - 1; k >= 0; k-- {
					if open[k].ids[id] {
						adherent++
						open = append(open[:k], open[k+1:]...)
						break
					}
				}
			}
		}
	}
	for _, g := range gs {
		g.FunnelSearches += total
		g.FunnelAdherent += adherent
	}
}

func addBigrams(m map[string]map[string]int, elems []Elem) {
	for i := 0; i+1 < len(elems); i++ {
		for _, a := range elems[i].Classes() {
			for _, b := range elems[i+1].Classes() {
				row := m[string(a)]
				if row == nil {
					row = map[string]int{}
					m[string(a)] = row
				}
				row[string(b)]++
			}
		}
	}
}

func median(firstNav map[string]int) int {
	if len(firstNav) == 0 {
		return -1
	}
	vals := make([]int, 0, len(firstNav))
	for _, v := range firstNav {
		vals = append(vals, v)
	}
	sort.Ints(vals)
	return vals[len(vals)/2]
}
