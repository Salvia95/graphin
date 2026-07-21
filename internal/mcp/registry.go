package mcp

import (
	"context"
	"encoding/json"
	"sync"
)

// ToolHandler returns the XML payload for a tool call. isError marks the MCP
// result as failed while still carrying the XML <error> body.
type ToolHandler func(ctx context.Context, args json.RawMessage) (xml string, isError bool)

// Tool describes one MCP tool and its handler.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     ToolHandler
}

// Registry holds the tool table in registration order.
type Registry struct {
	mu    sync.RWMutex
	order []string
	m     map[string]*Tool
}

func NewRegistry() *Registry { return &Registry{m: map[string]*Tool{}} }

func (r *Registry) Register(t *Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.m[t.Name]; !ok {
		r.order = append(r.order, t.Name)
	}
	r.m[t.Name] = t
}

func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.m[name]
	return t, ok
}

func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.m[name])
	}
	return out
}
