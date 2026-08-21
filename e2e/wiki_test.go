package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/wiki"
)

// wikiWorkspace builds a workspace holding a document and a knowledge set
// that cites two of its sections.
func wikiWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write(t, filepath.Join(root, "docs", "handbook.md"), `# Handbook

## Versioning rules

In 0.x, minor means the user has to fix something.

## Layering rules

Handlers never touch storage directly.
`)
	write(t, filepath.Join(root, wiki.DirName, "sets", "conventions.md"), `---
roles: [backend]
mode: live
---

# Conventions

The standing rules for backend work.

## Rules

- [versioning](../../handbook.md#versioning-rules) — In 0.x, minor means the user has to fix something.
- [layering](../../handbook.md#layering-rules) — Handlers never touch storage directly.
`)

	store, err := wiki.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	pins, problems := wiki.Repin(root, store.SetList())
	if len(problems) != 0 {
		t.Fatalf("fixture does not resolve: %v", problems)
	}
	if err := pins.Save(store.PinsPath()); err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWikiPreflightThenResolve(t *testing.T) {
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	man, isErr := c.tool("wiki_preflight", map[string]any{
		"role": "backend", "task": "add a handler",
	})
	if isErr {
		t.Fatalf("preflight failed: %s", man)
	}
	if !strings.Contains(man, `sets="1"`) {
		t.Fatalf("expected one set:\n%s", man)
	}
	if !strings.Contains(man, `name="conventions"`) {
		t.Fatalf("set not named:\n%s", man)
	}
	// The catalogue must stay a catalogue: no bodies, so a delegation stays
	// cheap and the delegate loads only what it turns out to need.
	if strings.Contains(man, "Handlers never touch storage directly") {
		t.Fatalf("manifest leaked a section body:\n%s", man)
	}
	tok := wiki.FindToken(man)
	if tok == "" {
		t.Fatalf("no token in manifest:\n%s", man)
	}

	body, isErr := c.tool("wiki_resolve", map[string]any{"sets": []string{"conventions"}})
	if isErr {
		t.Fatalf("resolve failed: %s", body)
	}
	if !strings.Contains(body, `returned="2"`) {
		t.Fatalf("expected both sections:\n%s", body)
	}
	if !strings.Contains(body, "Handlers never touch storage directly") {
		t.Fatalf("resolve did not serve content:\n%s", body)
	}
	// Nothing changed since the pins were written, so no entry may claim it
	// did — a spurious drift flag teaches readers to ignore real ones.
	if strings.Contains(body, "drift=") {
		t.Fatalf("unexpected drift:\n%s", body)
	}
}

func TestWikiResolveFlagsDrift(t *testing.T) {
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	// Rewrite one section. Its anchor still resolves, so nothing dangles —
	// but the set's one-line summary may now describe text that is gone.
	write(t, filepath.Join(root, "docs", "handbook.md"), `# Handbook

## Versioning rules

In 0.x, minor means the user has to fix something.

## Layering rules

Handlers may now call storage directly after all.
`)

	body, isErr := c.tool("wiki_resolve", map[string]any{"sets": []string{"conventions"}})
	if isErr {
		t.Fatalf("resolve failed: %s", body)
	}
	if !strings.Contains(body, `drift="changed-since-registration"`) {
		t.Fatalf("drift not reported:\n%s", body)
	}
	// Live mode still serves: the reader can judge, but only with the text.
	if !strings.Contains(body, "Handlers may now call storage directly") {
		t.Fatalf("live mode withheld content:\n%s", body)
	}
	if strings.Count(body, "drift=") != 1 {
		t.Fatalf("only the edited section should be flagged:\n%s", body)
	}
}

func TestWikiPreflightEmptyIsUsable(t *testing.T) {
	// A workspace with no wiki at all: every agent is gated, so this has to
	// be a fast, successful answer that still lets the caller delegate.
	root := t.TempDir()
	write(t, filepath.Join(root, "docs", "readme.md"), "# Readme\n\nNothing here.\n")
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	man, isErr := c.tool("wiki_preflight", map[string]any{"role": "backend", "task": "anything"})
	if isErr {
		t.Fatalf("preflight failed: %s", man)
	}
	if !strings.Contains(man, `sets="0"`) {
		t.Fatalf("expected an empty catalogue:\n%s", man)
	}
	if !strings.Contains(man, "<none>") {
		t.Fatalf("empty catalogue must say what to do next:\n%s", man)
	}
	if wiki.FindToken(man) == "" {
		t.Fatalf("an empty catalogue must still carry a token, or the gate deadlocks:\n%s", man)
	}
}

func TestWikiResolveRequiresAnArgument(t *testing.T) {
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("wiki_resolve", map[string]any{})
	if !isErr {
		t.Fatalf("expected an error, got:\n%s", text)
	}
}

func TestWikiResolveFollowsRenamedHeading(t *testing.T) {
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	if body, _ := c.tool("wiki_resolve", map[string]any{"sets": []string{"conventions"}}); !strings.Contains(body, `returned="2"`) {
		t.Fatalf("premise broken, both sections should resolve first:\n%s", body)
	}

	// Rename a heading without touching its body. The set still links to the
	// old anchor, which is precisely the failure that used to be silent.
	write(t, filepath.Join(root, "docs", "handbook.md"), `# Handbook

## Versioning rules

In 0.x, minor means the user has to fix something.

## Layering rules, restated

Handlers never touch storage directly.
`)

	var body string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		body, _ = c.tool("wiki_resolve", map[string]any{"sets": []string{"conventions"}})
		if strings.Contains(body, "redirected_from=") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(body, `redirected_from="docs/handbook.md#layering-rules"`) {
		t.Fatalf("rename was not followed:\n%s", body)
	}
	// The content is intact and the pin still matches, so nothing drifted —
	// only the link the set records is out of date.
	if !strings.Contains(body, "Handlers never touch storage directly") {
		t.Fatalf("redirected section served no content:\n%s", body)
	}
	if strings.Contains(body, "drift=") {
		t.Fatalf("a pure rename must not read as drift:\n%s", body)
	}
	if !strings.Contains(body, `returned="2"`) {
		t.Fatalf("both sections should still be served:\n%s", body)
	}
}

func TestWikiProposeRejectsAnIdentifier(t *testing.T) {
	root := wikiWorkspace(t)
	// A Go file so the symbol table has something the wiki must refuse.
	write(t, filepath.Join(root, "order.go"), `package main

// OrderService cancels payments.
type OrderService struct{}

func (s *OrderService) Cancel() {}
`)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("wiki_propose", map[string]any{
		"canonical":  "OrderService",
		"definition": "The thing that cancels payments.",
		"evidence":   []string{"docs/handbook.md#versioning-rules", "docs/handbook.md#layering-rules"},
	})
	if isErr {
		t.Fatalf("propose errored: %s", text)
	}
	// The boundary as a function: the index resolves this, so search answers
	// it and a glossary entry would only drift from the code.
	if !strings.Contains(text, `accepted="false"`) || !strings.Contains(text, `rule="identifier"`) {
		t.Fatalf("an indexed identifier was not rejected:\n%s", text)
	}
}

func TestWikiProposeQueuesARealTerm(t *testing.T) {
	root := wikiWorkspace(t)
	write(t, filepath.Join(root, "docs", "other.md"), "# Other\n\n## Posting rules\n\nPostings are units.\n")
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("wiki_propose", map[string]any{
		"canonical":  "posting",
		"definition": "A unit of published writing.",
		"aliases":    []string{"post"},
		"evidence": []string{
			"docs/handbook.md#layering-rules",
			"docs/other.md#posting-rules",
		},
	})
	if isErr {
		t.Fatalf("propose errored: %s", text)
	}
	if !strings.Contains(text, `accepted="true"`) {
		t.Fatalf("candidate not queued:\n%s", text)
	}
	// Filed for review, never published: an agent that can write to the
	// glossary writes the artifact it just built.
	if !strings.Contains(text, "not in the glossary until a person moves it") {
		t.Fatalf("response does not say it is only queued:\n%s", text)
	}
	if _, err := os.Stat(wiki.ProposalPath(root, "posting")); err != nil {
		t.Fatalf("proposal file not written: %v", err)
	}
}

func TestWikiPreflightRecordsACoverageMiss(t *testing.T) {
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	if _, isErr := c.tool("wiki_preflight", map[string]any{"task": "rotate the tls certificates"}); isErr {
		t.Fatal("preflight errored")
	}
	// Misses are the only thing that grows the wiki — there is no
	// retroactive sweep — so they have to be written down where they happen.
	report := wiki.Summarize(wiki.ReadFriction(root))
	if len(report.Misses) != 1 {
		t.Fatalf("misses = %+v", report.Misses)
	}
	if !strings.Contains(report.Misses[0].Task, "tls") {
		t.Fatalf("the task was not recorded: %+v", report.Misses[0])
	}
}

func TestWikiResolveWithNothingNamedClearsTheCaller(t *testing.T) {
	// The gate tells a blocked caller to run wiki_resolve even when preflight
	// matched nothing. Refusing that call would leave them with no legal next
	// move — a block whose own instructions cannot be followed.
	root := wikiWorkspace(t)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("wiki_resolve", map[string]any{})
	if isErr {
		t.Fatalf("naming nothing must be a valid call: %s", text)
	}
	if !strings.Contains(text, "<none>") {
		t.Fatalf("no acknowledgement to act on:\n%s", text)
	}
}

func TestWikiResolveWithNothingNamedWorksBeforeBootstrap(t *testing.T) {
	// Reading no sections needs no index. Requiring bootstrap here deadlocked
	// every workspace whose server had not indexed yet: the gate blocks, and
	// the tool it names refuses.
	root := wikiWorkspace(t)
	c := newClient(t, root)

	text, isErr := c.tool("wiki_resolve", map[string]any{})
	if isErr {
		t.Fatalf("must not require bootstrap when nothing is named: %s", text)
	}
}
