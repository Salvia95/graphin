package wiki

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Errors the two displacement operations can fail with for reasons that are
// not bugs.
var (
	// ErrNoTerm means the glossary has no entry under that name.
	ErrNoTerm = errors.New("no such term in the glossary")
	// ErrNoSet means no set file is named that.
	ErrNoSet = errors.New("no such set")
)

// RetireTerm removes a glossary entry, freeing a slot under the cap.
//
// # Why it deletes rather than archives
//
// Deprecated is not retirement: a deprecated term is still served, on purpose,
// because a reader arriving with the old word needs to be told it is the old
// word. So there is no status that frees a slot, and an "archive" directory
// would be a third place with its own rules — not counted, not served, not
// read by anything — which is a graveyard nobody prunes.
//
// Deleting is the same answer Discard gives for a candidate, for the same
// reason: a term that mattered will come back from real evidence, and the
// deletion is an ordinary line in a diff that `git checkout` undoes. Like every
// other write here it stops at the working tree.
func RetireTerm(root, canonical string) (string, error) {
	path := GlossaryPath(root, canonical)
	rel := pathpkg.Join(DirName, glossarySubdir, safeName(canonical)+".md")
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return rel, ErrNoTerm
	}
	return rel, err
}

// SetFrontEdits are the two frontmatter fields a reader can change from the
// console. Both are nil-able: nil leaves the field exactly as authored, which
// is not the same as setting it to empty.
type SetFrontEdits struct {
	// Description is the catalogue line — the one sentence that decides
	// whether anyone opens the set. Rewriting it is the whole answer to a set
	// that is offered and never opened.
	Description *string `json:"description,omitempty"`
	// Roles is the push list. Emptying it demotes the set from every
	// delegation of that role down to task matching, which is what "demote"
	// means here.
	Roles *[]string `json:"roles,omitempty"`
}

// EditSetFront rewrites those fields in place and touches nothing else.
//
// Surgical rather than a re-render, and that is the whole design of it. A set
// file is hand-authored prose and links; running it through a writer would
// reformat every line an author chose, and the property this console rests on
// is that the review is an ordinary diff. A diff of one line is reviewable. A
// diff of the whole file is a rewrite nobody can check.
func EditSetFront(root, name string, edits SetFrontEdits) (string, error) {
	store, err := Load(root)
	if err != nil {
		return "", err
	}
	var target *Set
	for _, s := range store.SetList() {
		if s.Name == name {
			target = s
		}
	}
	if target == nil {
		return "", fmt.Errorf("%w: %s", ErrNoSet, name)
	}

	path := filepath.Join(root, filepath.FromSlash(target.RelPath))
	src, err := os.ReadFile(path)
	if err != nil {
		return target.RelPath, err
	}
	fm, body := splitFrontmatter(src)
	if fm == nil {
		// No header to edit. Writing one would put a block above prose the
		// author never framed that way, so this reports instead.
		return target.RelPath, fmt.Errorf("%s has no frontmatter to edit", target.RelPath)
	}

	lines := strings.Split(strings.TrimRight(string(fm), "\n"), "\n")
	if edits.Description != nil {
		lines = setScalar(lines, "description", *edits.Description)
	}
	if edits.Roles != nil {
		lines = setList(lines, "roles", *edits.Roles)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.WriteString(strings.Join(lines, "\n"))
	out.WriteString("\n---\n")
	out.Write(body)
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return target.RelPath, err
	}
	return target.RelPath, nil
}

// keyAt reports whether a frontmatter line opens the given key.
func keyAt(line, key string) bool {
	return strings.HasPrefix(line, key+":") &&
		(len(line) == len(key)+1 || line[len(key)+1] == ' ' || line[len(key)+1] == '\r')
}

// spanOf finds the key's line and everything that belongs to it — a block list
// continues in `  - item` lines until something at column zero ends it.
func spanOf(lines []string, key string) (start, end int) {
	for i, l := range lines {
		if !keyAt(l, key) {
			continue
		}
		j := i + 1
		for j < len(lines) {
			t := strings.TrimRight(lines[j], "\r")
			if strings.HasPrefix(t, "-") || strings.HasPrefix(t, " ") || strings.HasPrefix(t, "\t") {
				j++
				continue
			}
			break
		}
		return i, j
	}
	return -1, -1
}

func setScalar(lines []string, key, value string) []string {
	repl := []string{key + ": " + value}
	start, end := spanOf(lines, key)
	if start < 0 {
		return append(lines, repl...)
	}
	return append(lines[:start:start], append(repl, lines[end:]...)...)
}

func setList(lines []string, key string, items []string) []string {
	start, end := spanOf(lines, key)
	// Keep the shape the author used. `tags: [a, b]` and the block form parse
	// the same, and turning a one-line key into three lines makes a one-key
	// change look like a rewrite of the header — which is the only thing this
	// surgical writer exists to avoid.
	inline := len(items) == 0
	if start >= 0 && end == start+1 {
		if v := strings.TrimSpace(strings.TrimPrefix(lines[start], key+":")); strings.HasPrefix(v, "[") {
			inline = true
		}
	}
	var repl []string
	switch {
	case inline:
		repl = []string{key + ": [" + strings.Join(items, ", ") + "]"}
	default:
		repl = []string{key + ":"}
		for _, it := range items {
			repl = append(repl, "  - "+it)
		}
	}
	if start < 0 {
		return append(lines, repl...)
	}
	return append(lines[:start:start], append(repl, lines[end:]...)...)
}
