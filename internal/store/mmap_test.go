package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestMunmapDeferredUntilLastRelease: a retired mapping stays readable while
// readers hold references (exercised under -race).
func TestMunmapDeferredUntilLastRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shard.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := OpenMMap(path)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		ref.Acquire()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer ref.Release()
			// Read while the owner may already have retired the mapping.
			if ref.Data[0] != '0' || ref.Data[9] != '9' {
				t.Error("unexpected bytes")
			}
		}()
	}
	ref.Retire() // owner drops its reference; unmap waits for readers
	wg.Wait()
}
