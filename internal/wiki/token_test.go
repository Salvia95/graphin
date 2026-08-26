package wiki

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenRoundTrip(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "a", "")
	store, _ := Load(root)

	secret, err := LoadOrCreateSecret(root)
	if err != nil {
		t.Fatal(err)
	}
	tok := store.MintToken(secret)
	if !store.VerifyToken(secret, tok) {
		t.Fatal("freshly minted token must verify")
	}
	if store.VerifyToken(secret, "wk1:0000000000000000") {
		t.Fatal("an invented token must not verify")
	}
}

func TestTokenIsMintedForAnEmptyWiki(t *testing.T) {
	// Every agent is gated, so work the wiki has nothing to say about still
	// has to be able to delegate. Withholding the token here would deadlock
	// exactly the tasks that needed no knowledge.
	root := t.TempDir()
	store, _ := Load(root)
	secret, _ := LoadOrCreateSecret(root)

	sel := store.Select("", "anything at all")
	if !sel.Empty() {
		t.Fatal("expected an empty selection")
	}
	man := store.Manifest(sel, secret)
	if man.Token == "" {
		t.Fatal("an empty manifest must still carry a token")
	}
	if !store.VerifyToken(secret, man.Token) {
		t.Fatal("the empty-wiki token must verify")
	}
}

func TestTokenGoesStaleWhenTheWikiChanges(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs", "target.md"), targetDoc)
	writeSet(t, root, "a", "")
	store, _ := Load(root)
	secret, _ := LoadOrCreateSecret(root)
	tok := store.MintToken(secret)

	// Adding an entry changes what a delegation would receive, so a manifest
	// minted before it must stop being accepted. This fails safe: the gate
	// blocks and the caller preflights again.
	mustWrite(t, filepath.Join(root, DirName, setsSubdir, "a.md"),
		"# a\n\n## G\n\n- [x](../../target.md#section-one) — a summary\n"+
			"- [y](../../target.md#section-one) — another\n")
	reloaded, _ := Load(root)
	if reloaded.VerifyToken(secret, tok) {
		t.Fatal("a token minted before the wiki changed must not verify after")
	}
}

func TestSecretIsStableAndPrivate(t *testing.T) {
	root := t.TempDir()
	first, err := LoadOrCreateSecret(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateSecret(root)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("the secret must not be regenerated on every call")
	}
	fi, err := os.Stat(SecretPath(root))
	if err != nil {
		t.Fatal(err)
	}
	// A world-readable secret invites a reader to conclude it was meant to
	// be shared.
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("secret mode = %o, want 600", perm)
	}
}

func TestFindTokenInPrompt(t *testing.T) {
	root := t.TempDir()
	store, _ := Load(root)
	secret, _ := LoadOrCreateSecret(root)
	tok := store.MintToken(secret)

	prompt := "Do the thing.\n\nKnowledge manifest token: " + tok + "\n\nProceed carefully."
	if got := FindToken(prompt); got != tok {
		t.Errorf("FindToken = %q, want %q", got, tok)
	}
	if got := FindToken("no token here"); got != "" {
		t.Errorf("FindToken = %q, want empty", got)
	}
	// A truncated token must not read as a valid one.
	if got := FindToken("wk1:abc"); got != "" {
		t.Errorf("FindToken(truncated) = %q, want empty", got)
	}
}

func TestFingerprintCoversTheGlossary(t *testing.T) {
	root := t.TempDir()
	term := filepath.Join(root, DirName, glossarySubdir, "gate.md")
	mustWrite(t, term, "---\ncanonical: gate\n---\n\n# gate\n\nA checkpoint. It blocks.\n")

	before := fingerprintOf(t, root)

	// A definition is delivered inline in the catalogue, so rewriting one
	// changes what a delegate is told while every set stays put. A token
	// minted before the edit must stop verifying.
	mustWrite(t, term, "---\ncanonical: gate\n---\n\n# gate\n\nA turnstile. It counts.\n")
	if after := fingerprintOf(t, root); after == before {
		t.Fatal("a rewritten definition left the fingerprint unchanged")
	}
}

func TestFingerprintIgnoresProseNobodyReceives(t *testing.T) {
	root := t.TempDir()
	term := filepath.Join(root, DirName, glossarySubdir, "gate.md")
	mustWrite(t, term, "---\ncanonical: gate\n---\n\n# gate\n\nA checkpoint. It blocks.\n")

	before := fingerprintOf(t, root)

	// Only the first sentence reaches a catalogue. Churning tokens over a
	// paragraph no delegate was ever handed would be cost with no reader.
	mustWrite(t, term, "---\ncanonical: gate\n---\n\n# gate\n\nA checkpoint. It blocks.\n\nA later paragraph.\n")
	if after := fingerprintOf(t, root); after != before {
		t.Error("prose outside the delivered definition invalidated outstanding tokens")
	}
}

func TestFingerprintIsStableAcrossCalls(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"alpha", "beta", "gamma", "delta"} {
		mustWrite(t, filepath.Join(root, DirName, glossarySubdir, n+".md"),
			"---\ncanonical: "+n+"\n---\n\n# "+n+"\n\nA thing.\n")
	}
	// Terms live in a map. Walking it unordered would make an unchanged wiki
	// sign differently each call, which reads at the gate as "the wiki
	// changed" and blocks a delegation that did nothing wrong.
	first := fingerprintOf(t, root)
	for i := 0; i < 20; i++ {
		if got := fingerprintOf(t, root); got != first {
			t.Fatalf("fingerprint is not stable: call %d gave %s, want %s", i, got, first)
		}
	}
}

// fingerprintOf loads the wiki fresh each time, so a caller can compare two
// states of the same directory.
func fingerprintOf(t *testing.T, root string) string {
	t.Helper()
	st, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return st.Fingerprint()
}
