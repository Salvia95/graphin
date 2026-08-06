package provision

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// TestORTPinsWellFormed guards the pin table itself: a typo'd SHA or a URL
// left behind at the previous ORT version fails here rather than as a
// mid-warmup download error on someone else's machine.
func TestORTPinsWellFormed(t *testing.T) {
	if len(ortByPlatform) == 0 {
		t.Fatal("no ORT platforms pinned")
	}
	for key, p := range ortByPlatform {
		goos, goarch, ok := strings.Cut(key, "/")
		if !ok || goos == "" || goarch == "" {
			t.Errorf("%s: key must be <goos>/<goarch>", key)
		}
		if len(p.Archive.SHA256) != 64 {
			t.Errorf("%s: SHA256 is %d chars, want 64", key, len(p.Archive.SHA256))
		}
		if _, err := hex.DecodeString(p.Archive.SHA256); err != nil {
			t.Errorf("%s: SHA256 is not hex: %v", key, err)
		}
		if !strings.Contains(p.Archive.Name, ORTVersion) {
			t.Errorf("%s: asset %q does not carry ORTVersion %s", key, p.Archive.Name, ORTVersion)
		}
		if !strings.HasSuffix(p.Archive.URL, "/"+p.Archive.Name) {
			t.Errorf("%s: URL %q does not end in the asset name %q", key, p.Archive.URL, p.Archive.Name)
		}
		if !strings.Contains(p.Archive.URL, "/v"+ORTVersion+"/") {
			t.Errorf("%s: URL %q points at a different release than ORTVersion %s", key, p.Archive.URL, ORTVersion)
		}
		if !strings.Contains(p.LibName, ORTVersion) {
			t.Errorf("%s: lib %q does not carry ORTVersion %s", key, p.LibName, ORTVersion)
		}
		gotArt, gotLib, err := ORTFor(goos, goarch)
		if err != nil {
			t.Errorf("%s: ORTFor: %v", key, err)
			continue
		}
		if gotArt != p.Archive || gotLib != p.LibName {
			t.Errorf("%s: ORTFor returned a different pin than the table", key)
		}
		if !SemanticSupported(goos, goarch) {
			t.Errorf("%s: pinned but SemanticSupported says no", key)
		}
	}
	// Every platform we promise must actually be pinned (D2 + v1.1 darwin).
	for _, key := range []string{"linux/amd64", "linux/arm64", "darwin/arm64"} {
		if _, ok := ortByPlatform[key]; !ok {
			t.Errorf("%s missing from the pin table", key)
		}
	}
}

// TestUnsupportedPlatformIsExplicit: an unpinned platform must say so, not
// fabricate an asset URL and surface a 404 (§4). darwin/amd64 is the live
// case — no 1.26.0 build exists for it.
func TestUnsupportedPlatformIsExplicit(t *testing.T) {
	if _, _, err := ORTFor("darwin", "amd64"); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("ORTFor(darwin/amd64) = %v, want ErrUnsupportedPlatform", err)
	}
	if SemanticSupported("darwin", "amd64") {
		t.Fatal("darwin/amd64 reported as semantic-capable")
	}
	_, err := resolveORT(Options{
		RuntimeDir: t.TempDir(), Platform: "darwin/amd64", Log: obs.Nop(),
	})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("resolveORT on unpinned platform = %v, want ErrUnsupportedPlatform", err)
	}
	// The message has to name the platform — it is what the operator reads
	// out of semantic_unavailable.
	if !strings.Contains(err.Error(), "darwin/amd64") {
		t.Errorf("error %q does not name the platform", err)
	}
}

// TestOrtLibOverridesUnsupportedPlatform: --ort-lib is the documented escape
// hatch, so it must win before the platform lookup can reject the caller.
func TestOrtLibOverridesUnsupportedPlatform(t *testing.T) {
	lib := filepath.Join(t.TempDir(), "libonnxruntime.dylib")
	if err := os.WriteFile(lib, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := resolveORT(Options{
		RuntimeDir: t.TempDir(), Platform: "darwin/amd64", OrtLib: lib, Log: obs.Nop(),
	})
	if err != nil {
		t.Fatalf("--ort-lib rejected on unpinned platform: %v", err)
	}
	if got != lib {
		t.Fatalf("resolveORT = %q, want %q", got, lib)
	}
}

// writeTarGz builds a gzipped tar from name→content pairs, in order.
func writeTarGz(t *testing.T, dest string, entries [][2]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name: e[0], Mode: 0o755, Size: int64(len(e[1])), Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestExtractORTLibIgnoresDSYMDecoy: the macOS archive ships a second regular
// file with the identical base name under
// libonnxruntime.<ver>.dylib.dSYM/Contents/Resources/DWARF/. A base-name match
// would take whichever tar order puts first — here the decoy — and install
// 52MB of debug symbols as the shared library. That failure surfaces only at
// dlopen during semantic warmup, so it has to be impossible by construction.
func TestExtractORTLibIgnoresDSYMDecoy(t *testing.T) {
	const lib = "libonnxruntime.1.26.0.dylib"
	dir := t.TempDir()
	tgz := filepath.Join(dir, "ort.tgz")
	root := "./onnxruntime-osx-arm64-1.26.0/"

	writeTarGz(t, tgz, [][2]string{
		// Decoy deliberately first.
		{root + "lib/" + lib + ".dSYM/Contents/Resources/DWARF/" + lib, "DWARF DEBUG SYMBOLS"},
		{root + "lib/" + lib, "REAL DYLIB"},
		{root + "lib/libonnxruntime.dylib", "UNVERSIONED COPY"},
	})

	dest := filepath.Join(dir, lib)
	if err := extractORTLib(tgz, dest, lib); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "REAL DYLIB" {
		t.Fatalf("extracted %q, want the real dylib", got)
	}
}

// An archive that only has the decoy must fail loudly rather than install it.
func TestExtractORTLibRefusesDSYMOnly(t *testing.T) {
	const lib = "libonnxruntime.1.26.0.dylib"
	dir := t.TempDir()
	tgz := filepath.Join(dir, "ort.tgz")
	writeTarGz(t, tgz, [][2]string{
		{"./onnxruntime-osx-arm64-1.26.0/lib/" + lib + ".dSYM/Contents/Resources/DWARF/" + lib, "DWARF"},
	})
	dest := filepath.Join(dir, lib)
	if err := extractORTLib(tgz, dest, lib); err == nil {
		t.Fatal("expected an error when only the dSYM copy is present")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("nothing should have been written")
	}
}

// The Linux layout must keep working through the same path match.
func TestExtractORTLibLinuxLayout(t *testing.T) {
	const lib = "libonnxruntime.so.1.26.0"
	dir := t.TempDir()
	tgz := filepath.Join(dir, "ort.tgz")
	writeTarGz(t, tgz, [][2]string{
		{"onnxruntime-linux-x64-1.26.0/lib/" + lib, "REAL SO"}, // no ./ prefix
	})
	dest := filepath.Join(dir, lib)
	if err := extractORTLib(tgz, dest, lib); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "REAL SO" {
		t.Fatalf("extracted %q", got)
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
