package wiki

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
)

// DirName is the wiki's location inside a workspace. It sits under docs/ so
// that the wiki is itself part of the documentation corpus the index already
// covers: every page here is a set of section nodes the moment it is written,
// with no separate ingestion step.
const DirName = "docs/wiki"

const (
	setsSubdir     = "sets"
	glossarySubdir = "glossary"
	proposeSubdir  = "propose"
	agentsFile     = "agents.md"
)

// Store is a loaded wiki: the authored pages plus the lockfile.
type Store struct {
	Root   string // workspace root (absolute or relative, as given)
	Dir    string // workspace-relative wiki directory
	Sets   map[string]*Set
	Terms  map[string]*Term
	Pins   *Pins
	Agents *AgentTable

	// redirector is set by callers that have an index. See Redirector.
	redirector Redirector
}

// SetRedirector attaches redirect resolution to this store.
func (s *Store) SetRedirector(r Redirector) { s.redirector = r }

// current maps a node ID through any recorded redirect. Nil-safe, so the
// index-free paths need no special case.
func (s *Store) current(id string) string {
	if s.redirector == nil {
		return id
	}
	return s.redirector.ResolveID(id)
}

// Load reads the whole wiki. A workspace with no wiki directory loads as an
// empty store rather than an error: preflight has to answer "no knowledge
// applies" for such a project, and it cannot do that from a failure.
func Load(root string) (*Store, error) {
	s := &Store{
		Root:   root,
		Dir:    DirName,
		Sets:   map[string]*Set{},
		Terms:  map[string]*Term{},
		Pins:   NewPins(),
		Agents: NewAgentTable(),
	}
	base := filepath.Join(root, filepath.FromSlash(DirName))
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return s, nil
	}

	if err := s.loadDir(setsSubdir, func(rel string, src []byte) error {
		set, err := ParseSet(rel, src)
		if err != nil {
			return err
		}
		if prev, dup := s.Sets[set.Name]; dup {
			return fmt.Errorf("set %q declared twice: %s and %s", set.Name, prev.RelPath, rel)
		}
		s.Sets[set.Name] = set
		return nil
	}); err != nil {
		return nil, err
	}

	if err := s.loadDir(glossarySubdir, func(rel string, src []byte) error {
		t, err := ParseTerm(rel, src)
		if err != nil {
			return err
		}
		if prev, dup := s.Terms[t.Canonical]; dup {
			return fmt.Errorf("term %q declared twice: %s and %s", t.Canonical, prev.RelPath, rel)
		}
		s.Terms[t.Canonical] = t
		return nil
	}); err != nil {
		return nil, err
	}

	if raw, err := os.ReadFile(filepath.Join(base, agentsFile)); err == nil {
		s.Agents = ParseAgents(raw)
	}

	pins, err := LoadPins(filepath.Join(base, PinsFile))
	if err != nil {
		return nil, err
	}
	s.Pins = pins
	return s, nil
}

// loadDir applies fn to every .md file in one wiki subdirectory, in name
// order so that a duplicate is always reported against the same file.
func (s *Store) loadDir(sub string, fn func(rel string, src []byte) error) error {
	dir := filepath.Join(s.Root, filepath.FromSlash(pathpkg.Join(s.Dir, sub)))
	ents, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		rel := pathpkg.Join(s.Dir, sub, name)
		src, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if err := fn(rel, src); err != nil {
			return err
		}
	}
	return nil
}

// SetList returns every set in name order.
func (s *Store) SetList() []*Set {
	names := make([]string, 0, len(s.Sets))
	for n := range s.Sets {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Set, 0, len(names))
	for _, n := range names {
		out = append(out, s.Sets[n])
	}
	return out
}

// PinsPath is where the lockfile lives.
func (s *Store) PinsPath() string {
	return filepath.Join(s.Root, filepath.FromSlash(pathpkg.Join(s.Dir, PinsFile)))
}

// Expand resolves a selection of set names into the full reading list,
// prerequisites first.
//
// The order is the point: a prerequisite that arrives after the set that
// assumes it has already failed at its one job. Depth-first post-order gives
// that, and a set already on the list is not added twice.
//
// Unknown names are returned separately instead of being dropped. A caller
// asking for a set that does not exist has a coverage miss worth recording,
// and silently returning a shorter list would hide it.
func (s *Store) Expand(names []string) (sets []*Set, missing []string) {
	seen := map[string]bool{}
	inProgress := map[string]bool{}

	var visit func(string)
	visit = func(name string) {
		if seen[name] || inProgress[name] {
			// inProgress catches a prerequisite cycle. Stopping quietly is
			// right here: the cycle is a wiki authoring bug, but a reader
			// asking for knowledge should still get everything reachable.
			return
		}
		set, ok := s.Sets[name]
		if !ok {
			missing = append(missing, name)
			seen[name] = true
			return
		}
		inProgress[name] = true
		for _, p := range set.Prerequisites {
			visit(p)
		}
		delete(inProgress, name)
		seen[name] = true
		sets = append(sets, set)
	}
	for _, n := range names {
		visit(n)
	}
	return sets, missing
}

// ForRole returns the sets tagged for a role, in name order. A set tagged
// "all" is returned for every role; a set with no roles at all is pull-only
// and never arrives this way.
func (s *Store) ForRole(role string) []*Set {
	var out []*Set
	for _, set := range s.SetList() {
		for _, r := range set.Roles {
			if strings.EqualFold(r, role) || strings.EqualFold(r, "all") {
				out = append(out, set)
				break
			}
		}
	}
	return out
}
