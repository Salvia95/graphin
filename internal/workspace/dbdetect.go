package workspace

import (
	pathpkg "path"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/scan"
)

// DBInfo summarizes the graphindb bootstrap heuristic: RDB traces present in
// the scanned tree vs snapshot files that make them indexable. graphin never
// connects to a database — when traces exist without a snapshot, the agent
// (or CI) is expected to generate one and commit it (schema/graphindb.md).
type DBInfo struct {
	Sources   []string // trace labels, dedup+sorted, capped
	Snapshots int      // *.graphindb.json files found
}

const maxDBSources = 5

// detectDB inspects already-enumerated scan results only (paths, no file
// reads), so it costs one metadata walk.
func detectDB(files []scan.FileInfo) DBInfo {
	var info DBInfo
	seen := map[string]bool{}
	add := func(label string) {
		if label != "" && !seen[label] {
			seen[label] = true
			info.Sources = append(info.Sources, label)
		}
	}
	for _, f := range files {
		rel := f.RelPath
		if strings.HasSuffix(rel, ".graphindb.json") {
			info.Snapshots++
			continue
		}
		base := pathpkg.Base(rel)
		switch base {
		case "schema.prisma":
			add("prisma")
			continue
		case "flyway.conf":
			add("flyway")
			continue
		case "liquibase.properties":
			add("liquibase")
			continue
		case "alembic.ini":
			add("alembic")
			continue
		case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
			add("docker-compose")
			continue
		}
		// migration directories: any path segment named migration(s) that
		// actually holds .sql files (flyway/supabase/rails/사내 관례 공통).
		if strings.HasSuffix(rel, ".sql") {
			if dir := migrationDir(rel); dir != "" {
				add(dir)
			}
		}
	}
	sort.Strings(info.Sources)
	if len(info.Sources) > maxDBSources {
		info.Sources = info.Sources[:maxDBSources]
	}
	return info
}

// migrationDir returns the path up to and including the migration(s) segment,
// or "" when the path has none.
func migrationDir(rel string) string {
	segs := strings.Split(rel, "/")
	for i, s := range segs[:len(segs)-1] {
		if s == "migrations" || s == "migration" {
			return strings.Join(segs[:i+1], "/")
		}
	}
	return ""
}
