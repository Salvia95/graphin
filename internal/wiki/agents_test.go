package wiki

import "testing"

func TestAgentTableBuiltinsAreExempt(t *testing.T) {
	tbl := NewAgentTable()
	// graphin ships the first two and they run in other people's
	// repositories, where no project file of ours describes them; the rest
	// are stock agents that hold Bash while never editing for their own
	// reasons. A gate keyed on tools alone would stop all of them for
	// knowledge they will never use.
	for _, name := range []string{
		"graphin-explorer", "release", "Explore", "Plan", "statusline-setup",
	} {
		if _, v := tbl.Role(name); v != AgentExempt {
			t.Errorf("%s should be exempt, got verdict %d", name, v)
		}
	}
}

func TestAgentTableForkInherits(t *testing.T) {
	// Not exempt. A fork begins with its caller's context, so what it needs
	// is a fact about the caller — exempting it outright exempts it even when
	// the caller had loaded nothing.
	role, v := NewAgentTable().Role("fork")
	if v != AgentInherits {
		t.Fatalf("fork verdict = %d, want AgentInherits", v)
	}
	if role != "" {
		t.Errorf("role = %q, want empty: inherit names no knowledge to preflight for", role)
	}
}

func TestAgentTableStripsPluginNamespace(t *testing.T) {
	tbl := NewAgentTable()
	// What actually arrives in the payload for a plugin agent. The built-in
	// table is written with bare names, so without the fallback graphin's own
	// explorer is gated by graphin's own gate.
	if _, v := tbl.Role("graphin-guide:graphin-explorer"); v != AgentExempt {
		t.Error("a namespaced built-in should resolve to its bare entry")
	}
	// Stripping is a fallback, not a licence: an unknown agent stays gated
	// whichever form it arrives in.
	if _, v := tbl.Role("some-plugin:some-new-agent"); v != AgentGated {
		t.Error("a namespaced unknown agent must stay gated")
	}
	// An empty suffix is not a name.
	if _, v := tbl.Role("graphin-guide:"); v != AgentGated {
		t.Error("a bare namespace must not resolve to anything")
	}
}

func TestParseAgentsExactNameBeatsStripped(t *testing.T) {
	// A project that spells the namespaced agent out in full is overriding
	// the built-in for the bare one, not restating it.
	tbl := ParseAgents([]byte("---\nagents:\n  - graphin-guide:graphin-explorer — backend\n---\n"))

	if role, v := tbl.Role("graphin-guide:graphin-explorer"); v != AgentGated || role != "backend" {
		t.Errorf("namespaced override = %q verdict=%d, want backend/gated", role, v)
	}
	if _, v := tbl.Role("graphin-explorer"); v != AgentExempt {
		t.Error("the bare built-in should be untouched by a namespaced override")
	}
}

func TestAgentTableUnknownIsGated(t *testing.T) {
	tbl := NewAgentTable()
	role, v := tbl.Role("some-new-agent")
	if v != AgentGated {
		t.Fatal("an unknown agent must be gated: defaulting to exempt means every new agent silently opts out")
	}
	if role != "" {
		t.Errorf("role = %q, want empty for an unlisted agent", role)
	}
}

func TestParseAgentsOverlay(t *testing.T) {
	tbl := ParseAgents([]byte("---\nagents:\n  - backend-dev — backend\n  - docs-bot — exempt\n---\n"))

	if role, v := tbl.Role("backend-dev"); v != AgentGated || role != "backend" {
		t.Errorf("backend-dev = %q verdict=%d", role, v)
	}
	if _, v := tbl.Role("docs-bot"); v != AgentExempt {
		t.Error("docs-bot should be exempt")
	}
	// The overlay adds to the built-ins rather than replacing them.
	if _, v := tbl.Role("graphin-explorer"); v != AgentExempt {
		t.Error("built-in exemption lost when a project table exists")
	}
}

func TestParseAgentsBareNameIsExempt(t *testing.T) {
	// A line with no role reads as "leave this one alone", which is the only
	// reason to name an agent without giving it knowledge.
	tbl := ParseAgents([]byte("---\nagents:\n  - lint-bot\n---\n"))
	if _, v := tbl.Role("lint-bot"); v != AgentExempt {
		t.Error("a bare name should be exempt")
	}
}
