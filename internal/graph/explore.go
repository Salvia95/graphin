package graph

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/llls2542/graphin/internal/graph/fbsgen"
)

// ErrNodeNotFound: the explored node is not on the read path.
var ErrNodeNotFound = errors.New("graph: node not found")

// PageSize caps edges per explore_graph response (§3.3).
const PageSize = 20

// EdgeOut is one edge in an explore page.
type EdgeOut struct {
	NodeID     string
	Type       string
	Confidence float32
}

// Page is one explore_graph result page.
type Page struct {
	Target     string
	Uses       []EdgeOut
	UsedBy     []EdgeOut
	HasMore    bool
	NextCursor string
}

// entry is the internal sortable form. section: 0 = uses, 1 = used_by.
type entry struct {
	section int
	id      string
	typ     EdgeType
	conf    float32
	samePkg bool
}

// cursorKey is the §4.4-b seek-key: pagination survives shard regeneration
// because it resumes strictly after a sort position, not a byte offset.
type cursorKey struct {
	V       int     `json:"v"`
	Node    string  `json:"n"`
	Dir     string  `json:"d"`
	MinConf float32 `json:"m"`
	Section int     `json:"s"`
	Conf    float32 `json:"c"`
	SamePkg bool    `json:"p"`
	ID      string  `json:"f"`
}

func encodeCursor(k cursorKey) string {
	b, _ := json.Marshal(k)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursorKey, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursorKey{}, false
	}
	var k cursorKey
	if err := json.Unmarshal(b, &k); err != nil || k.V != 1 {
		return cursorKey{}, false
	}
	return k, true
}

// Explore returns one deterministic page of edges (§3.3 정렬: confidence
// desc → 동일 패키지 우선 → FQN 사전순; uses section before used_by).
func (e *Engine) Explore(nodeID, direction, cursor string, minConf float32) (*Page, error) {
	e.mu.RLock()
	loc, ok := e.nodeLoc[nodeID]
	e.mu.RUnlock()
	if !ok {
		return nil, ErrNodeNotFound
	}
	ownPkg := loc.pkg

	var entries []entry
	if direction == "uses" || direction == "both" {
		for _, u := range e.readUses(nodeID) {
			if u.Confidence < minConf {
				continue
			}
			entries = append(entries, entry{
				section: 0, id: u.TargetID, typ: u.Type, conf: u.Confidence,
				samePkg: inPackage(u.TargetID, ownPkg),
			})
		}
	}
	if direction == "used_by" || direction == "both" {
		for _, r := range e.rev.Query(nodeID, minConf) {
			entries = append(entries, entry{
				section: 1, id: r.SourceID, typ: r.Type, conf: r.Confidence,
				samePkg: inPackage(r.SourceID, ownPkg),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entryLess(entries[i], entries[j]) })

	if cursor != "" {
		if k, ok := decodeCursor(cursor); ok && k.Node == nodeID {
			after := entry{section: k.Section, id: k.ID, conf: k.Conf, samePkg: k.SamePkg}
			i := sort.Search(len(entries), func(i int) bool {
				return !entryLess(entries[i], after) && !entryEq(entries[i], after)
			})
			// skip the cursor element itself if still present
			for i < len(entries) && entryEq(entries[i], after) {
				i++
			}
			entries = entries[i:]
		}
	}

	page := &Page{Target: nodeID}
	n := len(entries)
	take := entries
	if n > PageSize {
		take = entries[:PageSize]
		page.HasMore = true
		last := take[len(take)-1]
		page.NextCursor = encodeCursor(cursorKey{
			V: 1, Node: nodeID, Dir: direction, MinConf: minConf,
			Section: last.section, Conf: last.conf, SamePkg: last.samePkg, ID: last.id,
		})
	}
	for _, en := range take {
		out := EdgeOut{NodeID: en.id, Type: EdgeTypeName(en.typ), Confidence: en.conf}
		if en.section == 0 {
			page.Uses = append(page.Uses, out)
		} else {
			page.UsedBy = append(page.UsedBy, out)
		}
	}
	return page, nil
}

func entryLess(a, b entry) bool {
	if a.section != b.section {
		return a.section < b.section
	}
	if a.conf != b.conf {
		return a.conf > b.conf
	}
	if a.samePkg != b.samePkg {
		return a.samePkg
	}
	return a.id < b.id
}

func entryEq(a, b entry) bool {
	return a.section == b.section && a.conf == b.conf && a.samePkg == b.samePkg && a.id == b.id
}

// readUses walks the mmapped shard zero-copy; strings are copied out because
// the mapping may be retired after Release.
func (e *Engine) readUses(nodeID string) []Edge {
	e.mu.RLock()
	loc, ok := e.nodeLoc[nodeID]
	if !ok {
		e.mu.RUnlock()
		return nil
	}
	h := e.shards[loc.pkg]
	if h == nil {
		e.mu.RUnlock()
		return nil
	}
	h.ref.Acquire()
	e.mu.RUnlock()
	defer h.ref.Release()

	sh := fbsgen.GetRootAsShard(h.ref.Data, 0)
	var node fbsgen.Node
	if !sh.Nodes(&node, loc.idx) {
		return nil
	}
	var edge fbsgen.Edge
	out := make([]Edge, 0, node.UsesLength())
	for i := 0; i < node.UsesLength(); i++ {
		if node.Uses(&edge, i) {
			out = append(out, Edge{
				TargetID:   string(edge.TargetId()),
				Type:       edge.Type(),
				Confidence: edge.Confidence(),
			})
		}
	}
	return out
}

// inPackage is the deterministic same-package predicate used for sorting.
func inPackage(id, pkg string) bool {
	if pkg == "" || pkg == "_root" {
		return !strings.Contains(id, ".")
	}
	return strings.HasPrefix(id, pkg+".")
}
