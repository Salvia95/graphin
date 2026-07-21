package store

import (
	"os"
	"sync"

	mmapgo "github.com/edsrzf/mmap-go"
)

// MMapRef is a reference-counted read-only memory mapping (§4 동시성 모델).
// The owner holds one reference; readers Acquire/Release around access. The
// mapping is unmapped when the owner retires it AND the last reader leaves,
// so a shard swap never invalidates bytes a reader is still walking.
type MMapRef struct {
	Data []byte

	mu   sync.Mutex
	m    mmapgo.MMap
	refs int64 // starts at 1 (owner)
}

// OpenMMap maps path read-only. The file handle is closed immediately; the
// mapping keeps the pages alive.
func OpenMMap(path string) (*MMapRef, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := mmapgo.Map(f, mmapgo.RDONLY, 0)
	if err != nil {
		return nil, err
	}
	return &MMapRef{Data: m, m: m, refs: 1}, nil
}

// Acquire takes a reader reference.
func (r *MMapRef) Acquire() {
	r.mu.Lock()
	r.refs++
	r.mu.Unlock()
}

// Release drops one reference; the last one unmaps.
func (r *MMapRef) Release() {
	r.mu.Lock()
	r.refs--
	last := r.refs == 0
	r.mu.Unlock()
	if last {
		_ = r.m.Unmap()
	}
}

// Retire drops the owner's reference after a swap.
func (r *MMapRef) Retire() { r.Release() }
