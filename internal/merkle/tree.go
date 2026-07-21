// Package merkle maintains the BLAKE3 hash tree (§2.1.2) and the 2-Track
// diff that decides which work an edit actually requires: offsets always
// refresh (Track A), embeddings and edge extraction run only for subtrees
// whose content hash changed (Track B).
package merkle

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"

	"github.com/zeebo/blake3"

	"github.com/llls2542/graphin/internal/parse"
	"github.com/llls2542/graphin/internal/store"
)

// Sum is the project-wide content hash.
func Sum(b []byte) [32]byte { return blake3.Sum256(b) }

// Hex renders a hash for JSON/logs.
func Hex(h [32]byte) string { return hex.EncodeToString(h[:]) }

// FileEntry records one file's hashes.
type FileEntry struct {
	Hash  string            `json:"hash"`  // whole-file BLAKE3, hex
	Nodes map[string]string `json:"nodes"` // node ID → subtree BLAKE3, hex
}

// Tree is the persisted merkle state (merkle.json). It is owned by the
// single-writer indexer; concurrent mutation is the caller's responsibility.
type Tree struct {
	Files map[string]FileEntry `json:"files"` // relPath → entry
}

func NewTree() *Tree { return &Tree{Files: map[string]FileEntry{}} }

// Load reads merkle.json; a missing file yields an empty tree.
func Load(path string) (*Tree, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewTree(), nil
		}
		return nil, err
	}
	t := NewTree()
	if err := json.Unmarshal(b, t); err != nil {
		return NewTree(), nil // corrupt state → full re-index
	}
	if t.Files == nil {
		t.Files = map[string]FileEntry{}
	}
	return t, nil
}

// Save persists the tree atomically.
func (t *Tree) Save(path string) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, b, 0o644)
}

// FileHash returns the recorded whole-file hash ("" if unknown).
func (t *Tree) FileHash(relPath string) string {
	return t.Files[relPath].Hash
}

// Update records the fresh parse result for one file.
func (t *Tree) Update(res *parse.FileResult) {
	nodes := make(map[string]string, len(res.Nodes))
	for _, n := range res.Nodes {
		nodes[n.ID] = Hex(n.Hash)
	}
	t.Files[res.RelPath] = FileEntry{Hash: Hex(res.FileHash), Nodes: nodes}
}

// Remove drops a deleted file and returns the node IDs that vanished with it.
func (t *Tree) Remove(relPath string) []string {
	e, ok := t.Files[relPath]
	if !ok {
		return nil
	}
	delete(t.Files, relPath)
	ids := make([]string, 0, len(e.Nodes))
	for id := range e.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Root aggregates file hashes into one deterministic workspace hash, used by
// the vector store to detect what changed across a crash (§2.2).
func (t *Tree) Root() [32]byte {
	paths := make([]string, 0, len(t.Files))
	for p := range t.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := blake3.New()
	for _, p := range paths {
		h.WriteString(p)
		h.Write([]byte{0})
		h.WriteString(t.Files[p].Hash)
		h.Write([]byte{0})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}
