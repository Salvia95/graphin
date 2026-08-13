package e2e

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// diagnose_index is the only window onto index health now that the admin page
// is gone, so it has to answer before bootstrap too — that is exactly when
// something is wrong. The other tools refuse with NOT_BOOTSTRAPPED here.
func TestDiagnoseIndexBeforeBootstrap(t *testing.T) {
	root := t.TempDir()
	c := newClient(t, root)

	text, isErr := c.tool("diagnose_index", map[string]any{})
	if isErr {
		t.Fatalf("diagnose_index reported an error before bootstrap:\n%s", text)
	}
	for _, want := range []string{
		`<system_status state="not_bootstrapped"`, // status prefix still leads
		`bootstrapped="false"`,
		`<graph nodes="0" edges="0" shards="0" />`,
		`workspace="` + root + `"`,
		`<storage path=`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// Nothing is indexed, so there is nothing to warn about.
	if strings.Contains(text, "<hint>") {
		t.Errorf("empty workspace produced a hint:\n%s", text)
	}
}

func TestDiagnoseIndexAfterBootstrap(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("diagnose_index", map[string]any{})
	if isErr {
		t.Fatalf("diagnose_index failed:\n%s", text)
	}
	for _, want := range []string{
		`bootstrapped="true"`,
		`<dangling code=`,
		`<partial count=`,
		`<reverse targets=`,
		`<semantic ready=`,
		`<storage path=`,
		// OrtLib is overridden by the harness, so the changed marker must fire.
		`ort_lib_changed="true"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}

	nodes := attrInt(t, text, "nodes")
	if nodes == 0 {
		t.Errorf("indexed workspace reports zero nodes:\n%s", text)
	}
	if edges := attrInt(t, text, "edges"); edges == 0 {
		t.Errorf("indexed workspace reports zero edges:\n%s", text)
	}
	if total := attrInt(t, text, "total_bytes"); total == 0 {
		t.Errorf(".graphin exists but storage totals zero:\n%s", text)
	}
	// The java fixtures carry a deliberately broken file, so partial detection
	// and its hint both have to fire — this is the signal the admin page's
	// diagnostics tab used to be the only way to see.
	if got := attrInt(t, text, "count"); got == 0 {
		t.Errorf("broken fixture not reported as partial:\n%s", text)
	}
	if !strings.Contains(text, "broken/Broken.java") {
		t.Errorf("partial sample does not name the broken file:\n%s", text)
	}
	if !strings.Contains(text, "<hint>") || !strings.Contains(text, "syntax errors") {
		t.Errorf("partial nodes did not produce an actionable hint:\n%s", text)
	}
}

// attrInt pulls the first `name="<digits>"` attribute out of an XML response.
func attrInt(t *testing.T, xml, name string) int {
	t.Helper()
	m := regexp.MustCompile(name + `="(\d+)"`).FindStringSubmatch(xml)
	if m == nil {
		t.Fatalf("attribute %q not found in:\n%s", name, xml)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return n
}
