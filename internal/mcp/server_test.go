package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

// TestInitializeUnder100ms proves §7-P1-①: initialize answers immediately
// even while a slow tool call (standing in for indexing) is in flight.
func TestInitializeUnder100ms(t *testing.T) {
	reg := NewRegistry()
	started := make(chan struct{})
	reg.Register(&Tool{
		Name:        "slow",
		Description: "simulates a long-running bootstrap",
		Handler: func(ctx context.Context, _ json.RawMessage) (string, bool) {
			close(started)
			time.Sleep(2 * time.Second)
			return "<ok />", false
		},
	})

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer(inR, outW, reg, "test", obs.Nop())
	go func() { _ = srv.Serve(context.Background()) }()
	defer inW.Close()

	enc := json.NewEncoder(inW)
	dec := json.NewDecoder(outR)

	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "slow"},
	}); err != nil {
		t.Fatal(err)
	}
	<-started // the slow tool is now blocking its own goroutine

	start := time.Now()
	if err := enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "initialize",
		"params": map[string]any{"protocolVersion": ProtocolVersion},
	}); err != nil {
		t.Fatal(err)
	}

	var resp struct {
		ID     json.Number    `json:"id"`
		Result map[string]any `json:"result"`
	}
	if err := dec.Decode(&resp); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	if resp.ID.String() != "2" {
		t.Fatalf("first response should be initialize (id 2), got id %s", resp.ID)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("initialize took %v, want < 100ms", elapsed)
	}
	if got := resp.Result["protocolVersion"]; got != ProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %s", got, ProtocolVersion)
	}
}

// TestStdoutOnlyJSONRPC proves the transport stream carries nothing but
// JSON-RPC messages (§5: stdout 오염 금지).
func TestStdoutOnlyJSONRPC(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "echo",
		Description: "echo",
		Handler: func(_ context.Context, _ json.RawMessage) (string, bool) {
			return "<echo />", false
		},
	})

	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo"}}` + "\n")

	var out bytes.Buffer
	srv := NewServer(in, &out, reg, "test", obs.Nop())
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	// tools/call runs async; wait for its response to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Count(out.Bytes(), []byte("\n")) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d: %s", len(lines), out.String())
	}
	for _, line := range lines {
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			t.Fatalf("non-JSON bytes on transport: %q", line)
		}
		if msg["jsonrpc"] != "2.0" {
			t.Fatalf("non-JSON-RPC message on transport: %q", line)
		}
	}
}

// session drives a server over pipes and correlates responses by id, because
// tools/call answers on its own goroutine and may land out of order.
type session struct {
	t   *testing.T
	enc *json.Encoder
	dec *json.Decoder
	id  int
}

type rawResp struct {
	ID     json.Number    `json:"id"`
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	} `json:"error"`
}

func newSession(t *testing.T, reg *Registry) *session {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer(inR, outW, reg, "test", obs.Nop())
	go func() { _ = srv.Serve(context.Background()) }()
	t.Cleanup(func() { _ = inW.Close() })
	return &session{t: t, enc: json.NewEncoder(inW), dec: json.NewDecoder(outR)}
}

func (s *session) call(method string, params any) rawResp {
	s.t.Helper()
	s.id++
	id := s.id
	if err := s.enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		s.t.Fatal(err)
	}
	for {
		var resp rawResp
		if err := s.dec.Decode(&resp); err != nil {
			s.t.Fatal(err)
		}
		if got, _ := resp.ID.Int64(); got == int64(id) {
			return resp
		}
	}
}

// modernParams stamps a request with the per-request protocol metadata that
// puts it in the modern era.
func modernParams(version string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	params["_meta"] = map[string]any{
		metaProtocolVersion: version,
		metaClientCaps:      map[string]any{},
	}
	return params
}

func echoRegistry() *Registry {
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "echo",
		Description: "echo",
		Handler: func(_ context.Context, _ json.RawMessage) (string, bool) {
			return "<echo />", false
		},
	})
	return reg
}

// TestInitializeVersionNegotiation pins the legacy half of the handshake: a
// revision we speak comes back unchanged, anything else gets ours. Answering
// with a version the client did not ask for is legal but costs the session if
// the client speaks only what it asked for, so the echo is the whole point.
func TestInitializeVersionNegotiation(t *testing.T) {
	s := newSession(t, NewRegistry())
	for _, tc := range []struct{ ask, want string }{
		{ProtocolVersion, ProtocolVersion},
		{"2025-06-18", "2025-06-18"},
		{"2024-11-05", "2024-11-05"},
		{"1900-01-01", ProtocolVersion},
		{"", ProtocolVersion},
	} {
		res := s.call("initialize", map[string]any{"protocolVersion": tc.ask})
		if res.Error != nil {
			t.Fatalf("initialize(%q): %+v", tc.ask, res.Error)
		}
		if got := res.Result["protocolVersion"]; got != tc.want {
			t.Errorf("initialize(%q): protocolVersion = %v, want %s", tc.ask, got, tc.want)
		}
		// The legacy envelope must stay exactly what it was: a revision that
		// does not define resultType should never see one.
		if _, ok := res.Result["resultType"]; ok {
			t.Errorf("initialize(%q): answered in the modern envelope", tc.ask)
		}
	}
}

// TestServerDiscover covers the method 2026-07-28 requires of every server.
// The request carries no _meta here on purpose: that is how the stdio
// backward-compatibility probe may arrive, and the answer is what tells a
// dual-era client this process is not legacy.
func TestServerDiscover(t *testing.T) {
	s := newSession(t, NewRegistry())
	res := s.call("server/discover", map[string]any{})
	if res.Error != nil {
		t.Fatalf("server/discover: %+v", res.Error)
	}
	versions, _ := res.Result["supportedVersions"].([]any)
	if len(versions) == 0 || versions[0] != ModernVersion {
		t.Errorf("supportedVersions = %v, want [%s]", res.Result["supportedVersions"], ModernVersion)
	}
	if _, ok := res.Result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Errorf("capabilities = %v, want a tools entry", res.Result["capabilities"])
	}
	if got := res.Result["resultType"]; got != "complete" {
		t.Errorf("resultType = %v, want complete", got)
	}
	info, _ := res.Result["_meta"].(map[string]any)[metaServerInfo].(map[string]any)
	if info["name"] != serverName || info["version"] != "test" {
		t.Errorf("_meta serverInfo = %v, want {graphin test}", info)
	}
}

// TestModernRequestMissingClientCapabilities: once a request declares itself
// modern, clientCapabilities is required and its absence is malformed. The
// rejection is also load-bearing for the probe — a dual-era client reads any
// non-modern error as "legacy server" and falls back to initialize, which
// this server still answers.
func TestModernRequestMissingClientCapabilities(t *testing.T) {
	s := newSession(t, NewRegistry())
	res := s.call("tools/list", map[string]any{
		"_meta": map[string]any{metaProtocolVersion: ModernVersion},
	})
	if res.Error == nil || res.Error.Code != codeInvalidParams {
		t.Fatalf("error = %+v, want code %d", res.Error, codeInvalidParams)
	}
}

// TestModernUnsupportedVersion pins UnsupportedProtocolVersionError, whose
// data.supported is the client's only route back to a version we speak.
func TestModernUnsupportedVersion(t *testing.T) {
	s := newSession(t, NewRegistry())
	res := s.call("tools/list", modernParams("1900-01-01", nil))
	if res.Error == nil || res.Error.Code != codeUnsupportedVersion {
		t.Fatalf("error = %+v, want code %d", res.Error, codeUnsupportedVersion)
	}
	supported, _ := res.Error.Data["supported"].([]any)
	if len(supported) == 0 || supported[0] != ModernVersion {
		t.Errorf("data.supported = %v, want [%s]", res.Error.Data["supported"], ModernVersion)
	}
	if got := res.Error.Data["requested"]; got != "1900-01-01" {
		t.Errorf("data.requested = %v, want 1900-01-01", got)
	}
}

// TestToolCallEnvelopePerEra: same handler, same tool table, two envelopes.
// The legacy shape is asserted field-for-field because a client that
// validates strictly drops the whole response over one field it does not
// know, and it would drop it silently.
func TestToolCallEnvelopePerEra(t *testing.T) {
	s := newSession(t, echoRegistry())

	legacy := s.call("tools/call", map[string]any{"name": "echo"})
	if legacy.Error != nil {
		t.Fatalf("legacy tools/call: %+v", legacy.Error)
	}
	for _, k := range []string{"resultType", "_meta"} {
		if _, ok := legacy.Result[k]; ok {
			t.Errorf("legacy result carries %q", k)
		}
	}
	if legacy.Result["isError"] != false {
		t.Errorf("legacy isError = %v, want false", legacy.Result["isError"])
	}

	mod := s.call("tools/call", modernParams(ModernVersion, map[string]any{"name": "echo"}))
	if mod.Error != nil {
		t.Fatalf("modern tools/call: %+v", mod.Error)
	}
	if got := mod.Result["resultType"]; got != "complete" {
		t.Errorf("modern resultType = %v, want complete", got)
	}
	info, _ := mod.Result["_meta"].(map[string]any)[metaServerInfo].(map[string]any)
	if info["name"] != serverName {
		t.Errorf("modern _meta serverInfo = %v, want name %s", info, serverName)
	}
	content, _ := mod.Result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("modern content = %v, want one block", mod.Result["content"])
	}
	if text, _ := content[0].(map[string]any)["text"]; text != "<echo />" {
		t.Errorf("modern content text = %v, want <echo />", text)
	}
}

// pipedServer runs a server whose input the test sequences by hand and whose
// output lands in a buffer. Serve returns only after every in-flight tool call
// has finished writing, so closing the input and waiting on Serve is what
// makes "this response was never sent" a fact rather than a timeout.
type pipedServer struct {
	enc  *json.Encoder
	in   *io.PipeWriter
	out  *bytes.Buffer
	done chan error
}

func newPipedServer(t *testing.T, reg *Registry, lg *obs.Logger) *pipedServer {
	t.Helper()
	inR, inW := io.Pipe()
	p := &pipedServer{enc: json.NewEncoder(inW), in: inW, out: &bytes.Buffer{}, done: make(chan error, 1)}
	srv := NewServer(inR, p.out, reg, "test", lg)
	go func() { p.done <- srv.Serve(context.Background()) }()
	return p
}

func (p *pipedServer) write(t *testing.T, msg map[string]any) {
	t.Helper()
	if err := p.enc.Encode(msg); err != nil {
		t.Fatal(err)
	}
}

// finish closes the input and returns every response the server wrote, by id.
func (p *pipedServer) finish(t *testing.T) map[string]rawResp {
	t.Helper()
	_ = p.in.Close()
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not shut down")
	}
	byID := map[string]rawResp{}
	for _, line := range bytes.Split(bytes.TrimSpace(p.out.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var resp rawResp
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("non-JSON bytes on transport: %q", line)
		}
		byID[resp.ID.String()] = resp
	}
	return byID
}

// TestCancelledToolCallGoesQuiet covers both halves of notifications/cancelled:
// the handler's context is really cancelled, and nothing is sent for the
// abandoned request afterwards.
func TestCancelledToolCallGoesQuiet(t *testing.T) {
	started, observed := make(chan struct{}), make(chan struct{})
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "block",
		Description: "blocks until its call is cancelled",
		Handler: func(ctx context.Context, _ json.RawMessage) (string, bool) {
			close(started)
			select {
			case <-ctx.Done():
				close(observed)
			case <-time.After(5 * time.Second):
			}
			return "<done />", false
		},
	})

	p := newPipedServer(t, reg, obs.Nop())
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "block"}})
	<-started
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{"requestId": 1, "reason": "user stopped"}})
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler's context was never cancelled")
	}
	// A request the server does answer, so the assertion below is about
	// silence on id 1 rather than about a transport that stopped working.
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "ping"})

	byID := p.finish(t)
	if resp, ok := byID["1"]; ok {
		t.Errorf("cancelled call was still answered: %+v", resp)
	}
	if _, ok := byID["2"]; !ok {
		t.Errorf("ping went unanswered: %v", byID)
	}
}

// TestCancelMismatchStaysHarmless: a cancellation that names nothing in
// flight, and one that names the string "1" while the call is the number 1.
// Neither may silence a live call — a cancellation is only allowed to stop
// the request it actually names.
func TestCancelMismatchStaysHarmless(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "wait",
		Description: "waits for the test to release it",
		Handler: func(_ context.Context, _ json.RawMessage) (string, bool) {
			close(started)
			<-release
			return "<ok />", false
		},
	})

	p := newPipedServer(t, reg, obs.Nop())
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "wait"}})
	<-started
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{"requestId": "1"}})
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{"requestId": 99}})
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{}})
	close(release)

	byID := p.finish(t)
	resp, ok := byID["1"]
	if !ok {
		t.Fatalf("live call was silenced by a cancellation that did not name it: %v", byID)
	}
	content, _ := resp.Result["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != "<ok />" {
		t.Errorf("result = %v, want the handler's own output", resp.Result)
	}
}

// TestCancelLogsReason: a cancellation is the one thing that happens to a tool
// call for reasons the server cannot see, so the reason the client gave is
// the only record of why the work stopped. The spec asks both parties to log
// it; agent-nav.log is where that lands.
func TestCancelLogsReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-nav.log")
	lg, err := obs.New(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lg.Close()

	started, observed := make(chan struct{}), make(chan struct{})
	reg := NewRegistry()
	reg.Register(&Tool{
		Name:        "block",
		Description: "blocks until its call is cancelled",
		Handler: func(ctx context.Context, _ json.RawMessage) (string, bool) {
			close(started)
			select {
			case <-ctx.Done():
				close(observed)
			case <-time.After(5 * time.Second):
			}
			return "<done />", false
		},
	})

	p := newPipedServer(t, reg, lg)
	p.write(t, map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call",
		"params": map[string]any{"name": "block"}})
	<-started
	p.write(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled",
		"params": map[string]any{"requestId": 7, "reason": "user stopped"}})
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler's context was never cancelled")
	}
	p.finish(t)

	log, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := ""
	for _, l := range strings.Split(string(log), "\n") {
		if strings.Contains(l, `"event":"tool_cancelled"`) {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no tool_cancelled event in the log: %s", log)
	}
	for _, want := range []string{`"request_id":"7"`, `"reason":"user stopped"`} {
		if !strings.Contains(line, want) {
			t.Errorf("tool_cancelled event = %s, want it to carry %s", line, want)
		}
	}
}
