package lexical

import (
	"strings"
	"unicode/utf8"
)

// Stem folds an English inflection onto the form an identifier would spell it
// in. It exists because BM25 matched raw tokens: a question typed as prose
// says "where is the lock **acquired**" while the symbol is `Acquire`, and
// those were two different terms, so the definition never scored (measured
// 2026-08-31: prose recall 33% lexical-only, and `internal/lock/lockfile.go`
// absent from every slot).
//
// It is deliberately not Porter. Porter is tuned for English prose and folds
// `embed` → `emb`, which would push the verb further from `embeddings`
// (→ `embed`) rather than onto it — the opposite of what a code index needs.
// The rules here are the four inflections that actually collide with
// identifier naming, each guarded by a minimum stem length so short symbols
// survive intact.
//
// Korean is the same problem with a harsher failure: nothing separates a word
// from the particle glued to it, so "이벤트를" and "이벤트" are two terms that
// never meet, and a whole question can miss every term it has. Measured
// 2026-08-31 on this repository, lexical-only: three of six questions asked in
// natural Korean returned *nothing at all*, where the same question typed as
// bare keywords returned ten. Trailing particles are stripped for that reason.
//
// Only all-letter ASCII tokens take the English rules. Anything carrying a
// digit is a version, an offset or a name like `utf8`, where a trailing "s" is
// not a plural.
func Stem(t string) string {
	if endsHangul(t) {
		return stemParticle(t)
	}
	if len(t) < 4 || !asciiLetters(t) {
		return t
	}
	t = stemPlural(t)
	t = stemVerb(t)
	return stemFinalE(t)
}

// Korean particles and the verb endings that attach the same way, longest
// first so "에서는" is not read as "는". This is a fixed list rather than a
// morphological analysis: the goal is only to let a noun meet itself.
var particles = []string{
	"에서는", "으로는", "에게서", "에게는", "이라는", "이라도", "으로써", "으로서",
	"에서", "에게", "으로", "라도", "부터", "까지", "보다", "처럼", "이나", "이란", "이라",
	"에는", "하지", "하는", "한다", "했다", "하며", "하고", "되는", "되지", "된다",
	"은", "는", "이", "가", "을", "를", "에", "의", "와", "과", "도", "만", "로", "나",
}

// stemParticle drops one trailing particle, but only when at least two runes
// survive. That floor is what protects the two-syllable words which simply end
// in a particle-shaped syllable — 회의, 결과, 국가, 대로 are whole nouns, and
// stripping them would leave a single syllable that matches everything. The
// cost is that a one-syllable noun keeps its particle ("락을" stays whole).
//
// The token may be mixed script: `splitWords` keeps letters together, so a
// Korean sentence about the tool yields "graphin을" as one token. Measuring
// the remainder in runes rather than syllables is what lets that one work too.
func stemParticle(t string) string {
	for _, p := range particles {
		if !strings.HasSuffix(t, p) {
			continue
		}
		stem := t[:len(t)-len(p)]
		if utf8.RuneCountInString(stem) < 2 {
			return t
		}
		return stem
	}
	return t
}

// endsHangul reports whether the token's last rune is a Hangul syllable —
// the only position a particle can occupy.
func endsHangul(t string) bool {
	r, size := utf8.DecodeLastRuneInString(t)
	return size > 0 && r >= 0xAC00 && r <= 0xD7A3
}

// stemPlural: classes → class, policies → policy, files → file. "ss", "us"
// and "is" endings are left alone — `class`, `status` and `analysis` are
// singular already, and stripping them invents a word.
func stemPlural(t string) string {
	switch {
	case strings.HasSuffix(t, "sses"):
		return t[:len(t)-2]
	case len(t) >= 5 && strings.HasSuffix(t, "ies"):
		return t[:len(t)-3] + "y"
	case strings.HasSuffix(t, "ss"), strings.HasSuffix(t, "us"), strings.HasSuffix(t, "is"):
		return t
	case strings.HasSuffix(t, "s"):
		return t[:len(t)-1]
	}
	return t
}

// stemVerb: acquired → acquir, watching → watch, embedded → embed. The
// minimum stem of four is what keeps `embed` itself out of the rule: stripping
// its "ed" would leave `emb`, and the verb would stop meeting its own gerund.
func stemVerb(t string) string {
	for _, suf := range []string{"ing", "ed"} {
		if !strings.HasSuffix(t, suf) {
			continue
		}
		stem := t[:len(t)-len(suf)]
		if len(stem) < 4 {
			return t
		}
		return undouble(stem)
	}
	return t
}

// undouble reverses the consonant a gerund doubles: embedd → embed, stopp →
// stop. Doubled l/s/z are left as they are — `call`, `pass` and `buzz` end
// that way on their own.
func undouble(t string) string {
	n := len(t)
	if n < 4 || t[n-1] != t[n-2] {
		return t
	}
	switch c := t[n-1]; {
	case c == 'l' || c == 's' || c == 'z':
		return t
	case isVowel(c):
		return t
	}
	return t[:n-1]
}

// stemFinalE closes the gap the verb rule opens: `acquired` becomes `acquir`,
// so `acquire` has to as well. Six characters is the floor because `file`,
// `node` and `code` are whole words whose "e" carries their identity, and
// "ee" is exempt so `degree` does not become a prefix of `degr`.
func stemFinalE(t string) string {
	if len(t) < 6 || !strings.HasSuffix(t, "e") || strings.HasSuffix(t, "ee") {
		return t
	}
	return t[:len(t)-1]
}

func isVowel(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

func asciiLetters(t string) bool {
	for i := 0; i < len(t); i++ {
		if c := t[i]; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// stemAll maps a token list one-for-one. Keeping the written form beside the
// stem was measured too (2026-08-31) and bought nothing the replacement did
// not: identical on every lexical-only question, better on one hybrid question
// and worse on another, at the price of a longer posting list for every
// document that inflects. One-for-one also leaves BM25's length normalization
// exactly where it was.
func stemAll(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = Stem(t)
	}
	return out
}
