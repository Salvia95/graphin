// Package mcp implements the stdio MCP transport: newline-delimited JSON-RPC
// 2.0, the initialize handshake, and tool dispatch. It knows nothing about
// indexing — handlers are injected through the Registry.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/llls2542/graphin/internal/obs"
)

// ProtocolVersion is the MCP revision this server speaks.
const ProtocolVersion = "2025-06-18"

const serverName = "graphin"

// Server serves MCP over a byte stream (stdin/stdout in production).
type Server struct {
	in      io.Reader
	w       *writer
	reg     *Registry
	log     *obs.Logger
	version string
	calls   sync.WaitGroup // in-flight tools/call goroutines
}

func NewServer(in io.Reader, out io.Writer, reg *Registry, version string, lg *obs.Logger) *Server {
	return &Server{in: in, w: &writer{out: out}, reg: reg, log: lg, version: version}
}

// Serve reads newline-delimited JSON-RPC messages until EOF or ctx cancel.
// initialize is answered inline and never waits on indexing (§3.0);
// tools/call runs on its own goroutine so a slow tool cannot delay protocol
// handling.
func (s *Server) Serve(ctx context.Context) error {
	sc := bufio.NewScanner(s.in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = s.w.send(&response{JSONRPC: "2.0", ID: json.RawMessage("null"),
				Error: &rpcError{Code: codeParseError, Message: "parse error"}})
			continue
		}
		s.dispatch(ctx, &req)
	}
	// stdin closed: let in-flight tool calls finish writing their responses.
	s.calls.Wait()
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, req *request) {
	switch req.Method {
	case "initialize":
		s.reply(req, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": serverName, "version": s.version},
		})
	case "notifications/initialized", "notifications/cancelled":
		// notifications: no response
	case "ping":
		s.reply(req, struct{}{})
	case "tools/list":
		s.reply(req, s.toolsList())
	case "tools/call":
		s.calls.Add(1)
		go func() {
			defer s.calls.Done()
			s.handleToolCall(ctx, req)
		}()
	default:
		if !req.isNotification() {
			s.sendErr(req, codeMethodNotFound, "method not found: "+req.Method)
		}
	}
}

func (s *Server) toolsList() any {
	tools := make([]map[string]any, 0)
	for _, t := range s.reg.List() {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		tools = append(tools, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
	}
	return map[string]any{"tools": tools}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(ctx context.Context, req *request) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Event("tool_panic", map[string]any{"panic": fmt.Sprint(r)})
			s.sendErr(req, codeInternalError, "internal error")
		}
	}()

	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		s.sendErr(req, codeInvalidParams, "invalid tools/call params")
		return
	}
	t, ok := s.reg.Get(p.Name)
	if !ok {
		s.sendErr(req, codeInvalidParams, "unknown tool: "+p.Name)
		return
	}
	text, isErr := t.Handler(ctx, p.Arguments)
	s.reply(req, map[string]any{
		"content": []map[string]any{{"type": "text", "text": Truncate(text)}},
		"isError": isErr,
	})
}

func (s *Server) reply(req *request, result any) {
	if req.isNotification() {
		return
	}
	_ = s.w.send(&response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) sendErr(req *request, code int, msg string) {
	if req.isNotification() {
		return
	}
	_ = s.w.send(&response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: code, Message: msg}})
}
