package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBrowseIndexListsPackages(t *testing.T) {
	s, _ := bootstrappedServer(t)
	rec := get(t, s, "/browse")
	// java 픽스처는 패키지 선언이 없어 _root 샤드에 쌓인다.
	wantContains(t, rec, http.StatusOK, "구조", "/browse?pkg=_root", "파일")
}

func TestBrowsePkgGroupsByFile(t *testing.T) {
	s, ws := bootstrappedServer(t)
	rec := get(t, s, "/browse?pkg=_root")
	wantContains(t, rec, http.StatusOK, "Svc.java", "Flow.java", "/node?id=")

	// 파일 그룹 안에 소스 순서대로 노드가 있고, 각 노드가 상세로 링크된다.
	runID := findNode(t, ws, "Flow.run()")
	if !strings.Contains(rec.Body.String(), url.QueryEscape(runID)) &&
		!strings.Contains(rec.Body.String(), runID) {
		t.Fatalf("node link for %s missing", runID)
	}
}

func TestBrowseUnknownPkg(t *testing.T) {
	s, _ := bootstrappedServer(t)
	wantContains(t, get(t, s, "/browse?pkg=no.such.pkg"), http.StatusOK, "노드가 없습니다")
}

func TestNodePageLinksToBrowse(t *testing.T) {
	s, ws := bootstrappedServer(t)
	runID := findNode(t, ws, "Flow.run()")
	rec := get(t, s, "/node?id="+url.QueryEscape(runID))
	wantContains(t, rec, http.StatusOK, "/browse?pkg=", "#f-Flow-java")
}

func TestFileAnchorSanitizes(t *testing.T) {
	if a := fileAnchor("src/a b/Flow.java"); a != "f-src-a-b-Flow-java" {
		t.Fatalf("anchor: %s", a)
	}
}
