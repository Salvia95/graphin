package wiki

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeSubdir holds per-workspace wiki state that is generated, not
// authored: the signing secret and the gate's flag files.
//
// The ".graphin" element is workspace.DataDirName. It is spelled out here
// rather than imported because this package must stay reachable from a
// process that never opens the index — the same reason the plugin's shell
// hooks spell it out too.
const RuntimeSubdir = ".graphin/wiki"

// tokenPrefix versions the token format. A token minted by an older graphin
// should fail closed rather than be reinterpreted.
const tokenPrefix = "wk1:"

// tokenHexLen is how much of the MAC the token carries. 64 bits is far more
// than guessing needs to be hopeless, and the token has to sit inside a
// delegation prompt a person may read.
const tokenHexLen = 16

// SecretPath is where the workspace's signing secret lives.
func SecretPath(root string) string {
	return filepath.Join(root, filepath.FromSlash(RuntimeSubdir), "secret")
}

// LoadOrCreateSecret returns the workspace's signing secret, creating it on
// first use.
//
// The secret is not a security boundary and must not be described as one: an
// agent that can run shell commands can read the file. What it defends
// against is the failure that actually happens — a model that has seen the
// shape of a manifest marker writing a plausible one without ever having run
// preflight. Making the marker unforgeable-by-invention is the whole job.
func LoadOrCreateSecret(root string) ([]byte, error) {
	path := SecretPath(root)
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		return b, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	// 0600: no other user needs it, and a world-readable secret invites a
	// reader to conclude it was meant to be shared.
	if err := os.WriteFile(path, secret, 0o600); err != nil {
		return nil, err
	}
	return secret, nil
}

// Fingerprint identifies the state of the whole wiki: every set, the entries
// it holds right now, and every glossary entry.
//
// The whole wiki, and not the selected subset, for a reason that only shows
// up at the other end. A gate reading a delegation prompt can recover the
// token and nothing else — it does not know which sets the preflight chose,
// so a signature over the selection would be one it can never recompute.
// Signing what both sides can see independently is what makes the token
// checkable at all.
//
// It covers what a delegate would actually receive — names, modes, roles,
// prerequisites, and every entry's id, title and summary — rather than the
// node ids alone. A rewritten summary changes what a reader decides from
// while every id stays put, so ids alone would call that unchanged.
//
// The cost is churn: any wiki edit invalidates outstanding tokens. That
// fails in the safe direction — a stale token blocks and the caller runs
// preflight again.
func (st *Store) Fingerprint() string {
	h := sha256.New()
	field := func(parts ...string) {
		for _, p := range parts {
			h.Write([]byte(p))
			h.Write([]byte{0})
		}
		h.Write([]byte{1})
	}
	for _, s := range st.SetList() {
		field(s.Name, s.Title, string(s.Mode), s.StaleAfter)
		field(s.Roles...)
		field(s.Prerequisites...)
		field(s.Tags...)
		// Aliases decide which tasks reach the set, so an edit to them changes
		// what a preflight would have answered. Tags are signed without being
		// delivered; aliases at least do work.
		field(s.Aliases...)
		// Provenance and the review flag: both travel with what is served,
		// so flipping either changes what a reader was told.
		field(s.Origin, reviewedField(s))
		// Summary, not Intro: a declared description overrides the opening
		// prose in the catalogue, so hashing Intro alone would let an edit
		// change what a delegate sees while its token still verified.
		field(s.Summary())
		for _, g := range s.Groups {
			field(g.Title)
			for _, e := range g.Entries {
				// Summaries are in here because they are what a manifest
				// shows and what a reader decides on. Hashing node IDs
				// alone would let a rewritten summary — or a second entry
				// citing a section already listed — pass as unchanged.
				field(e.NodeID, e.Title, e.Summary)
			}
		}
	}
	// A marker between the halves. Without it a set could in principle be
	// renamed into the encoding of a glossary entry and leave the digest
	// unchanged.
	field("glossary")
	for _, t := range st.TermList() {
		// Definitions arrive inline in the catalogue — unlike a set, whose
		// sections are fetched fresh at resolve time — so an edited
		// definition changes what a delegate was told while every set stays
		// put. Signing only the sets called that unchanged.
		//
		// What is signed is what is delivered, and only that: manifestTerm
		// is the same reduction Manifest uses, so a paragraph nobody ever
		// receives cannot invalidate a token.
		mt := manifestTerm(t)
		field(mt.Canonical, mt.Definition)
		field(mt.Aliases...)
		for _, c := range mt.Confusions {
			field(c.Term, c.Why)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// reviewedField renders the review state the way the frontmatter says it.
func reviewedField(s *Set) string {
	if s.Unreviewed {
		return "false"
	}
	return ""
}

// MintToken signs the wiki's current state so a gate can tell a real
// preflight from a remembered one.
//
// A token is minted even when nothing matched. That case is not an error and
// must not be one: every agent is gated, so a task the wiki has nothing to
// say about still has to be able to delegate. Withholding the token there
// would deadlock exactly the work that needed no knowledge.
func (st *Store) MintToken(secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(tokenPrefix))
	mac.Write([]byte(st.Fingerprint()))
	return tokenPrefix + hex.EncodeToString(mac.Sum(nil))[:tokenHexLen]
}

// VerifyToken reports whether tok was minted against the wiki as it stands.
//
// Note what this does NOT establish: that the sets in the manifest are the
// right ones for the task. A token proves a preflight ran and that the wiki
// has not changed since. Matching knowledge to work is the manifest's job;
// the signature only rules out a manifest that was invented.
func (st *Store) VerifyToken(secret []byte, tok string) bool {
	want := st.MintToken(secret)
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(tok)))
}

// FindToken extracts a manifest token from free text, such as a delegation
// prompt. It returns "" when the text carries none.
func FindToken(text string) string {
	i := strings.Index(text, tokenPrefix)
	if i < 0 {
		return ""
	}
	rest := text[i+len(tokenPrefix):]
	n := 0
	for n < len(rest) && n < tokenHexLen && isHex(rest[n]) {
		n++
	}
	if n < tokenHexLen {
		return ""
	}
	return tokenPrefix + rest[:tokenHexLen]
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}
