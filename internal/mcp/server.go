// Package mcp implements the stdio MCP transport: newline-delimited JSON-RPC
// 2.0, the entry points of both protocol eras, and tool dispatch. It knows
// nothing about indexing — handlers are injected through the Registry.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/Salvia95/graphin/internal/obs"
)

// Protocol revisions this server speaks.
//
// The server is dual-era. 2026-07-28 retired the initialize handshake: a
// modern client carries its protocol version and capabilities in the `_meta`
// of every request and expects the server to keep no session state. Legacy
// clients still open with initialize. Both eras reach the same tool table and
// the same handlers; only the envelope differs, and which era a request
// belongs to is read off the request itself — see parseMeta.
const (
	// ProtocolVersion is the legacy revision this server prefers, and what an
	// initialize handshake is answered with when the client names a revision
	// we do not recognise.
	ProtocolVersion = "2025-11-25"

	// ModernVersion is the stateless, per-request-metadata revision.
	ModernVersion = "2026-07-28"
)

// modernVersions are the revisions a client may name in
// `_meta["io.modelcontextprotocol/protocolVersion"]`, and the list a rejected
// request is told to choose from. Legacy revisions are deliberately absent: a
// client picks its per-request version out of this list, and no revision
// before 2026-07-28 has a per-request version to pick.
var modernVersions = []string{ModernVersion}

// legacyVersions are the handshake revisions answered with the client's own
// version rather than ours. Nothing a tools-only stdio server puts on the
// wire changed across them — they differ in icons, elicitation, tasks and
// authorization, none of which graphin exposes — so echoing the client's
// revision back is accurate rather than a courtesy.
var legacyVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// Reserved `_meta` keys (2026-07-28 §General fields). The
// io.modelcontextprotocol prefix belongs to the spec; nothing else may be
// minted under it.
const (
	metaProtocolVersion = "io.modelcontextprotocol/protocolVersion"
	metaClientCaps      = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo      = "io.modelcontextprotocol/serverInfo"
)

const serverName = "graphin"

// Server serves MCP over a byte stream (stdin/stdout in production).
type Server struct {
	in      io.Reader
	w       *writer
	reg     *Registry
	log     *obs.Logger
	version string
	calls   sync.WaitGroup // in-flight tools/call goroutines

	mu       sync.Mutex
	inflight map[string]*inflightCall // by request id, for cancellation
}

// inflightCall is one tools/call the client can still cancel.
type inflightCall struct {
	cancel    context.CancelFunc
	cancelled bool
}

func NewServer(in io.Reader, out io.Writer, reg *Registry, version string, lg *obs.Logger) *Server {
	return &Server{in: in, w: &writer{out: out}, reg: reg, log: lg, version: version,
		inflight: map[string]*inflightCall{}}
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

// meta is the per-request protocol metadata of a modern request.
//
// present is what selects the era, and it is deliberately keyed on the
// protocol version alone: a legacy client never sends these keys, so their
// absence is not a malformed request, it is the other era. Only once a
// request has declared itself modern does the rest become mandatory.
type meta struct {
	present bool
	version string
	hasCaps bool
	badType bool // protocolVersion is there but is not a string
}

// modern is the metadata of a request that can only belong to the modern era,
// whatever the caller sent.
func modern() meta { return meta{present: true, version: ModernVersion, hasCaps: true} }

func parseMeta(params json.RawMessage) meta {
	if len(params) == 0 {
		return meta{}
	}
	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	// Params that are not an object (or not there) cannot carry `_meta`; the
	// method's own handler reports whatever is wrong with them.
	if err := json.Unmarshal(params, &p); err != nil || len(p.Meta) == 0 {
		return meta{}
	}
	raw, ok := p.Meta[metaProtocolVersion]
	if !ok {
		return meta{}
	}
	m := meta{present: true}
	if err := json.Unmarshal(raw, &m.version); err != nil {
		m.badType = true
	}
	_, m.hasCaps = p.Meta[metaClientCaps]
	return m
}

func (s *Server) dispatch(ctx context.Context, req *request) {
	m := parseMeta(req.Params)
	if m.present && !s.checkModern(req, m) {
		return
	}
	switch req.Method {
	case "initialize":
		// Legacy handshake, always answered in the legacy envelope: a modern
		// client has no reason to send it, and a legacy client must not be
		// handed fields its revision does not define.
		s.reply(req, meta{}, map[string]any{
			"protocolVersion": negotiate(req.Params),
			"capabilities":    capabilities(),
			"serverInfo":      map[string]any{"name": serverName, "version": s.version},
		})
	case "server/discover":
		// Modern discovery (2026-07-28 §Discovery: servers MUST implement it).
		// It doubles as the stdio backward-compatibility probe, so a client
		// that sends it without `_meta` still gets the modern answer — what it
		// gets back is precisely what tells it which era this process is.
		s.reply(req, modern(), map[string]any{
			"supportedVersions": modernVersions,
			"capabilities":      capabilities(),
		})
	case "notifications/initialized":
		// notification: no response
	case "notifications/cancelled":
		s.cancelCall(req.Params)
	case "ping":
		s.reply(req, m, map[string]any{})
	case "tools/list":
		s.reply(req, m, s.toolsList())
	case "tools/call":
		s.calls.Add(1)
		go func() {
			defer s.calls.Done()
			s.handleToolCall(ctx, req, m)
		}()
	default:
		if !req.isNotification() {
			s.sendErr(req, codeMethodNotFound, "method not found: "+req.Method)
		}
	}
}

// checkModern validates the per-request protocol metadata of a request that
// declared itself modern. It answers the request itself when the request
// cannot proceed, and reports whether dispatch should continue.
func (s *Server) checkModern(req *request, m meta) bool {
	if m.badType || !m.hasCaps {
		// 2026-07-28 §General fields: protocolVersion and clientCapabilities
		// are required on every modern request, and one that is missing either
		// is malformed (-32602). Rejecting is also what keeps a dual-era
		// client's probe honest — it reads any non-modern error as "legacy
		// server" and falls back to the initialize handshake, which this
		// server answers.
		s.sendErr(req, codeInvalidParams,
			"malformed _meta: "+metaProtocolVersion+" (string) and "+metaClientCaps+
				" are required on every request")
		return false
	}
	for _, v := range modernVersions {
		if v == m.version {
			return true
		}
	}
	s.sendErrData(req, codeUnsupportedVersion, "Unsupported protocol version", map[string]any{
		"supported": modernVersions,
		"requested": m.version,
	})
	return false
}

// idKey canonicalises a request id so a cancellation can name the call it
// wants stopped. Ids are strings or numbers and the two forms stay distinct:
// "1" and 1 are different requests, and cancelling one must not silence the
// other.
func idKey(id json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, id); err != nil {
		return string(id)
	}
	return buf.String()
}

// cancelCall handles notifications/cancelled: stop the work, and go quiet for
// that request.
//
// Invalid or unknown cancellations are ignored rather than reported —
// notifications are fire-and-forget, and the race a cancellation usually
// loses (the call finished while the notification was in flight) is the
// normal case, not an error. initialize is never in the table, which is also
// why a client is forbidden from cancelling it.
func (s *Server) cancelCall(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
		Reason    string          `json:"reason"`
	}
	if err := json.Unmarshal(params, &p); err != nil || len(p.RequestID) == 0 {
		return
	}
	key := idKey(p.RequestID)
	s.mu.Lock()
	c, ok := s.inflight[key]
	if ok {
		c.cancelled = true
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	c.cancel()
	s.log.Event("tool_cancelled", map[string]any{"request_id": key, "reason": p.Reason})
}

func (s *Server) track(id json.RawMessage, cancel context.CancelFunc) {
	s.mu.Lock()
	s.inflight[idKey(id)] = &inflightCall{cancel: cancel}
	s.mu.Unlock()
}

func (s *Server) untrack(id json.RawMessage) {
	s.mu.Lock()
	delete(s.inflight, idKey(id))
	s.mu.Unlock()
}

// silenced reports whether a cancellation has arrived for this request. Every
// write to the transport goes through it, because "send nothing further for a
// cancelled request" (2026-07-28 §stdio) has to hold on the error paths too —
// including the panic recovery, which is the one writer that runs when the
// handler did not return at all.
func (s *Server) silenced(id json.RawMessage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.inflight[idKey(id)]
	return ok && c.cancelled
}

// negotiate picks the revision an initialize response names: the client's own
// when we speak it, ours otherwise (§lifecycle version negotiation). Naming a
// version the client did not ask for is legal, but a client that speaks only
// the version it asked for disconnects over it — so guessing high costs the
// whole session, and echoing costs nothing.
func negotiate(params json.RawMessage) string {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ProtocolVersion
	}
	for _, v := range legacyVersions {
		if v == p.ProtocolVersion {
			return v
		}
	}
	return ProtocolVersion
}

// capabilities is what both eras advertise. Fresh each call: reply decorates
// the result map it is handed, and a shared map would collect that decoration.
func capabilities() map[string]any {
	return map[string]any{"tools": map[string]any{"listChanged": false}}
}

func (s *Server) toolsList() map[string]any {
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

func (s *Server) handleToolCall(ctx context.Context, req *request, m meta) {
	// The record of a cancellation has to outlive every writer below, so
	// untrack is registered first and therefore runs last — after the panic
	// recovery, which would otherwise answer a call the client already
	// abandoned. cancel() is idempotent and releases the context either way.
	ctx, cancel := context.WithCancel(ctx)
	if !req.isNotification() {
		s.track(req.ID, cancel)
		defer s.untrack(req.ID)
	}
	defer cancel()
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
	if ctx.Err() != nil {
		return // cancelled before the handler started: nothing to run, nothing to say
	}
	text, isErr := t.Handler(ctx, p.Arguments)
	s.reply(req, m, map[string]any{
		"content": []map[string]any{{"type": "text", "text": Truncate(text)}},
		"isError": isErr,
	})
}

// reply answers req in the envelope of its era. A modern result carries the
// resultType its revision requires and the server identity in `_meta`; a
// legacy result is byte-for-byte what this server sent before it was
// dual-era, because an unexpected field is exactly the kind of thing that
// empties a validating client's tool table (see TestInputSchemasMarshalValid).
func (s *Server) reply(req *request, m meta, result map[string]any) {
	if req.isNotification() || s.silenced(req.ID) {
		return
	}
	if m.present {
		result["resultType"] = "complete"
		result["_meta"] = map[string]any{
			metaServerInfo: map[string]any{"name": serverName, "version": s.version},
		}
	}
	_ = s.w.send(&response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func (s *Server) sendErr(req *request, code int, msg string) {
	s.sendErrData(req, code, msg, nil)
}

func (s *Server) sendErrData(req *request, code int, msg string, data any) {
	if req.isNotification() || s.silenced(req.ID) {
		return
	}
	_ = s.w.send(&response{JSONRPC: "2.0", ID: req.ID,
		Error: &rpcError{Code: code, Message: msg, Data: data}})
}
