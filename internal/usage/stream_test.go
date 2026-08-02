package usage

import "testing"

func ev(session, agent, prompt, use, tool string, parallel bool, p map[string]any) Event {
	return Event{V: 1, TS: "2026-08-02T10:00:00Z", SessionID: session, AgentID: agent,
		PromptID: prompt, ToolUseID: use, Tool: tool, Parallel: parallel, P: p}
}

func TestBuildStreamsPartitionsBySessionAndAgent(t *testing.T) {
	events := []Event{
		ev("s1", "", "p1", "u1", "Grep", false, nil),
		ev("s1", "sub1", "p1", "u2", "Read", false, nil),
		ev("s2", "", "p1", "u3", "Glob", false, nil),
		ev("s1", "", "p1", "u4", "Read", false, nil),
	}
	streams := BuildStreams(events)
	if len(streams) != 3 {
		t.Fatalf("got %d streams", len(streams))
	}
	if len(streams[0].Elems) != 2 || streams[0].Agent != "" {
		t.Fatalf("main s1 stream = %+v", streams[0])
	}
}

func TestBuildStreamsDedupesToolUseID(t *testing.T) {
	events := []Event{
		ev("s1", "", "p1", "u1", "Grep", false, nil),
		ev("s1", "", "p1", "u1", "Grep", false, nil), // duplicate delivery
		ev("s1", "", "p1", "", "Read", false, nil),   // empty id: never deduped
		ev("s1", "", "p1", "", "Read", false, nil),
	}
	streams := BuildStreams(events)
	if n := len(streams[0].Elems); n != 3 {
		t.Fatalf("got %d elems, want 3", n)
	}
}

func TestBuildStreamsCollapsesParallelBatch(t *testing.T) {
	events := []Event{
		ev("s1", "", "p1", "u1", "mcp__k__search_hybrid", false, nil),
		ev("s1", "", "p1", "u2", "Grep", true, nil),
		ev("s1", "", "p1", "u3", "Read", true, nil),
		ev("s1", "", "p1", "u4", "Read", true, nil),
		ev("s1", "", "p1", "u5", "Edit", false, nil),
	}
	streams := BuildStreams(events)
	elems := streams[0].Elems
	if len(elems) != 3 {
		t.Fatalf("got %d elems, want 3 (single, batch, single)", len(elems))
	}
	if len(elems[1].Events) != 3 || !elems[1].Has(ClassSearch) || !elems[1].Has(ClassRead) {
		t.Fatalf("batch = %+v", elems[1])
	}
}

func TestBuildStreamsParallelAcrossPromptsNotCollapsed(t *testing.T) {
	events := []Event{
		ev("s1", "", "p1", "u1", "Grep", true, nil),
		ev("s1", "", "p2", "u2", "Grep", true, nil), // new prompt: new batch
	}
	streams := BuildStreams(events)
	if len(streams[0].Elems) != 2 {
		t.Fatalf("elems = %d, want 2", len(streams[0].Elems))
	}
}

func TestWindowsSegmentByPromptID(t *testing.T) {
	events := []Event{
		ev("s1", "", "p1", "u1", "Grep", false, nil),
		ev("s1", "", "p1", "u2", "Read", false, nil),
		ev("s1", "", "p2", "u3", "Glob", false, nil),
	}
	ws := BuildStreams(events)[0].Windows()
	if len(ws) != 2 || len(ws[0].Elems) != 2 || ws[1].PromptID != "p2" {
		t.Fatalf("windows = %+v", ws)
	}
}
