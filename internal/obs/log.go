// Package obs provides structured JSONL logging (§5). stdout is reserved for
// the MCP transport, so this package can only ever write to a file and,
// optionally, stderr.
package obs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger appends JSON Lines events to .graphin/agent-nav.log. A nil Logger is
// valid and discards everything.
type Logger struct {
	mu     sync.Mutex
	f      *os.File
	mirror io.Writer
}

// New opens (creating if needed) the log file at path. When verbose is true,
// every event is mirrored to stderr.
func New(path string, verbose bool) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	l := &Logger{f: f}
	if verbose {
		l.mirror = os.Stderr
	}
	return l, nil
}

// Nop returns a logger that discards all events.
func Nop() *Logger { return &Logger{} }

// Event records one structured event. fields may be nil.
func (l *Logger) Event(name string, fields map[string]any) {
	if l == nil {
		return
	}
	rec := make(map[string]any, len(fields)+2)
	for k, v := range fields {
		rec[k] = v
	}
	rec["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	rec["event"] = name
	line, err := json.Marshal(rec)
	if err != nil {
		line = fmt.Appendf(nil, `{"ts":%q,"event":%q,"marshal_error":%q}`,
			time.Now().UTC().Format(time.RFC3339Nano), name, err.Error())
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_, _ = l.f.Write(line)
	}
	if l.mirror != nil {
		_, _ = l.mirror.Write(line)
	}
}

// Close closes the underlying file, if any.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}
