package graph

// Incremental re-resolution.
//
// A record's edges are a function of two things: its own raw references
// (calls, supers, imports, DB refs) and the definition table those references
// are looked up in. Track B re-resolves a record when the first changes. This
// file covers the second: each resolution records which lookups it made
// (dep), the resolver records which lookups a definition change can affect
// (resolver.changed), and Flush re-resolves exactly the records where the two
// meet. Without it, a caller whose target was deleted, renamed, or newly
// shadowed by a closer definition kept its old edge until its own bytes
// changed — the stale edges behind 1,661 dangling references in one TS
// workspace that a fresh index did not have.

// dep records one lookup made by the resolution in progress.
func (e *Engine) dep(key string) {
	if e.curDeps != nil {
		e.curDeps[key] = struct{}{}
	}
}

func (e *Engine) candidates(name string) []*DefInfo {
	e.dep(depName + name)
	return e.res.candidates(name)
}

func (e *Engine) classByFQN(fqn string) *DefInfo {
	e.dep(depID + fqn)
	return e.res.classByFQN(fqn)
}

func (e *Engine) defByID(id string) *DefInfo {
	e.dep(depID + id)
	return e.res.defByID(id)
}

func (e *Engine) dbActive() bool {
	e.dep(depDB)
	return e.res.dbActive()
}

func (e *Engine) dbSole() (string, string, bool) {
	e.dep(depDB)
	return e.res.dbSole()
}

// resolveTracked resolves one record and replaces its dependency set with
// the lookups this resolution made.
func (e *Engine) resolveTracked(rec *nodeRecord, sameFileDefs map[string][]*DefInfo) []Edge {
	e.curDeps = map[string]struct{}{}
	uses := e.resolveEdges(rec, sameFileDefs)
	e.setDepsLocked(rec, e.curDeps)
	e.curDeps = nil
	return uses
}

func (e *Engine) setDepsLocked(rec *nodeRecord, deps map[string]struct{}) {
	e.clearDepsLocked(rec)
	rec.deps = make([]string, 0, len(deps))
	for k := range deps {
		rec.deps = append(rec.deps, k)
		m := e.dependents[k]
		if m == nil {
			m = map[string]string{}
			e.dependents[k] = m
		}
		m[rec.ID] = rec.Pkg
	}
}

func (e *Engine) clearDepsLocked(rec *nodeRecord) {
	for _, k := range rec.deps {
		if m := e.dependents[k]; m != nil {
			delete(m, rec.ID)
			if len(m) == 0 {
				delete(e.dependents, k)
			}
		}
	}
	rec.deps = nil
}

// invalidateLocked marks for resolution every record that looked up a key
// the resolver reports changed. A record whose raw references have not been
// loaded this session is skipped: resolving it would rebuild its edges from
// nothing. It is resolved anyway when its file is first applied.
func (e *Engine) invalidateLocked() {
	for key := range e.res.takeChanged() {
		for id, pkg := range e.dependents[key] {
			if rec := e.records[pkg][id]; rec != nil && rec.rawLoaded {
				rec.needsResolve = true
				e.pending[pkg] = true
			}
		}
	}
}

func sameEdges(a, b []Edge) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
