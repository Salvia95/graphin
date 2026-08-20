package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/Salvia95/graphin/internal/graph/fbsgen"
	"github.com/Salvia95/graphin/internal/merkle"
	"github.com/Salvia95/graphin/internal/nodeid"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
	"github.com/Salvia95/graphin/internal/store"
)

// nodeRecord is the engine's canonical per-node state. Records are owned by
// the single-writer indexer; shards are their read-optimized projection.
type nodeRecord struct {
	ID          string
	DisplayName string
	SimpleName  string
	Kind        string
	Container   string
	FilePath    string
	Start, End  uint32
	Hash        []byte
	// RenameKey identifies content that outlives its own name, so a node
	// that vanished can be matched to the one that replaced it. Persisted
	// with the shard: without it, the first rename after a restart looks
	// like a plain deletion, because the old record came back from disk.
	RenameKey    []byte
	Partial      bool
	Pkg          string // shard key
	ArityMin     int
	ArityMax     int      // nodeid.UnboundedArity when open
	Supers       []string // for changed nodes; DB 노드는 확정 참조 FQN
	LogicalRefs  []string // graphindb: enforced:false 논리 참조 FQN
	Contains     []string // markdown: 파싱 시점에 확정된 자식 노드 ID
	RawCalls     []parse.Call
	DBRefs       []parse.DBRef // 코드→DB 크로스 도메인 참조 (Phase 7a)
	Imports      []string
	Uses         []Edge // current resolved edges (loaded or computed)
	needsResolve bool
}

type shardHandle struct {
	ref *store.MMapRef
	gen uint64
	ids []string // node IDs in this shard, in vector order
	idx map[string]int
}

type nodeLoc struct {
	pkg string
	idx int
}

// Engine is the heuristic graph engine (§2.1.3).
type Engine struct {
	dir string
	log *obs.Logger

	// Read path: shards + node locator, swapped under mu (readers RLock).
	mu      sync.RWMutex
	shards  map[string]*shardHandle
	nodeLoc map[string]nodeLoc

	// Write path: single-writer state (indexer goroutine / indexMu holder).
	stateMu sync.Mutex
	records map[string]map[string]*nodeRecord // pkg → id → record
	dirty   map[string]bool
	gens    map[string]uint64

	res *resolver
	rev *Reverse

	// clock stamps redirect epochs. It is a field so a test can pin the
	// value; nothing else in the engine reads a clock.
	clock func() uint64
}

// now returns the current redirect epoch.
func (e *Engine) now() uint64 {
	if e.clock != nil {
		return e.clock()
	}
	return uint64(time.Now().Unix())
}

// Open loads existing shards and the reverse index from dir (creating it).
func Open(dir string, lg *obs.Logger) (*Engine, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	rev, err := OpenReverse(dir, lg)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		dir:     dir,
		log:     lg,
		shards:  map[string]*shardHandle{},
		nodeLoc: map[string]nodeLoc{},
		records: map[string]map[string]*nodeRecord{},
		dirty:   map[string]bool{},
		gens:    map[string]uint64{},
		res:     newResolver(),
		rev:     rev,
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "chunk_*.fb"))
	sort.Strings(paths)
	for _, p := range paths {
		if err := e.loadShard(p); err != nil {
			lg.Event("shard_load_error", map[string]any{"path": p, "error": err.Error()})
			_ = os.Remove(p) // corrupt shard: rebuilt from the next scan
		}
	}
	return e, nil
}

// loadShard mmaps one shard and reconstructs records (edges only — raw calls
// reappear when the file is next parsed as Changed).
func (e *Engine) loadShard(path string) error {
	ref, err := store.OpenMMap(path)
	if err != nil {
		return err
	}
	if len(ref.Data) < 8 {
		ref.Retire()
		return fmt.Errorf("short shard file")
	}
	sh := fbsgen.GetRootAsShard(ref.Data, 0)
	pkg := string(sh.PackagePrefix())
	h := &shardHandle{ref: ref, gen: sh.Generation(), idx: map[string]int{}}

	recs := map[string]*nodeRecord{}
	var node fbsgen.Node
	for i := 0; i < sh.NodesLength(); i++ {
		if !sh.Nodes(&node, i) {
			continue
		}
		id := string(node.Id())
		rec := &nodeRecord{
			ID:          id,
			DisplayName: string(node.DisplayName()),
			SimpleName:  nodeid.Simple(id),
			Kind:        string(node.Kind()),
			FilePath:    string(node.FilePath()),
			Start:       node.StartByte(),
			End:         node.EndByte(),
			RenameKey:   append([]byte(nil), node.RenameKeyBytes()...),
			Hash:        append([]byte(nil), node.SubtreeHashBytes()...),
			Partial:     node.Partial(),
			Pkg:         pkg,
		}
		var edge fbsgen.Edge
		for j := 0; j < node.UsesLength(); j++ {
			if node.Uses(&edge, j) {
				rec.Uses = append(rec.Uses, Edge{
					TargetID:   string(edge.TargetId()),
					Type:       edge.Type(),
					Confidence: edge.Confidence(),
				})
			}
		}
		recs[id] = rec
		h.ids = append(h.ids, id)
		h.idx[id] = i
	}
	e.records[pkg] = recs
	e.gens[pkg] = h.gen
	e.shards[pkg] = h
	for id, i := range h.idx {
		e.nodeLoc[id] = nodeLoc{pkg: pkg, idx: i}
	}
	return nil
}

// shardKey groups nodes into shards: declared package for JVM languages,
// parent package for file-scoped module languages (Python, JS/TS), "_root"
// for the empty package.
func shardKey(res *parse.FileResult) string {
	pkg := res.Package
	if res.Lang.FileScoped() {
		if i := strings.LastIndexByte(pkg, '.'); i >= 0 {
			pkg = pkg[:i]
		}
	}
	if pkg == "" {
		return "_root"
	}
	return pkg
}

// ApplyFile ingests one parsed file using the 2-Track diff: every node gets
// fresh offsets (Track A); only Changed nodes get new raw calls and a
// resolve at the next Flush (Track B).
func (e *Engine) ApplyFile(res *parse.FileResult, diff merkle.FileDiff) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	pkg := shardKey(res)
	recs := e.records[pkg]
	if recs == nil {
		recs = map[string]*nodeRecord{}
		e.records[pkg] = recs
	}
	changed := make(map[string]bool, len(diff.Changed))
	for _, n := range diff.Changed {
		changed[n.ID] = true
	}

	// created maps each newly appeared node's content hash to its ID, for the
	// rename detection below. A hash that two new nodes share maps to "",
	// marking it ambiguous: guessing which one a vanished node became would
	// silently point every reference at the wrong section.
	created := map[string]string{}

	touched := false
	for _, n := range res.Nodes {
		rec := recs[n.ID]
		if rec == nil {
			rec = &nodeRecord{ID: n.ID, Pkg: pkg}
			recs[n.ID] = rec
			touched = true
			if key := renameKeyOf(n.RenameKey[:]); key != "" {
				if _, dup := created[key]; dup {
					created[key] = ""
				} else {
					created[key] = n.ID
				}
			}
		}
		if rec.Start != n.StartByte || rec.End != n.EndByte || rec.FilePath != res.RelPath ||
			rec.Partial != res.Partial || changed[n.ID] {
			touched = true
		}
		rec.DisplayName = n.DisplayName
		rec.SimpleName = n.SimpleName
		rec.Kind = n.Kind
		rec.Container = n.Container
		rec.FilePath = res.RelPath
		rec.Start, rec.End = n.StartByte, n.EndByte
		rec.Hash = append(rec.Hash[:0], n.Hash[:]...)
		rec.RenameKey = append(rec.RenameKey[:0], n.RenameKey[:]...)
		rec.Partial = res.Partial
		rec.ArityMin, rec.ArityMax = n.ArityMin, n.ArityMax

		if changed[n.ID] {
			rec.Supers = n.Supers
			rec.LogicalRefs = n.LogicalRefs
			rec.RawCalls = n.Calls
			rec.DBRefs = n.DBRefs
			rec.Imports = res.Imports
			rec.needsResolve = true
		}

		// Contains is the one raw field that is not derived from this node's
		// own bytes, so it cannot ride on the hash. A markdown section hashes
		// its own span only — heading to first child heading — which means a
		// heading added further down the document changes the child list while
		// leaving the parent "unchanged" forever. Refresh it with Track A.
		//
		// Compare rather than assign-and-resolve: shards persist Uses but not
		// the raw fields, so after a restart every RawCalls is nil, and an
		// unconditional resolve would rebuild a code node's edges from nothing
		// and erase them. Code nodes never set Contains, so nil == nil keeps
		// them out of this branch entirely.
		if !slices.Equal(rec.Contains, n.Contains) {
			rec.Contains = n.Contains
			rec.needsResolve = true
			touched = true
		}

		classFQN := nodeid.Class(res.Package, firstSegmentChain(n))
		e.res.register(&DefInfo{
			ID: n.ID, Pkg: pkg, ClassFQN: classFQN, Simple: n.SimpleName,
			Kind: n.Kind, FilePath: res.RelPath, Aliases: n.Aliases,
			ArityMin: n.ArityMin, ArityMax: n.ArityMax,
		})
	}

	for _, id := range diff.Removed {
		if rec := recs[id]; rec != nil {
			// A node that disappeared while a new one with identical content
			// appeared in the same file is a rename, not a delete. Recording
			// it keeps references to the old ID resolving.
			//
			// Identical content is the whole test, which is why this catches
			// pure renames and nothing else. A heading rewritten at the same
			// time as its body hashes differently, and a section split into
			// two matches neither — both fall through to a dangling reference,
			// which is the honest answer since no automatic choice is right.
			key := renameKeyOf(rec.RenameKey)
			if to := created[key]; key != "" && to != "" && rec.FilePath == res.RelPath {
				e.rev.Redirect(id, to, e.now())
			}
			e.dropRecordLocked(pkg, rec)
			touched = true
		}
	}
	if touched {
		e.dirty[pkg] = true
	}
}

// renameKeyOf returns a map key for a rename key, or "" when there is none to
// compare. An all-zero key means the node kind carries no rename identity, and
// an empty body hashes to a constant that many sections share — matching on
// either would pair up nodes that have nothing in common.
func renameKeyOf(k []byte) string {
	if len(k) == 0 {
		return ""
	}
	for _, b := range k {
		if b != 0 {
			return string(k)
		}
	}
	return ""
}

// firstSegmentChain yields the node's enclosing class chain (its own chain
// for class kinds) used as ClassFQN for import-scope matching.
func firstSegmentChain(n parse.Node) string {
	if n.Kind == nodeid.KindClass || n.Kind == nodeid.KindInterface {
		if n.Container == "" {
			return n.SimpleName
		}
		return n.Container + "." + n.SimpleName
	}
	return n.Container
}

// dropRecordLocked removes one record: tombstones its outgoing edges,
// unregisters its definition and hides it from the read path immediately.
func (e *Engine) dropRecordLocked(pkg string, rec *nodeRecord) {
	for _, u := range rec.Uses {
		e.rev.Tombstone(u.TargetID, rec.ID, u.Type)
	}
	e.res.unregister(rec.ID)
	delete(e.records[pkg], rec.ID)
	e.mu.Lock()
	delete(e.nodeLoc, rec.ID)
	e.mu.Unlock()
}

// RemoveNodes drops nodes by ID (deleted files). §7-P3-①: used_by reflects
// the deletion immediately via tombstones, before any compaction.
func (e *Engine) RemoveNodes(ids []string) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	for _, id := range ids {
		e.mu.RLock()
		loc, ok := e.nodeLoc[id]
		e.mu.RUnlock()
		pkg := loc.pkg
		if !ok { // not yet in a shard: search pending records
			for p, recs := range e.records {
				if _, exists := recs[id]; exists {
					pkg = p
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}
		if rec := e.records[pkg][id]; rec != nil {
			e.dropRecordLocked(pkg, rec)
			e.dirty[pkg] = true
		}
	}
}

// Flush resolves pending edges and regenerates every dirty shard: build →
// atomic rename → new mmap → pointer swap → retire old mapping (§4).
func (e *Engine) Flush() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	for pkg := range e.dirty {
		recs := e.records[pkg]

		// Same-file definition table for self-call certainty.
		fileDefs := map[string]map[string][]*DefInfo{} // file → simple → defs
		for _, rec := range recs {
			m := fileDefs[rec.FilePath]
			if m == nil {
				m = map[string][]*DefInfo{}
				fileDefs[rec.FilePath] = m
			}
			m[rec.SimpleName] = append(m[rec.SimpleName], &DefInfo{
				ID: rec.ID, Pkg: rec.Pkg, Simple: rec.SimpleName, Kind: rec.Kind,
				FilePath: rec.FilePath,
				ArityMin: rec.ArityMin, ArityMax: rec.ArityMax,
			})
		}

		for _, rec := range recs {
			if !rec.needsResolve {
				continue
			}
			newUses := e.resolveEdges(rec, fileDefs[rec.FilePath])
			e.diffReverse(rec.ID, rec.Uses, newUses)
			rec.Uses = newUses
			rec.needsResolve = false
		}

		if err := e.rebuildShard(pkg, recs); err != nil {
			e.log.Event("shard_write_error", map[string]any{"pkg": pkg, "error": err.Error()})
		}
	}
	e.dirty = map[string]bool{}
	e.rev.Sync()
}

// diffReverse emits delta records for the edge set transition of one source.
func (e *Engine) diffReverse(source string, old, new []Edge) {
	type key struct {
		target string
		t      EdgeType
	}
	prev := make(map[key]float32, len(old))
	for _, u := range old {
		prev[key{u.TargetID, u.Type}] = u.Confidence
	}
	for _, u := range new {
		k := key{u.TargetID, u.Type}
		if conf, ok := prev[k]; !ok || conf != u.Confidence {
			e.rev.Upsert(u.TargetID, source, u.Type, u.Confidence)
		}
		delete(prev, k)
	}
	for k := range prev {
		e.rev.Tombstone(k.target, source, k.t)
	}
}

func (e *Engine) rebuildShard(pkg string, recs map[string]*nodeRecord) error {
	e.gens[pkg]++
	data := buildShard(pkg, e.gens[pkg], recs)
	path := filepath.Join(e.dir, shardFileName(pkg))

	if len(recs) == 0 { // package vanished entirely
		_ = os.Remove(path)
		e.mu.Lock()
		old := e.shards[pkg]
		delete(e.shards, pkg)
		e.mu.Unlock()
		if old != nil {
			old.ref.Retire()
		}
		return nil
	}

	if err := store.WriteFileAtomic(path, data, 0o644); err != nil {
		return err
	}
	ref, err := store.OpenMMap(path)
	if err != nil {
		return err
	}
	sh := fbsgen.GetRootAsShard(ref.Data, 0)
	h := &shardHandle{ref: ref, gen: sh.Generation(), idx: map[string]int{}}
	var node fbsgen.Node
	for i := 0; i < sh.NodesLength(); i++ {
		if sh.Nodes(&node, i) {
			id := string(node.Id())
			h.ids = append(h.ids, id)
			h.idx[id] = i
		}
	}

	e.mu.Lock()
	old := e.shards[pkg]
	if old != nil {
		for _, id := range old.ids { // drop stale locations first
			if loc, ok := e.nodeLoc[id]; ok && loc.pkg == pkg {
				delete(e.nodeLoc, id)
			}
		}
	}
	e.shards[pkg] = h
	for id, i := range h.idx {
		e.nodeLoc[id] = nodeLoc{pkg: pkg, idx: i}
	}
	e.mu.Unlock()
	if old != nil {
		old.ref.Retire() // munmap deferred until the last reader releases
	}
	return nil
}

// shardFileName sanitizes the package into chunk_<pkg>.fb (§2.5).
func shardFileName(pkg string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			return r
		}
		return '_'
	}, pkg)
	if len(safe) > 200 {
		sum := merkle.Sum([]byte(pkg))
		safe = fmt.Sprintf("%s_%x", safe[:180], sum[:8])
	}
	return "chunk_" + safe + ".fb"
}

// buildShard serializes records (sorted by ID) into FlatBuffers bytes.
func buildShard(pkg string, gen uint64, recs map[string]*nodeRecord) []byte {
	ids := make([]string, 0, len(recs))
	for id := range recs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	b := flatbuffers.NewBuilder(4096)
	nodeOffs := make([]flatbuffers.UOffsetT, 0, len(ids))
	for _, id := range ids {
		rec := recs[id]
		edgeOffs := make([]flatbuffers.UOffsetT, 0, len(rec.Uses))
		for _, u := range rec.Uses {
			tid := b.CreateString(u.TargetID)
			fbsgen.EdgeStart(b)
			fbsgen.EdgeAddTargetId(b, tid)
			fbsgen.EdgeAddType(b, u.Type)
			fbsgen.EdgeAddConfidence(b, u.Confidence)
			edgeOffs = append(edgeOffs, fbsgen.EdgeEnd(b))
		}
		fbsgen.NodeStartUsesVector(b, len(edgeOffs))
		for i := len(edgeOffs) - 1; i >= 0; i-- {
			b.PrependUOffsetT(edgeOffs[i])
		}
		usesVec := b.EndVector(len(edgeOffs))

		idOff := b.CreateString(rec.ID)
		dispOff := b.CreateString(rec.DisplayName)
		kindOff := b.CreateString(rec.Kind)
		fileOff := b.CreateString(rec.FilePath)
		hashOff := b.CreateByteVector(rec.Hash)
		renameOff := b.CreateByteVector(rec.RenameKey)

		fbsgen.NodeStart(b)
		fbsgen.NodeAddId(b, idOff)
		fbsgen.NodeAddDisplayName(b, dispOff)
		fbsgen.NodeAddKind(b, kindOff)
		fbsgen.NodeAddFilePath(b, fileOff)
		fbsgen.NodeAddStartByte(b, rec.Start)
		fbsgen.NodeAddEndByte(b, rec.End)
		fbsgen.NodeAddSubtreeHash(b, hashOff)
		fbsgen.NodeAddRenameKey(b, renameOff)
		fbsgen.NodeAddPartial(b, rec.Partial)
		fbsgen.NodeAddUses(b, usesVec)
		nodeOffs = append(nodeOffs, fbsgen.NodeEnd(b))
	}

	fbsgen.ShardStartNodesVector(b, len(nodeOffs))
	for i := len(nodeOffs) - 1; i >= 0; i-- {
		b.PrependUOffsetT(nodeOffs[i])
	}
	nodesVec := b.EndVector(len(nodeOffs))
	pkgOff := b.CreateString(pkg)

	fbsgen.ShardStart(b)
	fbsgen.ShardAddPackagePrefix(b, pkgOff)
	fbsgen.ShardAddGeneration(b, gen)
	fbsgen.ShardAddNodes(b, nodesVec)
	b.Finish(fbsgen.ShardEnd(b))
	return b.FinishedBytes()
}

// UsedBySources lists the current used_by source IDs of one node,
// unpaginated — Phase 7b invalidation internal, not an MCP surface.
func (e *Engine) UsedBySources(id string) []string {
	edges := e.rev.Query(id, 0)
	out := make([]string, 0, len(edges))
	for _, r := range edges {
		out = append(out, r.SourceID)
	}
	return out
}

// ResolveID follows any redirect recorded for id and returns the node that is
// current now, flattening the chain as a side effect.
func (e *Engine) ResolveID(id string) string { return e.rev.ResolveID(id) }

// HasNode reports whether id is currently visible on the read path.
func (e *Engine) HasNode(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.nodeLoc[id]
	return ok
}

// Close retires all mappings and closes the reverse log.
func (e *Engine) Close() {
	e.mu.Lock()
	for _, h := range e.shards {
		h.ref.Retire()
	}
	e.shards = map[string]*shardHandle{}
	e.mu.Unlock()
	e.rev.Close()
}
