// Package provision downloads and verifies the ONNX Runtime shared library
// and embedding models (§2.2). Every artifact is pinned by URL + SHA256; a
// checksum mismatch refuses the artifact outright (§7-P4-③). --offline
// restricts resolution to local paths.
package provision

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

// ORT is the pinned linux-x64 runtime archive.
var ORT = Artifact{
	Name:   "onnxruntime-linux-x64-" + ORTVersion + ".tgz",
	URL:    "https://github.com/microsoft/onnxruntime/releases/download/v" + ORTVersion + "/onnxruntime-linux-x64-" + ORTVersion + ".tgz",
	SHA256: ortSHA256,
}

// ORTLibName is the versioned shared object inside the archive.
const ORTLibName = "libonnxruntime.so." + ORTVersion

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
