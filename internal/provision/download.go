package provision

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/store"
)

// ErrChecksum marks a refused artifact (§7-P4-③).
var ErrChecksum = errors.New("checksum mismatch")

// ErrOffline marks a download blocked by --offline.
var ErrOffline = errors.New("offline mode: artifact not present locally")

// Options steers resolution (§6.4 플래그).
type Options struct {
	RuntimeDir string // <workspace>/.graphin/runtime
	CacheDir   string // shared cache (default ~/.cache/graphin); "" disables
	Offline    bool
	ModelDir   string // local model source override (--model-dir)
	OrtLib     string // explicit shared library path (--ort-lib)
	BaseURL    string // test hook: rewrites artifact URL hosts when set
	Platform   string // test hook: "<goos>/<goarch>"; empty = this binary's
	Log        *obs.Logger
}

// platform resolves which ORT pin applies, honouring the test hook.
func (o Options) platform() string {
	if o.Platform != "" {
		return o.Platform
	}
	return PlatformKey(runtime.GOOS, runtime.GOARCH)
}

// Paths is the resolved set the semantic engine needs.
type Paths struct {
	OrtLib    string
	Model     string
	Tokenizer string
	Spec      ModelSpec
}

// Resolve materializes the ORT library and the model artifacts for
// modelType, verifying every byte against the §pins manifest.
func Resolve(modelType string, opt Options) (*Paths, error) {
	spec, ok := Models[modelType]
	if !ok {
		return nil, fmt.Errorf("unknown model_type %q", modelType)
	}
	if err := os.MkdirAll(opt.RuntimeDir, 0o755); err != nil {
		return nil, err
	}

	ortLib, err := resolveORT(opt)
	if err != nil {
		return nil, err
	}
	model, err := resolveFile(spec.Model, opt)
	if err != nil {
		return nil, err
	}
	tok, err := resolveFile(spec.Tokenizer, opt)
	if err != nil {
		return nil, err
	}
	return &Paths{OrtLib: ortLib, Model: model, Tokenizer: tok, Spec: spec}, nil
}

// resolveFile returns a verified local path for one artifact: runtime dir →
// --model-dir → cache → network (unless offline). Every hit is re-verified.
func resolveFile(a Artifact, opt Options) (string, error) {
	dest := filepath.Join(opt.RuntimeDir, a.Name)
	if verifyFile(dest, a.SHA256) == nil {
		return dest, nil
	}
	if opt.ModelDir != "" {
		src := filepath.Join(opt.ModelDir, a.Name)
		if err := verifyFile(src, a.SHA256); err == nil {
			if err := copyVerified(src, dest, a.SHA256); err != nil {
				return "", err
			}
			return dest, nil
		}
	}
	if opt.CacheDir != "" {
		src := filepath.Join(opt.CacheDir, a.Name)
		if verifyFile(src, a.SHA256) == nil {
			if err := copyVerified(src, dest, a.SHA256); err != nil {
				return "", err
			}
			return dest, nil
		}
	}
	if opt.Offline {
		return "", fmt.Errorf("%w: %s", ErrOffline, a.Name)
	}
	if err := download(a, dest, opt); err != nil {
		return "", err
	}
	if opt.CacheDir != "" {
		_ = os.MkdirAll(opt.CacheDir, 0o755)
		_ = copyVerified(dest, filepath.Join(opt.CacheDir, a.Name), a.SHA256)
	}
	return dest, nil
}

// resolveORT yields a verified libonnxruntime path: --ort-lib wins (used
// as-is, §2.2 폐쇄망), otherwise the archive pinned for this platform is
// fetched and its shared object extracted.
//
// The --ort-lib check comes first on purpose: it is the only way a platform
// without a pin can still run semantic search, so the platform lookup must
// not reject the caller before that escape hatch is read.
func resolveORT(opt Options) (string, error) {
	if opt.OrtLib != "" {
		if _, err := os.Stat(opt.OrtLib); err != nil {
			return "", fmt.Errorf("--ort-lib: %w", err)
		}
		return opt.OrtLib, nil
	}
	goos, goarch, _ := strings.Cut(opt.platform(), "/")
	art, libName, err := ORTFor(goos, goarch)
	if err != nil {
		return "", err
	}
	libDest := filepath.Join(opt.RuntimeDir, libName)
	if _, err := os.Stat(libDest); err == nil {
		return libDest, nil
	}

	tgz := filepath.Join(opt.RuntimeDir, art.Name)
	if verifyFile(tgz, art.SHA256) != nil {
		found := false
		if opt.CacheDir != "" {
			src := filepath.Join(opt.CacheDir, art.Name)
			if verifyFile(src, art.SHA256) == nil {
				if err := copyVerified(src, tgz, art.SHA256); err != nil {
					return "", err
				}
				found = true
			}
		}
		if !found {
			if opt.Offline {
				return "", fmt.Errorf("%w: %s", ErrOffline, art.Name)
			}
			if err := download(art, tgz, opt); err != nil {
				return "", err
			}
			if opt.CacheDir != "" {
				_ = os.MkdirAll(opt.CacheDir, 0o755)
				_ = copyVerified(tgz, filepath.Join(opt.CacheDir, art.Name), art.SHA256)
			}
		}
	}
	if err := extractORTLib(tgz, libDest, libName); err != nil {
		return "", err
	}
	_ = os.Remove(tgz) // keep runtime/ lean; the cache retains the archive
	return libDest, nil
}

func download(a Artifact, dest string, opt Options) error {
	url := a.URL
	if opt.BaseURL != "" {
		if i := strings.Index(url, "://"); i >= 0 {
			if j := strings.IndexByte(url[i+3:], '/'); j >= 0 {
				url = opt.BaseURL + url[i+3+j:]
			}
		}
	}
	opt.Log.Event("provision_download", map[string]any{"artifact": a.Name, "url": url})

	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", a.Name, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dl-*")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != a.SHA256 {
		opt.Log.Event("provision_checksum_mismatch", map[string]any{
			"artifact": a.Name, "want": a.SHA256, "got": got,
		})
		return fmt.Errorf("%w: %s (got %s)", ErrChecksum, a.Name, got)
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

func verifyFile(path, wantSHA string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSHA {
		return fmt.Errorf("%w: %s", ErrChecksum, path)
	}
	return nil
}

func copyVerified(src, dest, wantSHA string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != wantSHA {
		return fmt.Errorf("%w: %s", ErrChecksum, src)
	}
	return store.WriteFileAtomic(dest, b, 0o644)
}

// extractORTLib pulls the versioned shared object named libName out of the
// release tarball.
//
// It matches on the "lib/<libName>" path, not on the base name alone. The
// macOS archive carries a second regular file with the identical base name
// under libonnxruntime.<ver>.dylib.dSYM/Contents/Resources/DWARF/ — 52MB of
// debug symbols. A base-name match picks whichever comes first in tar order,
// and installing the DWARF blob as the shared library would fail at dlopen
// during semantic warmup: the least diagnosable place it could break.
func extractORTLib(tgz, dest, libName string) error {
	f, err := os.Open(tgz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// tar entry names always use forward slashes and may be "./"-prefixed.
		if hdr.Typeflag != tar.TypeReg || !strings.HasSuffix(path.Clean(hdr.Name), "/lib/"+libName) {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		return store.WriteFileAtomic(dest, b, 0o755)
	}
	return fmt.Errorf("lib/%s not found in %s", libName, tgz)
}
