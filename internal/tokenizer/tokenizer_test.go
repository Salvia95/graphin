package tokenizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fixture struct {
	Cases []struct {
		Text string  `json:"text"`
		IDs  []int64 `json:"ids"`
	} `json:"cases"`
}

func loadFixture(t *testing.T, path string) *fixture {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return &f
}

func runParity(t *testing.T, tok Tokenizer, f *fixture) {
	t.Helper()
	for _, c := range f.Cases {
		ids, mask := tok.Encode(c.Text)
		if len(mask) != len(ids) {
			t.Fatalf("mask length mismatch for %q", c.Text)
		}
		if len(ids) != len(c.IDs) {
			t.Errorf("%q:\n got %v\nwant %v", c.Text, ids, c.IDs)
			continue
		}
		for i := range ids {
			if ids[i] != c.IDs[i] {
				t.Errorf("%q: id[%d] = %d, want %d\n got %v\nwant %v",
					c.Text, i, ids[i], c.IDs[i], ids, c.IDs)
				break
			}
		}
	}
}

// TestMatchesHFReferenceFixtures_E5V2 proves §7-P4 parity for the WordPiece
// path against HuggingFace tokenizers output (30 mixed sentences).
func TestMatchesHFReferenceFixtures_E5V2(t *testing.T) {
	tok, err := Load("../../testdata/tokenizer/e5v2-tokenizer.json")
	if err != nil {
		t.Fatal(err)
	}
	runParity(t, tok, loadFixture(t, "../../testdata/tokenizer/e5v2.json"))
}

// TestMatchesHFReferenceFixtures_ME5 covers the Unigram path. The 17MB
// tokenizer.json is not committed; the test uses the provisioning cache and
// skips with instructions when absent.
func TestMatchesHFReferenceFixtures_ME5(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".cache", "graphin", "test", "me5-tokenizer.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("multilingual tokenizer not cached at %s (download intfloat/multilingual-e5-small tokenizer.json to run)", path)
	}
	tok, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	runParity(t, tok, loadFixture(t, "../../testdata/tokenizer/me5.json"))
}
