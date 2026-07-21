package lexical

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lexical.idx")
	ix, st := NewIndex(), NewSymbolTable()
	ix.Upsert("com.example.A.run()", []string{"run", "a"})
	st.Put("com.example.A.run()", "run")

	if err := Save(path, ix, st); err != nil {
		t.Fatal(err)
	}
	ix2, st2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if ix2.Len() != 1 || st2.Len() != 1 {
		t.Fatalf("roundtrip lost data: idx=%d sym=%d", ix2.Len(), st2.Len())
	}
	if got := st2.Lookup("run"); len(got) != 1 || got[0] != "com.example.A.run()" {
		t.Fatalf("symbol lookup after load: %v", got)
	}
}

func TestLoadMissingFileYieldsEmpty(t *testing.T) {
	ix, st, err := Load(filepath.Join(t.TempDir(), "nope.idx"))
	if err != nil {
		t.Fatal(err)
	}
	if ix.Len() != 0 || st.Len() != 0 {
		t.Fatal("expected empty structures")
	}
}
