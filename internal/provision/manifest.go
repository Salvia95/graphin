// Package provision downloads and verifies the ONNX Runtime shared library
// and embedding models (§2.2). Every artifact is pinned by URL + SHA256; a
// checksum mismatch refuses the artifact outright (§7-P4-③). --offline
// restricts resolution to local paths.
package provision

import (
	"errors"
	"fmt"
)

// Artifact is one pinned downloadable.
type Artifact struct {
	Name   string
	URL    string
	SHA256 string
}

// ModelSpec pins one embedding model family (§6.1 리스크: ORT-모델 호환
// 튜플은 함께 갱신).
type ModelSpec struct {
	ID             string
	Model          Artifact
	Tokenizer      Artifact
	Dim            int
	QueryPrefix    string // e5 규약 (§2.1.1)
	PassagePrefix  string
	NeedsTokenType bool // BERT-family inputs include token_type_ids
}

// ORTVersion must match the C API the Go binding is built against
// (yalue/onnxruntime_go v1.31.0 → ORT 1.26.0; 1.27.x 금지).
const ORTVersion = "1.26.0"

// ErrUnsupportedPlatform marks a GOOS/GOARCH with no pinned ORT build. It is
// returned instead of letting a fabricated asset URL fail as a 404 download:
// warmupSemantic surfaces the error verbatim, so the reason a machine runs
// lexical-only has to be readable (docs/plugin-distribution.md §4).
var ErrUnsupportedPlatform = errors.New("no onnxruntime build is published for this platform")

// ortPlatform pins one platform's release archive together with the shared
// object to extract from it — the two travel together because the library
// filename is platform-specific (.so.<ver> on Linux, .<ver>.dylib on macOS).
type ortPlatform struct {
	Archive Artifact
	LibName string
}

// Shared-library names inside the archives, read out of the real tarballs —
// both verified rather than assumed (linux 2026-08-05, darwin 2026-08-06).
const (
	ortSOName    = "libonnxruntime.so." + ORTVersion // both linux targets
	ortDylibName = "libonnxruntime." + ORTVersion + ".dylib"
)

// ortByPlatform is the pinned ORT release asset per GOOS/GOARCH. A platform
// absent from this map has no semantic search — deliberately, not by
// oversight: darwin/amd64 has no 1.26.0 asset at all (confirmed against the
// release's asset list, 2026-08-06), and Windows ships a .zip that
// extractORTLib cannot read.
var ortByPlatform = map[string]ortPlatform{
	"linux/amd64":  {Archive: ortArchive("linux-x64", ortLinuxAMD64SHA256), LibName: ortSOName},
	"linux/arm64":  {Archive: ortArchive("linux-aarch64", ortLinuxARM64SHA256), LibName: ortSOName},
	"darwin/arm64": {Archive: ortArchive("osx-arm64", ortDarwinARM64SHA256), LibName: ortDylibName},
}

// ortArchive builds the GitHub release asset for one platform slug.
func ortArchive(slug, sha string) Artifact {
	name := "onnxruntime-" + slug + "-" + ORTVersion + ".tgz"
	return Artifact{
		Name:   name,
		URL:    "https://github.com/microsoft/onnxruntime/releases/download/v" + ORTVersion + "/" + name,
		SHA256: sha,
	}
}

// PlatformKey is the ortByPlatform key for a GOOS/GOARCH pair.
func PlatformKey(goos, goarch string) string { return goos + "/" + goarch }

// ORTFor returns the pinned archive and shared-object name for a platform,
// or ErrUnsupportedPlatform.
func ORTFor(goos, goarch string) (Artifact, string, error) {
	key := PlatformKey(goos, goarch)
	p, ok := ortByPlatform[key]
	if !ok {
		return Artifact{}, "", fmt.Errorf("%w: %s", ErrUnsupportedPlatform, key)
	}
	return p.Archive, p.LibName, nil
}

// SemanticSupported reports whether a platform can run semantic search at
// all. `graphin version --json` publishes it so an operator can tell a
// missing model from an impossible one without reading logs.
func SemanticSupported(goos, goarch string) bool {
	_, ok := ortByPlatform[PlatformKey(goos, goarch)]
	return ok
}

// Models maps the bootstrap_workspace model_type enum to pinned artifacts.
var Models = map[string]ModelSpec{
	"english_optimal": {
		ID: "e5-small-v2",
		Model: Artifact{
			Name:   "e5-small-v2-quantized.onnx",
			URL:    "https://huggingface.co/Xenova/e5-small-v2/resolve/main/onnx/model_quantized.onnx",
			SHA256: e5ModelSHA256,
		},
		Tokenizer: Artifact{
			Name:   "e5-small-v2-tokenizer.json",
			URL:    "https://huggingface.co/intfloat/e5-small-v2/resolve/main/tokenizer.json",
			SHA256: "d241a60d5e8f04cc1b2b3e9ef7a4921b27bf526d9f6050ab90f9267a1f9e5c66",
		},
		Dim:            384,
		QueryPrefix:    "query: ",
		PassagePrefix:  "passage: ",
		NeedsTokenType: true,
	},
	"multilingual_cjk": {
		ID: "multilingual-e5-small",
		Model: Artifact{
			Name:   "multilingual-e5-small-quantized.onnx",
			URL:    "https://huggingface.co/Xenova/multilingual-e5-small/resolve/main/onnx/model_quantized.onnx",
			SHA256: me5ModelSHA256,
		},
		Tokenizer: Artifact{
			Name:   "multilingual-e5-small-tokenizer.json",
			URL:    "https://huggingface.co/intfloat/multilingual-e5-small/resolve/main/tokenizer.json",
			SHA256: "0b44a9d7b51c3c62626640cda0e2c2f70fdacdc25bbbd68038369d14ebdf4c39",
		},
		Dim:            384,
		QueryPrefix:    "query: ",
		PassagePrefix:  "passage: ",
		NeedsTokenType: false, // XLM-R
	},
}
