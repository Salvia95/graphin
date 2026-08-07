package parse

import (
	"slices"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func mdParse(t *testing.T, path, src string) *FileResult {
	t.Helper()
	res, err := File(path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if res.Lang != LangMarkdown {
		t.Fatalf("lang = %v", res.Lang)
	}
	return res
}

func sections(res *FileResult) []Node {
	var out []Node
	for _, n := range res.Nodes {
		if n.Kind == nodeid.KindSection {
			out = append(out, n)
		}
	}
	return out
}

func TestMarkdownFileNodeSurvivesWithPreambleTokensOnly(t *testing.T) {
	src := "머리말 프렐류드 문장.\n\n# 제목\n\n본문 배선하지 않는다\n"
	res := mdParse(t, "docs/guide.md", src)

	f := res.Nodes[0]
	if f.ID != "docs/guide.md" || f.Kind != nodeid.KindFile {
		t.Fatalf("file node = %+v", f)
	}
	// Span stays the whole file so read_code on the file is unchanged (D1).
	if f.StartByte != 0 || int(f.EndByte) != len(src) {
		t.Fatalf("file span = %d..%d, want whole file", f.StartByte, f.EndByte)
	}
	// …but the tokens are the preamble only (D5), or section 1's text would
	// enter BM25 twice.
	if !slices.Contains(f.BodyTokens, "프렐류드") {
		t.Fatalf("preamble token missing: %v", f.BodyTokens)
	}
	if slices.Contains(f.BodyTokens, "배선하지") {
		t.Fatalf("file node absorbed section text: %v", f.BodyTokens)
	}
	if res.Package != "docs" {
		t.Fatalf("package = %q", res.Package)
	}
}

func TestMarkdownNoHeadingsBehavesLikePlain(t *testing.T) {
	src := "just prose, no headings at all\n"
	res := mdParse(t, "NOTES.md", src)
	if len(res.Nodes) != 1 {
		t.Fatalf("want file node only, got %d nodes", len(res.Nodes))
	}
	if !slices.Contains(res.Nodes[0].BodyTokens, "prose") {
		t.Fatalf("whole file should be the preamble: %v", res.Nodes[0].BodyTokens)
	}
}

// The measured trap: this repository's docs quote shell scripts, and 26 of 302
// candidate headings are `#` comments inside fences (spec §3.2).
func TestMarkdownFencedHashesAreNotHeadings(t *testing.T) {
	src := "# 진짜 제목\n\n```sh\n# 가짜 제목 (셸 주석)\necho hi\n```\n\n~~~\n## 물결 펜스 안\n~~~\n\n## 진짜 둘째\n"
	res := mdParse(t, "docs/a.md", src)
	secs := sections(res)
	if len(secs) != 2 {
		var got []string
		for _, s := range secs {
			got = append(got, s.DisplayName)
		}
		t.Fatalf("want 2 sections, got %d: %v", len(secs), got)
	}
	if secs[0].DisplayName != "진짜 제목" || secs[1].DisplayName != "진짜 둘째" {
		t.Fatalf("sections = %q, %q", secs[0].DisplayName, secs[1].DisplayName)
	}
	// The fenced block must live inside the first section's span, not be cut.
	body := src[secs[0].StartByte:secs[0].EndByte]
	if !strings.Contains(body, "echo hi") {
		t.Fatalf("fenced content fell outside the section: %q", body)
	}
}

// A closing fence must match the opening character and be at least as long;
// a shorter run or the other character does not close it.
func TestMarkdownFenceClosingRules(t *testing.T) {
	src := "````\n# 안쪽\n~~~\n# 여전히 안쪽\n````\n\n# 바깥\n"
	res := mdParse(t, "a.md", src)
	secs := sections(res)
	if len(secs) != 1 || secs[0].DisplayName != "바깥" {
		t.Fatalf("sections = %+v", secs)
	}
}

func TestMarkdownSpansAreFlat(t *testing.T) {
	src := "## 13. 버저닝\n\n서문.\n\n### 13.1 첫째\n\n내용.\n\n### 13.2 둘째\n\n끝.\n"
	res := mdParse(t, "docs/spec.md", src)
	secs := sections(res)
	if len(secs) != 3 {
		t.Fatalf("want 3 sections, got %d", len(secs))
	}
	// §13's span stops at §13.1 (D3): a parent and a child requested together
	// must never return the same bytes twice.
	parent := string(src[secs[0].StartByte:secs[0].EndByte])
	if strings.Contains(parent, "13.1") {
		t.Fatalf("parent span swallowed its child:\n%s", parent)
	}
	if !strings.Contains(parent, "서문") {
		t.Fatalf("parent lost its own prose:\n%s", parent)
	}
	// Spans partition the file: each starts where the previous ended.
	for i := 1; i < len(secs); i++ {
		if secs[i-1].EndByte != secs[i].StartByte {
			t.Fatalf("gap/overlap between %q and %q", secs[i-1].DisplayName, secs[i].DisplayName)
		}
	}
	if int(secs[len(secs)-1].EndByte) != len(src) {
		t.Fatal("last section must run to EOF")
	}
}

func TestMarkdownParentIsNearestShallower(t *testing.T) {
	// Level skip (h2 → h4) and a file that never uses h1 — both exist in this
	// repository, so "exactly one level up" and "h1 is the root" are wrong.
	src := "## 부모\n\n#### 건너뛴 손자\n\n### 자식\n\n## 다음 부모\n"
	res := mdParse(t, "a.md", src)
	secs := sections(res)
	want := map[string]string{
		"부모":     "",
		"건너뛴 손자": "부모",
		"자식":     "부모",
		"다음 부모":  "",
	}
	for _, s := range secs {
		if got := s.Container; got != want[s.DisplayName] {
			t.Errorf("%q parent = %q, want %q", s.DisplayName, got, want[s.DisplayName])
		}
	}
}

// Expectations read off GitHub's own rendering of this repository
// (`gh api /repos/.../contents/docs/plugin-distribution.md -H
// "Accept: application/vnd.github.html"`), not guessed. Consecutive hyphens
// are not collapsed and the ends are not trimmed: a dropped character leaves
// nothing, but the spaces that surrounded it each still become a hyphen.
func TestMarkdownSlugMatchesGitHubAnchors(t *testing.T) {
	cases := map[string]string{
		"13. 버저닝": "13-버저닝",
		"10.3 릴리스 게이트 실측 (2026-08-07)":      "103-릴리스-게이트-실측-2026-08-07",
		"플러그인 배포 스펙 — 바이너리 동봉 자기완결 설치 (v1)": "플러그인-배포-스펙--바이너리-동봉-자기완결-설치-v1",
		"2. Phase 0 — 선행 (코드 없음)":           "2-phase-0--선행-코드-없음",
		"3.4 부수 개선 ✅":                       "34-부수-개선-",
		"4.1 darwin/arm64 핀 (v1.1)":         "41-darwinarm64-핀-v11",
		"3.1 `const` → `var`":               "31-const--var",
	}
	for heading, want := range cases {
		if got := slugify(heading); got != want {
			t.Errorf("slugify(%q)\n  got  %q\n  want %q", heading, got, want)
		}
	}
}

func TestMarkdownSlugCollisionsGetSuffixes(t *testing.T) {
	src := "# 같은 제목\n\n# 같은 제목\n\n# 같은 제목\n"
	res := mdParse(t, "a.md", src)
	var ids []string
	for _, s := range sections(res) {
		ids = append(ids, s.ID)
	}
	want := []string{"a.md#같은-제목", "a.md#같은-제목-1", "a.md#같은-제목-2"}
	if !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

func TestMarkdownHeadingSyntaxEdges(t *testing.T) {
	src := "#hashtag 아님\n\n####### 일곱개 아님\n\n## 닫는 해시 ##\n\n#\n"
	res := mdParse(t, "a.md", src)
	secs := sections(res)
	if len(secs) != 2 {
		var got []string
		for _, s := range secs {
			got = append(got, s.DisplayName)
		}
		t.Fatalf("want 2 sections, got %v", got)
	}
	if secs[0].DisplayName != "닫는 해시" {
		t.Fatalf("closing run not stripped: %q", secs[0].DisplayName)
	}
	// A bare "#" is a heading with empty text; its slug falls back.
	if secs[1].ID != "a.md#section" {
		t.Fatalf("empty heading id = %q", secs[1].ID)
	}
}

func TestMarkdownSectionTokensAndHash(t *testing.T) {
	src := "# 하나\n\n프렐류드 라는 말은 여기에만 있다\n\n# 둘\n\n다른 내용\n"
	res := mdParse(t, "a.md", src)
	secs := sections(res)
	if !slices.Contains(secs[0].BodyTokens, "프렐류드") {
		t.Fatalf("section body tokens missing: %v", secs[0].BodyTokens)
	}
	if slices.Contains(secs[1].BodyTokens, "프렐류드") {
		t.Fatal("token leaked into the next section")
	}
	if secs[0].Hash == secs[1].Hash {
		t.Fatal("sections with different bodies must hash differently")
	}
}

// Contains targets are decided by the parser — same file, exact IDs — so the
// graph engine emits them without a candidate lookup (spec §3.5).
func TestMarkdownContainsHierarchy(t *testing.T) {
	// A level skip and a document that never uses h1: both exist in this
	// repository, and both break a "one level up" or "h1 is the root" rule.
	src := "머리말\n\n## 부모\n\n#### 건너뛴 손자\n\n### 자식\n\n## 다음 부모\n"
	res := mdParse(t, "docs/a.md", src)

	got := map[string][]string{}
	for _, n := range res.Nodes {
		if len(n.Contains) > 0 {
			got[n.ID] = n.Contains
		}
	}
	want := map[string][]string{
		"docs/a.md":    {"docs/a.md#부모", "docs/a.md#다음-부모"},
		"docs/a.md#부모": {"docs/a.md#건너뛴-손자", "docs/a.md#자식"},
	}
	if len(got) != len(want) {
		t.Fatalf("contains map = %v, want %v", got, want)
	}
	for parent, kids := range want {
		if !slices.Equal(got[parent], kids) {
			t.Errorf("%s contains %v, want %v", parent, got[parent], kids)
		}
	}
}

func TestMarkdownNoHeadingsHasNoContains(t *testing.T) {
	res := mdParse(t, "a.md", "prose only\n")
	if len(res.Nodes[0].Contains) != 0 {
		t.Fatalf("file node should contain nothing: %v", res.Nodes[0].Contains)
	}
}

// The file node's hash covers its PREAMBLE, not the whole file, because the
// preamble is what it indexes (D5). Two consequences are load-bearing:
// editing a section must not churn the file node, and a workspace indexed
// before sections existed must see this node as changed exactly once so it
// drops its stale whole-file tokens.
func TestMarkdownFileNodeHashesThePreamble(t *testing.T) {
	base := "머리말 그대로\n\n# 제목\n\n"
	a := mdParse(t, "a.md", base+"본문 하나\n")
	b := mdParse(t, "a.md", base+"본문 완전히 다름\n")
	if a.Nodes[0].Hash != b.Nodes[0].Hash {
		t.Fatal("a section edit churned the file node")
	}
	if sections(a)[0].Hash == sections(b)[0].Hash {
		t.Fatal("the section itself must register the change")
	}

	c := mdParse(t, "a.md", "머리말이 바뀌었다\n\n# 제목\n\n본문 하나\n")
	if a.Nodes[0].Hash == c.Nodes[0].Hash {
		t.Fatal("a preamble edit must change the file node")
	}

	// The old whole-file hash is what a pre-section index recorded; the new
	// hash must differ from it, or the upgrade would never land.
	if a.Nodes[0].Hash == a.FileHash {
		t.Fatal("file node still hashes the whole file; upgrades would not re-tokenize it")
	}
}

// Start lines are recorded at parse time so search can name a location with no
// file I/O. Nodes are visited in byte order, so a document's sections must come
// back with strictly increasing lines regardless of the order they were built.
func TestMarkdownStartLinesAreRecorded(t *testing.T) {
	src := "머리말\n\n# 첫째\n\n본문\n\n## 둘째\n\n본문\n\n### 셋째\n"
	res := mdParse(t, "a.md", src)
	if res.Nodes[0].StartLine != 1 {
		t.Fatalf("file node starts at line %d, want 1", res.Nodes[0].StartLine)
	}
	want := map[string]uint32{"첫째": 3, "둘째": 7, "셋째": 11}
	for _, s := range sections(res) {
		if got := s.StartLine; got != want[s.DisplayName] {
			t.Errorf("%q line = %d, want %d", s.DisplayName, got, want[s.DisplayName])
		}
	}
}
