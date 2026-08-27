package mcp

import (
	"encoding/json"
	"io"
	"sync"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *request) isNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603

	// codeUnsupportedVersion is UnsupportedProtocolVersionError (2026-07-28).
	// -32020..-32099 is reserved for the specification: a code from that range
	// may only be emitted with the meaning the spec gives it, and no
	// implementation may mint new ones there.
	codeUnsupportedVersion = -32022
)

// writer serializes newline-delimited JSON-RPC messages onto one stream.
// tools/call handlers run on their own goroutines, so every send goes through
// the mutex.
type writer struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *writer) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.out.Write(b)
	return err
}
