// Package e2e drives the real MCP server over an in-process pipe against
// fixture workspaces: the §7-P5 acceptance suite.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/llls2542/graphin/internal/mcp"
	"github.com/llls2542/graphin/internal/mcp/tools"
	"github.com/llls2542/graphin/internal/obs"
	"github.com/llls2542/graphin/internal/workspace"
)

const (
	javaFixtures = "../testdata/fixtures/java"
	cancelID     = "com.example.order.domain.OrderService.cancelPayment(long,String)"
)

type client struct {
	t      *testing.T
	enc    *json.Encoder
	dec    *json.Decoder
	nextID int
}

type rpcResp struct {
	ID     json.Number     `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newClient(t *testing.T, root string) *client {
	t.Helper()
	ws := workspace.New(workspace.Config{Root: root, Log: obs.Nop(), OrtLib: "/nonexistent-ort"})
	reg := mcp.NewRegistry()
	tools.Register(reg, ws)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := mcp.NewServer(inR, outW, reg, "e2e", obs.Nop())
	go func() { _ = srv.Serve(context.Background()) }()
	t.Cleanup(func() {
		_ = inW.Close()
		ws.Close()
	})

	c := &client{t: t, enc: json.NewEncoder(inW), dec: json.NewDecoder(outR)}
	res := c.call("initialize", map[string]any{"protocolVersion": mcp.ProtocolVersion})
	if res.Error != nil {
		t.Fatalf("initialize: %+v", res.Error)
	}
	return c
}

func (c *client) call(method string, params any) *rpcResp {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	if err := c.enc.Encode(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	}); err != nil {
		c.t.Fatal(err)
	}
	for {
		var resp rpcResp
		if err := c.dec.Decode(&resp); err != nil {
			c.t.Fatal(err)
		}
		if got, _ := resp.ID.Int64(); got == int64(id) {
			return &resp
		}
	}
}

// tool invokes tools/call and returns (text, isError).
func (c *client) tool(name string, args any) (string, bool) {
	c.t.Helper()
	resp := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if resp.Error != nil {
		c.t.Fatalf("%s: protocol error %+v", name, resp.Error)
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		c.t.Fatal(err)
	}
	if len(result.Content) != 1 {
		c.t.Fatalf("%s: expected one content block, got %d", name, len(result.Content))
	}
	return result.Content[0].Text, result.IsError
}

func (c *client) bootstrapAndWait(root string) {
	c.t.Helper()
	if text, isErr := c.tool("bootstrap_workspace", map[string]any{}); isErr {
		c.t.Fatalf("bootstrap failed: %s", text)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		text, _ := c.tool("search_hybrid", map[string]any{"query": "___warmup___"})
		if strings.Contains(text, `lexical_ready="true"`) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatal("lexical never became ready")
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestE2EFixtureRoundtripByteSavings: the full agent journey — search →
// explore → read — plus the benchmark proving byte savings vs grep -C20
// (§7-P5-①).
func TestE2EFixtureRoundtripByteSavings(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	// 1) search_hybrid: Tier-0 exact match surfaces the entry point.
	text, isErr := c.tool("search_hybrid", map[string]any{"query": "cancelPayment"})
	if isErr || !strings.Contains(text, cancelID) || !strings.Contains(text, `match_type="exact"`) {
		t.Fatalf("search: %s", text)
	}

	// 2) explore_graph: uses edges with confidence.
	text, isErr = c.tool("explore_graph", map[string]any{"node_id": cancelID})
	if isErr || !strings.Contains(text, "<graph_context") || !strings.Contains(text, "confidence=") {
		t.Fatalf("explore: %s", text)
	}

	// 3) read_code: exact slice.
	text, isErr = c.tool("read_code", map[string]any{"node_id": cancelID})
	if isErr || !strings.Contains(text, "public void cancelPayment(long orderId, String reason)") ||
		!strings.Contains(text, `reparsed="false"`) {
		t.Fatalf("read_code: %s", text)
	}

	// 4) benchmark: graphin bytes must beat grep -C20 bytes, and the
	//    expected node must be reported as a hit.
	text, isErr = c.tool("run_local_benchmark", map[string]any{
		"target_query": "cancelPayment", "expected_node": cancelID,
	})
	if isErr {
		t.Fatalf("benchmark: %s", text)
	}
	attrs := regexp.MustCompile(`(\w+)="([^"]*)"`).FindAllStringSubmatch(text, -1)
	vals := map[string]string{}
	for _, kv := range attrs {
		vals[kv[1]] = kv[2]
	}
	c20, _ := strconv.Atoi(vals["grep_c20_bytes"])
	nav, _ := strconv.Atoi(vals["graphin_bytes"])
	if vals["hit"] != "true" {
		t.Fatalf("expected node not hit: %s", text)
	}
	if nav <= 0 || c20 <= 0 || nav >= c20 {
		t.Fatalf("byte savings not proven: graphin=%d vs grep-C20=%d\n%s", nav, c20, text)
	}
	if !strings.Contains(text, "| Grep Full") || !strings.Contains(text, "| graphin") {
		t.Fatalf("markdown table missing: %s", text)
	}
}

// TestResponseTruncatedAt12KB proves §7-P5-②.
func TestResponseTruncatedAt12KB(t *testing.T) {
	root := t.TempDir()
	var body strings.Builder
	body.WriteString("package big;\n\npublic class Big {\n    public void giant() {\n")
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&body, "        int v%04d = %d; // padding line to exceed the cap\n", i, i)
	}
	body.WriteString("    }\n}\n")
	if err := os.WriteFile(filepath.Join(root, "Big.java"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, isErr := c.tool("read_code", map[string]any{"node_id": "big.Big.giant()"})
	if isErr {
		t.Fatalf("read_code: %s", text)
	}
	if len(text) > mcp.MaxResponseBytes {
		t.Fatalf("response is %d bytes, cap %d", len(text), mcp.MaxResponseBytes)
	}
	if !strings.Contains(text, `<truncated reason="size"`) {
		t.Fatal("missing <truncated> marker")
	}
}

// TestSystemStatusDuringIndexing proves §7-P5-③: while not fully ready,
// every tool response embeds <system_status> (semantic stays warming here,
// so state remains "indexing" with lexical_ready=true).
func TestSystemStatusDuringIndexing(t *testing.T) {
	root := t.TempDir()
	copyTree(t, javaFixtures, root)
	c := newClient(t, root)
	c.bootstrapAndWait(root)

	text, _ := c.tool("search_hybrid", map[string]any{"query": "cancelPayment"})
	if !strings.Contains(text, `<system_status state="indexing"`) ||
		!strings.Contains(text, `lexical_ready="true"`) ||
		!strings.Contains(text, `semantic_ready="false"`) {
		t.Fatalf("system_status missing/wrong: %s", text)
	}
	if !strings.Contains(text, `progress="`) {
		t.Fatalf("progress attribute missing: %s", text)
	}
}

func TestErrorCodes(t *testing.T) {
	t.Run("NOT_BOOTSTRAPPED", func(t *testing.T) {
		c := newClient(t, t.TempDir())
		text, isErr := c.tool("search_hybrid", map[string]any{"query": "x"})
		if !isErr || !strings.Contains(text, `code="NOT_BOOTSTRAPPED"`) {
			t.Fatalf("got %v %s", isErr, text)
		}
	})

	t.Run("NODE_NOT_FOUND", func(t *testing.T) {
		root := t.TempDir()
		copyTree(t, javaFixtures, root)
		c := newClient(t, root)
		c.bootstrapAndWait(root)
		text, isErr := c.tool("explore_graph", map[string]any{"node_id": "no.such.Node.f()"})
		if !isErr || !strings.Contains(text, `code="NODE_NOT_FOUND"`) {
			t.Fatalf("got %v %s", isErr, text)
		}
	})

	t.Run("NODE_GONE", func(t *testing.T) {
		root := t.TempDir()
		copyTree(t, javaFixtures, root)
		c := newClient(t, root)
		c.bootstrapAndWait(root)
		// Delete the file, then read before the watcher batch lands.
		if err := os.Remove(filepath.Join(root,
			"src/main/java/com/example/order/domain/OrderService.java")); err != nil {
			t.Fatal(err)
		}
		text, isErr := c.tool("read_code", map[string]any{"node_id": cancelID})
		if !isErr || !strings.Contains(text, `code="NODE_GONE"`) {
			t.Fatalf("got %v %s", isErr, text)
		}
	})

	t.Run("LOCK_HELD", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".graphin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A live owner: our own PID with a fresh heartbeat.
		if err := os.WriteFile(filepath.Join(dir, "lockfile"),
			[]byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644); err != nil {
			t.Fatal(err)
		}
		c := newClient(t, root)
		text, isErr := c.tool("bootstrap_workspace", map[string]any{})
		if !isErr || !strings.Contains(text, `code="LOCK_HELD"`) {
			t.Fatalf("got %v %s", isErr, text)
		}
	})

	t.Run("MODEL_UNAVAILABLE", func(t *testing.T) {
		root := t.TempDir()
		copyTree(t, javaFixtures, root)
		c := newClient(t, root) // bogus --ort-lib → warmup fails permanently
		c.bootstrapAndWait(root)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			text, _ := c.tool("search_hybrid", map[string]any{"query": "cancelPayment"})
			if strings.Contains(text, `code="MODEL_UNAVAILABLE"`) &&
				strings.Contains(text, "lexical fallback") {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("MODEL_UNAVAILABLE hint never surfaced")
	})
}
