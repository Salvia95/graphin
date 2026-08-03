package admin

import (
	"net/http"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/graph"
)

// 구조 브라우저: 검색 없이 패키지(샤드) → 파일 → 노드로 내려가는 진입점.

// browseNodeCap bounds one package page; larger packages get a truncation note.
const browseNodeCap = 2000

type browsePkgRow struct {
	Pkg   string
	Nodes int
	Edges int
	Files int
}

type browseIndexVM struct {
	pageVM
	Rows       []browsePkgRow
	TotalNodes int
	TotalFiles int
}

type browseNode struct {
	ID          string
	DisplayName string
	Kind        string
	Start       uint32
	Partial     bool
	EdgeCount   int
}

type browseFileGroup struct {
	Path   string
	Anchor string
	Nodes  []browseNode
}

type browsePkgVM struct {
	pageVM
	Pkg       string
	Files     []browseFileGroup
	Total     int
	Truncated bool
}

// fileAnchor turns a rel path into a stable fragment id.
func fileAnchor(path string) string {
	return "f-" + strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '-'
	}, path)
}

func (s *Server) handleBrowseIndex(w http.ResponseWriter, r *http.Request) {
	st := s.cachedStats()
	files := map[string]map[string]bool{} // pkg → file set
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		set := files[n.Pkg]
		if set == nil {
			set = map[string]bool{}
			files[n.Pkg] = set
		}
		set[n.FilePath] = true
		return true
	})
	vm := browseIndexVM{pageVM: s.pageVM("browse"), TotalNodes: st.Nodes}
	allFiles := map[string]bool{}
	for _, sh := range st.Shards {
		row := browsePkgRow{Pkg: sh.Pkg, Nodes: sh.Nodes, Edges: sh.Edges, Files: len(files[sh.Pkg])}
		vm.Rows = append(vm.Rows, row)
		for f := range files[sh.Pkg] {
			allFiles[f] = true
		}
	}
	vm.TotalFiles = len(allFiles)
	sort.Slice(vm.Rows, func(i, j int) bool { return vm.Rows[i].Pkg < vm.Rows[j].Pkg })
	s.renderPage(w, "browse.html", vm)
}

func (s *Server) handleBrowsePkg(w http.ResponseWriter, r *http.Request) {
	pkg := r.URL.Query().Get("pkg")
	vm := browsePkgVM{pageVM: s.pageVM("browse"), Pkg: pkg}
	groups := map[string][]browseNode{}
	s.ws.GraphForEach(func(n graph.NodeInfo) bool {
		if n.Pkg != pkg {
			return true
		}
		vm.Total++
		if vm.Total > browseNodeCap {
			vm.Truncated = true
			return true
		}
		groups[n.FilePath] = append(groups[n.FilePath], browseNode{
			ID:          n.ID,
			DisplayName: n.DisplayName,
			Kind:        n.Kind,
			Start:       n.Start,
			Partial:     n.Partial,
			EdgeCount:   len(n.Uses),
		})
		return true
	})
	paths := make([]string, 0, len(groups))
	for p := range groups {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		nodes := groups[p]
		sort.Slice(nodes, func(i, j int) bool { // 파일 내 소스 순서
			if nodes[i].Start != nodes[j].Start {
				return nodes[i].Start < nodes[j].Start
			}
			return nodes[i].ID < nodes[j].ID
		})
		vm.Files = append(vm.Files, browseFileGroup{Path: p, Anchor: fileAnchor(p), Nodes: nodes})
	}
	s.renderPage(w, "browse_pkg.html", vm)
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("pkg") != "" {
		s.handleBrowsePkg(w, r)
		return
	}
	s.handleBrowseIndex(w, r)
}
