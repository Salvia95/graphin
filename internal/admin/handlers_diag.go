package admin

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/Salvia95/graphin/internal/graph"
	"github.com/Salvia95/graphin/internal/provision"
	"github.com/Salvia95/graphin/internal/semantic"
	"github.com/Salvia95/graphin/internal/workspace"
)

// diagRowCap bounds diagnostics tables; totals still count everything.
const diagRowCap = 500

// ---- 진단 ----

type danglingRowVM struct {
	Source        string
	SourceDisplay string
	Type          string
	Conf          float32
	Target        string
	DB            bool
}

type diagDanglingVM struct {
	Filter    string // all | code | db
	Rows      []danglingRowVM
	Totals    graph.DanglingTotals
	Truncated bool
}

type diagPartialVM struct {
	Rows  []graph.NodeInfo
	Total int
}

type diagSemanticVM struct {
	Ready           bool
	Gated           bool
	GateNodes       int
	GateMax         int
	Failure         string
	Pending         int64
	QueueDepth      int
	Drained         bool
	Header          *semantic.Header
	HeaderFiles     int
	ExpectedModelID string
	Warning         *modelWarningVM
}

type diagVM struct {
	pageVM
	Tab      string
	Dangling *diagDanglingVM
	Partial  *diagPartialVM
	Semantic *diagSemanticVM
	Reverse  *graph.ReverseStats
}

func (s *Server) diagDangling(filter string) *diagDanglingVM {
	vm := &diagDanglingVM{Filter: filter}
	rows, totals := s.ws.GraphDangling(diagRowCap * 4)
	vm.Totals = totals
	for _, d := range rows {
		if filter == "code" && d.DBDomain {
			continue
		}
		if filter == "db" && !d.DBDomain {
			continue
		}
		if len(vm.Rows) >= diagRowCap {
			vm.Truncated = true
			break
		}
		vm.Rows = append(vm.Rows, danglingRowVM{
			Source:        d.SourceID,
			SourceDisplay: s.ws.DisplayName(d.SourceID),
			Type:          graph.EdgeTypeName(d.Edge.Type),
			Conf:          d.Edge.Confidence,
			Target:        d.Edge.TargetID,
			DB:            d.DBDomain,
		})
	}
	return vm
}

func (s *Server) diagPartial() *diagPartialVM {
	vm := &diagPartialVM{}
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		if !n.Partial {
			return true
		}
		vm.Total++
		if len(vm.Rows) < diagRowCap {
			n.Uses = nil
			vm.Rows = append(vm.Rows, n)
		}
		return true
	})
	return vm
}

func (s *Server) diagSemantic() *diagSemanticVM {
	st := s.ws.Status()
	pending, depth, drained := s.ws.SemanticQueue()
	vm := &diagSemanticVM{
		Ready:      st.SemanticReady,
		Gated:      st.SemanticGated,
		Failure:    s.ws.SemUnavailable(),
		Pending:    pending,
		QueueDepth: depth,
		Drained:    drained,
		Header:     s.ws.SemanticHeader(),
	}
	vm.GateNodes, vm.GateMax, _ = s.ws.GateInfo()
	if vm.Header != nil {
		vm.HeaderFiles = len(vm.Header.Files)
	}
	vm.ExpectedModelID, _ = modelExpectation(s.ws.EffectiveConfig(), vm.Header)
	vm.Warning = s.modelWarning()
	return vm
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	tab := r.URL.Query().Get("tab")
	vm := diagVM{pageVM: s.pageVM("diagnostics")}
	switch tab {
	case "partial":
		vm.Partial = s.diagPartial()
	case "semantic":
		vm.Semantic = s.diagSemantic()
	case "reverse":
		rs := s.ws.GraphReverseStats()
		vm.Reverse = &rs
	default:
		tab = "dangling"
		filter := r.URL.Query().Get("filter")
		if filter != "code" && filter != "db" {
			filter = "all"
		}
		vm.Dangling = s.diagDangling(filter)
	}
	vm.Tab = tab
	s.renderPage(w, "diagnostics.html", vm)
}

// ---- 설정 (읽기 전용) ----

type dirSize struct {
	Name  string
	Bytes int64
}

type gateVM struct {
	Nodes int
	Max   int
	Gated bool
}

type settingsVM struct {
	pageVM
	Cfg        workspace.ConfigView
	Spec       *provision.ModelSpec
	ORTVersion string

	Header  *semantic.Header
	Warning *modelWarningVM
	Gate    gateVM

	DataDir string
	Sizes   []dirSize
	Total   int64
}

// duDir sums file sizes under dir (0 when absent).
func duDir(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	cfg := s.ws.EffectiveConfig()
	vm := settingsVM{
		pageVM:     s.pageVM("settings"),
		Cfg:        cfg,
		ORTVersion: provision.ORTVersion,
		Header:     s.ws.SemanticHeader(),
		DataDir:    s.ws.Dir,
	}
	if spec, ok := provision.Models[cfg.ModelType]; ok {
		vm.Spec = &spec
	}
	vm.Warning = s.modelWarning()
	vm.Gate.Nodes, vm.Gate.Max, vm.Gate.Gated = s.ws.GateInfo()
	for _, sub := range []string{"search", "graph", "runtime", "usage"} {
		n := duDir(filepath.Join(s.ws.Dir, sub))
		vm.Sizes = append(vm.Sizes, dirSize{Name: sub, Bytes: n})
		vm.Total += n
	}
	s.renderPage(w, "settings.html", vm)
}
