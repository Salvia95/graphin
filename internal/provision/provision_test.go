package provision

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Salvia95/graphin/internal/obs"
)

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// TestChecksumMismatchRefusesLibrary proves §7-P4-③: corrupted bytes are
// rejected and nothing gets installed.
func TestChecksumMismatchRefusesLibrary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("EVIL BYTES"))
	}))
	defer srv.Close()

	runtimeDir := t.TempDir()
	art := Artifact{
		Name:   "model.onnx",
		URL:    srv.URL + "/model.onnx",
		SHA256: sha([]byte("the real model")),
	}
	dest := filepath.Join(runtimeDir, art.Name)
	err := download(art, dest, Options{RuntimeDir: runtimeDir, Log: obs.Nop()})
	if !errors.Is(err, ErrChecksum) {
		t.Fatalf("expected ErrChecksum, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("corrupt artifact must not be installed")
	}
	entries, _ := os.ReadDir(runtimeDir)
	if len(entries) != 0 {
		t.Fatalf("leftover files after refused download: %v", entries)
	}
}

// TestOfflineModeUsesModelDir: --offline + --model-dir resolves without any
// network access (§2.2 폐쇄망).
func TestOfflineModeUsesModelDir(t *testing.T) {
	good := []byte("verified model bytes")
	modelDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modelDir, "model.onnx"), good, 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeDir := t.TempDir()
	art := Artifact{Name: "model.onnx", URL: "https://unreachable.invalid/x", SHA256: sha(good)}

	got, err := resolveFile(art, Options{
		RuntimeDir: runtimeDir, ModelDir: modelDir, Offline: true, Log: obs.Nop(),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(got)
	if err != nil || string(b) != string(good) {
		t.Fatalf("resolved artifact wrong: %v", err)
	}
}

func TestOfflineMissingIsErrOffline(t *testing.T) {
	art := Artifact{Name: "model.onnx", URL: "https://unreachable.invalid/x", SHA256: sha([]byte("x"))}
	_, err := resolveFile(art, Options{RuntimeDir: t.TempDir(), Offline: true, Log: obs.Nop()})
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("expected ErrOffline, got %v", err)
	}
}

// Corrupt files sitting in --model-dir are also refused (verification runs
// on every source, not just downloads).
func TestModelDirCorruptRefused(t *testing.T) {
	modelDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modelDir, "model.onnx"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	art := Artifact{Name: "model.onnx", URL: "https://unreachable.invalid/x", SHA256: sha([]byte("legit"))}
	_, err := resolveFile(art, Options{
		RuntimeDir: t.TempDir(), ModelDir: modelDir, Offline: true, Log: obs.Nop(),
	})
	if !errors.Is(err, ErrOffline) { // tampered source is skipped, then offline blocks
		t.Fatalf("expected ErrOffline after refusing tampered file, got %v", err)
	}
}
