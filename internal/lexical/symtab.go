package lexical

import (
	"sort"
	"strings"
	"sync"
)

// SymbolTable backs Tier-0 exact-match routing (§2.1.1): a query equal to a
// node ID or a simple name short-circuits ranked search entirely. Simple
// names map many-to-one onto node IDs; all matches are returned.
type SymbolTable struct {
	mu       sync.RWMutex
	ids      map[string]struct{}
	bySimple map[string]map[string]struct{} // simple name → node ID set
	simpleOf map[string]string              // node ID → simple name
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{
		ids:      map[string]struct{}{},
		bySimple: map[string]map[string]struct{}{},
		simpleOf: map[string]string{},
	}
}

// Put registers or updates a node.
func (t *SymbolTable) Put(nodeID, simpleName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deleteLocked(nodeID)
	t.ids[nodeID] = struct{}{}
	t.simpleOf[nodeID] = simpleName
	set := t.bySimple[simpleName]
	if set == nil {
		set = map[string]struct{}{}
		t.bySimple[simpleName] = set
	}
	set[nodeID] = struct{}{}
}

// Delete removes a node if present.
func (t *SymbolTable) Delete(nodeID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deleteLocked(nodeID)
}

func (t *SymbolTable) deleteLocked(nodeID string) {
	simple, ok := t.simpleOf[nodeID]
	if !ok {
		return
	}
	delete(t.simpleOf, nodeID)
	delete(t.ids, nodeID)
	if set := t.bySimple[simple]; set != nil {
		delete(set, nodeID)
		if len(set) == 0 {
			delete(t.bySimple, simple)
		}
	}
}

// SimpleName returns the display name recorded for nodeID ("" if unknown).
func (t *SymbolTable) SimpleName(nodeID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.simpleOf[nodeID]
}

// Len returns the number of registered nodes.
func (t *SymbolTable) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.ids)
}

// Snapshot copies nodeID → simple name for persistence.
func (t *SymbolTable) Snapshot() map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]string, len(t.simpleOf))
	for id, s := range t.simpleOf {
		out[id] = s
	}
	return out
}

// Lookup returns every node ID exactly matching q as a full ID or simple
// name: exact-ID match first, then simple-name matches in lexicographic
// order. Multi-word queries never hit Tier-0.
func (t *SymbolTable) Lookup(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" || strings.ContainsAny(q, " \t\n") {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []string
	if _, ok := t.ids[q]; ok {
		out = append(out, q)
	}
	var simple []string
	for id := range t.bySimple[q] {
		if id != q {
			simple = append(simple, id)
		}
	}
	sort.Strings(simple)
	return append(out, simple...)
}
