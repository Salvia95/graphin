package wiki

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func queued(t *testing.T, root string, evidence ...string) *Store {
	t.Helper()
	st, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Propose(&Term{
		Canonical:   "posting",
		Description: "A unit of published writing.",
		Body:        "A unit of published writing.",
		Evidence:    evidence,
	}); err != nil {
		t.Fatal(err)
	}
	return st
}

// TestApproveMovesTheCandidateAndRecordsWhoVouched covers the whole operation:
// the file leaves the queue, lands in the glossary, and comes back as a term a
// reader would be served. Status and Reviewed are checked separately because
// they answer different questions — "is this current" and "did a person say
// so" — and the type comments are explicit that conflating them was the
// original mistake.
func TestApproveMovesTheCandidateAndRecordsWhoVouched(t *testing.T) {
	root := t.TempDir()
	st := queued(t, root, "pkg.a", "pkg.b")

	got, err := st.Approve("posting", nil, "human:tipa")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got.Status != StatusStable {
		t.Errorf("status = %q, want %q", got.Status, StatusStable)
	}
	if got.Trust() != TrustHuman {
		t.Errorf("trust = %q, want %q", got.Trust(), TrustHuman)
	}

	if _, err := os.Stat(ProposalPath(root, "posting")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the candidate is still in the queue")
	}
	raw, err := os.ReadFile(GlossaryPath(root, "posting"))
	if err != nil {
		t.Fatalf("glossary entry not written: %v", err)
	}
	back, err := ParseTerm("docs/wiki/glossary/posting.md", raw)
	if err != nil {
		t.Fatalf("what was written does not parse back: %v\n%s", err, raw)
	}
	if back.Status != StatusStable || back.Trust() != TrustHuman {
		t.Errorf("round-trip lost the approval: status=%q trust=%q\n%s", back.Status, back.Trust(), raw)
	}
	if len(back.Evidence) != 2 {
		t.Errorf("evidence = %v, want the two citations from the candidate", back.Evidence)
	}
}

// TestApproveWritesTheWorkingTreeAndNothingElse is the contract the console's
// whole design rests on: approving is a file move because the review is an
// ordinary diff, and what would destroy that is committing on the reviewer's
// behalf rather than moving the file. So the tree must be dirty and HEAD must
// not have moved.
func TestApproveWritesTheWorkingTreeAndNothingElse(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "seed"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "seed")
	st := queued(t, root, "pkg.a", "pkg.b")
	run("add", "-A")
	run("commit", "-qm", "queue the candidate")
	queuedAt := run("rev-parse", "HEAD")

	if _, err := st.Approve("posting", nil, "human:tipa"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if after := run("rev-parse", "HEAD"); after != queuedAt {
		t.Errorf("HEAD moved: %s → %s; approving must not commit", queuedAt, after)
	}
	// -uall so a brand-new directory is listed as its files rather than
	// collapsed to the directory, which is what the move actually produced.
	status := run("status", "--porcelain", "-uall")
	if status == "" {
		t.Error("working tree is clean; the approval left nothing for a person to review")
	}
	if !strings.Contains(status, "glossary/posting.md") || !strings.Contains(status, "propose/posting.md") {
		t.Errorf("the diff does not show the move:\n%s", status)
	}
}

// TestApproveNeedsAPerson guards the one field a machine must not be able to
// mint. Trust reads the human: prefix, so an unattributed approval would
// manufacture human-reviewed trust for nobody.
func TestApproveNeedsAPerson(t *testing.T) {
	root := t.TempDir()
	st := queued(t, root, "pkg.a", "pkg.b")
	for _, by := range []string{"", "tipa", "agent/1.0", "human:", "human:   "} {
		if _, err := st.Approve("posting", nil, by); !errors.Is(err, ErrNotHuman) {
			t.Errorf("Approve(by=%q) = %v, want ErrNotHuman", by, err)
		}
	}
}

// TestApproveRefusesToClobber and the cap test below are the two refusals a
// reviewer can hit for reasons that are not bugs, which is why they are
// distinguishable errors rather than one generic failure.
func TestApproveRefusesToClobber(t *testing.T) {
	root := t.TempDir()
	st := queued(t, root, "pkg.a", "pkg.b")
	mustWrite(t, GlossaryPath(root, "posting"),
		"---\ntype: glossary\ncanonical: posting\n---\n\nAlready here.\n")

	if _, err := st.Approve("posting", nil, "human:tipa"); !errors.Is(err, ErrAlreadyInGlossary) {
		t.Errorf("Approve = %v, want ErrAlreadyInGlossary", err)
	}
	if _, err := os.Stat(ProposalPath(root, "posting")); err != nil {
		t.Error("a refused approval consumed the candidate")
	}
}

func TestApproveRefusesWhenFull(t *testing.T) {
	root := t.TempDir()
	for i := range GlossaryCap {
		mustWrite(t, GlossaryPath(root, fmt.Sprintf("term%02d", i)),
			fmt.Sprintf("---\ntype: glossary\ncanonical: term%02d\n---\n\nFull.\n", i))
	}
	st := queued(t, root, "pkg.a", "pkg.b")
	if _, err := st.Approve("posting", nil, "human:tipa"); !errors.Is(err, ErrGlossaryFull) {
		t.Errorf("Approve = %v, want ErrGlossaryFull", err)
	}
}

// TestEditsCannotRewriteEvidence pins the boundary of what a form may change.
// Evidence is the record of why the candidate cleared admission; if approving
// could rewrite it, the rule demanding two independent citations would be
// satisfiable by typing two.
func TestEditsCannotRewriteEvidence(t *testing.T) {
	root := t.TempDir()
	st := queued(t, root, "pkg.a", "pkg.b")

	got, err := st.Approve("posting", &Term{
		Canonical: "something-else",
		Title:     "Posting",
		Evidence:  []string{"made.up"},
		Status:    StatusDeprecated,
	}, "human:tipa")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if strings.Join(got.Evidence, ",") != "pkg.a,pkg.b" {
		t.Errorf("evidence = %v, want the candidate's own citations", got.Evidence)
	}
	if got.Canonical != "posting" {
		t.Errorf("canonical = %q; identity is not editable", got.Canonical)
	}
	if got.Status != StatusStable {
		t.Errorf("status = %q; the form does not set status", got.Status)
	}
	if got.Title != "Posting" {
		t.Errorf("title = %q, want the edited value", got.Title)
	}
}

func TestDiscardRemovesTheCandidate(t *testing.T) {
	root := t.TempDir()
	st := queued(t, root, "pkg.a", "pkg.b")

	if err := st.Discard("posting"); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(ProposalPath(root, "posting")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the candidate survived discard")
	}
	if _, err := os.Stat(GlossaryPath(root, "posting")); !errors.Is(err, os.ErrNotExist) {
		t.Error("discard published the term")
	}
	if err := st.Discard("posting"); !errors.Is(err, ErrNoProposal) {
		t.Errorf("second discard = %v, want ErrNoProposal", err)
	}
}
