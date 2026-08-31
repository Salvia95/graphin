package lexical

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"

	"github.com/Salvia95/graphin/internal/store"
)

// Bumped to 2 when the index began stemming its terms: a v1 snapshot holds
// raw tokens, and restoring those beside a stemmed query would answer every
// inflected question with nothing.
const snapshotVersion = 2

type snapshot struct {
	Version int
	Docs    map[string][]string // docID → tokens
	Simple  map[string]string   // docID → simple name
}

// Save persists the index and symbol table to path atomically (lexical.idx).
func Save(path string, ix *Index, st *SymbolTable) error {
	snap := snapshot{Version: snapshotVersion, Docs: ix.Snapshot(), Simple: st.Snapshot()}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&snap); err != nil {
		return err
	}
	return store.WriteFileAtomic(path, buf.Bytes(), 0o644)
}

// Load restores an index and symbol table from path. A missing file or a
// stale snapshot version yields empty structures (callers re-index).
func Load(path string) (*Index, *SymbolTable, error) {
	ix, st := NewIndex(), NewSymbolTable()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ix, st, nil
		}
		return nil, nil, err
	}
	var snap snapshot
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&snap); err != nil {
		return nil, nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if snap.Version != snapshotVersion {
		return ix, st, nil
	}
	for id, toks := range snap.Docs {
		ix.restore(id, toks)
	}
	for id, simple := range snap.Simple {
		st.Put(id, simple)
	}
	return ix, st, nil
}
