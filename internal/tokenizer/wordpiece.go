package tokenizer

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxWordChars = 100 // WordPiece max_input_chars_per_word

type wordPiece struct {
	vocab      map[string]int64
	unk        int64
	cls, sep   int64
	contPrefix string

	lowercase    bool
	stripAccents bool
	cleanText    bool
	chineseChars bool
}

func newWordPiece(tj *tokenizerJSON) (*wordPiece, error) {
	wp := &wordPiece{contPrefix: tj.Model.ContinuingSubwordPrefix}
	if wp.contPrefix == "" {
		wp.contPrefix = "##"
	}
	if err := json.Unmarshal(tj.Model.Vocab, &wp.vocab); err != nil {
		return nil, fmt.Errorf("wordpiece vocab: %w", err)
	}

	var normCfg struct {
		Type         string `json:"type"`
		CleanText    bool   `json:"clean_text"`
		Chinese      bool   `json:"handle_chinese_chars"`
		StripAccents *bool  `json:"strip_accents"`
		Lowercase    bool   `json:"lowercase"`
	}
	if len(tj.Normalizer) > 0 {
		_ = json.Unmarshal(tj.Normalizer, &normCfg)
	}
	wp.cleanText = normCfg.CleanText
	wp.chineseChars = normCfg.Chinese
	wp.lowercase = normCfg.Lowercase
	if normCfg.StripAccents != nil {
		wp.stripAccents = *normCfg.StripAccents
	} else {
		wp.stripAccents = normCfg.Lowercase // BERT convention
	}

	lookup := func(tok string) (int64, error) {
		if id, ok := tj.addedID(tok); ok {
			return id, nil
		}
		if id, ok := wp.vocab[tok]; ok {
			return id, nil
		}
		return 0, fmt.Errorf("special token %q missing", tok)
	}
	var err error
	unkTok := tj.Model.UnkToken
	if unkTok == "" {
		unkTok = "[UNK]"
	}
	if wp.unk, err = lookup(unkTok); err != nil {
		return nil, err
	}
	if wp.cls, err = lookup("[CLS]"); err != nil {
		return nil, err
	}
	if wp.sep, err = lookup("[SEP]"); err != nil {
		return nil, err
	}
	return wp, nil
}

func (wp *wordPiece) Encode(text string) ([]int64, []int64) {
	words := wp.preTokenize(wp.normalize(text))

	ids := make([]int64, 0, len(words)+2)
	ids = append(ids, wp.cls)
	for _, w := range words {
		ids = append(ids, wp.encodeWord(w)...)
		if len(ids) >= MaxSeqLen-1 {
			ids = ids[:MaxSeqLen-1]
			break
		}
	}
	ids = append(ids, wp.sep)
	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask
}

// encodeWord is greedy longest-match-first with the ## continuation prefix.
func (wp *wordPiece) encodeWord(word string) []int64 {
	runes := []rune(word)
	if len(runes) > maxWordChars {
		return []int64{wp.unk}
	}
	var out []int64
	start := 0
	for start < len(runes) {
		end := len(runes)
		var id int64
		found := false
		for end > start {
			piece := string(runes[start:end])
			if start > 0 {
				piece = wp.contPrefix + piece
			}
			if v, ok := wp.vocab[piece]; ok {
				id, found = v, true
				break
			}
			end--
		}
		if !found {
			return []int64{wp.unk} // whole word becomes [UNK], per HF
		}
		out = append(out, id)
		start = end
	}
	return out
}

// normalize implements BertNormalizer: clean_text, CJK spacing, lowercase,
// accent stripping via NFD + Mn removal.
func (wp *wordPiece) normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if wp.cleanText {
			if r == 0 || r == 0xFFFD || isControl(r) {
				continue
			}
			if unicode.IsSpace(r) {
				b.WriteRune(' ')
				continue
			}
		}
		if wp.chineseChars && isCJKIdeograph(r) {
			b.WriteRune(' ')
			b.WriteRune(r)
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()

	if wp.stripAccents {
		var sb strings.Builder
		sb.Grow(len(s))
		for _, r := range norm.NFD.String(s) {
			if unicode.Is(unicode.Mn, r) {
				continue
			}
			sb.WriteRune(r)
		}
		s = sb.String()
	}
	if wp.lowercase {
		s = strings.ToLower(s)
	}
	return s
}

// preTokenize implements BertPreTokenizer: whitespace split plus punctuation
// isolation.
func (wp *wordPiece) preTokenize(s string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			flush()
		case isBertPunct(r):
			flush()
			words = append(words, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return words
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false // treated as whitespace by clean_text
	}
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r)
}

// isBertPunct mirrors BERT's _is_punctuation: ASCII symbol ranges plus
// Unicode P categories.
func isBertPunct(r rune) bool {
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) || (r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

// isCJKIdeograph mirrors BERT's _is_chinese_char ranges (Hangul and kana are
// intentionally excluded).
func isCJKIdeograph(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF,
		r >= 0x3400 && r <= 0x4DBF,
		r >= 0x20000 && r <= 0x2A6DF,
		r >= 0x2A700 && r <= 0x2B73F,
		r >= 0x2B740 && r <= 0x2B81F,
		r >= 0x2B820 && r <= 0x2CEAF,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0x2F800 && r <= 0x2FA1F:
		return true
	}
	return false
}
