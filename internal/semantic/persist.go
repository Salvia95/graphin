package semantic

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/llls2542/graphin/internal/store"
)

// vectors.bin framing (§2.2): [magic][u32 hdrLen][header JSON][chromem
// gzip export stream]. The header records the merkle state at export time so
// startup can re-embed exactly the files that moved since (§7-P4-②).
var magic = []byte("GRVX1\x00")

// Header describes the persisted vector set.
type Header struct {
	ModelID string            `json:"model_id"`
	Root    string            `json:"root"`  // merkle root hex at export
	Files   map[string]string `json:"files"` // relPath → file hash hex at export
}

// LoadedHeader returns the header found on startup (nil if no vectors.bin).
func (e *Engine) LoadedHeader() *Header { return e.header }

// StaleFiles compares the loaded header with the current merkle file map:
// files whose hash moved (or that the header never saw) need re-embedding
// even when the 2-Track diff calls them unchanged.
func (e *Engine) StaleFiles(current map[string]string) map[string]bool {
	stale := map[string]bool{}
	if e.header == nil { // no vector state at all → embed everything
		for rel := range current {
			stale[rel] = true
		}
		return stale
	}
	for rel, hash := range current {
		if e.header.Files[rel] != hash {
			stale[rel] = true
		}
	}
	return stale
}

// Export writes the current vector set atomically.
func (e *Engine) Export() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	root, files := "", map[string]string{}
	if e.snapshot != nil {
		root, files = e.snapshot()
	}
	hdr, err := json.Marshal(Header{ModelID: e.modelID, Root: root, Files: files})
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.Write(magic)
	var lenb [4]byte
	binary.LittleEndian.PutUint32(lenb[:], uint32(len(hdr)))
	buf.Write(lenb[:])
	buf.Write(hdr)
	if err := e.db.ExportToWriter(&buf, true, ""); err != nil {
		return err
	}
	if err := store.WriteFileAtomic(e.persistPath, buf.Bytes(), 0o644); err != nil {
		return err
	}
	e.dirty.Store(false)
	e.log.Event("vectors_export", map[string]any{"bytes": buf.Len(), "docs": e.col.Count()})
	return nil
}

// load restores vectors.bin; returns the parsed header.
func (e *Engine) load() (*Header, error) {
	b, err := os.ReadFile(e.persistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) < len(magic)+4 || !bytes.Equal(b[:len(magic)], magic) {
		return nil, fmt.Errorf("bad vectors.bin magic")
	}
	off := len(magic)
	hdrLen := int(binary.LittleEndian.Uint32(b[off:]))
	off += 4
	if off+hdrLen > len(b) {
		return nil, fmt.Errorf("truncated vectors.bin header")
	}
	var hdr Header
	if err := json.Unmarshal(b[off:off+hdrLen], &hdr); err != nil {
		return nil, err
	}
	off += hdrLen

	if err := e.db.ImportFromReader(bytes.NewReader(b[off:]), ""); err != nil {
		return nil, fmt.Errorf("chromem import: %w", err)
	}
	e.col = e.db.GetCollection(collectionName, noEmbed)
	if e.col == nil {
		e.col, _ = e.db.CreateCollection(collectionName, nil, noEmbed)
	}
	e.log.Event("vectors_loaded", map[string]any{"docs": e.col.Count(), "root": hdr.Root})
	return &hdr, nil
}
