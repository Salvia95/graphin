package wiki

import "strings"

// RoleExempt marks an agent that the knowledge gate should let through
// untouched.
const RoleExempt = "exempt"

// AgentTable maps a subagent type to the role whose knowledge it needs, or to
// exemption.
//
// One table answers two questions on purpose. The delegation side needs a role
// to preflight for, and the gate needs to know whether this agent is subject
// at all; keeping those in separate files is how they drift into disagreeing
// about the same agent.
//
// The gate reads the resulting decision from a flag file, never from here —
// this table is consulted once per agent at spawn, not on every edit.
type AgentTable struct {
	roles map[string]string
}

// builtinAgents are the agents graphin itself ships. They are compiled in
// rather than configured because they run in other people's repositories,
// where no project file of ours exists to describe them: both are read-only
// investigators that hold Bash, so a gate keyed on tools alone would stop
// them for knowledge they will never use.
var builtinAgents = map[string]string{
	"graphin-explorer": RoleExempt,
	"release":          RoleExempt,
	"Explore":          RoleExempt,
	"Plan":             RoleExempt,
}

// NewAgentTable returns the built-in table.
func NewAgentTable() *AgentTable {
	roles := make(map[string]string, len(builtinAgents))
	for k, v := range builtinAgents {
		roles[k] = v
	}
	return &AgentTable{roles: roles}
}

// ParseAgents overlays a project's agents page onto the built-ins. Its
// frontmatter carries one `agents:` list of "subagent-type — role" lines,
// where the role may be `exempt`.
func ParseAgents(src []byte) *AgentTable {
	t := NewAgentTable()
	fm, _ := splitFrontmatter(src)
	for _, raw := range parseFrontmatter(fm).List("agents") {
		name, role := splitPair(raw)
		if name == "" {
			continue
		}
		if role == "" {
			role = RoleExempt
		}
		t.roles[name] = role
	}
	return t
}

// Role returns the role to preflight for, and whether the agent is gated at
// all. An agent the table has never heard of is gated with no specific role:
// defaulting to exempt would mean every new agent silently opts out, which is
// the failure mode the whole gate exists to prevent.
func (t *AgentTable) Role(subagentType string) (role string, gated bool) {
	r, ok := t.roles[subagentType]
	if !ok {
		return "", true
	}
	if strings.EqualFold(r, RoleExempt) {
		return "", false
	}
	return r, true
}
