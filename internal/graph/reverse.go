package graph

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/store"
)

// Compaction triggers (§2.1.3).
const (
	compactLogBytes      = 10 << 20 // 10MB
	compactTombstoneFrac = 0.30
)

const (
	baseName       = "reverse_base.bin"
	deltaName      = "reverse_delta.log"
	compactingName = "reverse_delta.compacting"
)

// Reverse is the global used_by index: a heap map replayed from the compacted
// base plus the tombstone delta log (§2.1.3). The single-writer indexer calls
// the mutating methods; queries take the read lock.
type Reverse struct {
	dir string
	log *obs.Logger

	mu         sync.RWMutex
	m          map[string][]RevEdge // targetID → incoming edges
	delta      *deltaLog
	records    int // records in the current delta log
	tombstones int

	compactMu sync.Mutex     // one compaction at a time
	compactWG sync.WaitGroup // tracks async compactions for Close
}

// OpenReverse loads base + (crash-leftover compacting log) + delta log.
func OpenReverse(dir string, lg *obs.Logger) (*Reverse, error) {
	r := &Reverse{dir: dir, log: lg, m: map[string][]RevEdge{}}

	if data, err := os.ReadFile(filepath.Join(dir, baseName)); err == nil {
		recs, _ := replay(data)
		for _, rec := range recs {
			r.apply(rec)
		}
	}
	// A leftover .compacting log means we crashed mid-compaction after
	// rotation: its records are not yet in the base, replay them next.
	if data, err := os.ReadFile(filepath.Join(dir, compactingName)); err == nil {
		recs, _ := replay(data)
		for _, rec := range recs {
			r.apply(rec)
		}
	}

	delta, recs, err := openDeltaLog(filepath.Join(dir, deltaName))
	if err != nil {
		return nil, err
	}
	r.delta = delta
	for _, rec := range recs {
		r.apply(rec)
		r.records++
		if rec.Op == opTombstone {
			r.tombstones++
		}
	}
	return r, nil
}

// apply mutates the in-memory map only (no logging). Caller holds mu or is
// still single-threaded during load.
func (r *Reverse) apply(rec record) {
	edges := r.m[rec.TargetID]
	idx := -1
	for i, e := range edges {
		if e.SourceID == rec.SourceID && e.Type == rec.Type {
			idx = i
			break
		}
	}
	switch rec.Op {
	case opUpsert:
		e := RevEdge{SourceID: rec.SourceID, Type: rec.Type, Confidence: rec.Confidence}
		if idx >= 0 {
			edges[idx] = e
		} else {
			edges = append(edges, e)
		}
		r.m[rec.TargetID] = edges
	case opTombstone:
		if idx >= 0 {
			edges = append(edges[:idx], edges[idx+1:]...)
			if len(edges) == 0 {
				delete(r.m, rec.TargetID)
			} else {
				r.m[rec.TargetID] = edges
			}
		}
	}
}

// Upsert records source→target and logs it.
func (r *Reverse) Upsert(target, source string, t EdgeType, conf float32) {
	r.write(record{Op: opUpsert, TargetID: target, SourceID: source, Type: t, Confidence: conf})
}

// Tombstone removes source→target and logs a DELETE marker.
func (r *Reverse) Tombstone(target, source string, t EdgeType) {
	r.write(record{Op: opTombstone, TargetID: target, SourceID: source, Type: t})
}

func (r *Reverse) write(rec record) {
	r.mu.Lock()
	r.apply(rec)
	if err := r.delta.append(encodeRecord(nil, rec)); err != nil {
		r.log.Event("delta_append_error", map[string]any{"error": err.Error()})
	}
	r.records++
	if rec.Op == opTombstone {
		r.tombstones++
	}
	r.mu.Unlock()
}

// Query returns a sorted copy of target's incoming edges at or above minConf.
func (r *Reverse) Query(target string, minConf float32) []RevEdge {
	r.mu.RLock()
	edges := r.m[target]
	out := make([]RevEdge, 0, len(edges))
	for _, e := range edges {
		if e.Confidence >= minConf {
			out = append(out, e)
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return out[i].Confidence > out[j].Confidence
		}
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

// Sync flushes the delta log and kicks compaction if a trigger fired.
func (r *Reverse) Sync() {
	r.mu.Lock()
	_ = r.delta.sync()
	needCompact := r.delta.size > compactLogBytes ||
		(r.records > 0 && float64(r.tombstones)/float64(r.records) > compactTombstoneFrac)
	r.mu.Unlock()
	if needCompact {
		r.compactWG.Add(1)
		go func() {
			defer r.compactWG.Done()
			r.Compact()
		}()
	}
}

// Compact rotates the delta log, dumps a snapshot to a fresh base and swaps
// it in atomically (§2.1.3). Readers keep querying throughout: only the
// rotation itself takes the write lock.
func (r *Reverse) Compact() {
	r.compactMu.Lock()
	defer r.compactMu.Unlock()

	deltaPath := filepath.Join(r.dir, deltaName)
	compactingPath := filepath.Join(r.dir, compactingName)

	// 1) Rotate under the write lock: current log → .compacting, fresh log,
	//    deep snapshot of the map. New writes land in the fresh log.
	r.mu.Lock()
	if err := r.delta.close(); err != nil {
		r.log.Event("compact_error", map[string]any{"step": "close", "error": err.Error()})
	}
	if err := os.Rename(deltaPath, compactingPath); err != nil && !os.IsNotExist(err) {
		r.log.Event("compact_error", map[string]any{"step": "rotate", "error": err.Error()})
		r.mu.Unlock()
		return
	}
	fresh, _, err := openDeltaLog(deltaPath)
	if err != nil {
		r.log.Event("compact_error", map[string]any{"step": "reopen", "error": err.Error()})
		r.mu.Unlock()
		return
	}
	r.delta = fresh
	r.records, r.tombstones = 0, 0
	snapshot := make(map[string][]RevEdge, len(r.m))
	for k, v := range r.m {
		cp := make([]RevEdge, len(v))
		copy(cp, v)
		snapshot[k] = cp
	}
	r.mu.Unlock()

	// 2) Serialize the snapshot (upserts only) into the new base.
	targets := make([]string, 0, len(snapshot))
	for t := range snapshot {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	var buf []byte
	edgeCount := 0
	for _, t := range targets {
		for _, e := range snapshot[t] {
			buf = encodeRecord(buf, record{
				Op: opUpsert, TargetID: t, SourceID: e.SourceID,
				Type: e.Type, Confidence: e.Confidence,
			})
			edgeCount++
		}
	}
	if err := store.WriteFileAtomic(filepath.Join(r.dir, baseName), buf, 0o644); err != nil {
		r.log.Event("compact_error", map[string]any{"step": "base", "error": err.Error()})
		return // .compacting stays; replayed on next start
	}

	// 3) The snapshot is durable: the rotated log is no longer needed.
	_ = os.Remove(compactingPath)
	r.log.Event("compaction", map[string]any{"edges": edgeCount, "bytes": len(buf)})
}

// Close drains any in-flight compaction, then flushes and closes the log.
func (r *Reverse) Close() {
	r.compactWG.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.delta.sync()
	_ = r.delta.close()
}
