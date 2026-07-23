// Package semantic is the vector search engine (§2.1.1): chromem-go storage,
// e5 query/passage prefixes, progressive availability, and lazy persistence
// with merkle-root crash recovery (§2.2).
package semantic

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	chromem "github.com/philippgille/chromem-go"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/semantic/onnx"
	"github.com/Salvia95/graphin/internal/tokenizer"
)

const (
	collectionName = "nodes"
	queueCap       = 16384
	idlePersist    = 5 * time.Second // §2.2: 유휴 5s 후 비동기 Export
)

// Vectorizer turns text into an embedding. The ONNX pipeline implements it;
// tests substitute deterministic fakes. A Vectorizer holding native
// resources also implements closableVectorizer so Close/Abandon can free
// them — without that, every Engine leaks a full ORT session (model weights
// + inference arena), which repeated bootstraps turn into an OOM.
type Vectorizer interface {
	Embed(text string) ([]float32, error)
}

type closableVectorizer interface{ Close() }

// ortVectorizer is the production tokenizer+ONNX pipeline.
type ortVectorizer struct {
	tok  tokenizer.Tokenizer
	sess *onnx.Session
}

func (v *ortVectorizer) Embed(text string) ([]float32, error) {
	ids, mask := v.tok.Encode(text)
	return v.sess.Embed(ids, mask)
}

// Close releases the ONNX session's native memory.
func (v *ortVectorizer) Close() { v.sess.Close() }

type op struct {
	remove  bool
	id      string
	summary string
}

// Engine owns the vector collection and its background embedding queue.
type Engine struct {
	log         *obs.Logger
	persistPath string

	mu  sync.Mutex
	db  *chromem.DB
	col *chromem.Collection

	vec           Vectorizer
	queryPrefix   string
	passagePrefix string
	modelID       string

	ready    atomic.Bool
	readyCh  chan struct{}
	queue    chan op
	dirty    atomic.Bool
	lastTick atomic.Int64 // unix nanos of last mutation
	pending  atomic.Int64 // queued + in-flight ops (drain signal, Phase 7c 후속)
	droppedN atomic.Int64 // backpressure drops since start (coverage caveat)

	// snapshot provides the current merkle state for the export header.
	snapshot func() (root string, files map[string]string)

	header   *Header // header of the loaded vectors.bin, nil if none
	stop     chan struct{}
	stopOnce sync.Once
	stopped  sync.WaitGroup
}

// New loads any persisted vectors and starts the background workers.
// snapshot supplies the merkle root + file hashes recorded on export.
func New(persistPath string, snapshot func() (string, map[string]string), lg *obs.Logger) *Engine {
	e := &Engine{
		log:         lg,
		persistPath: persistPath,
		db:          chromem.NewDB(),
		readyCh:     make(chan struct{}),
		queue:       make(chan op, queueCap),
		snapshot:    snapshot,
		stop:        make(chan struct{}),
	}
	if hdr, err := e.load(); err != nil {
		lg.Event("vectors_load_error", map[string]any{"error": err.Error()})
	} else {
		e.header = hdr
	}
	if e.col == nil {
		e.col, _ = e.db.CreateCollection(collectionName, nil, noEmbed)
	}

	e.stopped.Add(2)
	go e.worker()
	go e.persister()
	return e
}

// noEmbed guards against accidental text-embedding through chromem: all
// embeddings are computed by our pipeline and passed pre-computed.
func noEmbed(_ context.Context, _ string) ([]float32, error) {
	return nil, errors.New("graphin computes embeddings itself")
}

// Warmup loads the tokenizer + ONNX session; call from a background
// goroutine (§2.1.1 단계적 가용성 — never blocks tool calls).
func (e *Engine) Warmup(ortLib, modelPath, tokenizerPath, modelID string, dim int, queryPrefix, passagePrefix string) error {
	if err := onnx.Init(ortLib); err != nil {
		return err
	}
	tok, err := tokenizer.Load(tokenizerPath)
	if err != nil {
		return err
	}
	sess, err := onnx.NewSession(modelPath, dim)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.vec = &ortVectorizer{tok: tok, sess: sess}
	e.queryPrefix = queryPrefix
	e.passagePrefix = passagePrefix
	e.modelID = modelID
	e.mu.Unlock()
	e.markReady()
	e.log.Event("semantic_ready", map[string]any{"model": modelID})
	return nil
}

// WarmupWith injects a custom vectorizer (tests).
func (e *Engine) WarmupWith(v Vectorizer, modelID, queryPrefix, passagePrefix string) {
	e.mu.Lock()
	e.vec = v
	e.modelID = modelID
	e.queryPrefix = queryPrefix
	e.passagePrefix = passagePrefix
	e.mu.Unlock()
	e.markReady()
}

func (e *Engine) markReady() {
	if e.ready.CompareAndSwap(false, true) {
		close(e.readyCh)
	}
}

// Ready implements search.Semantic.
func (e *Engine) Ready() bool { return e.ready.Load() }

// Enqueue schedules (re-)embedding of one node summary. Non-blocking: under
// backpressure the doc is dropped and logged (the next content change or
// stale-file pass re-enqueues it).
func (e *Engine) Enqueue(id, summary string) {
	select {
	case e.queue <- op{id: id, summary: summary}:
		e.pending.Add(1)
	default:
		e.droppedN.Add(1)
		e.log.Event("embed_queue_full", map[string]any{"dropped": id})
	}
}

// Remove schedules deletion of a node's vector.
func (e *Engine) Remove(id string) {
	select {
	case e.queue <- op{remove: true, id: id}:
		e.pending.Add(1)
	default:
		e.droppedN.Add(1)
		e.log.Event("embed_queue_full", map[string]any{"dropped": id})
	}
}

// Drained reports an idle embedding pipeline: no queued or in-flight ops.
// Meaningful once Ready() — before warmup the worker parks on the first
// non-remove op, so pending stays high by design.
func (e *Engine) Drained() bool { return e.pending.Load() == 0 }

// Dropped counts backpressure drops since start: a non-zero value means the
// vector index is missing that many (re-)embeds until the affected files
// next change — eval runs must disclose it as a coverage caveat.
func (e *Engine) Dropped() int64 { return e.droppedN.Load() }

// worker drains the queue once the model is warm.
func (e *Engine) worker() {
	defer e.stopped.Done()
	for {
		select {
		case <-e.stop:
			return
		case o := <-e.queue:
			if !o.remove {
				select {
				case <-e.readyCh:
				case <-e.stop:
					return
				}
			}
			e.apply(o)
			e.pending.Add(-1)
		}
	}
}

func (e *Engine) apply(o op) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if o.remove {
		_ = e.col.Delete(context.Background(), nil, nil, o.id)
	} else {
		if e.vec == nil { // released mid-shutdown
			return
		}
		vec, err := e.vec.Embed(e.passagePrefix + o.summary)
		if err != nil {
			e.log.Event("embed_error", map[string]any{"id": o.id, "error": err.Error()})
			return
		}
		err = e.col.AddDocument(context.Background(), chromem.Document{
			ID: o.id, Content: o.summary, Embedding: vec,
		})
		if err != nil {
			e.log.Event("embed_error", map[string]any{"id": o.id, "error": err.Error()})
			return
		}
	}
	e.dirty.Store(true)
	e.lastTick.Store(time.Now().UnixNano())
}

// Search implements search.Semantic: ranked node IDs, best first.
func (e *Engine) Search(query string, topK int) []string {
	if !e.Ready() {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.vec == nil || e.col.Count() == 0 { // vec nil: closed engine
		return nil
	}
	vec, err := e.vec.Embed(e.queryPrefix + query)
	if err != nil {
		e.log.Event("query_embed_error", map[string]any{"error": err.Error()})
		return nil
	}
	if topK > e.col.Count() {
		topK = e.col.Count()
	}
	res, err := e.col.QueryEmbedding(context.Background(), vec, topK, nil, nil)
	if err != nil {
		e.log.Event("vector_query_error", map[string]any{"error": err.Error()})
		return nil
	}
	ids := make([]string, len(res))
	for i, r := range res {
		ids[i] = r.ID
	}
	return ids
}

// QueueDepth reports pending embedding work (observability).
func (e *Engine) QueueDepth() int { return len(e.queue) }

// persister exports after 5s of idleness (§2.2 지연 영속화).
func (e *Engine) persister() {
	defer e.stopped.Done()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-t.C:
			if !e.dirty.Load() {
				continue
			}
			if time.Since(time.Unix(0, e.lastTick.Load())) < idlePersist {
				continue
			}
			if err := e.Export(); err != nil {
				e.log.Event("vectors_export_error", map[string]any{"error": err.Error()})
			}
		}
	}
}

// Close stops workers and drains state to disk.
func (e *Engine) Close() {
	e.stopOnce.Do(func() { close(e.stop) })
	e.stopped.Wait()
	if e.dirty.Load() {
		if err := e.Export(); err != nil {
			e.log.Event("vectors_export_error", map[string]any{"error": err.Error()})
		}
	}
	e.releaseVectorizer()
}

// releaseVectorizer frees the vectorizer's native resources and clears the
// field, so a post-Close Search degrades to "no results" instead of using a
// destroyed ORT session. Workers are already stopped by both callers.
func (e *Engine) releaseVectorizer() {
	e.mu.Lock()
	v := e.vec
	e.vec = nil
	e.mu.Unlock()
	if c, ok := v.(closableVectorizer); ok {
		c.Close()
	}
}

// Abandon stops workers WITHOUT exporting — kill -9 simulation for the
// §7-P4-② crash-recovery test.
func (e *Engine) Abandon() {
	e.dirty.Store(false)
	e.stopOnce.Do(func() { close(e.stop) })
	e.stopped.Wait()
	e.releaseVectorizer()
}
