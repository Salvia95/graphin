package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
