package admin

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Salvia95/graphin/internal/search"
	"github.com/Salvia95/graphin/internal/workspace"
)

// defaultMinConf mirrors explore_graph's MCP default (docs/eval H2).
const defaultMinConf = 0.85

func parseMinConf(r *http.Request) float32 {
	v, err := strconv.ParseFloat(r.URL.Query().Get("min_conf"), 32)
	if err != nil || v < 0 || v > 1 {
		return defaultMinConf
	}
	return float32(v)
}

func fmtConf(c float32) string { return strconv.FormatFloat(float64(c), 'f', 2, 32) }

// ---- 검색 ----

type searchHit struct {
	search.Result
	Display string
	Kind    string
	RelPath string
}

type resultsVM struct {
	Query string
	Hits  []searchHit
}

type searchVM struct {
	pageVM
	Results resultsVM
}

func (s *Server) searchResults(q string, k int) resultsVM {
	vm := resultsVM{Query: q}
	if q == "" {
		return vm
	}
	for _, res := range s.ws.Router.Search(q, k) {
		meta, _ := s.ws.Meta(res.NodeID)
		vm.Hits = append(vm.Hits, searchHit{
			Result:  res,
			Display: meta.DisplayName,
			Kind:    meta.Kind,
			RelPath: meta.RelPath,
		})
	}
	return vm
}

func (s *Server) handleSearchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	s.renderPage(w, "search.html", searchVM{
		pageVM:  s.pageVM("search"),
		Results: s.searchResults(q, 20),
	})
}

func (s *Server) handleSearchPartial(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	s.renderPartial(w, "results.html", http.StatusOK, s.searchResults(q, 20))
}

// ---- 노드 상세 ----

type edgeRowVM struct {
	NodeID     string
	Type       string
	Confidence float32
	Display    string
	Dangling   bool
}

type edgeListVM struct {
	ID         string
	Dir        string
	MinConf    string
	Rows       []edgeRowVM
	NextCursor string
	FirstPage  bool
	Err        string
}

// edgeList assembles one direction's page. A target with neither node meta
// nor graph presence is dangling — rendered red, not linked.
func (s *Server) edgeList(id, dir, cursor string, minConf float32) edgeListVM {
	vm := edgeListVM{ID: id, Dir: dir, MinConf: fmtConf(minConf), FirstPage: cursor == ""}
	page, err := s.ws.Explore(id, dir, cursor, minConf)
	if err != nil {
		if !errors.Is(err, workspace.ErrNodeNotFound) {
			vm.Err = err.Error()
		}
		return vm
	}
	outs := page.Uses
	if dir == "used_by" {
		outs = page.UsedBy
	}
	for _, eo := range outs {
		row := edgeRowVM{
			NodeID: eo.NodeID, Type: eo.Type, Confidence: eo.Confidence,
			Display: s.ws.DisplayName(eo.NodeID),
		}
		if row.Display == "" {
			if _, ok := s.ws.Meta(eo.NodeID); !ok {
				_, inGraph := s.ws.GraphInfo(eo.NodeID)
				row.Dangling = !inGraph
			}
		}
		vm.Rows = append(vm.Rows, row)
	}
	if page.HasMore {
		vm.NextCursor = page.NextCursor
	}
	return vm
}

type codeVM struct {
	Block *workspace.CodeBlock
	Err   string
}

func (s *Server) codeVM(id string) codeVM {
	blk, err := s.ws.ReadCode(id)
	if err != nil {
		switch {
		case errors.Is(err, workspace.ErrNodeGone):
			return codeVM{Err: "소스가 바뀌어 노드가 사라졌습니다 (NODE_GONE)"}
		case errors.Is(err, workspace.ErrNodeNotFound):
			return codeVM{Err: "노드 메타데이터가 없습니다 — 인덱싱 중이거나 존재하지 않는 노드입니다"}
		default:
			return codeVM{Err: err.Error()}
		}
	}
	return codeVM{Block: blk}
}

// exploreVM is the min_conf-sensitive fragment: ego graph + both edge lists.
type exploreVM struct {
	ID      string
	MinConf string
	Uses    edgeListVM
	UsedBy  edgeListVM
	Ego     egoData
}

func (s *Server) exploreVM(id string, minConf float32) exploreVM {
	vm := exploreVM{
		ID:      id,
		MinConf: fmtConf(minConf),
		Uses:    s.edgeList(id, "uses", "", minConf),
		UsedBy:  s.edgeList(id, "used_by", "", minConf),
	}
	display := s.ws.DisplayName(id)
	if display == "" {
		display = id
	}
	vm.Ego = buildEgo(id, display, vm.Uses, vm.UsedBy)
	return vm
}

type nodeVM struct {
	pageVM
	ID         string
	Found      bool
	Meta       workspace.NodeMeta
	Pkg        string // 구조 브라우저 링크용 ("" = 그래프에 없음)
	FileAnchor string
	Explore    exploreVM
	Code       codeVM
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	minConf := parseMinConf(r)
	vm := nodeVM{pageVM: s.pageVM("search"), ID: id}
	meta, ok := s.ws.Meta(id)
	if info, inGraph := s.ws.GraphInfo(id); inGraph {
		vm.Pkg = info.Pkg
		if !ok {
			// 그래프에는 있는데 메타가 아직 없는 창(초기 스캔 중) 지원.
			meta = workspace.NodeMeta{
				RelPath: info.FilePath, StartByte: info.Start, EndByte: info.End,
				Kind: info.Kind, DisplayName: info.DisplayName, Partial: info.Partial,
			}
			ok = true
		}
	}
	vm.Found = ok
	vm.Meta = meta
	vm.FileAnchor = fileAnchor(meta.RelPath)
	if ok {
		vm.Explore = s.exploreVM(id, minConf)
		vm.Code = s.codeVM(id)
	}
	s.renderPage(w, "node.html", vm)
}

func (s *Server) handleExplorePartial(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	minConf := parseMinConf(r)
	// min_conf 변경이 주소창에 남도록 노드 페이지 URL로 치환 — 새로고침해도
	// 필터가 유지된다 (handleNode의 parseMinConf가 읽음).
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Replace-Url",
			"/node?id="+url.QueryEscape(id)+"&min_conf="+fmtConf(minConf))
	}
	s.renderPartial(w, "explore.html", http.StatusOK, s.exploreVM(id, minConf))
}

func (s *Server) handleEdgesPartial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir := q.Get("dir")
	if dir != "used_by" {
		dir = "uses"
	}
	s.renderPartial(w, "edges.html", http.StatusOK,
		s.edgeList(q.Get("id"), dir, q.Get("cursor"), parseMinConf(r)))
}
