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
)

const maxIDLen = 1 << 20 // sanity bound while replaying

// record is one §6.3 delta-log entry.
type record struct {
	Op         byte
	TargetID   string // used_by 대상 (피호출자)
	SourceID   string // 호출자
	Type       EdgeType
	Confidence float32
}

// encodeRecord appends the §6.3 wire form of r to buf:
// [1B op][4B LE len+target][4B LE len+source][1B type][4B LE f32 conf][4B LE CRC32].
func encodeRecord(buf []byte, r record) []byte {
	start := len(buf)
	buf = append(buf, r.Op)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.TargetID)))
	buf = append(buf, r.TargetID...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.SourceID)))
	buf = append(buf, r.SourceID...)
	buf = append(buf, byte(r.Type))
	buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(r.Confidence))
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
	const fixed = 1 + 4 + 4 + 1 + 4 + 4 // op + 2 lens + type + conf + crc
	if len(b) < fixed {
		return record{}, 0, false
	}
	pos := 0
	op := b[pos]
	pos++
	if op != opUpsert && op != opTombstone {
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
	if slen > maxIDLen || pos+int(slen)+1+4+4 > len(b) {
		return record{}, 0, false
	}
	source := string(b[pos : pos+int(slen)])
	pos += int(slen)
	etype := EdgeType(b[pos])
	pos++
	conf := math.Float32frombits(binary.LittleEndian.Uint32(b[pos:]))
	pos += 4
	want := binary.LittleEndian.Uint32(b[pos:])
	if crc32.ChecksumIEEE(b[:pos]) != want {
		return record{}, 0, false
	}
	pos += 4
	return record{Op: op, TargetID: target, SourceID: source, Type: etype, Confidence: conf}, pos, true
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
