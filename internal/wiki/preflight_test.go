package wiki

import (
	"path/filepath"
	"testing"
)

// richWiki builds a workspace with two sets whose vocabulary does not overlap.
func richWiki(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)

	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "release.md"),
		"---\nroles: []\n---\n\n# Release\n\n"+
			"What you need when cutting a release.\n\n"+
			"## Picking the version\n\n"+
			"- [the rule](../../target.md#section-one) — In 0.x, minor means the user has to fix something.\n")

	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "conventions.md"),
		"---\nroles: [backend]\n---\n\n# Conventions\n\n"+
			"Layer rules nobody can detect they are missing.\n\n"+
			"## Layers\n\n"+
			"- [layering](../../target.md#section-one) — Handlers never touch storage directly.\n")

	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSelectPushesRoleTaggedSets(t *testing.T) {
	store := richWiki(t)
	// A convention is something the reader cannot know they are missing, so
	// the role tag decides rather than the task text.
	sel := store.Select("backend", "rename a variable")
	if len(sel.Pushed) != 1 || sel.Pushed[0] != "conventions" {
		t.Fatalf("Pushed = %v", sel.Pushed)
	}
	if len(sel.Sets) != 1 {
		t.Fatalf("Sets = %v", names(sel.Sets))
	}
}

func TestSelectMatchesOnSetName(t *testing.T) {
	store := richWiki(t)
	// "release" naming the release set is not a coincidence.
	sel := store.Select("", "how do I cut a release")
	if len(sel.Matched) != 1 || sel.Matched[0] != "release" {
		t.Fatalf("Matched = %v", sel.Matched)
	}
}

func TestSelectIsConservativeAboutTaskText(t *testing.T) {
	store := richWiki(t)
	// A single shared common word must not attach a set to a task: a
	// manifest that names everything teaches the reader to skip it.
	sel := store.Select("", "the user asked something")
	if !sel.Empty() {
		t.Fatalf("expected no match, got %v", names(sel.Sets))
	}
}

func TestSelectEmptyIsANormalAnswer(t *testing.T) {
	store := richWiki(t)
	sel := store.Select("frontend", "tweak a css colour")
	if !sel.Empty() {
		t.Fatalf("expected empty selection, got %v", names(sel.Sets))
	}
}

func TestManifestCarriesNoBodies(t *testing.T) {
	store := richWiki(t)
	sel := store.Select("backend", "")
	man := store.Manifest(sel, []byte("secret"))

	if len(man.Sets) != 1 {
		t.Fatalf("Sets = %d", len(man.Sets))
	}
	s := man.Sets[0]
	if s.Name != "conventions" || s.Entries != 1 {
		t.Fatalf("set = %+v", s)
	}
	if s.NodeID != "docs/wiki/sets/conventions.md" {
		t.Errorf("NodeID = %q", s.NodeID)
	}
	// The catalogue's whole point is that it is cheap: one line per set, and
	// the reader loads only what it turns out to need.
	if s.Summary != "Layer rules nobody can detect they are missing." {
		t.Errorf("Summary = %q", s.Summary)
	}
	if len(s.Groups) != 1 || s.Groups[0].Title != "Layers" || s.Groups[0].Entries != 1 {
		t.Fatalf("Groups = %+v", s.Groups)
	}
}

func TestManifestExpandsPrerequisites(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "basics", "")
	writeSet(t, root, "advanced", "roles: [backend]\nprerequisites: [basics]\n")
	store, _ := Load(root)

	man := store.Manifest(store.Select("backend", ""), []byte("secret"))
	if len(man.Sets) != 2 {
		t.Fatalf("Sets = %d, want prerequisite pulled in", len(man.Sets))
	}
	// A prerequisite arriving after the set that assumes it has failed.
	if man.Sets[0].Name != "basics" {
		t.Fatalf("order = %s, %s", man.Sets[0].Name, man.Sets[1].Name)
	}
}

func TestFirstSentenceKeepsCatalogueLinesShort(t *testing.T) {
	long := ""
	for len(long) < 400 {
		long += "가나다라마바사아자차카타파하 "
	}
	got := firstSentence(long)
	if len(got) > 200 {
		t.Fatalf("len = %d, want trimmed", len(got))
	}
	// Trimming must never split a rune: the result goes straight into XML.
	for _, r := range got {
		if r == '�' {
			t.Fatal("trim split a UTF-8 rune")
		}
	}
}

// koreanWiki builds a workspace whose set labels are Korean, in a directory
// named after the project — the shape every real wiki here has.
func koreanWiki(t *testing.T) *Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "graphin")
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "release.md"),
		"---\nroles: []\n---\n\n# 릴리스\n\n"+
			"graphin 릴리스를 낼 때 필요한 지식. 버전 자리를 판단한다.\n\n"+
			"## 버전 자리\n\n"+
			"- [규칙](../../target.md#section-one) — minor는 사용자가 손댈 게 생겼다는 뜻이다.\n")
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSelectIgnoresKoreanGrammar(t *testing.T) {
	st := koreanWiki(t)
	// "…는지"와 "…한다"는 모든 한국어 작업 문장에 나온다. 둘이 문턱을 넘으면
	// 어떤 세트든 어떤 작업에나 붙는다.
	sel := st.Select("", "이 화면이 맞는지 확인한다")
	if len(sel.Sets) != 0 {
		t.Errorf("grammar alone matched %v", sel.Matched)
	}
	// 진짜 어휘가 둘이면 여전히 걸린다 — 조사가 붙어 있어도.
	if sel := st.Select("", "버전 자리를 정하고 사용자에게 알린다"); len(sel.Sets) != 1 {
		t.Errorf("real vocabulary stopped matching: %v", sel.Matched)
	}
}

func TestSelectIgnoresTheProjectName(t *testing.T) {
	st := koreanWiki(t)
	// 위키의 모든 세트가 그 프로젝트에 관한 것이므로, 이름을 부르는 것은 어느
	// 세트인지를 말해 주지 않는다.
	sel := st.Select("", "graphin 지식을 정리한다")
	if len(sel.Sets) != 0 {
		t.Errorf("the project name carried a match: %v", sel.Matched)
	}
}
