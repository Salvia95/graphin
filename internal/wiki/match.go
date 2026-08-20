package wiki

import (
	"strings"
	"unicode"

	"github.com/Salvia95/graphin/internal/lexical"
)

// minWordRunes ignores words too short to mean anything on their own —
// articles in English, particles in Korean.
const minWordRunes = 2

// wordKeys expands one word into every form that should be able to match it.
//
// Latin text is handled by the index's own tokenizer, so "cancelPayment" and
// "cancel_payment" meet here exactly as they do in search. CJK text needs
// more: nothing separates a Korean word from the particle glued to it, so
// "릴리스를" and "릴리스" are two different tokens that would never meet.
// Character bigrams bridge that — the standard answer for scripts that do not
// space their words, and enough for deciding whether a set is relevant.
func wordKeys(word string) []string {
	runes := []rune(word)
	if len(runes) < minWordRunes {
		return nil
	}
	keys := lexical.Tokenize(word)
	if !hasCJK(runes) {
		return keys
	}
	for i := 0; i+1 < len(runes); i++ {
		keys = append(keys, string(runes[i:i+2]))
	}
	return keys
}

func hasCJK(runes []rune) bool {
	for _, r := range runes {
		if unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Han, r) ||
			unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

// splitWords breaks text at anything that cannot be part of a word.
func splitWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
}

// keySet is every matchable key in a body of text.
func keySet(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range splitWords(text) {
		for _, k := range wordKeys(w) {
			out[k] = true
		}
	}
	return out
}

// countMatchingWords reports how many distinct words of query appear in keys.
//
// Distinct WORDS, not distinct keys, and the difference decides the
// threshold's meaning. One CJK word expands to several bigrams, so counting
// keys would let a single shared word clear a bar meant to require two.
func countMatchingWords(query string, keys map[string]bool) int {
	seen := map[string]bool{}
	n := 0
	for _, w := range splitWords(query) {
		lower := strings.ToLower(w)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		for _, k := range wordKeys(w) {
			if keys[k] {
				n++
				break
			}
		}
	}
	return n
}
