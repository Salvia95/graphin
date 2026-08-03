// Package admin serves the read-only local admin page: a browser view of the
// graph (dashboard, search, node detail with an ego-graph, diagnostics,
// effective settings). It mounts inside the MCP server process via
// --admin-addr, so it always sees the live workspace and never fights the
// single-writer lock.
package admin

import (
	"html/template"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

// cacheTTL bounds how often the O(nodes+edges) dashboard scans rerun.
const cacheTTL = 10 * time.Second

// healthCounts is the dashboard's problem summary, computed in one graph walk.
type healthCounts struct {
	Dangling     graph.DanglingTotals
	PartialNodes int
}

// Server is the admin HTTP handler. All routes are GET — v1 mutates nothing.
type Server struct {
	ws      *workspace.Workspace
	version string
	log     *obs.Logger
	tmpl    map[string]*template.Template
	mux     *http.ServeMux

	cacheMu  sync.Mutex
	statsAt  time.Time
	stats    graph.Stats
	healthAt time.Time
	health   healthCounts
}

// NewServer builds the handler; it fails only on template parse errors.
func NewServer(ws *workspace.Workspace, version string, lg *obs.Logger) (*Server, error) {
	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	s := &Server{ws: ws, version: version, log: lg, tmpl: tmpl, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleDashboard)
	s.mux.HandleFunc("GET /partial/status", s.handleStatusPartial)
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler()))
}

// ServeHTTP guards every route with the loopback Host check.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !hostAllowed(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// cachedStats serializes and memoizes the per-shard scan.
func (s *Server) cachedStats() graph.Stats {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if !s.statsAt.IsZero() && time.Since(s.statsAt) < cacheTTL {
		return s.stats
	}
	s.stats = s.ws.GraphStats()
	s.statsAt = time.Now()
	return s.stats
}

// cachedHealth memoizes the dangling/partial walk (O(nodes+edges)).
func (s *Server) cachedHealth() healthCounts {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if !s.healthAt.IsZero() && time.Since(s.healthAt) < cacheTTL {
		return s.health
	}
	var h healthCounts
	_, h.Dangling = s.ws.GraphDangling(0)
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		if n.Partial {
			h.PartialNodes++
		}
		return true
	})
	s.health = h
	s.healthAt = time.Now()
	return h
}

// topShards returns the largest shards by node count (stable order).
func topShards(st graph.Stats, n int) []graph.ShardStat {
	out := append([]graph.ShardStat(nil), st.Shards...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Nodes > out[j].Nodes })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
