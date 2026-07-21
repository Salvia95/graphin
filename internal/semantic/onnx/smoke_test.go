package onnx_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/llls2542/graphin/internal/obs"
	"github.com/llls2542/graphin/internal/provision"
	"github.com/llls2542/graphin/internal/semantic"
)

// TestRealONNXPipeline is the full provision → tokenize → infer → search
// smoke test. It downloads/uses ~150MB of artifacts, so it only runs when
// GRAPHIN_ORT_SMOKE=1 (artifacts come from the shared cache when present).
func TestRealONNXPipeline(t *testing.T) {
	if os.Getenv("GRAPHIN_ORT_SMOKE") != "1" {
		t.Skip("set GRAPHIN_ORT_SMOKE=1 to run the real ONNX smoke test")
	}
	dir := t.TempDir()
	cache := ""
	if ucd, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(ucd, "graphin", "artifacts")
	}
	paths, err := provision.Resolve("english_optimal", provision.Options{
		RuntimeDir: filepath.Join(dir, "runtime"),
		CacheDir:   cache,
		Log:        obs.Nop(),
	})
	if err != nil {
		t.Fatal(err)
	}

	eng := semantic.New(filepath.Join(dir, "vectors.bin"), nil, obs.Nop())
	defer eng.Close()
	if err := eng.Warmup(paths.OrtLib, paths.Model, paths.Tokenizer,
		paths.Spec.ID, paths.Spec.Dim, paths.Spec.QueryPrefix, paths.Spec.PassagePrefix); err != nil {
		t.Fatal(err)
	}

	eng.Enqueue("pay.cancel", "method OrderService.cancelPayment in com.example.order: cancel payment refund order")
	eng.Enqueue("logger.info", "method Logger.info in com.example.common: info logging message sink")
	eng.Enqueue("money.sum", "function sum in com.example.util: sum money list total")

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && eng.QueueDepth() > 0 {
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond) // let the in-flight op land

	got := eng.Search("how do I cancel a payment and refund it", 1)
	if len(got) != 1 || got[0] != "pay.cancel" {
		t.Fatalf("semantic search picked %v, want pay.cancel", got)
	}
	got = eng.Search("where are log messages written", 1)
	if len(got) != 1 || got[0] != "logger.info" {
		t.Fatalf("semantic search picked %v, want logger.info", got)
	}
}

// TestRealONNXPipelineMultilingual exercises the XLM-R path: Unigram
// tokenizer, no token_type_ids input, Korean query (§1.1 시나리오).
func TestRealONNXPipelineMultilingual(t *testing.T) {
	if os.Getenv("GRAPHIN_ORT_SMOKE") != "1" {
		t.Skip("set GRAPHIN_ORT_SMOKE=1 to run the real ONNX smoke test")
	}
	dir := t.TempDir()
	cache := ""
	if ucd, err := os.UserCacheDir(); err == nil {
		cache = filepath.Join(ucd, "graphin", "artifacts")
	}
	paths, err := provision.Resolve("multilingual_cjk", provision.Options{
		RuntimeDir: filepath.Join(dir, "runtime"),
		CacheDir:   cache,
		Log:        obs.Nop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := semantic.New(filepath.Join(dir, "vectors.bin"), nil, obs.Nop())
	defer eng.Close()
	if err := eng.Warmup(paths.OrtLib, paths.Model, paths.Tokenizer,
		paths.Spec.ID, paths.Spec.Dim, paths.Spec.QueryPrefix, paths.Spec.PassagePrefix); err != nil {
		t.Fatal(err)
	}

	eng.Enqueue("pay.cancel", "method OrderService.cancelPayment: cancel payment refund order")
	eng.Enqueue("money.sum", "function sum: sum money list total amount")

	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) && eng.QueueDepth() > 0 {
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)

	got := eng.Search("결제 취소 로직은 어디에 있어?", 1)
	if len(got) != 1 || got[0] != "pay.cancel" {
		t.Fatalf("Korean query picked %v, want pay.cancel", got)
	}
}
