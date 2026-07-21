package workspace

import (
	"context"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llls2542/graphin/internal/obs"
)

type recVec struct {
	calls atomic.Int64
	mu    sync.Mutex
	seen  []string
}

func (v *recVec) Embed(text string) ([]float32, error) {
	v.calls.Add(1)
	v.mu.Lock()
	v.seen = append(v.seen, text)
	v.mu.Unlock()
	vec := make([]float32, 16)
	h := fnv.New32a()
	_, _ = h.Write([]byte(text))
	vec[h.Sum32()%16] = 1
	return vec, nil
}

func (v *recVec) texts() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.seen...)
}

func waitPhase(t *testing.T, w *Workspace, p Phase) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && w.FSM.Phase() < p {
		time.Sleep(20 * time.Millisecond)
	}
	if w.FSM.Phase() < p {
		t.Fatalf("phase %d never reached", p)
	}
}

func waitEmbeds(t *testing.T, v *recVec, atLeast int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n := v.calls.Load()
		if n >= atLeast {
			time.Sleep(150 * time.Millisecond) // settle in-flight ops
			if v.calls.Load() == n || v.calls.Load() >= atLeast {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("embeds never reached %d (got %d)", atLeast, v.calls.Load())
}

// TestKill9RecoveryReembedsOnlyChangedFiles proves §7-P4-②: after a crash
// that loses vector exports but not merkle.json, restart re-embeds exactly
// the files whose hash moved past the vectors.bin header — nothing else.
func TestKill9RecoveryReembedsOnlyChangedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("src/Alpha.java", `package demo;

public class Alpha {
    public void alphaOnly() {
    }
}
`)
	writeFile("src/Beta.java", `package demo;

public class Beta {
    public void betaOnly() {
    }
}
`)

	// ---- session 1: index, embed, export, then change Beta and "crash". ----
	w1 := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	if _, err := w1.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitPhase(t, w1, PhaseLexicalReady)
	v1 := &recVec{}
	w1.sem.WarmupWith(v1, "test-model", "query: ", "passage: ")
	waitEmbeds(t, v1, 4) // 2 classes + 2 methods
	if err := w1.sem.Export(); err != nil {
		t.Fatal(err)
	}

	// Beta changes; the indexer persists merkle.json, but the process dies
	// before the 5s idle export fires → vectors.bin is now stale for Beta.
	writeFile("src/Beta.java", `package demo;

public class Beta {
    public void betaOnly() {
        System.out.println("changed body");
    }
}
`)
	indexOneFile(t, w1, "src/Beta.java")
	w1.persistIndexes()
	w1.sem.Abandon() // kill -9: no final export
	w1.Close()

	// ---- session 2: only Beta may be re-embedded. ----
	w2 := New(Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	defer w2.Close()
	if _, err := w2.Bootstrap(context.Background(), "", false); err != nil {
		t.Fatal(err)
	}
	waitPhase(t, w2, PhaseLexicalReady)
	v2 := &recVec{}
	w2.sem.WarmupWith(v2, "test-model", "query: ", "passage: ")
	waitEmbeds(t, v2, 2) // Beta class + method

	texts := v2.texts()
	for _, txt := range texts {
		if strings.Contains(txt, "Alpha") || strings.Contains(txt, "alpha") {
			t.Fatalf("unchanged file re-embedded after restart: %q (all: %v)", txt, texts)
		}
	}
	beta := 0
	for _, txt := range texts {
		if strings.Contains(txt, "Beta") || strings.Contains(txt, "beta") {
			beta++
		}
	}
	if beta == 0 {
		t.Fatalf("stale file was not re-embedded: %v", texts)
	}
}
