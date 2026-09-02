package wiki

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// minTaskMatches is how many distinct task words a set must contain before a
// task alone pulls it in.
//
// One is too loose: a single shared common word would attach any set to any
// task, and a manifest that names everything teaches a reader to skip it.
// The exception below — a direct hit on the set's own name — exists because
// "how do I cut a release" naming the `release` set is not a coincidence.
const minTaskMatches = 2

// minSetsForCommonStop is how many sets a wiki needs before "many sets share
// this key" is evidence of anything. Below it the sample is too small to tell
// a common word from a coincidence.
const minSetsForCommonStop = 4

// Selection is the outcome of matching a task to the wiki.
type Selection struct {
	// Sets is the reading list, prerequisites first.
	Sets []*Set
	// Terms are glossary entries the task's own wording touched.
	Terms []*Term
	// Pushed names the sets included because the role always gets them.
	Pushed []string
	// Matched names the sets the task text pulled in.
	Matched []string
	// Missing names prerequisites that no set defines — an authoring bug,
	// surfaced rather than swallowed.
	Missing []string
}

// Empty reports a coverage miss: nothing in the wiki applies to this work.
//
// Terms count. A task that used a project word and got its definition was
// answered, even if no set matched.
//
// This is a normal, successful answer, not a failure. Every agent is gated,
// so most preflights for most tasks must return nothing quickly — and each
// one that does is a signal about where the wiki is thin.
func (s Selection) Empty() bool { return len(s.Sets) == 0 && len(s.Terms) == 0 }

// Select decides which sets a piece of work needs.
//
// Two mechanisms, deliberately different in kind. Role tagging is a standing
// decision by the wiki's authors that a class of agent always needs something
// — the reader cannot be expected to know it is missing. Task matching is a
// guess about this particular job, and it has to be conservative because a
// wrong guess costs the reader attention on every delegation.
func (st *Store) Select(role, task string) Selection {
	var sel Selection
	wanted := map[string]bool{}
	stop := st.stopKeys()

	for _, s := range st.ForRole(role) {
		if !wanted[s.Name] {
			wanted[s.Name] = true
			sel.Pushed = append(sel.Pushed, s.Name)
		}
	}

	for _, s := range st.SetList() {
		if wanted[s.Name] {
			continue
		}
		if matchesTask(s, task, stop) {
			wanted[s.Name] = true
			sel.Matched = append(sel.Matched, s.Name)
		}
	}

	names := make([]string, 0, len(wanted))
	for n := range wanted {
		names = append(names, n)
	}
	sort.Strings(names)

	sel.Sets, sel.Missing = st.Expand(names)
	sel.Terms = st.termsIn(task)
	return sel
}

// termsIn finds glossary entries the task's own wording touched, in name
// order. Matching is on the canonical form and its aliases only: a definition
// offered because the task shared a word with its body would be noise, and
// noise in a glossary is worse than absence — it teaches the reader to skip
// the whole block.
func (st *Store) termsIn(task string) []*Term {
	if strings.TrimSpace(task) == "" {
		return nil
	}
	names := make([]string, 0, len(st.Terms))
	for n := range st.Terms {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []*Term
	for _, n := range names {
		t := st.Terms[n]
		if t.Status == StatusDraft {
			continue
		}
		keys := keySet(strings.Join(append([]string{t.Canonical}, t.Aliases...), " "))
		if countMatchingWords(task, keys) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// grammarKeys are the matching keys Korean grammar produces rather than
// vocabulary.
//
// Bigram expansion exists to bridge the particle glued to a word — "릴리스를"
// and "릴리스" would never meet otherwise — but it also emits the particle,
// and "…는지" or "…한다" ends a sentence in every task anyone will ever write.
// Two of those clear a bar meant to require two shared subjects.
//
// Only closed-class morphemes are listed, and that is the whole reason this
// list is allowed to exist: a stoplist of domain words rots as the vocabulary
// moves, so it would have to be maintained by whoever adds a set. Korean
// grammar does not move. Anything here that could also be content — 하는,
// 없는, 하지 — was left out on purpose.
var grammarKeys = map[string]bool{
	"는지": true, "을지": true, "는가": true, "인가": true, "느냐": true,
	"니다": true, "습니": true, "한다": true, "했다": true, "된다": true,
	"이다": true, "에서": true, "으로": true, "에게": true, "부터": true,
	"까지": true, "처럼": true, "마다": true, "라도": true, "하면": true,
	"되면": true, "려면": true,
}

// englishGrammar is grammarKeys for English: articles, prepositions,
// conjunctions, pronouns and auxiliaries. The same licence — English grammar
// does not move either, so the list never has to follow the vocabulary — and
// the same failure. A set whose labels quote an English phrase carries "do"
// and "and", and every English task carries both by its second sentence;
// those two alone cleared a bar meant to require two shared subjects. Nothing
// here can be a subject.
var englishGrammar = []string{
	"a", "an", "the", "this", "that", "these", "those", "it", "its",
	"and", "or", "but", "nor", "so", "if", "than", "as", "not", "no",
	"of", "to", "in", "on", "at", "by", "for", "from", "with", "into", "onto",
	"over", "under", "about", "after", "before", "between", "without", "within",
	"through", "up", "down", "out", "off",
	"is", "are", "was", "were", "be", "been", "being", "am",
	"do", "does", "did", "done", "has", "have", "had", "having",
	"can", "could", "will", "would", "shall", "should", "may", "might", "must",
	"we", "our", "us", "you", "your", "they", "their", "them", "he", "she",
	"his", "her", "i", "me", "my", "who", "whom", "whose", "what", "which",
	"when", "where", "why", "how", "there", "here", "then", "also", "only",
	"any", "some", "each", "every", "all", "such", "very", "just",
}

// stopKeys are keys that cannot tell one set in this wiki from another, so
// counting them toward the threshold only manufactures matches.
//
// Beyond grammar there is one more: the project's own name. Every set in a
// wiki is about the project the wiki belongs to, so naming it says nothing
// about which set — and it is in most of them, which is exactly what a key
// with no discriminating power looks like. A set genuinely named after the
// project still matches through the name path below, which does not read this.
func (st *Store) stopKeys() map[string]bool {
	out := make(map[string]bool, len(grammarKeys)+2*len(englishGrammar)+4)
	for k := range grammarKeys {
		out[k] = true
	}
	// Through wordKeys, so the stem goes in beside the word: a label's "does"
	// also yields "doe", and a stop list that only knew the word would let
	// the stem match.
	for _, w := range englishGrammar {
		for _, k := range wordKeys(w) {
			out[k] = true
		}
	}
	for _, k := range wordKeys(strings.ToLower(filepath.Base(st.Root))) {
		out[k] = true
	}

	// 여러 세트의 라벨에 걸쳐 나타나는 키는 변별하지 못한다 — 문법어를 빼는 것과
	// 같은 이유이고, 같은 규칙을 데이터에서 읽는 것뿐이다.
	//
	// 한국어 라벨은 2-gram으로도 쪼개지므로("게이트" → "게이"·"이트") 서로 다른
	// 낱말이 같은 키를 공유하고, 엔트리가 많은 세트일수록 그런 키를 많이 갖는다.
	// 그래서 minTaskMatches=2가 큰 세트에는 사실상 문턱이 아니게 된다 — 2026-09-02
	// 통합 벤치에서 delegation-gate(18엔트리)와 console(14)이 릴리스 질문에도,
	// 지표 질문에도 붙은 기제가 이것이다.
	//
	// 세트가 적으면 적용하지 않는다. 셋뿐인 위키에서 "둘 이상이 공유"는 흔한 일이라
	// 규칙이 라벨을 통째로 지워 버린다.
	sets := st.SetList()
	if len(sets) >= minSetsForCommonStop {
		count := map[string]int{}
		for _, s := range sets {
			for k := range keySet(setText(s)) {
				count[k]++
			}
		}
		// 내림이다. 올림이면 세트 하나를 더하는 것만으로 문턱이 1 올라가고,
		// 걸러지던 키가 한꺼번에 되돌아온다 — 세트 7번째를 추가하자 여분이 3에서
		// 8로 튄 것이 그 일이다. 내림은 6·7세트에서 같은 문턱(3)을 유지한다.
		limit := len(sets) / 2
		if limit < 2 {
			limit = 2
		}
		for k, n := range count {
			if n >= limit {
				out[k] = true
			}
		}
	}
	return out
}

// matchesTask scores one set against the task description.
func matchesTask(s *Set, task string, stop map[string]bool) bool {
	// The set's own name is a stronger signal than any single label word: a
	// task that says "release" and a set called "release" are about the same
	// thing far more often than not. Both the filename and the heading count,
	// because one is frequently an ASCII slug for the other and a reader
	// naming their work will have used whichever reads naturally to them.
	if countMatchingWords(task, keySet(s.Name+" "+s.Title)) > 0 {
		return true
	}
	// Aliases ride the name path, not the label path. They exist for the
	// reader whose task is written in a language the labels are not — this
	// wiki's labels are Korean, and in the 2026-09-02 combined bench an
	// English task reached a set only when its filename slug happened to be
	// the word used — and an alias is the author saying "this is what the
	// subject is called in that vocabulary", which is exactly the claim the
	// name makes. So it earns the same single hit, and the stop list is not
	// applied: an alias is chosen, not accumulated. The difference from the
	// name is that a multi-word alias is a phrase. "gate tier" means the pair,
	// so a task that merely says "gate" does not pull a release set in.
	if len(s.Aliases) > 0 {
		taskKeys := keySet(task)
		for _, a := range s.Aliases {
			if aliasMatches(a, taskKeys) {
				return true
			}
		}
	}
	keys := keySet(setText(s))
	for k := range stop {
		delete(keys, k)
	}
	return countMatchingWords(task, keys) >= minTaskMatches
}

// aliasMatches reports whether every word of one alias appears in the task.
//
// The direction is deliberate: alias words are looked up in the task's keys,
// not the other way round. A task writes "discovery-failure" as one word and
// an alias writes "discovery failure" as two; counting task words would score
// that as one hit against a phrase that needs two, while looking each alias
// word up in the task's expanded keys finds both. Words too short to carry a
// key are skipped rather than made impossible to satisfy.
//
// A hyphen splits here even though splitWords keeps it. "re-score" as one
// word expands to the fragment "re", and any key of a word counting as a hit
// would let "re-run" satisfy it; as two words, both halves are required, which
// is what the author who wrote a compound meant.
func aliasMatches(alias string, taskKeys map[string]bool) bool {
	matched := 0
	words := strings.FieldsFunc(alias, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		ks := aliasKeys(w)
		if len(ks) == 0 {
			continue
		}
		hit := false
		for _, k := range ks {
			if taskKeys[k] {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
		matched++
	}
	return matched > 0
}

// setText is the set's labels: its name, the paragraph saying what it is for,
// its group titles and its entry titles.
//
// Entry summaries are deliberately left out, and that was learned rather than
// assumed. Summaries are prose written to explain a section, so a set of any
// size accumulates enough ordinary words to share two with almost any task —
// "the user has to fix something" pulled a release set into a task about
// users. Labels are chosen to name a subject, so they discriminate; prose is
// written to be read, so it does not.
func setText(s *Set) string {
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteString(" ")
	b.WriteString(s.Title)
	b.WriteString(" ")
	b.WriteString(s.Summary())
	for _, g := range s.Groups {
		b.WriteString(" ")
		b.WriteString(g.Title)
		for _, e := range g.Entries {
			b.WriteString(" ")
			b.WriteString(e.Title)
		}
	}
	return b.String()
}

// Manifest is what a delegation carries: enough to decide, not enough to
// read. Set bodies are deliberately absent — the point of the catalogue is
// that the delegate loads only what it turns out to need.
//
// Glossary terms are the exception, and carry their definitions inline. A
// definition is one paragraph, and a reader who has to fetch it will not:
// the failure a glossary prevents is using the wrong word without noticing,
// which is precisely the state in which nobody goes looking.
type Manifest struct {
	Sets    []ManifestSet
	Terms   []ManifestTerm
	Token   string
	Missing []string
}

// ManifestTerm is one glossary entry as delivered.
type ManifestTerm struct {
	Canonical  string
	Aliases    []string
	Definition string
	Confusions []Confusion
}

// manifestTerm reduces a glossary entry to what a delegate is actually handed.
//
// Manifest renders this and Fingerprint signs it, from here and nowhere else.
// Two copies of the field list would let the signature come to cover something
// other than what was delivered, which is the one way a verifying token can
// still be a lie.
func manifestTerm(t *Term) ManifestTerm {
	return ManifestTerm{
		Canonical:  t.Canonical,
		Aliases:    t.Aliases,
		Definition: firstSentence(t.Body),
		Confusions: t.Confusions,
	}
}

// ManifestSet is one set as it appears in a catalogue.
type ManifestSet struct {
	Name string
	// NodeID of the set file itself, so a reader that wants the whole
	// catalogue can load it in one call.
	NodeID string
	// Summary is the set's opening prose, trimmed to one line.
	Summary string
	Groups  []ManifestGroup
	// Entries counts what resolving this set would bring in, so a reader can
	// tell a two-line set from a thirty-line one before asking for it.
	Entries int
	// Unreviewed says an agent changed this set and nobody has checked. It
	// is in the catalogue rather than only on the sections because the
	// catalogue is where a reader decides whether to open a set at all.
	Unreviewed bool
}

// ManifestGroup names one group and its size.
type ManifestGroup struct {
	Title   string
	NodeID  string
	Entries int
}

// Manifest renders a selection as a catalogue and signs the wiki state it was
// produced from.
func (st *Store) Manifest(sel Selection, secret []byte) Manifest {
	m := Manifest{Missing: sel.Missing, Token: st.MintToken(secret)}
	for _, t := range sel.Terms {
		m.Terms = append(m.Terms, manifestTerm(t))
	}
	for _, s := range sel.Sets {
		ms := ManifestSet{
			Name:       s.Name,
			NodeID:     s.RelPath,
			Summary:    firstSentence(s.Summary()),
			Entries:    len(s.Entries()),
			Unreviewed: s.Unreviewed,
		}
		for _, g := range s.Groups {
			if g.Title == "" {
				continue
			}
			ms.Groups = append(ms.Groups, ManifestGroup{
				Title: g.Title, NodeID: g.NodeID, Entries: len(g.Entries),
			})
		}
		m.Sets = append(m.Sets, ms)
	}
	return m
}

// firstSentence keeps a catalogue line to one line. A set whose intro runs to
// a paragraph still has to fit next to nine others.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, end := range []string{". ", "다. ", "? ", "! "} {
		if i := strings.Index(s, end); i >= 0 {
			return strings.TrimSpace(s[:i+len(end)-1])
		}
	}
	const max = 160
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut-- // never split a UTF-8 rune
	}
	return strings.TrimSpace(s[:cut]) + "…"
}
