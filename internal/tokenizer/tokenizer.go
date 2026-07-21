// Package tokenizer is an in-house implementation of the HuggingFace
// tokenizer.json subset needed by the pinned e5 models (사용자 승인 결정:
// 외부 토크나이저 의존성 대신 자체 구현). Two model families are covered:
//
//	WordPiece + BertNormalizer/BertPreTokenizer  (e5-small-v2)
//	Unigram  + Metaspace                         (multilingual-e5-small)
//
// Parity is proven against committed HF reference token IDs
// (testdata/tokenizer). The Precompiled charsmap normalizer is approximated
// with NFKC.
package tokenizer

import (
	"encoding/json"
	"fmt"
	"os"
)

// MaxSeqLen is the encoder input cap (both e5 models are 512).
const MaxSeqLen = 512

// Tokenizer encodes text into model input IDs (special tokens included) and
// the matching attention mask.
type Tokenizer interface {
	Encode(text string) (ids, attentionMask []int64)
}

// tokenizerJSON is the minimal file schema we interpret.
type tokenizerJSON struct {
	Normalizer json.RawMessage `json:"normalizer"`
	Model      struct {
		Type                    string          `json:"type"`
		UnkToken                string          `json:"unk_token"`
		UnkID                   *int            `json:"unk_id"`
		ContinuingSubwordPrefix string          `json:"continuing_subword_prefix"`
		FuseUnk                 *bool           `json:"fuse_unk"`
		Vocab                   json.RawMessage `json:"vocab"`
	} `json:"model"`
	AddedTokens []struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
	} `json:"added_tokens"`
}

// Load reads a tokenizer.json and returns the matching implementation.
func Load(path string) (Tokenizer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tj tokenizerJSON
	if err := json.Unmarshal(b, &tj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	switch tj.Model.Type {
	case "WordPiece":
		return newWordPiece(&tj)
	case "Unigram":
		return newUnigram(&tj)
	default:
		return nil, fmt.Errorf("unsupported tokenizer model: %q", tj.Model.Type)
	}
}

func (tj *tokenizerJSON) addedID(content string) (int64, bool) {
	for _, t := range tj.AddedTokens {
		if t.Content == content {
			return t.ID, true
		}
	}
	return 0, false
}
