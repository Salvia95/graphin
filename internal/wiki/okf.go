package wiki

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OKFVersion is the Open Knowledge Format revision this exporter targets.
const OKFVersion = "0.2"

// ExportOKF writes the wiki as an Open Knowledge Format bundle.
//
// Exporting rather than adopting is settled, not deferred. OKF identity is a
// file path; this system addresses a heading inside a document, and measuring
// what that is worth in this repository decided it:
//
//   - a cited section runs ~1.5% of the file holding it, so file-scoped
//     reading costs ~65x the bytes for the same answer;
//   - 31 curated entries resolve to 4 distinct files, so file-scoped identity
//     collapses 31 decisions into 4 and turns the catalogue back into a table
//     of contents;
//   - over 30 days one of those files changed 16 times while the section a set
//     cites changed once, so a file-scoped pin would raise 15 false drift
//     alarms for every true one — which is how a drift flag stops being read.
//
// The last one is decisive: this system already refuses to report a renamed
// heading as drift for exactly that reason.
//
// So the authored form keeps section identity and this writes a projection
// beside it. It costs nothing until someone wants to consume one.
//
// Nothing here parses YAML; it only writes it. That is why this needs no
// dependency the authoring parser deliberately does without.
func (st *Store) ExportOKF(dir string) ([]string, error) {
	usage := Summarize(ReadFriction(st.Root))
	var written []string

	emit := func(rel, body string) error {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	}

	names := make([]string, 0, len(st.Terms))
	for n := range st.Terms {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := emit("glossary/"+safeName(n)+".md", st.termConcept(st.Terms[n])); err != nil {
			return nil, err
		}
	}
	for _, s := range st.SetList() {
		if err := emit("sets/"+safeName(s.Name)+".md", st.setConcept(s, usage)); err != nil {
			return nil, err
		}
	}
	// An index per directory, which is what published bundles do. The spec
	// only requires the root one to be possible; a consumer walking a
	// subdirectory with no index has to guess what is in it.
	if len(st.Terms) > 0 {
		if err := emit("glossary/index.md", st.dirIndex("Glossary", st.termLines())); err != nil {
			return nil, err
		}
	}
	if len(st.Sets) > 0 {
		if err := emit("sets/index.md", st.dirIndex("Knowledge Sets", st.setLines())); err != nil {
			return nil, err
		}
	}
	if err := emit("index.md", st.bundleIndex()); err != nil {
		return nil, err
	}
	return written, nil
}

// dirIndex renders one directory listing. Index files carry no frontmatter —
// the bundle root is the sole exception, and only for the version.
func (st *Store) dirIndex(heading string, lines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", heading)
	for _, l := range lines {
		b.WriteString(l)
	}
	return b.String()
}

// termLines and setLines are shared by the root index and the directory ones,
// so a rename cannot make the two disagree about what is in the bundle.
func (st *Store) termLines() []string {
	names := make([]string, 0, len(st.Terms))
	for n := range st.Terms {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		t := st.Terms[n]
		out = append(out, fmt.Sprintf("* [%s](%s.md) - %s\n",
			t.Title, safeName(n), firstSentence(descriptionOf(t))))
	}
	return out
}

func (st *Store) setLines() []string {
	sets := st.SetList()
	out := make([]string, 0, len(sets))
	for _, s := range sets {
		out = append(out, fmt.Sprintf("* [%s](%s.md) - %s\n",
			setTitle(s), safeName(s.Name), firstSentence(s.Summary())))
	}
	return out
}

// termConcept renders one glossary entry as an OKF concept.
func (st *Store) termConcept(t *Term) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: Glossary Term\n")
	yamlField(&b, "title", t.Title)
	// The underlying asset is the authored file: that is the thing this
	// concept is a rendering of, and the path is what still finds it in the
	// repository the bundle was produced from.
	yamlField(&b, "resource", t.RelPath)
	yamlField(&b, "description", firstSentence(descriptionOf(t)))
	yamlList(&b, "tags", t.Tags)
	yamlField(&b, "status", string(t.Status))
	yamlField(&b, "stale_after", t.StaleAfter)

	// verified, in the shape the spec asks for. The authored file carries
	// this as flat "actor — date" strings under a key of our own precisely so
	// that this translation happens once, here, instead of every consumer
	// meeting a wrongly-typed field.
	if len(t.Reviewed) > 0 {
		b.WriteString("verified:\n")
		for _, r := range t.Reviewed {
			fmt.Fprintf(&b, "  - by: %s\n", yamlScalar(r.By))
			if r.At != "" {
				fmt.Fprintf(&b, "    at: %s\n", yamlScalar(r.At))
			}
		}
	}
	if len(t.Evidence) > 0 {
		b.WriteString("sources:\n")
		for _, e := range t.Evidence {
			file, anchor, _ := strings.Cut(e, "#")
			fmt.Fprintf(&b, "  - resource: %s\n", yamlScalar(file))
			if anchor != "" {
				fmt.Fprintf(&b, "    graphin_node: %s\n", yamlScalar(e))
			}
		}
	}

	// Extension keys. The spec makes these first class — producers may add
	// any key and consumers must not reject or discard them — so the parts of
	// this vocabulary OKF has no field for travel with the bundle instead of
	// being dropped at the door.
	yamlList(&b, "graphin_aliases", t.Aliases)
	yamlList(&b, "graphin_scope", t.Scope)
	yamlField(&b, "graphin_derives_from", t.DerivesFrom)
	if len(t.Confusions) > 0 {
		b.WriteString("graphin_not_to_be_confused_with:\n")
		for _, c := range t.Confusions {
			fmt.Fprintf(&b, "  - term: %s\n", yamlScalar(c.Term))
			fmt.Fprintf(&b, "    why: %s\n", yamlScalar(c.Why))
		}
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", t.Title)
	b.WriteString(strings.TrimSpace(t.Body))
	b.WriteString("\n")
	if len(t.Confusions) > 0 {
		b.WriteString("\n## Not to be confused with\n\n")
		for _, c := range t.Confusions {
			fmt.Fprintf(&b, "- **%s** — %s\n", c.Term, c.Why)
		}
	}
	return b.String()
}

// setConcept renders one knowledge set as an OKF concept.
func (st *Store) setConcept(s *Set, usage FrictionReport) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: Knowledge Set\n")
	yamlField(&b, "title", setTitle(s))
	yamlField(&b, "resource", s.RelPath)
	yamlField(&b, "description", firstSentence(s.Summary()))
	yamlList(&b, "tags", s.Tags)
	// Sets have no status field of their own; the OKF default is stable and
	// declaring it here would only be a place for the two to disagree later.
	yamlField(&b, "stale_after", s.StaleAfter)

	// The sections this set is made of.
	//
	// `resource` names the FILE, because that is the only thing an OKF
	// consumer can resolve — its concept identity is a path, with no notion
	// of an address inside a document. The heading this entry actually means
	// goes in graphin_node beside it.
	//
	// Putting the full node id in `resource` would have been quietly wrong in
	// the direction that matters: a consumer would read it as the whole file
	// and never learn that the entry meant 1.5% of it. Saying less in the
	// standard field and saying the rest in our own is the honest shape.
	entries := s.Entries()
	if len(entries) > 0 {
		b.WriteString("sources:\n")
		for _, e := range entries {
			file, anchor, _ := strings.Cut(e.NodeID, "#")
			fmt.Fprintf(&b, "  - resource: %s\n", yamlScalar(file))
			fmt.Fprintf(&b, "    title: %s\n", yamlScalar(e.Title))
			if anchor != "" {
				fmt.Fprintf(&b, "    graphin_node: %s\n", yamlScalar(e.NodeID))
			}
			if pin, ok := st.Pins.Get(s.Name, e.NodeID); ok {
				// Integrity travels with the bundle. OKF has no content
				// hash — its freshness signal is a declared date — so
				// dropping these at export would hand a consumer knowledge
				// with no way to tell whether it still matches its source.
				fmt.Fprintf(&b, "    graphin_hash: %s\n", yamlScalar(pin.Hash))
				if pin.Rename != "" {
					fmt.Fprintf(&b, "    graphin_rename_key: %s\n", yamlScalar(pin.Rename))
				}
			}
		}
	}
	if n := usage.Resolved[s.Name]; n > 0 {
		fmt.Fprintf(&b, "graphin_resolve_count: %d\n", n)
	}
	yamlList(&b, "graphin_roles", s.Roles)
	yamlList(&b, "graphin_prerequisites", s.Prerequisites)
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", setTitle(s))
	if s.Summary() != "" {
		b.WriteString(s.Summary())
		b.WriteString("\n")
	}
	for _, g := range s.Groups {
		if g.Title != "" {
			fmt.Fprintf(&b, "\n## %s\n\n", g.Title)
		} else {
			b.WriteString("\n")
		}
		for _, e := range g.Entries {
			fmt.Fprintf(&b, "* [%s](%s) - %s\n", e.Title, e.NodeID, e.Summary)
		}
	}
	return b.String()
}

// bundleIndex is the root listing. It is the only index file that may carry
// frontmatter, and the only place the format version is declared.
func (st *Store) bundleIndex() string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nokf_version: %q\n---\n\n", OKFVersion)
	b.WriteString("# Knowledge\n\n")
	b.WriteString("Exported from a graphin wiki.\n\n" +
		"**This bundle is a projection, not the source.** The knowledge here is scoped to\n" +
		"individual headings inside the documents it cites — a set entry typically means\n" +
		"one or two percent of the file it points at. The Open Knowledge Format addresses\n" +
		"whole files, so each `sources[].resource` names the file and `graphin_node`\n" +
		"beside it carries the address that was actually meant, in the form\n" +
		"`path/to/file.md#heading-slug`. Ignore `graphin_node` and you get the right\n" +
		"documents; use it and you get the right paragraphs.\n\n" +
		"The cited documents live in the source repository, not in this bundle.\n")

	b.WriteString("\n# Subdirectories\n\n")
	if len(st.Terms) > 0 {
		b.WriteString("* [glossary](glossary/index.md) - Words this project uses precisely.\n")
	}
	if len(st.Sets) > 0 {
		b.WriteString("* [sets](sets/index.md) - Curated reading lists over the repository's own documents.\n")
	}
	return b.String()
}

func setTitle(s *Set) string {
	if s.Title != "" {
		return s.Title
	}
	return s.Name
}

func descriptionOf(t *Term) string {
	if t.Description != "" {
		return t.Description
	}
	return t.Body
}

func yamlField(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s: %s\n", key, yamlScalar(value))
}

func yamlList(b *strings.Builder, key string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", key)
	for _, it := range items {
		fmt.Fprintf(b, "  - %s\n", yamlScalar(it))
	}
}

// yamlScalar quotes a value unless it is unambiguously safe bare.
//
// This writer has no YAML library behind it, so it errs toward quoting: a
// double-quoted scalar with backslashes and quotes escaped is valid YAML for
// any content, and the cost of quoting something that did not need it is a
// pair of quotes.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return `""`
	}
	safe := true
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == ' ' && i > 0 && i < len(s)-1:
		case r == '-' || r == '_' || r == '.' || r == '/':
		default:
			safe = false
		}
		if !safe {
			break
		}
	}
	// A bare scalar that YAML would read as something other than a string —
	// a date, a bool, a number — has to be quoted even though its characters
	// are safe, or a consumer gets a different type than was written.
	if safe && !looksTyped(s) {
		return s
	}
	esc := strings.ReplaceAll(s, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return `"` + esc + `"`
}

// looksTyped reports whether a bare scalar would be read as a non-string.
func looksTyped(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return true
	}
	digits, dashes := 0, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '-' || r == '.' || r == '+':
			dashes++
		default:
			return false
		}
	}
	return digits > 0 && digits+dashes == len(s)
}
