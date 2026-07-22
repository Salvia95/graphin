package workspace

import (
	"slices"
	"strings"
	"testing"

	"github.com/Salvia95/graphin/internal/scan"
)

func files(paths ...string) []scan.FileInfo {
	out := make([]scan.FileInfo, 0, len(paths))
	for _, p := range paths {
		out = append(out, scan.FileInfo{RelPath: p})
	}
	return out
}

func TestDetectDBTracesAndSnapshots(t *testing.T) {
	info := detectDB(files(
		"supabase/migrations/20260101000000_init.sql",
		"supabase/migrations/20260102000000_more.sql", // 같은 디렉터리는 1회
		"src/main/resources/db/migration/V1__init.sql",
		"prisma/schema.prisma",
		"docker-compose.yml",
		"db/main.graphindb.json",
		"db/main.rls.graphindb.json",
		"src/app/service.py",
		"docs/design.sql", // migration 디렉터리 밖 SQL은 흔적 아님
	))
	wantSources := []string{
		"docker-compose", "prisma",
		"src/main/resources/db/migration", "supabase/migrations",
	}
	if !slices.Equal(info.Sources, wantSources) {
		t.Fatalf("sources = %v", info.Sources)
	}
	if info.Snapshots != 2 {
		t.Fatalf("snapshots = %d", info.Snapshots)
	}
}

func TestDetectDBEmpty(t *testing.T) {
	info := detectDB(files("src/App.java", "README.md"))
	if len(info.Sources) != 0 || info.Snapshots != 0 {
		t.Fatalf("expected empty: %+v", info)
	}
}

func TestStatusWithDBHint(t *testing.T) {
	w := New(Config{Root: t.TempDir()})
	w.dbInfo = DBInfo{Sources: []string{"supabase/migrations"}}
	st := w.statusWithDB()
	if st.DBSources != "supabase/migrations" || st.DBSnapshots != 0 || st.DBHint == "" {
		t.Fatalf("hint expected: %+v", st)
	}
	xml := st.XML()
	for _, want := range []string{`db_sources_detected="supabase/migrations"`, "<hint>", "dbimport"} {
		if !strings.Contains(xml, want) {
			t.Fatalf("XML missing %q:\n%s", want, xml)
		}
	}

	// 스냅샷이 생기면 힌트는 사라지고 속성만 남는다
	w.dbInfo.Snapshots = 2
	st = w.statusWithDB()
	if st.DBHint != "" || st.DBSnapshots != 2 {
		t.Fatalf("hint must clear once snapshots exist: %+v", st)
	}
}
