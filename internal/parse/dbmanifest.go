package parse

// graphindb manifest (schema/graphindb.md §매니페스트): the repo commits an
// SSOT that already exists (schema.sql dump, prisma schema, project JSON) and
// a manifest routes those files to format-specific extractors. The SSOT file
// itself is the parse target, so node spans point at real DDL/model blocks.

import (
	"encoding/json"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"
)

// DBManifestBasename is the fixed manifest file name (any directory). It is
// distinct from the inline-snapshot *suffix* ".graphindb.json" — a basename
// of exactly "graphindb.json" never carries the leading dot of the suffix.
const DBManifestBasename = "graphindb.json"

// IsDBManifest reports whether relPath is a manifest candidate.
func IsDBManifest(relPath string) bool {
	return pathpkg.Base(relPath) == DBManifestBasename
}

type DBManifest struct {
	Version     int                         `json:"version"`
	Datasources map[string]*DBDatasourceCfg `json:"datasources"`
}

type DBDatasourceCfg struct {
	Engine        string        `json:"engine"`
	DefaultSchema string        `json:"default_schema"`
	Sources       []DBSourceCfg `json:"sources"`
	SQL           *DBSQLRules   `json:"sql"`
	JSON          *DBJSONRules  `json:"json"`
}

type DBSourceCfg struct {
	Path   string `json:"path"`   // repo-relative, "/"-separated, exact (no glob)
	Format string `json:"format"` // sql | schema | json | graphindb; "" → infer
}

type DBSQLRules struct {
	Dialect string `json:"dialect"`
}

type DBJSONRules struct {
	Preset  string         `json:"preset"` // "tbls"
	Mapping *DBJSONMapping `json:"mapping"`
}

// DBJSONMapping is the minimal selector DSL for project-native JSON SSOTs.
// Selectors: dot-separated segments; "seg[]" iterates an array under key seg;
// "*" iterates object keys (capturing the key); leaf getters are "key",
// "field:<name>" (dots allowed for nesting) and "const:<value>".
type DBJSONMapping struct {
	Tables     string `json:"tables"`        // e.g. "schemas.*.tables.*" | "tables.*" | "tables[]"
	TableName  string `json:"table_name"`    // default "key" for *, "field:name" for []
	Columns    string `json:"columns"`       // table-relative, e.g. "columns[]"
	ColumnName string `json:"column_name"`   // default "field:name" ("key" for ".*")
	ColumnType string `json:"column_type"`   // default "field:type"
	FKs        string `json:"foreign_keys"`  // table-relative, e.g. "foreign_keys[]"
	FKRef      string `json:"fk_references"` // default "field:references"
	FKEnforced string `json:"fk_enforced"`   // optional, e.g. "field:enforced"
}

// DBRoute is one file's resolved parsing instruction.
type DBRoute struct {
	Datasource    string
	Engine        string
	DefaultSchema string
	Format        string
	SQL           *DBSQLRules
	JSON          *DBJSONRules
}

// LoadDBManifest parses and validates a manifest. Structural errors are
// returned as agent-readable strings (they surface via db_manifest_errors);
// valid datasources still load so the agent can iterate incrementally.
func LoadDBManifest(src []byte) (*DBManifest, []string) {
	var m DBManifest
	if err := json.Unmarshal(src, &m); err != nil {
		return nil, []string{"manifest: invalid JSON: " + err.Error()}
	}
	if len(m.Datasources) == 0 {
		return nil, []string{"manifest: no datasources declared"}
	}
	return &m, nil
}

// Routes flattens the manifest into path → route. Later datasources (sorted
// order) never steal a path claimed earlier; conflicts are reported.
func (m *DBManifest) Routes() (map[string]*DBRoute, []string) {
	routes := map[string]*DBRoute{}
	var errs []string
	for _, ds := range sortedKeys(m.Datasources) {
		cfg := m.Datasources[ds]
		if cfg == nil || len(cfg.Sources) == 0 {
			errs = append(errs, fmt.Sprintf("datasource %q: no sources", ds))
			continue
		}
		defSchema := cfg.DefaultSchema
		if defSchema == "" {
			if strings.EqualFold(cfg.Engine, "postgresql") || strings.EqualFold(cfg.Engine, "postgres") {
				defSchema = "public"
			} else {
				defSchema = "main"
			}
		}
		for _, s := range cfg.Sources {
			path := strings.TrimPrefix(pathpkg.Clean(s.Path), "./")
			if path == "" || path == "." || strings.HasPrefix(path, "..") {
				errs = append(errs, fmt.Sprintf("datasource %q: bad source path %q", ds, s.Path))
				continue
			}
			format := s.Format
			if format == "" {
				format = inferDBFormat(path)
			}
			switch format {
			case "sql", "schema", "graphindb":
			case "json":
				if cfg.JSON == nil || (cfg.JSON.Preset == "" && cfg.JSON.Mapping == nil) {
					errs = append(errs, fmt.Sprintf("datasource %q: format json needs json.preset or json.mapping", ds))
					continue
				}
				if cfg.JSON.Preset != "" && cfg.JSON.Preset != "tbls" {
					errs = append(errs, fmt.Sprintf("datasource %q: unknown json preset %q (supported: tbls)", ds, cfg.JSON.Preset))
					continue
				}
			default:
				errs = append(errs, fmt.Sprintf("datasource %q: unknown format %q for %s (formats: sql, schema, json, graphindb)", ds, format, path))
				continue
			}
			if prev, dup := routes[path]; dup {
				errs = append(errs, fmt.Sprintf("path %s claimed by datasources %q and %q; keeping %q", path, prev.Datasource, ds, prev.Datasource))
				continue
			}
			routes[path] = &DBRoute{
				Datasource: ds, Engine: cfg.Engine, DefaultSchema: defSchema,
				Format: format, SQL: cfg.SQL, JSON: cfg.JSON,
			}
		}
	}
	return routes, errs
}

func inferDBFormat(path string) string {
	switch {
	case strings.HasSuffix(path, graphindbSuffix):
		return "graphindb"
	case strings.HasSuffix(path, ".sql"):
		return "sql"
	case strings.HasSuffix(path, ".prisma"):
		return "schema"
	case strings.HasSuffix(path, ".json"):
		return "json"
	}
	return ""
}

// SortedRoutePaths is a deterministic view for diffing/logging.
func SortedRoutePaths(routes map[string]*DBRoute) []string {
	out := make([]string, 0, len(routes))
	for p := range routes {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
