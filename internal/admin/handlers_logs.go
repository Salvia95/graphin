package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 활동 로그 뷰: obs JSONL(agent-nav.log)의 꼬리를 읽어 이벤트를 보여준다.
// 파일은 append-only라 tail 읽기는 항상 안전하고, 마지막 줄이 쓰다 만
// JSON이면 그냥 건너뛴다.
const (
	logTailBytes = 256 << 10
	logRowCap    = 200
)

type logEntry struct {
	TS     string
	Event  string
	Detail string
	Error  bool
}

type logsPartialVM struct {
	Filter  string
	Names   []string // 필터 셀렉트용 — tail에 등장한 이벤트 이름
	Rows    []logEntry
	Total   int // 필터 적용 전 tail 내 이벤트 수
	Missing bool
	Path    string
}

type logsVM struct {
	pageVM
	Logs logsPartialVM
}

func parseLogLine(line []byte) (logEntry, bool) {
	var m map[string]any
	if json.Unmarshal(line, &m) != nil {
		return logEntry{}, false
	}
	name, ok := m["event"].(string)
	if !ok || name == "" {
		return logEntry{}, false
	}
	e := logEntry{Event: name}
	if s, ok := m["ts"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			e.TS = t.Local().Format("01-02 15:04:05")
		} else {
			e.TS = s
		}
	}
	_, hasErr := m["error"]
	e.Error = hasErr || strings.Contains(name, "error") || strings.Contains(name, "unavailable")

	keys := make([]string, 0, len(m))
	for k := range m {
		if k == "event" || k == "ts" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	e.Detail = strings.Join(parts, " · ")
	return e, true
}

// logsPartial reads the log tail and applies the event-name filter.
func (s *Server) logsPartial(filter string) logsPartialVM {
	path := filepath.Join(s.ws.Dir, "agent-nav.log")
	vm := logsPartialVM{Filter: filter, Path: path}

	f, err := os.Open(path)
	if err != nil {
		vm.Missing = true
		return vm
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		vm.Missing = true
		return vm
	}
	off := int64(0)
	if fi.Size() > logTailBytes {
		off = fi.Size() - logTailBytes
	}
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && len(buf) > 0 {
		vm.Missing = true
		return vm
	}
	lines := bytes.Split(buf, []byte("\n"))
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // 잘린 첫 줄 폐기
	}

	names := map[string]bool{}
	var all []logEntry
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		e, ok := parseLogLine(line)
		if !ok {
			continue
		}
		names[e.Event] = true
		all = append(all, e)
	}
	vm.Total = len(all)
	for n := range names {
		vm.Names = append(vm.Names, n)
	}
	sort.Strings(vm.Names)

	// 최신 우선으로 필터 적용 + 상한.
	for i := len(all) - 1; i >= 0 && len(vm.Rows) < logRowCap; i-- {
		if filter != "" && all[i].Event != filter {
			continue
		}
		vm.Rows = append(vm.Rows, all[i])
	}
	return vm
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "logs.html", logsVM{
		pageVM: s.pageVM("logs"),
		Logs:   s.logsPartial(r.URL.Query().Get("event")),
	})
}

func (s *Server) handleLogsPartial(w http.ResponseWriter, r *http.Request) {
	s.renderPartial(w, "logs.html", http.StatusOK, s.logsPartial(r.URL.Query().Get("event")))
}
