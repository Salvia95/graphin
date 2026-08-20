package wiki

import "testing"

func TestAgentTableBuiltinsAreExempt(t *testing.T) {
	tbl := NewAgentTable()
	// graphin ships these and they run in other people's repositories, where
	// no project file of ours describes them. Both hold Bash while never
	// editing, so a gate keyed on tools alone would stop them for knowledge
	// they will never use.
	for _, name := range []string{"graphin-explorer", "release"} {
		if _, gated := tbl.Role(name); gated {
			t.Errorf("%s should be exempt", name)
		}
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
