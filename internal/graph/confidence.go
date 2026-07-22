package graph

import (
	"sort"
	"strings"
	"sync"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// Scope-weighted confidence tiers (§2.1.3): 1.0 syntactically certain,
// 0.95 same package, 0.90 imported symbol, 0.80 global same-name.
const (
	confCertain  = 1.0
	confSamePkg  = 0.95
	confImported = 0.90
	confGlobal   = 0.80
)

// maxGlobalCandidates bounds global same-name fan-out.
const maxGlobalCandidates = 5

// graphindb confidence tiers (schema/graphindb.md): FQN refs are exact (1.0),
// enforced:false logical refs 0.9, definition-token matches 0.8.
const (
	confDBLogical   = 0.9
	confDBHeuristic = 0.8
)

// DefInfo is one known definition, fed by the indexer as files parse.
type DefInfo struct {
	ID       string
	Pkg      string // shard key (declared package / python parent package)
	ClassFQN string // enclosing class FQN, or the def's own FQN for classes
	Simple   string
	Kind     string
	FilePath string
	ArityMin int
	ArityMax int // nodeid.UnboundedArity when open
}

func (d *DefInfo) isCallable() bool {
	return d.Kind == nodeid.KindMethod || d.Kind == nodeid.KindFunction
}

func (d *DefInfo) arityAccepts(args int) bool {
	if args < 0 { // unknowable call site (method reference)
		return true
	}
	if args < d.ArityMin {
		return false
	}
	return d.ArityMax == nodeid.UnboundedArity || args <= d.ArityMax
}

// resolver is the cross-file symbol table used for scope-weighted matching.
type resolver struct {
	mu     sync.RWMutex
	byName map[string]map[string]*DefInfo // simple name → id → def
	byID   map[string]*DefInfo
}

func newResolver() *resolver {
	return &resolver{byName: map[string]map[string]*DefInfo{}, byID: map[string]*DefInfo{}}
}

func (r *resolver) register(d *DefInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.byID[d.ID]; ok {
		delete(r.byName[old.Simple], d.ID)
	}
	r.byID[d.ID] = d
	set := r.byName[d.Simple]
	if set == nil {
		set = map[string]*DefInfo{}
		r.byName[d.Simple] = set
	}
	set[d.ID] = d
}

func (r *resolver) unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.byID[id]
	if !ok {
		return
	}
	delete(r.byID, id)
	if set := r.byName[d.Simple]; set != nil {
		delete(set, id)
		if len(set) == 0 {
			delete(r.byName, d.Simple)
		}
	}
}

func (r *resolver) candidates(simple string) []*DefInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := r.byName[simple]
	out := make([]*DefInfo, 0, len(set))
	for _, d := range set {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *resolver) defByID(id string) *DefInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[id]
}

func (r *resolver) classByFQN(fqn string) *DefInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d := r.byID[fqn]
	if d != nil && (d.Kind == nodeid.KindClass || d.Kind == nodeid.KindInterface) {
		return d
	}
	return nil
}

// imported reports whether def is visible through the source file's imports:
// exact class/member import or wildcard package import.
func imported(imports []string, def *DefInfo) bool {
	for _, imp := range imports {
		if imp == def.ClassFQN {
			return true
		}
		// member import (e.g. Kotlin top-level fn, Python from-import)
		if strings.HasSuffix(imp, "."+def.Simple) {
			prefix := strings.TrimSuffix(imp, "."+def.Simple)
			if strings.HasPrefix(def.ID, prefix+".") {
				return true
			}
		}
		if pkg, ok := strings.CutSuffix(imp, ".*"); ok {
			if strings.HasPrefix(def.ClassFQN, pkg+".") || def.Pkg == pkg {
				return true
			}
		}
	}
	return false
}

// resolveEdges computes the uses list for one node record (§2.1.3).
// sameFileDefs maps simple name → defs declared in the same source file.
func (e *Engine) resolveEdges(rec *nodeRecord, sameFileDefs map[string][]*DefInfo) []Edge {
	if nodeid.IsDBKind(rec.Kind) {
		return e.resolveDBEdges(rec)
	}
	best := map[string]Edge{} // targetID → strongest edge
	put := func(target string, t EdgeType, conf float32) {
		if target == rec.ID {
			return // self-edges add no navigation value
		}
		if cur, ok := best[target]; !ok || conf > cur.Confidence {
			best[target] = Edge{TargetID: target, Type: t, Confidence: conf}
		}
	}

	// 1) Supertypes: the relation is syntactically certain (§2.1.3 1.0).
	for _, super := range rec.Supers {
		name := simpleTypeName(super)
		tier, cands := bestTier(e.res.candidates(name), rec)
		for _, c := range cands {
			if c.Kind != nodeid.KindClass && c.Kind != nodeid.KindInterface {
				continue
			}
			t := EdgeExtends
			if c.Kind == nodeid.KindInterface {
				t = EdgeImplements
			}
			conf := float32(confCertain)
			if tier == tierGlobal {
				conf = confGlobal
			}
			put(c.ID, t, conf)
		}
	}

	// 2) Imports resolving to indexed classes (top-level class nodes carry
	//    the file's import edges).
	if rec.Container == "" && (rec.Kind == nodeid.KindClass || rec.Kind == nodeid.KindInterface) {
		for _, imp := range rec.Imports {
			if d := e.res.classByFQN(imp); d != nil {
				put(d.ID, EdgeImport, confCertain)
			}
		}
	}

	// 3) Calls: name + arity-range + scope ranking.
	for _, call := range rec.RawCalls {
		// Same-file self call → syntactically certain (§2.1.3).
		selfHit := false
		for _, d := range sameFileDefs[call.Name] {
			if d.isCallable() && d.arityAccepts(call.Args) {
				put(d.ID, EdgeCall, confCertain)
				selfHit = true
			}
		}
		if selfHit {
			continue
		}

		all := e.res.candidates(call.Name)
		var callable []*DefInfo
		var classes []*DefInfo
		for _, d := range all {
			if d.isCallable() && d.arityAccepts(call.Args) {
				callable = append(callable, d)
			} else if d.Kind == nodeid.KindClass || d.Kind == nodeid.KindInterface {
				classes = append(classes, d)
			}
		}
		if len(callable) > 0 {
			tier, cands := bestTier(callable, rec)
			conf := tierConf(tier)
			if tier == tierGlobal && (Stoplisted(call.Name) || len(cands) > maxGlobalCandidates) {
				continue
			}
			for _, c := range cands {
				put(c.ID, EdgeCall, conf)
			}
			continue
		}
		// Constructor-ish: the name matches a type (Kotlin data class,
		// Python class instantiation) → Reference edge.
		if len(classes) > 0 {
			tier, cands := bestTier(classes, rec)
			if tier == tierGlobal && (Stoplisted(call.Name) || len(cands) > maxGlobalCandidates) {
				continue
			}
			for _, c := range cands {
				put(c.ID, EdgeReference, tierConf(tier))
			}
		}
	}

	return sortEdges(best)
}

// resolveDBEdges is the graphindb branch: refs arrive as exact node FQNs, so
// scope ranking is unnecessary. Targets outside the snapshot (예: supabase
// auth.users) stay as dangling edges — 정직한 표현이며 스텁은 만들지 않는다.
func (e *Engine) resolveDBEdges(rec *nodeRecord) []Edge {
	best := map[string]Edge{}
	put := func(target string, t EdgeType, conf float32) {
		if target == rec.ID {
			return
		}
		if cur, ok := best[target]; !ok || conf > cur.Confidence {
			best[target] = Edge{TargetID: target, Type: t, Confidence: conf}
		}
	}

	// 1) Exact FQN refs: table FKs, explicit view/routine references, the
	//    RLS/trigger table anchor and the trigger function.
	for _, fqn := range rec.Supers {
		put(fqn, dbEdgeType(rec.Kind, e.res.defByID(fqn)), confCertain)
	}
	// 2) enforced:false logical refs (다형성, 크로스 데이터소스 ETL 등).
	for _, fqn := range rec.LogicalRefs {
		t := EdgeReference
		if rec.Kind == nodeid.KindTable {
			t = EdgeForeignKey
		}
		put(fqn, t, confDBLogical)
	}
	// 3) Definition-token heuristics: same-datasource tables/views only.
	for _, call := range rec.RawCalls {
		cands := e.res.candidates(call.Name)
		if len(cands) == 0 { // 스냅샷이 대문자 식별자(Oracle 등)를 쓰는 경우
			cands = e.res.candidates(strings.ToUpper(call.Name))
		}
		for _, c := range cands {
			if c.Pkg != rec.Pkg {
				continue
			}
			if c.Kind != nodeid.KindTable && c.Kind != nodeid.KindView {
				continue
			}
			put(c.ID, EdgeReference, confDBHeuristic)
		}
	}
	return sortEdges(best)
}

// dbEdgeType picks the edge type for an exact FQN ref: tables emit FKs,
// refs resolving to a routine are calls (trigger→function), the rest are
// references (dangling targets included).
func dbEdgeType(srcKind string, target *DefInfo) EdgeType {
	if srcKind == nodeid.KindTable {
		return EdgeForeignKey
	}
	if target != nil && (target.Kind == nodeid.KindDBFunction || target.Kind == nodeid.KindProcedure) {
		return EdgeCall
	}
	return EdgeReference
}

func sortEdges(best map[string]Edge) []Edge {
	out := make([]Edge, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TargetID != out[j].TargetID {
			return out[i].TargetID < out[j].TargetID
		}
		return out[i].Type < out[j].Type
	})
	return out
}

type tier int

const (
	tierSamePkg tier = iota
	tierImported
	tierGlobal
)

func tierConf(t tier) float32 {
	switch t {
	case tierSamePkg:
		return confSamePkg
	case tierImported:
		return confImported
	default:
		return confGlobal
	}
}

// bestTier buckets candidates by scope (§2.1.3 스코프 랭킹: 동일 패키지 >
// import된 심볼 > 전역 동명) and returns only the strongest occupied bucket.
func bestTier(cands []*DefInfo, rec *nodeRecord) (tier, []*DefInfo) {
	var same, imp, global []*DefInfo
	for _, c := range cands {
		switch {
		case c.Pkg == rec.Pkg:
			same = append(same, c)
		case imported(rec.Imports, c):
			imp = append(imp, c)
		default:
			global = append(global, c)
		}
	}
	if len(same) > 0 {
		return tierSamePkg, same
	}
	if len(imp) > 0 {
		return tierImported, imp
	}
	return tierGlobal, global
}

// simpleTypeName strips generics and qualifiers: "a.b.Foo<Bar>" → "Foo".
func simpleTypeName(s string) string {
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '.'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}
