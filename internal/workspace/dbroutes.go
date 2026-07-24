package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Salvia95/graphin/internal/ignore"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/parse"
	"github.com/Salvia95/graphin/internal/scan"
)

// graphindb manifest routing: a committed `graphindb.json` declares which
// SSOT files (schema.sql, schema.prisma, project JSON…) parse as DB nodes
// and how. State lives behind dbMu because parse workers read routes while
// the watcher may be reloading the manifest.

// routeFor resolves the manifest route for one file ("" separator paths).
func (w *Workspace) routeFor(rel string) *parse.DBRoute {
	w.dbMu.RLock()
	defer w.dbMu.RUnlock()
	return w.dbRoutes[rel]
}

// parseFile is the single parse entry for the indexer: manifest-routed files
// extract DB nodes, everything else parses by surface language.
func (w *Workspace) parseFile(rel string, src []byte) (*parse.FileResult, error) {
	return parse.FileWithRoute(rel, src, w.routeFor(rel))
}

// setDBStateFromScan picks the manifest (lexicographically first candidate;
// others warned), loads routes, and refreshes the trace heuristic. Called
// from Bootstrap and from manifest-triggered reloads.
func (w *Workspace) setDBStateFromScan(files []scan.FileInfo) {
	var manifests []string
	for _, f := range files {
		if parse.IsDBManifest(f.RelPath) {
			manifests = append(manifests, f.RelPath)
		}
	}
	sort.Strings(manifests)

	routes := map[string]*parse.DBRoute{}
	var errs []string
	active := ""
	if len(manifests) > 0 {
		active = manifests[0]
		for _, extra := range manifests[1:] {
			errs = append(errs, "ignored extra manifest "+extra+" (active: "+active+")")
			w.Log.Event("db_manifest_ignored", map[string]any{"path": extra, "active": active})
		}
		src, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(active)))
		if err != nil {
			errs = append(errs, "manifest "+active+": "+err.Error())
		} else if m, lerrs := parse.LoadDBManifest(src); m == nil {
			errs = append(errs, lerrs...)
		} else {
			var rerrs []string
			routes, rerrs = m.Routes()
			errs = append(errs, rerrs...)
		}
	}

	w.dbMu.Lock()
	w.dbRoutes = routes
	w.dbManifest = active
	w.dbManifestErrs = errs
	w.dbInfo = detectDB(files)
	w.dbMu.Unlock()
	if len(errs) > 0 {
		w.Log.Event("db_manifest_errors", map[string]any{"path": active, "errors": errs})
	}
}

// reloadDBRoutesLocked re-walks the tree after a manifest change, diffs the
// routing table, and force-reindexes every file whose route changed (removed
// routes degrade the file back to a plain node). Callers hold w.indexMu.
func (w *Workspace) reloadDBRoutesLocked(m *ignore.Matcher) int {
	res, err := scan.Walk(w.Root, w.Log)
	if err != nil {
		return 0
	}
	w.dbMu.RLock()
	old := w.dbRoutes
	w.dbMu.RUnlock()
	w.setDBStateFromScan(res.Files)
	w.dbMu.RLock()
	nw := w.dbRoutes
	w.dbMu.RUnlock()

	affected := map[string]bool{}
	for p, r := range old {
		if !reflect.DeepEqual(nw[p], r) {
			affected[p] = true
		}
	}
	for p, r := range nw {
		if !reflect.DeepEqual(old[p], r) {
			affected[p] = true
		}
	}
	touched := 0
	for _, p := range sortedStrings(affected) {
		abs := filepath.Join(w.Root, filepath.FromSlash(p))
		fi, err := os.Stat(abs)
		if err != nil || fi.IsDir() {
			continue
		}
		// 라우트 변경은 파일 바이트가 그대로여도 파스 결과를 바꾼다 —
		// merkle 항목을 지워 unchanged-skip(§indexPathLocked)을 우회한다.
		w.removeFileLocked(p)
		if w.indexPathLocked(p, abs, fi.Size(), m) {
			touched++
		}
	}
	if touched > 0 {
		w.Log.Event("db_reroute", map[string]any{"files": touched})
	}
	return touched
}

func sortedStrings(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// statusWithDB decorates Status with the graphindb heuristic and manifest
// state for the agent feedback loop.
func (w *Workspace) statusWithDB() mcp.Status {
	w.dbMu.RLock()
	defer w.dbMu.RUnlock()
	st := w.Status()
	st.DBSources = strings.Join(w.dbInfo.Sources, ", ")
	st.DBSnapshots = w.dbInfo.Snapshots
	st.DBManifestErrors = strings.Join(w.dbManifestErrs, "; ")
	if len(w.dbInfo.Sources) > 0 && w.dbInfo.Snapshots == 0 && w.dbManifest == "" {
		st.DBHint = "Database traces detected but no schema source is wired up, so tables and " +
			"foreign keys are not indexed. Either commit *.graphindb.json snapshots " +
			"(schema/graphindb.md; `graphin dbimport`), or — if this repo already has a schema " +
			"SSOT (schema.sql dump, prisma schema, a schema JSON) — declare it in a `graphindb.json` " +
			"manifest with formats sql|schema|json and graphin will parse it in place. " +
			"Manifest problems surface in the db_manifest_errors attribute."
	}
	return st
}
