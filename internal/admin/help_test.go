package admin

import (
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// termCall matches a {{term "key" "라벨"}} call in a template.
var termCall = regexp.MustCompile(`\{\{\s*term\s+"([^"]+)"`)

// TestTemplateTermKeysExist is the typo guard: termHelp degrades an unknown
// key to a bare label, so a misspelled key would silently drop the ⓘ button
// instead of failing. Catch it here rather than in the browser.
func TestTemplateTermKeysExist(t *testing.T) {
	files, err := fs.Glob(assets, "templates/*/*.html")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range files {
		body, err := fs.ReadFile(assets, f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range termCall.FindAllStringSubmatch(string(body), -1) {
			seen++
			if _, ok := helpTerms[m[1]]; !ok {
				t.Errorf("%s: term %q is not in helpTerms", f, m[1])
			}
		}
	}
	if seen == 0 {
		t.Fatal("no {{term}} calls found — the help affordance vanished from the templates")
	}
}

// TestHelpRouteServesEveryTerm walks the dictionary so a term that is defined
// but unreachable (or renders to an error) fails here.
func TestHelpRouteServesEveryTerm(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	for key, term := range helpTerms {
		rec := get(t, s, "/help/"+key)
		if rec.Code != http.StatusOK {
			t.Errorf("/help/%s = %d", key, rec.Code)
			continue
		}
		body := rec.Body.String()
		if !strings.Contains(body, term.Title) {
			t.Errorf("/help/%s missing title %q", key, term.Title)
		}
		if !strings.Contains(body, `class="popover-title"`) {
			t.Errorf("/help/%s missing popover-title wrapper", key)
		}
	}
}

func TestHelpUnknownTermIs404(t *testing.T) {
	s := newTestServer(t, newTestWS(t, nil))
	if rec := get(t, s, "/help/no-such-term"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown term = %d, want 404", rec.Code)
	}
}

// TestTermHelpEscapes keeps the hand-built HTML in termHelp from becoming an
// injection point if a label ever carries user data.
func TestTermHelpEscapes(t *testing.T) {
	got := string(termHelp("dangling", `<script>x</script>`))
	if strings.Contains(got, "<script>") {
		t.Fatalf("label not escaped: %s", got)
	}
	if !strings.Contains(got, `hx-get="/help/dangling"`) {
		t.Fatalf("missing htmx trigger: %s", got)
	}
	// 사전에 없는 키는 버튼 없이 라벨만 남는다.
	if got := string(termHelp("nope", "라벨")); got != "라벨" {
		t.Fatalf("unknown key should degrade to bare label, got %s", got)
	}
}
