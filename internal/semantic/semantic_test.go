package semantic

import (
	"hash/fnv"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llls2542/graphin/internal/obs"
)

// bowVec is a deterministic bag-of-words vectorizer: similar texts embed
// close together, which is all the engine tests need.
type bowVec struct {
	calls atomic.Int64
	mu    sync.Mutex
	seen  []string
}

func (v *bowVec) Embed(text string) ([]float32, error) {
	v.calls.Add(1)
	v.mu.Lock()
	v.seen = append(v.seen, text)
	v.mu.Unlock()

	vec := make([]float32, 32)
	for _, w := range strings.Fields(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		vec[h.Sum32()%32]++
	}
	var norm float64
	for _, x := range vec {
		norm += float64(x * x)
	}
	if norm > 0 {
		inv := float32(1 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= inv
		}
	}
	return vec, nil
}

func (v *bowVec) texts() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.seen...)
}

func waitDrain(t *testing.T, e *Engine, vec *bowVec, want int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e.QueueDepth() == 0 && vec.calls.Load() >= want {
			time.Sleep(50 * time.Millisecond) // let the in-flight op land
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue never drained (depth=%d calls=%d want>=%d)", e.QueueDepth(), vec.calls.Load(), want)
}

func TestExportImportRoundtripWithHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.bin")
	snapshot := func() (string, map[string]string) {
		return "root-1", map[string]string{"a.java": "h1", "b.java": "h2"}
	}
	vec := &bowVec{}
	e := New(path, snapshot, obs.Nop())
	e.WarmupWith(vec, "test-model", "query: ", "passage: ")
	e.Enqueue("node.hello", "hello world greetings")
	e.Enqueue("node.pay", "payment cancel refund")
	waitDrain(t, e, vec, 2)
	if err := e.Export(); err != nil {
		t.Fatal(err)
	}
	e.Abandon()

	vec2 := &bowVec{}
	e2 := New(path, nil, obs.Nop())
	defer e2.Abandon()
	hdr := e2.LoadedHeader()
	if hdr == nil || hdr.Root != "root-1" || hdr.Files["b.java"] != "h2" || hdr.ModelID != "test-model" {
		t.Fatalf("header lost in roundtrip: %+v", hdr)
	}
	e2.WarmupWith(vec2, "test-model", "query: ", "passage: ")
	ids := e2.Search("hello world", 1)
	if len(ids) != 1 || ids[0] != "node.hello" {
		t.Fatalf("vector search after import: %v", ids)
	}
}

func TestStaleFilesDiff(t *testing.T) {
	e := &Engine{header: &Header{Files: map[string]string{"a": "h1", "b": "h2"}}}
	stale := e.StaleFiles(map[string]string{"a": "h1", "b": "h3", "c": "h4"})
	if len(stale) != 2 || !stale["b"] || !stale["c"] {
		t.Fatalf("stale = %v, want {b, c}", stale)
	}

	fresh := &Engine{} // no vectors.bin at all → everything stale
	stale = fresh.StaleFiles(map[string]string{"a": "h1"})
	if len(stale) != 1 || !stale["a"] {
		t.Fatalf("no-header stale = %v", stale)
	}
}

func TestRemoveDeletesVector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.bin")
	vec := &bowVec{}
	e := New(path, nil, obs.Nop())
	defer e.Abandon()
	e.WarmupWith(vec, "m", "q: ", "p: ")
	e.Enqueue("gone.node", "temporary doc")
	waitDrain(t, e, vec, 1)
	e.Remove("gone.node")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(e.Search("temporary doc", 1)) != 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := e.Search("temporary doc", 1); len(got) != 0 {
		t.Fatalf("vector survived removal: %v", got)
	}
}
