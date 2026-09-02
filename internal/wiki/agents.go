package wiki

import "strings"

// RoleExempt marks an agent that the knowledge gate should let through
// untouched.
const RoleExempt = "exempt"

// RoleInherit marks an agent that starts holding its caller's context instead
// of an empty one. Such an agent is NOT exempt: what it needs is whatever the
// caller had, so it is cleared only when the caller was.
const RoleInherit = "inherit"

// AgentVerdict is what the table says about one agent.
//
// Three answers, because "does this agent need knowledge of its own" has
// three honest ones. Collapsing the third into exempt is how a gate grows a
// hole: an agent that inherits a cleared caller's knowledge and one that
// inherits an empty context are the same agent, and only the caller's state
// tells them apart.
type AgentVerdict int

const (
	// AgentGated: this agent needs a manifest of its own.
	AgentGated AgentVerdict = iota
	// AgentExempt: this agent never needs project knowledge, whoever
	// spawned it and whatever they had loaded.
	AgentExempt
	// AgentInherits: decided by looking at the caller, not at this table.
	AgentInherits
)

// AgentTable maps a subagent type to the role whose knowledge it needs, or to
// exemption, or to its caller.
//
// One table answers both questions on purpose. The delegation side needs a
// role to preflight for, and the gate needs a verdict on whether this agent is
// subject at all; keeping those in separate files is how they drift into
// disagreeing about the same agent.
//
// The gate reads the resulting decision from a flag file, never from here —
// this table is consulted once per agent at spawn, not on every edit.
type AgentTable struct {
	roles map[string]string
}

// builtinAgents are the agents graphin ships and the stock ones whose need
// for knowledge is already settled. They are compiled in rather than
// configured because they run in other people's repositories, where no
// project file of ours exists to describe them: they hold Bash while never
// editing for their own reasons, so a gate keyed on tools alone would stop
// them for knowledge they will never use.
var builtinAgents = map[string]string{
	"graphin-rag": RoleExempt,
	// Merged into graphin-rag in graphin-guide 0.6.0. Kept exempt because the
	// binary and the plugin ship separately: an install that still has the old
	// agent must not start failing a gate it passed yesterday.
	"graphin-explorer": RoleExempt,
	"release":          RoleExempt,
	// Works ON the wiki, not with it: it repairs sets through wiki_edit_set,
	// which judges every write, and never edits a project file. Its Bash
	// runs `graphin wiki check` and `graphin wiki queue`, and a gate that
	// stopped those for project knowledge would stop the maintenance the
	// wiki depends on to stay true.
	"wiki-maintainer": RoleExempt,
	"Explore":         RoleExempt,
	"Plan":            RoleExempt,
	// Only ever edits a settings file of the user's own.
	"statusline-setup": RoleExempt,
	// Deliberately not exempt. A fork begins with its caller's context, so
	// what it needs is whatever the caller had — which is a fact about the
	// caller, not about this agent. See AgentInherits.
	"fork": RoleInherit,
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
//
// The lookup falls back to the name without its plugin namespace, because a
// plugin agent arrives as "plugin:agent" — graphin's own explorer is
// "graphin-guide:graphin-explorer" in the payload, not the bare name this
// table is written with. Without the fallback the built-ins are dead letters
// exactly where they were meant to work: someone else's repository, where no
// agents page of ours exists to name them again.
//
// Exact match wins, so a project that spells a namespaced agent out in full
// still overrides the built-in entry for the bare one.
//
// Exempt and inherit both return an empty role. Neither names knowledge to
// preflight for, and a caller that printed one of those words as a role would
// be repeating a verdict back as an answer.
func (t *AgentTable) Role(subagentType string) (role string, verdict AgentVerdict) {
	r, ok := t.roles[subagentType]
	if !ok {
		r, ok = t.roles[stripNamespace(subagentType)]
	}
	if !ok {
		return "", AgentGated
	}
	switch {
	case strings.EqualFold(r, RoleExempt):
		return "", AgentExempt
	case strings.EqualFold(r, RoleInherit):
		return "", AgentInherits
	}
	return r, AgentGated
}

// stripNamespace drops a leading "plugin:" qualifier. It returns the input
// unchanged when there is nothing to strip, so the caller can look the result
// up unconditionally — an empty suffix ("plugin:") is not a name and stays a
// miss.
func stripNamespace(subagentType string) string {
	i := strings.LastIndex(subagentType, ":")
	if i < 0 {
		return subagentType
	}
	return subagentType[i+1:]
}
