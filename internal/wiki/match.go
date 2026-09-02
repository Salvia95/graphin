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
		// The stem too, as the index keeps it. Without it "releasing" and the
		// `release` set never meet, and an alias would have to list every
		// inflection an English reader might use — which is the maintenance
		// burden aliases were supposed to be cheap enough to avoid.
		out := append([]string(nil), keys...)
		for _, tok := range keys {
			if st := lexical.Stem(tok); st != tok {
				out = append(out, st)
			}
		}
		return out
	}
	// 바이그램은 **어간에서** 만든다. 위 문단이 라틴 문자에 대해 말한 것 — 인덱스의
	// 토크나이저를 그대로 써서 검색에서와 똑같이 만난다 — 이 한국어에는 적용되지
	// 않고 있었다. `lexical.Tokenize`는 조사를 떼지 않으므로("에이전트가"가 그대로
	// 한 토큰이다) 원본에서 자른 바이그램에는 "트가"·"다면" 같은 조사·어미 파편이
	// 섞이고, 그것들은 아무 주제도 가리키지 않으면서 minTaskMatches를 채운다.
	// 2026-09-02 통합 벤치에서 console 세트가 지표 질문에 붙은 것이 정확히 그 두
	// 키였다. `lexical.Stem`은 인덱스가 텀을 만들 때 쓰는 그 정규화다.
	//
	// 대가를 적어 둔다. 어간기는 세 음절 이상 낱말의 조사 모양 끝음절을 떼므로
	// "재평가"는 "재평"이 되고 "평가"와 만나지 못한다(재정의·재시도도 같다). 원본
	// 바이그램을 함께 넣으면 되돌아오지만 위의 조사 파편도 함께 돌아온다. 인덱스가
	// 같은 어간으로 텀을 만들므로 검색도 같은 자리에서 같은 것을 놓친다 — 고치려면
	// 여기가 아니라 어간기에서 고쳐야 한다.
	out := append([]string(nil), keys...)
	for _, tok := range keys {
		st := lexical.Stem(tok)
		if st != tok {
			out = append(out, st)
		}
		tr := []rune(st)
		for i := 0; i+1 < len(tr); i++ {
			out = append(out, string(tr[i:i+2]))
		}
	}
	return out
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
	// 낱말이 아니라 **개념**을 센다. 위 주석이 막으려던 것의 거울상이 실제로
	// 일어난다 — "서브에이전트를"과 "에이전트"는 서로 다른 낱말이라 각각 1점을
	// 받지만 둘 다 "에이" 하나로 매칭된 것이고, 그러면 한 개념이 두 낱말을
	// 요구하는 문턱을 혼자 넘는다. 키를 하나라도 공유하는 낱말은 같은 개념으로
	// 묶어 한 번만 센다.
	//
	// 묶기이지 봉인이 아니다. 앞 낱말이 쓴 키를 봉인하는 방식은 순서에 따라
	// 답이 달랐다 — "에이전트 이전"은 1, "이전 에이전트"는 2 — 그리고 매칭이
	// 질의의 어순에 따라 갈리면 같은 작업이 세트를 받기도 못 받기도 한다.
	seen := map[string]bool{}
	var hits [][]string
	for _, w := range splitWords(query) {
		lower := strings.ToLower(w)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		ks := wordKeys(w)
		for _, k := range ks {
			if keys[k] {
				hits = append(hits, ks)
				break
			}
		}
	}
	parent := make([]int, len(hits))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	owner := map[string]int{}
	for i, ks := range hits {
		for _, k := range ks {
			if j, ok := owner[k]; ok {
				if a, b := find(i), find(j); a != b {
					parent[a] = b
				}
			} else {
				owner[k] = i
			}
		}
	}
	n := 0
	for i := range hits {
		if find(i) == i {
			n++
		}
	}
	return n
}

// aliasKeys expands one alias word into the forms that may match it: the
// token and its stem, and for Latin text the identifier parts.
//
// No bigrams, unlike wordKeys. An alias is matched with one hit and outside
// the stop list, so a fragment of it must not be enough: "릴리스" split into
// "릴리"·"리스" would meet "리스트" on the second piece. The task side keeps
// its bigrams, which is what still lets "릴리스를" reach the alias through
// its stem.
func aliasKeys(word string) []string {
	if len([]rune(word)) < minWordRunes {
		return nil
	}
	keys := lexical.Tokenize(word)
	out := append([]string(nil), keys...)
	for _, tok := range keys {
		if st := lexical.Stem(tok); st != tok {
			out = append(out, st)
		}
	}
	return out
}
