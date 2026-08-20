package graph

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"os"
)

// §6.3 record ops.
const (
	opUpsert    byte = 0x01
	opTombstone byte = 0x02
	// opRedirect maps a node ID that disappeared to the one that replaced
	// it, so a reference recorded against the old ID keeps resolving.
	//
	// It shares this log but NOT the edge map: apply routes it to its own
	// table, which is what keeps used_by queries free of a filter on the
	// hottest read path. It is also upsert-class, not tombstone-class —
	// compaction drops tombstones, and a redirect that evaporated would
	// break exactly the pinned references it exists to protect.
	opRedirect byte = 0x03
)

const maxIDLen = 1 << 20 // sanity bound while replaying

// record is one §6.3 delta-log entry.
//
// The field meanings depend on Op. For the two edge ops they are an edge; for
// opRedirect, TargetID is the old node ID and SourceID the one that replaced
// it, Type and Confidence are unused, and Epoch is set.
type record struct {
	Op         byte
	TargetID   string // used_by 대상 (피호출자) / redirect: 옛 ID
	SourceID   string // 호출자 / redirect: 새 ID
	Type       EdgeType
	Confidence float32
	// Epoch is when a redirect was created, for the deferred GC in §4.3.
	// It is written only by opRedirect records: putting it on every edge
	// record would widen millions of them for a field two of them use.
	Epoch uint64
}

// encodeRecord appends the §6.3 wire form of r to buf.
//
//	edge:     [1B op][4B LE len+target][4B LE len+source][1B type][4B LE f32 conf][4B LE CRC32]
//	redirect: [1B op][4B LE len+old][4B LE len+new][8B LE epoch][4B LE CRC32]
//
// The payload is chosen by op, so the two edge ops keep the exact bytes they
// always had. A log written before redirects existed replays unchanged and
// needs no format version, which is the whole reason for the split.
//
// The reverse is not true: an OLDER binary meeting a redirect record rejects
// the unknown op, and replay truncates the log there. That costs used_by
// entries appended after the first redirect until the next reindex —
// fail-soft, and the reason the on-disk format is not a breaking surface
// (docs/plugin-distribution.md §13.2).
func encodeRecord(buf []byte, r record) []byte {
	start := len(buf)
	buf = append(buf, r.Op)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.TargetID)))
	buf = append(buf, r.TargetID...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.SourceID)))
	buf = append(buf, r.SourceID...)
	if r.Op == opRedirect {
		buf = binary.LittleEndian.AppendUint64(buf, r.Epoch)
	} else {
		buf = append(buf, byte(r.Type))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(r.Confidence))
	}
	crc := crc32.ChecksumIEEE(buf[start:])
	return binary.LittleEndian.AppendUint32(buf, crc)
}

// replay decodes records until EOF or the first corrupt record, returning the
// records and the byte offset of the corruption boundary (§6.3: 꼬리 절단
// 감지 — 그 지점이 새 append 오프셋).
func replay(data []byte) (recs []record, goodLen int) {
	off := 0
	for {
		r, n, ok := decodeRecord(data[off:])
		if !ok {
			return recs, off
		}
		recs = append(recs, r)
		off += n
	}
}

func decodeRecord(b []byte) (record, int, bool) {
	// The smallest record is an edge with two empty IDs. A redirect's 8-byte
	// epoch is wider than the type+conf it replaces, so this bound is the
	// loose one and each branch re-checks its own tail below.
	const fixed = 1 + 4 + 4 + 1 + 4 + 4 // op + 2 lens + type + conf + crc
	if len(b) < fixed {
		return record{}, 0, false
	}
	pos := 0
	op := b[pos]
	pos++
	if op != opUpsert && op != opTombstone && op != opRedirect {
		return record{}, 0, false
	}
	tlen := binary.LittleEndian.Uint32(b[pos:])
	pos += 4
	if tlen > maxIDLen || pos+int(tlen) > len(b) {
		return record{}, 0, false
	}
	target := string(b[pos : pos+int(tlen)])
	pos += int(tlen)
	if pos+4 > len(b) {
		return record{}, 0, false
	}
	slen := binary.LittleEndian.Uint32(b[pos:])
	pos += 4
	if slen > maxIDLen {
		return record{}, 0, false
	}
	tail := 1 + 4 // type + conf
	if op == opRedirect {
		tail = 8 // epoch
	}
	if pos+int(slen)+tail+4 > len(b) {
		return record{}, 0, false
	}
	source := string(b[pos : pos+int(slen)])
	pos += int(slen)

	rec := record{Op: op, TargetID: target, SourceID: source}
	if op == opRedirect {
		rec.Epoch = binary.LittleEndian.Uint64(b[pos:])
		pos += 8
	} else {
		rec.Type = EdgeType(b[pos])
		pos++
		rec.Confidence = math.Float32frombits(binary.LittleEndian.Uint32(b[pos:]))
		pos += 4
	}
	want := binary.LittleEndian.Uint32(b[pos:])
	if crc32.ChecksumIEEE(b[:pos]) != want {
		return record{}, 0, false
	}
	return rec, pos + 4, true
}

// deltaLog is the append-only on-disk log.
type deltaLog struct {
	f    *os.File
	size int64
}

func openDeltaLog(path string) (*deltaLog, []record, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	recs, good := replay(data)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	// Discard any corrupt tail so the next append starts at a clean boundary.
	if err := f.Truncate(int64(good)); err != nil {
		f.Close()
		return nil, nil, err
	}
	if _, err := f.Seek(int64(good), io.SeekStart); err != nil {
		f.Close()
		return nil, nil, err
	}
	return &deltaLog{f: f, size: int64(good)}, recs, nil
}

func (l *deltaLog) append(buf []byte) error {
	if l.f == nil {
		return errors.New("delta log closed")
	}
	n, err := l.f.Write(buf)
	l.size += int64(n)
	return err
}

func (l *deltaLog) sync() error {
	if l.f == nil {
		return nil
	}
	return l.f.Sync()
}

func (l *deltaLog) close() error {
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
