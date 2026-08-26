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
		"graphin-explorer", "release", "Explore", "Plan",
		"statusline-setup", "fork",
	} {
		if _, gated := tbl.Role(name); gated {
			t.Errorf("%s should be exempt", name)
		}
	}
}

func TestAgentTableStripsPluginNamespace(t *testing.T) {
	tbl := NewAgentTable()
	// What actually arrives in the payload for a plugin agent. The built-in
	// table is written with bare names, so without the fallback graphin's own
	// explorer is gated by graphin's own gate.
	if _, gated := tbl.Role("graphin-guide:graphin-explorer"); gated {
		t.Error("a namespaced built-in should resolve to its bare entry")
	}
	// Stripping is a fallback, not a licence: an unknown agent stays gated
	// whichever form it arrives in.
	if _, gated := tbl.Role("some-plugin:some-new-agent"); !gated {
		t.Error("a namespaced unknown agent must stay gated")
	}
	// An empty suffix is not a name.
	if _, gated := tbl.Role("graphin-guide:"); !gated {
		t.Error("a bare namespace must not resolve to anything")
	}
}

func TestParseAgentsExactNameBeatsStripped(t *testing.T) {
	// A project that spells the namespaced agent out in full is overriding
	// the built-in for the bare one, not restating it.
	tbl := ParseAgents([]byte("---\nagents:\n  - graphin-guide:graphin-explorer — backend\n---\n"))

	if role, gated := tbl.Role("graphin-guide:graphin-explorer"); !gated || role != "backend" {
		t.Errorf("namespaced override = %q gated=%v, want backend/true", role, gated)
	}
	if _, gated := tbl.Role("graphin-explorer"); gated {
		t.Error("the bare built-in should be untouched by a namespaced override")
	}
}

func TestAgentTableUnknownIsGated(t *testing.T) {
	tbl := NewAgentTable()
	role, gated := tbl.Role("some-new-agent")
	if !gated {
		t.Fatal("an unknown agent must be gated: defaulting to exempt means every new agent silently opts out")
	}
	if role != "" {
		t.Errorf("role = %q, want empty for an unlisted agent", role)
	}
}

func TestParseAgentsOverlay(t *testing.T) {
	tbl := ParseAgents([]byte("---\nagents:\n  - backend-dev — backend\n  - docs-bot — exempt\n---\n"))

	if role, gated := tbl.Role("backend-dev"); !gated || role != "backend" {
		t.Errorf("backend-dev = %q gated=%v", role, gated)
	}
	if _, gated := tbl.Role("docs-bot"); gated {
		t.Error("docs-bot should be exempt")
	}
	// The overlay adds to the built-ins rather than replacing them.
	if _, gated := tbl.Role("graphin-explorer"); gated {
		t.Error("built-in exemption lost when a project table exists")
	}
}

func TestParseAgentsBareNameIsExempt(t *testing.T) {
	// A line with no role reads as "leave this one alone", which is the only
	// reason to name an agent without giving it knowledge.
	tbl := ParseAgents([]byte("---\nagents:\n  - lint-bot\n---\n"))
	if _, gated := tbl.Role("lint-bot"); gated {
		t.Error("a bare name should be exempt")
	}
}
