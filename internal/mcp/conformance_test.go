package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/mcp/tools"
	"github.com/Salvia95/graphin/internal/obs"
)

// The official 2026-07-28 schema, byte for byte:
//
//	github.com/modelcontextprotocol/modelcontextprotocol
//	schema/2026-07-28/schema.json @ 271ecc9accafdd9b83a3c869fa67c22953b2af80
//
// It is vendored rather than paraphrased on purpose. Every modern-era test in
// server_test.go asserts our own reading of the spec, and our reading was
// what missed CacheableResult: server/discover and tools/list shipped without
// ttlMs and cacheScope, both required, and Claude Code 2.1.282 — the first
// release to speak the modern era to us — dropped the whole tool table over
// it. A test that reads its requirements from the spec's own file cannot
// share that blind spot.
const schemaPath = "testdata/schema-2026-07-28.json"

// modernResults maps each method this server answers in the modern era to
// the result definition the schema gives it.
var modernResults = map[string]string{
	"server/discover": "DiscoverResult",
	"tools/list":      "ListToolsResult",
	"tools/call":      "CallToolResult",
}

// TestModernResultsConformToSchema drives the real tool table through every
// modern method and validates each result against the official schema.
func TestModernResultsConformToSchema(t *testing.T) {
	defs := loadDefs(t)
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcp.ModernVersion,
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}
	byID := serve(t, []map[string]any{
		{"jsonrpc": "2.0", "id": "server/discover", "method": "server/discover",
			"params": map[string]any{"_meta": meta}},
		{"jsonrpc": "2.0", "id": "tools/list", "method": "tools/list",
			"params": map[string]any{"_meta": meta}},
		{"jsonrpc": "2.0", "id": "tools/call", "method": "tools/call",
			"params": map[string]any{"_meta": meta, "name": "conformance_echo"}},
	})

	for method, def := range modernResults {
		resp, ok := byID[method]
		if !ok {
			t.Errorf("%s: no response", method)
			continue
		}
		if resp.Error != nil {
			t.Errorf("%s: error %s", method, resp.Error)
			continue
		}
		schema, ok := defs[def]
		if !ok {
			t.Fatalf("schema has no %s: the vendored file is not the one this test was written against", def)
		}
		v := validator{defs: defs}
		v.check(schema, resp.Result, method+".result")
		for _, e := range v.errs {
			t.Error(e)
		}
	}

	// The loop above would pass on an empty table, which is exactly what a
	// client that rejected it would see.
	var list struct {
		Tools []any `json:"tools"`
	}
	_ = json.Unmarshal(byID["tools/list"].Result, &list)
	if len(list.Tools) < 2 {
		t.Errorf("tools/list returned %d tools, want the real table plus the echo", len(list.Tools))
	}
}

// TestLegacyListCarriesNoModernFields: the caching hints belong to 2026-07-28
// only. A legacy client validates against a revision that does not define
// them, and an unknown field is the kind of thing a strict client drops a
// whole response over.
func TestLegacyListCarriesNoModernFields(t *testing.T) {
	byID := serve(t, []map[string]any{
		{"jsonrpc": "2.0", "id": "init", "method": "initialize",
			"params": map[string]any{"protocolVersion": mcp.ProtocolVersion}},
		{"jsonrpc": "2.0", "id": "tools/list", "method": "tools/list"},
	})
	var res map[string]json.RawMessage
	if err := json.Unmarshal(byID["tools/list"].Result, &res); err != nil {
		t.Fatalf("legacy tools/list: %v", err)
	}
	for _, k := range []string{"ttlMs", "cacheScope", "resultType", "_meta"} {
		if _, ok := res[k]; ok {
			t.Errorf("legacy tools/list carries %q", k)
		}
	}
}

type rpcResp struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// serve runs the real tool table, plus one tool that can actually be called
// without a workspace, over the given requests and returns responses by id.
func serve(t *testing.T, reqs []map[string]any) map[string]rpcResp {
	t.Helper()
	reg := mcp.NewRegistry()
	tools.Register(reg, nil) // registration never touches the workspace
	reg.Register(&mcp.Tool{
		Name:        "conformance_echo",
		Description: "echo",
		Handler: func(_ context.Context, _ json.RawMessage) (string, bool) {
			return "<echo />", false
		},
	})

	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	for _, r := range reqs {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	// Serve returns only after every in-flight tools/call has written.
	if err := mcp.NewServer(&in, &out, reg, "test", obs.Nop()).Serve(context.Background()); err != nil {
		t.Fatal(err)
	}
	byID := map[string]rpcResp{}
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n")) {
		var r rpcResp
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("non-JSON bytes on transport: %q", line)
		}
		byID[r.ID] = r
	}
	return byID
}

func loadDefs(t *testing.T) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Defs
}

// validator checks a JSON value against the subset of JSON Schema the MCP
// schema file uses: $ref, type, enum, required, properties,
// additionalProperties, items, anyOf, minimum. A keyword outside that set
// fails the test instead of being skipped, so a newer schema cannot pass
// here by using something this checker silently ignores.
type validator struct {
	defs map[string]map[string]any
	errs []string
}

var knownKeywords = map[string]bool{
	"$ref": true, "type": true, "enum": true, "required": true, "properties": true,
	"additionalProperties": true, "items": true, "maxItems": true, "anyOf": true,
	"allOf": true, "const": true, "minimum": true, "maximum": true,
	"description": true, "format": true,
}

func (v *validator) fail(path, format string, args ...any) {
	v.errs = append(v.errs, path+": "+fmt.Sprintf(format, args...))
}

func (v *validator) check(schema map[string]any, raw json.RawMessage, path string) {
	var val any
	if err := json.Unmarshal(raw, &val); err != nil {
		v.fail(path, "not JSON: %v", err)
		return
	}
	v.value(schema, val, path)
}

func (v *validator) value(schema map[string]any, val any, path string) {
	for k := range schema {
		if !knownKeywords[k] {
			v.fail(path, "schema keyword %q is not understood by this checker", k)
		}
	}
	if ref, ok := schema["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		def, ok := v.defs[name]
		if !ok {
			v.fail(path, "unresolvable $ref %q", ref)
			return
		}
		v.value(def, val, path)
	}
	if branches, ok := schema["allOf"].([]any); ok {
		for _, b := range branches {
			v.value(b.(map[string]any), val, path)
		}
	}
	if branches, ok := schema["anyOf"].([]any); ok {
		matched := false
		for _, b := range branches {
			sub := validator{defs: v.defs}
			sub.value(b.(map[string]any), val, path)
			if len(sub.errs) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			v.fail(path, "matches no anyOf branch")
		}
	}
	if typ, ok := schema["type"]; ok && !typeMatches(typ, val) {
		v.fail(path, "type %s, want %v", jsonType(val), typ)
		return
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if e == val {
				found = true
			}
		}
		if !found {
			v.fail(path, "%v is not one of %v", val, enum)
		}
	}
	if c, ok := schema["const"]; ok && c != val {
		v.fail(path, "%v, want const %v", val, c)
	}
	if min, ok := schema["minimum"].(float64); ok {
		if n, ok := val.(float64); ok && n < min {
			v.fail(path, "%v is below minimum %v", n, min)
		}
	}
	if max, ok := schema["maximum"].(float64); ok {
		if n, ok := val.(float64); ok && n > max {
			v.fail(path, "%v is above maximum %v", n, max)
		}
	}
	switch x := val.(type) {
	case map[string]any:
		v.object(schema, x, path)
	case []any:
		if max, ok := schema["maxItems"].(float64); ok && float64(len(x)) > max {
			v.fail(path, "%d items, want at most %v", len(x), max)
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range x {
				v.value(items, e, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
}

func (v *validator) object(schema map[string]any, obj map[string]any, path string) {
	if req, ok := schema["required"].([]any); ok {
		for _, k := range req {
			if _, ok := obj[k.(string)]; !ok {
				v.fail(path, "missing required field %q", k)
			}
		}
	}
	props, _ := schema["properties"].(map[string]any)
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if p, ok := props[k].(map[string]any); ok {
			v.value(p, obj[k], path+"."+k)
			continue
		}
		switch ap := schema["additionalProperties"].(type) {
		case bool:
			if !ap {
				v.fail(path, "field %q is not allowed", k)
			}
		case map[string]any:
			v.value(ap, obj[k], path+"."+k)
		}
	}
}

func typeMatches(typ, val any) bool {
	switch t := typ.(type) {
	case string:
		return typeIs(t, val)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && typeIs(s, val) {
				return true
			}
		}
	}
	return false
}

func typeIs(t string, val any) bool {
	switch t {
	case "integer":
		n, ok := val.(float64)
		return ok && n == float64(int64(n))
	case "number":
		_, ok := val.(float64)
		return ok
	default:
		return jsonType(val) == t
	}
}

func jsonType(val any) string {
	switch val.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return fmt.Sprintf("%T", val)
}
