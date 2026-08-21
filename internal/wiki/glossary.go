package wiki

import (
	pathpkg "path"
	"strings"
)

// pairSeparators are the dashes an author may use to split "thing — reason".
// The em-dash is the one the documents use; the others are accepted because
// rejecting a line over its dash would be a format that fights its authors.
var pairSeparators = []string{" — ", " – ", " -- ", " - "}

// splitPair divides "left — right" into its halves. A line with no separator
// is all left, which keeps a bare term usable before someone writes the why.
func splitPair(s string) (left, right string) {
	for _, sep := range pairSeparators {
		if i := strings.Index(s, sep); i >= 0 {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])
		}
	}
	return strings.TrimSpace(s), ""
}

// ParseTerm reads one glossary page.
//
// The canonical name defaults to the filename, not to the first heading: the
// file is the identity of the term everywhere else in the system, and a page
// whose heading and filename disagree should keep resolving by the name other
// pages link to.
func ParseTerm(relPath string, src []byte) (*Term, error) {
	fm, body := splitFrontmatter(src)
	front := parseFrontmatter(fm)

	t := &Term{
		Canonical:    front.Get("canonical"),
		RelPath:      relPath,
		Title:        front.Get("title"),
		Description:  front.Get("description"),
		Tags:         front.List("tags"),
		StaleAfter:   front.Get("stale_after"),
		Aliases:      front.List("aliases"),
		DerivesFrom:  front.Get("derives_from"),
		Scope:        front.List("scope"),
		Evidence:     front.List("evidence"),
		Status:       Status(front.Get("status")),
		LastVerified: front.Get("last_verified"),
		Body:         strings.TrimSpace(string(body)),
	}
	if t.Canonical == "" {
		t.Canonical = strings.TrimSuffix(pathpkg.Base(relPath), ".md")
	}
	if t.Title == "" {
		t.Title = t.Canonical
	}
	// Stable is the default because most entries are: a file that exists and
	// says nothing about its own status is being used, not drafted.
	if t.Status == "" {
		t.Status = StatusStable
	}
	// `reviewed`, not `verified`. The Open Knowledge Format spells this field
	// as a list of {by, at} mappings, and this parser reads flat lists only —
	// writing strings under their key would hand a conforming consumer the
	// wrong type. An extra key of our own is explicitly tolerated by the
	// spec, and the exporter translates it into the real shape.
	for _, raw := range front.List("reviewed") {
		by, at := splitPair(raw)
		if by != "" {
			t.Reviewed = append(t.Reviewed, Review{By: by, At: at})
		}
	}
	for _, raw := range front.List("not_to_be_confused_with") {
		term, why := splitPair(raw)
		if term != "" {
			t.Confusions = append(t.Confusions, Confusion{Term: term, Why: why})
		}
	}
	return t, nil
}

// Matches reports whether a word refers to this term. Aliases are matched
// case-insensitively; a derived form is not matched here because it has its
// own page pointing back at this one.
func (t *Term) Matches(word string) bool {
	if strings.EqualFold(word, t.Canonical) {
		return true
	}
	for _, a := range t.Aliases {
		if strings.EqualFold(word, a) {
			return true
		}
	}
	return false
}

// SplitConfusion parses one "other term — why they differ" line.
func SplitConfusion(s string) Confusion {
	term, why := splitPair(s)
	return Confusion{Term: term, Why: why}
}
