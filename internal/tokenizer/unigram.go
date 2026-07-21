package tokenizer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const metaspace = '▁' // ▁

// unkPenalty mirrors sentencepiece's kUnkPenalty.
const unkPenalty = 10.0

type piece struct {
	id    int64
	score float64
}

type unigram struct {
	pieces   map[string]piece
	maxPiece int // longest piece in bytes
	unkID    int64
	unkScore float64
	bos, eos int64 // <s>, </s>
	fuseUnk  bool
}

func newUnigram(tj *tokenizerJSON) (*unigram, error) {
	var vocab [][2]json.RawMessage
	if err := json.Unmarshal(tj.Model.Vocab, &vocab); err != nil {
		return nil, fmt.Errorf("unigram vocab: %w", err)
	}
	u := &unigram{pieces: make(map[string]piece, len(vocab)), fuseUnk: true}
	if tj.Model.FuseUnk != nil {
		u.fuseUnk = *tj.Model.FuseUnk
	}
	minScore := math.Inf(1)
	for i, entry := range vocab {
		var tok string
		var score float64
		if err := json.Unmarshal(entry[0], &tok); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(entry[1], &score); err != nil {
			return nil, err
		}
		u.pieces[tok] = piece{id: int64(i), score: score}
		if len(tok) > u.maxPiece {
			u.maxPiece = len(tok)
		}
		if score < minScore {
			minScore = score
		}
	}
	u.unkScore = minScore - unkPenalty

	if tj.Model.UnkID == nil {
		return nil, fmt.Errorf("unigram: unk_id missing")
	}
	u.unkID = int64(*tj.Model.UnkID)
	var ok bool
	if u.bos, ok = tj.addedID("<s>"); !ok {
		return nil, fmt.Errorf("unigram: <s> missing")
	}
	if u.eos, ok = tj.addedID("</s>"); !ok {
		return nil, fmt.Errorf("unigram: </s> missing")
	}
	return u, nil
}

func (u *unigram) Encode(text string) ([]int64, []int64) {
	ids := make([]int64, 0, 32)
	ids = append(ids, u.bos)
	for _, pre := range u.preTokenize(u.normalize(text)) {
		ids = append(ids, u.viterbi(pre)...)
		if len(ids) >= MaxSeqLen-1 {
			ids = ids[:MaxSeqLen-1]
			break
		}
	}
	ids = append(ids, u.eos)
	mask := make([]int64, len(ids))
	for i := range mask {
		mask[i] = 1
	}
	return ids, mask
}

// normalize approximates the Precompiled charsmap with NFKC plus whitespace
// unification (§ risk register: parity proven on fixtures).
func (u *unigram) normalize(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\t', '\n', '\r', '\v', '\f', 0x00A0:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// preTokenize implements Metaspace(add_prefix_space=true): spaces become ▁
// and every ▁ starts a new pretoken.
func (u *unigram) preTokenize(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, " ", string(metaspace))
	if !strings.HasPrefix(s, string(metaspace)) {
		s = string(metaspace) + s
	}
	var out []string
	start := 0
	for i, r := range s {
		if r == metaspace && i != start {
			out = append(out, s[start:i])
			start = i
		}
	}
	out = append(out, s[start:])
	return out
}

// viterbi segments one pretoken by maximum total piece score; characters no
// piece covers get the unk penalty and consecutive unks fuse (fuse_unk).
func (u *unigram) viterbi(s string) []int64 {
	n := len(s)
	negInf := math.Inf(-1)
	score := make([]float64, n+1)
	backStart := make([]int, n+1)
	backID := make([]int64, n+1)
	for i := 1; i <= n; i++ {
		score[i] = negInf
		backStart[i] = -1
	}

	runeStart := make([]bool, n+1)
	for i := range s {
		runeStart[i] = true
	}
	runeStart[n] = true

	for end := 1; end <= n; end++ {
		if !runeStart[end] {
			continue
		}
		lo := end - u.maxPiece
		if lo < 0 {
			lo = 0
		}
		for start := lo; start < end; start++ {
			if !runeStart[start] || score[start] == negInf {
				continue
			}
			if p, ok := u.pieces[s[start:end]]; ok {
				if cand := score[start] + p.score; cand > score[end] {
					score[end] = cand
					backStart[end] = start
					backID[end] = p.id
				}
			}
		}
		// unk fallback: consume exactly one rune
		_, size := utf8.DecodeLastRuneInString(s[:end])
		start := end - size
		if start >= 0 && runeStart[start] && score[start] != negInf {
			if cand := score[start] + u.unkScore; cand > score[end] {
				score[end] = cand
				backStart[end] = start
				backID[end] = u.unkID
			}
		}
	}

	var rev []int64
	for i := n; i > 0; {
		prev := backStart[i]
		if prev < 0 { // unreachable in practice; bail out as one unk
			return []int64{u.unkID}
		}
		rev = append(rev, backID[i])
		i = prev
	}
	// reverse and fuse consecutive unks
	out := make([]int64, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		id := rev[i]
		if u.fuseUnk && id == u.unkID && len(out) > 0 && out[len(out)-1] == u.unkID {
			continue
		}
		out = append(out, id)
	}
	return out
}
