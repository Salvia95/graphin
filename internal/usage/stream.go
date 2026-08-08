package usage

// Elem is one sequence element: a single tool call, or a collapsed parallel
// batch treated as an unordered set (spec §4.1 — no intra-batch bigrams).
type Elem struct {
	Events []Event
}

// Classes returns the element's class set (deduplicated, insertion order).
func (e Elem) Classes() []Class {
	seen := map[Class]bool{}
	var out []Class
	for _, ev := range e.Events {
		c := Classify(ev.Tool, ev.P)
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// Has reports whether any event in the element classifies as c.
func (e Elem) Has(c Class) bool {
	for _, ev := range e.Events {
		if Classify(ev.Tool, ev.P) == c {
			return true
		}
	}
	return false
}

// HasSymbolSearch reports whether any search in the element used a
// symbol-shaped pattern — the subset of searches graphin could have answered
// (spec §4.2). A collapsed batch counts if any member qualifies, matching Has.
func (e Elem) HasSymbolSearch() bool {
	for _, ev := range e.Events {
		if Classify(ev.Tool, ev.P) == ClassSearch && PatternShape(ev.SearchPattern()) == ShapeSymbol {
			return true
		}
	}
	return false
}

// HasGraphinNav reports whether the element contains a graphin navigation call.
func (e Elem) HasGraphinNav() bool {
	for _, ev := range e.Events {
		if Classify(ev.Tool, ev.P).IsGraphinNav() {
			return true
		}
	}
	return false
}

// Stream is the ordered tool-call sequence of one (session, agent) pair.
// Subagents get their own streams so interleaved main/subagent calls never
// fabricate bigrams (spec §4.1).
type Stream struct {
	Session string
	Agent   string // "" == main loop
	Elems   []Elem
}

// Window is the metric unit: one stream's elements for one prompt_id
// (one user turn).
type Window struct {
	PromptID string
	Elems    []Elem
}

// BuildStreams partitions events into per-(session, agent) streams in file
// append order (never ts order — spec §4.1), dedupes tool_use_id within a
// stream, and collapses parallel batches.
func BuildStreams(events []Event) []Stream {
	type key struct{ session, agent string }
	idx := map[key]int{}
	var streams []Stream
	seenUse := map[key]map[string]bool{}

	for _, ev := range events {
		k := key{ev.SessionID, ev.AgentID}
		i, ok := idx[k]
		if !ok {
			i = len(streams)
			idx[k] = i
			streams = append(streams, Stream{Session: ev.SessionID, Agent: ev.AgentID})
			seenUse[k] = map[string]bool{}
		}
		if ev.ToolUseID != "" {
			if seenUse[k][ev.ToolUseID] {
				continue
			}
			seenUse[k][ev.ToolUseID] = true
		}
		s := &streams[i]
		// Collapse: consecutive parallel events sharing a prompt_id join the
		// open batch. There is no batch id on PostToolUse, so consecutiveness
		// is the heuristic (spec §4.1).
		if ev.Parallel && len(s.Elems) > 0 {
			last := &s.Elems[len(s.Elems)-1]
			lastEv := last.Events[len(last.Events)-1]
			if lastEv.Parallel && lastEv.PromptID == ev.PromptID {
				last.Events = append(last.Events, ev)
				continue
			}
		}
		s.Elems = append(s.Elems, Elem{Events: []Event{ev}})
	}
	return streams
}

// Windows segments the stream by consecutive prompt_id runs.
func (s Stream) Windows() []Window {
	var out []Window
	for _, el := range s.Elems {
		pid := el.Events[0].PromptID
		if n := len(out); n > 0 && out[n-1].PromptID == pid {
			out[n-1].Elems = append(out[n-1].Elems, el)
			continue
		}
		out = append(out, Window{PromptID: pid, Elems: []Elem{el}})
	}
	return out
}
