package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleRecords() []record {
	return []record{
		{Op: opUpsert, TargetID: "com.a.B.f()", SourceID: "com.a.C.g()", Type: EdgeCall, Confidence: 0.95},
		{Op: opUpsert, TargetID: "com.a.B.f()", SourceID: "com.a.D.h()", Type: EdgeCall, Confidence: 0.9},
		{Op: opTombstone, TargetID: "com.a.B.f()", SourceID: "com.a.C.g()", Type: EdgeCall},
	}
}

func TestRecordRoundtrip(t *testing.T) {
	var buf []byte
	for _, r := range sampleRecords() {
		buf = encodeRecord(buf, r)
	}
	recs, good := replay(buf)
	if good != len(buf) {
		t.Fatalf("clean log: good=%d want %d", good, len(buf))
	}
	if len(recs) != 3 {
		t.Fatalf("decoded %d records", len(recs))
	}
	if recs[0].Confidence != 0.95 || recs[2].Op != opTombstone {
		t.Fatalf("roundtrip mismatch: %+v", recs)
	}
}

// TestCRCTailTruncationRecovery proves §7-P3-③: a partial trailing write is
// discarded on replay and the file is truncated to the last good boundary.
func TestCRCTailTruncationRecovery(t *testing.T) {
	var buf []byte
	for _, r := range sampleRecords() {
		buf = encodeRecord(buf, r)
	}
	full := len(buf)

	// Case 1: tail cut mid-record.
	cut := append([]byte(nil), buf[:full-5]...)
	recs, good := replay(cut)
	if len(recs) != 2 {
		t.Fatalf("truncated tail: decoded %d records, want 2", len(recs))
	}

	// Case 2: bit flip inside the last record's payload.
	flip := append([]byte(nil), buf...)
	flip[full-6] ^= 0xFF
	recs, _ = replay(flip)
	if len(recs) != 2 {
		t.Fatalf("bit flip: decoded %d records, want 2", len(recs))
	}

	// openDeltaLog truncates the file so the next append starts clean.
	path := filepath.Join(t.TempDir(), deltaName)
	if err := os.WriteFile(path, cut, 0o644); err != nil {
		t.Fatal(err)
	}
	l, recs2, err := openDeltaLog(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if len(recs2) != 2 {
		t.Fatalf("replayed %d records", len(recs2))
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() != int64(good) {
		t.Fatalf("file not truncated: size=%d want %d", fi.Size(), good)
	}

	// A fresh append after recovery replays cleanly.
	if err := l.append(encodeRecord(nil, sampleRecords()[2])); err != nil {
		t.Fatal(err)
	}
	if err := l.sync(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	recs3, good3 := replay(data)
	if len(recs3) != 3 || good3 != len(data) {
		t.Fatalf("post-recovery append corrupt: %d records, good=%d/%d", len(recs3), good3, len(data))
	}
}
