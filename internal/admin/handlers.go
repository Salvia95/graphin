package admin

import (
	"net/http"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/workspace"
)

// pageVM is the chrome every full page carries. Bootstrapped drives the
// shared empty-state guidance on data screens (US-1). State and Nav feed the
// sidebar — identity + status at the top, per-item counts on the right
// (DESIGN.md §5).
type pageVM struct {
	Root         string
	Version      string
	Active       string // nav highlight key
	Bootstrapped bool
	State        string // raw mcp.Status.State — 배지 라벨/클래스는 labels.go
	Nav          navCounts
}

// navCounts are the sidebar's right-aligned decorations. Problems is the
// single "확인이 필요합니다" total the dashboard warning strip already sums.
type navCounts struct {
	Nodes    int
	Problems int
}

func (s *Server) pageVM(active string) pageVM {
	vm := pageVM{
		Root: s.ws.Root, Version: s.version, Active: active,
		Bootstrapped: s.ws.Bootstrapped(),
		State:        "not_bootstrapped",
	}
	if !vm.Bootstrapped {
		return vm
	}
	vm.State = s.ws.Status().State
	// 둘 다 cacheTTL(10s) 메모이즈라 페이지마다 전수 순회가 다시 돌지는
	// 않는다. 대신 캐시가 식은 첫 페이지는 O(nodes+edges) 한 번을 문다.
	vm.Nav.Nodes = s.cachedStats().Nodes
	h := s.cachedHealth()
	vm.Nav.Problems = h.Dangling.Code + h.PartialNodes
	if s.modelWarning() != nil {
		vm.Nav.Problems++
	}
	return vm
}

// statusVM decorates mcp.Status for the polling card. Done stops the htmx
// poller (HTTP 286) once nothing will change anymore: fully ready, or
// lexical-ready with semantic permanently off (gate / warmup failure).
type statusVM struct {
	mcp.Status
	SemFailure   string
	Bootstrapped bool
	Done         bool
}

func (s *Server) statusVM() statusVM {
	st := s.ws.Status()
	failure := s.ws.SemUnavailable()
	// 영구 비활성(게이트·워밍업 실패)에서는 큐가 다시 비지 않으므로
	// EmbedPending을 종료 조건에서 제외한다.
	done := (st.State == "ready" && st.EmbedPending == 0) ||
		st.SemanticGated || (st.LexicalReady && failure != "")
	return statusVM{
		Status:       st,
		SemFailure:   failure,
		Bootstrapped: s.ws.Bootstrapped(),
		Done:         done,
	}
}

type dashboardVM struct {
	pageVM
	Status    statusVM
	Stats     graph.Stats
	TopShards []graph.ShardStat
	Health    healthCounts
	Reverse   graph.ReverseStats

	Cfg             workspace.ConfigView
	Header          *semantic.Header
	ExpectedModelID string
	Warning         *modelWarningVM
}

// modelExpectation resolves the configured model type to its pinned ModelID
// and flags a vectors.bin written by a different model (mixed-index hazard).
func modelExpectation(cfg workspace.ConfigView, hdr *semantic.Header) (string, bool) {
	spec, ok := provision.Models[cfg.ModelType]
	if !ok {
		return "", false
	}
	return spec.ID, hdr != nil && hdr.ModelID != spec.ID
}

// modelWarningVM feeds the shared model_warning.html partial (nil = 정상).
type modelWarningVM struct {
	Stored   string
	Expected string
}

func (s *Server) modelWarning() *modelWarningVM {
	hdr := s.ws.SemanticHeader()
	expected, mismatch := modelExpectation(s.ws.EffectiveConfig(), hdr)
	if !mismatch {
		return nil
	}
	return &modelWarningVM{Stored: hdr.ModelID, Expected: expected}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	vm := dashboardVM{
		pageVM: s.pageVM("dashboard"),
		Status: s.statusVM(),
		Cfg:    s.ws.EffectiveConfig(),
		Header: s.ws.SemanticHeader(),
	}
	if vm.Status.Bootstrapped {
		vm.Stats = s.cachedStats()
		vm.TopShards = topShards(vm.Stats, 10)
		vm.Health = s.cachedHealth()
		vm.Reverse = s.ws.GraphReverseStats()
	}
	vm.ExpectedModelID, _ = modelExpectation(vm.Cfg, vm.Header)
	vm.Warning = s.modelWarning()
	s.renderPage(w, "dashboard.html", vm)
}

// handleHelp answers one ⓘ click with the term's explanation fragment.
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	h, ok := helpTerms[r.PathValue("term")]
	if !ok {
		http.Error(w, "알 수 없는 용어입니다", http.StatusNotFound)
		return
	}
	s.renderPartial(w, "help.html", http.StatusOK, h)
}

func (s *Server) handleStatusPartial(w http.ResponseWriter, r *http.Request) {
	vm := s.statusVM()
	code := http.StatusOK
	if vm.Done {
		code = 286 // htmx: stop polling
	}
	s.renderPartial(w, "status.html", code, vm)
}
