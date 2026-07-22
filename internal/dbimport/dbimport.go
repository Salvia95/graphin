// Package dbimport converts external schema-introspection output (tbls JSON)
// into graphindb snapshot files (schema/graphindb.md), and scaffolds empty
// snapshots. It is a pure file transform: graphin never connects to a
// database — 스냅샷 생성 주체는 에이전트/CI다.
package dbimport

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run executes the `graphin dbimport` subcommand. Returns a process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dbimport", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		from     = fs.String("from", "tbls", "input format (only: tbls)")
		ds       = fs.String("datasource", "", "datasource name for node IDs (required)")
		out      = fs.String("out", "db", "output directory for *.graphindb.json files")
		engine   = fs.String("engine", "", "override engine name (default: derived from input driver)")
		initName = fs.String("init", "", "scaffold an empty snapshot for this datasource and exit")
		syncedAt = fs.String("synced-at", "", "override _meta.synced_at (default: now, RFC3339)")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	stamp := *syncedAt
	if stamp == "" {
		stamp = time.Now().UTC().Format(time.RFC3339)
	}

	if *initName != "" {
		if err := runInit(*initName, *out, *engine, stamp); err != nil {
			fmt.Fprintf(stderr, "dbimport: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "scaffolded %s\n", filepath.Join(*out, *initName+".graphindb.json"))
		return 0
	}

	if *from != "tbls" {
		fmt.Fprintf(stderr, "dbimport: unsupported --from %q (supported: tbls)\n", *from)
		return 2
	}
	if *ds == "" {
		fmt.Fprintln(stderr, "dbimport: --datasource is required")
		return 2
	}
	src, err := readInput(fs.Arg(0), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 1
	}
	written, err := convertTbls(src, *ds, *out, *engine, stamp)
	if err != nil {
		fmt.Fprintf(stderr, "dbimport: %v\n", err)
		return 1
	}
	for _, w := range written {
		fmt.Fprintln(stdout, w)
	}
	return 0
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func runInit(name, out, engine, stamp string) error {
	if engine == "" {
		engine = "postgresql"
	}
	path := filepath.Join(out, name+".graphindb.json")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; refusing to overwrite a scaffold target", path)
	}
	doc := map[string]any{
		"_meta": map[string]any{
			"datasource": name, "engine": engine,
			"source": "manual", "synced_at": stamp,
		},
		// _readme is ignored by the parser (unknown fields are forward-
		// compatible) but tells whoever opens the scaffold what to do next.
		"_readme": "Fill schemas.<schema>.tables/views per schema/graphindb.md. " +
			"Sidecars: " + name + ".functions|.rls|.triggers.graphindb.json. " +
			"FK format: [db.<ds>.]<schema>.<table>[.<column>]; " +
			"logical/polymorphic refs use enforced:false + note.",
		"schemas": map[string]any{},
	}
	return writeJSON(path, doc)
}

// ---- tbls schema.json (defensive subset; unknown fields are ignored) ----

type tblsSchema struct {
	Name   string `json:"name"`
	Driver struct {
		Name string `json:"name"`
	} `json:"driver"`
	Tables []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Comment string `json:"comment"`
		Def     string `json:"def"`
		Columns []struct {
			Name     string          `json:"name"`
			Type     string          `json:"type"`
			Nullable bool            `json:"nullable"`
			Default  json.RawMessage `json:"default"`
			Comment  string          `json:"comment"`
		} `json:"columns"`
		Indexes []struct {
			Name string `json:"name"`
			Def  string `json:"def"`
		} `json:"indexes"`
		Constraints []struct {
			Name              string   `json:"name"`
			Type              string   `json:"type"`
			Def               string   `json:"def"`
			ReferencedTable   string   `json:"referenced_table"`
			Columns           []string `json:"columns"`
			ReferencedColumns []string `json:"referenced_columns"`
		} `json:"constraints"`
		Triggers []struct {
			Name string `json:"name"`
			Def  string `json:"def"`
		} `json:"triggers"`
	} `json:"tables"`
	Relations []struct {
		Table         string   `json:"table"`
		Columns       []string `json:"columns"`
		ParentTable   string   `json:"parent_table"`
		ParentColumns []string `json:"parent_columns"`
		Def           string   `json:"def"`
		Virtual       bool     `json:"virtual"`
	} `json:"relations"`
	Functions []struct {
		Name       string `json:"name"`
		ReturnType string `json:"return_type"`
		Arguments  string `json:"arguments"`
		Type       string `json:"type"`
	} `json:"functions"`
}

func convertTbls(src []byte, ds, out, engineOverride, stamp string) ([]string, error) {
	var in tblsSchema
	if err := json.Unmarshal(src, &in); err != nil {
		return nil, fmt.Errorf("parse tbls schema: %w", err)
	}
	engine := engineOverride
	if engine == "" {
		engine = engineName(in.Driver.Name)
	}
	defSchema := defaultSchema(in.Driver.Name)
	meta := func(source string) map[string]any {
		return map[string]any{
			"datasource": ds, "engine": engine, "source": source,
			"synced_at": stamp, "generator": "graphin dbimport (tbls)",
		}
	}

	// relations are keyed by child table; when tbls emitted them they are the
	// FK source of truth (virtual → enforced:false), else fall back to the
	// per-table FOREIGN KEY constraints.
	relByTable := map[string][]map[string]any{}
	for _, r := range in.Relations {
		fk := map[string]any{
			"column":     strings.Join(r.Columns, ","),
			"references": refString(defSchema, r.ParentTable, r.ParentColumns),
		}
		if od := onDelete(r.Def); od != "" {
			fk["on_delete"] = od
		}
		if r.Virtual {
			fk["enforced"] = false
			fk["note"] = "tbls virtual relation (comment/detected, no physical FK)"
		}
		relByTable[r.Table] = append(relByTable[r.Table], fk)
	}

	schemas := map[string]map[string]any{}   // schema → {"tables": …, "views": …}
	trgSchemas := map[string]map[string]any{} // schema → table → [trigger]
	section := func(m map[string]map[string]any, schema, key string) map[string]any {
		sc := m[schema]
		if sc == nil {
			sc = map[string]any{}
			m[schema] = sc
		}
		e, _ := sc[key].(map[string]any)
		if e == nil {
			e = map[string]any{}
			sc[key] = e
		}
		return e
	}

	for _, t := range in.Tables {
		schema, name := splitQualified(t.Name, defSchema)
		if strings.Contains(strings.ToUpper(t.Type), "VIEW") {
			v := map[string]any{}
			if t.Comment != "" {
				v["comment"] = t.Comment
			}
			if t.Def != "" {
				v["definition"] = t.Def
			}
			section(schemas, schema, "views")[name] = v
			continue
		}

		entry := map[string]any{}
		if t.Comment != "" {
			entry["comment"] = t.Comment
		}
		pk := map[string]bool{}
		for _, c := range t.Constraints {
			if strings.EqualFold(c.Type, "PRIMARY KEY") {
				for _, col := range c.Columns {
					pk[col] = true
				}
			}
		}
		var cols []any
		for _, c := range t.Columns {
			col := map[string]any{"name": c.Name, "type": c.Type, "nullable": c.Nullable}
			if d := rawString(c.Default); d != "" {
				col["default"] = d
			}
			if c.Comment != "" {
				col["comment"] = c.Comment
			}
			if pk[c.Name] {
				col["constraints"] = []any{"PK"}
			}
			cols = append(cols, col)
		}
		if len(cols) > 0 {
			entry["columns"] = cols
		}

		fks := relByTable[t.Name]
		if fks == nil {
			fks = relByTable[name]
		}
		if len(fks) == 0 {
			for _, c := range t.Constraints {
				if !strings.EqualFold(c.Type, "FOREIGN KEY") {
					continue
				}
				fk := map[string]any{
					"column":     strings.Join(c.Columns, ","),
					"references": refString(defSchema, c.ReferencedTable, c.ReferencedColumns),
				}
				if od := onDelete(c.Def); od != "" {
					fk["on_delete"] = od
				}
				fks = append(fks, fk)
			}
		}
		if len(fks) > 0 {
			entry["foreign_keys"] = fks
		}

		var idxs []any
		for _, ix := range t.Indexes {
			idxs = append(idxs, map[string]any{"name": ix.Name, "definition": ix.Def})
		}
		if len(idxs) > 0 {
			entry["indexes"] = idxs
		}
		section(schemas, schema, "tables")[name] = entry

		for _, trg := range t.Triggers {
			e := map[string]any{"name": trg.Name}
			if trg.Def != "" {
				e["definition"] = trg.Def
			}
			if fn := triggerFunction(trg.Def, defSchema); fn != "" {
				e["function"] = fn
			}
			sc := trgSchemas[schema]
			if sc == nil {
				sc = map[string]any{}
				trgSchemas[schema] = sc
			}
			list, _ := sc[name].([]any)
			sc[name] = append(list, e)
		}
	}

	fnSchemas := map[string]map[string]any{} // schema → functions/procedures
	for _, f := range in.Functions {
		schema, name := splitQualified(f.Name, defSchema)
		kind := "functions"
		if strings.Contains(strings.ToUpper(f.Type), "PROCEDURE") {
			kind = "procedures"
		}
		e := map[string]any{}
		if f.Arguments != "" {
			e["args"] = f.Arguments
		}
		if f.ReturnType != "" {
			e["returns"] = f.ReturnType
		}
		section(fnSchemas, schema, kind)[name] = e
	}

	var written []string
	write := func(suffix string, schemas any, empty bool) error {
		if empty {
			return nil
		}
		path := filepath.Join(out, ds+suffix+".graphindb.json")
		if err := writeJSON(path, map[string]any{"_meta": meta("tbls"), "schemas": schemas}); err != nil {
			return err
		}
		written = append(written, path)
		return nil
	}
	if err := write("", schemas, false); err != nil {
		return nil, err
	}
	if err := write(".functions", fnSchemas, len(fnSchemas) == 0); err != nil {
		return nil, err
	}
	if err := write(".triggers", trgSchemas, len(trgSchemas) == 0); err != nil {
		return nil, err
	}
	return written, nil
}

// writeJSON emits deterministic output: MarshalIndent sorts map keys, two-
// space indent, one trailing newline (스펙 §결정성).
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func splitQualified(name, defSchema string) (schema, simple string) {
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i], name[i+1:]
	}
	return defSchema, name
}

func refString(defSchema, parent string, parentCols []string) string {
	schema, name := splitQualified(parent, defSchema)
	ref := schema + "." + name
	if len(parentCols) == 1 {
		ref += "." + parentCols[0]
	}
	return ref
}

// rawString renders a JSON default value: null/absent → "", quoted strings
// unwrap, anything else keeps its literal text.
func rawString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	var out string
	if json.Unmarshal(raw, &out) == nil {
		return out
	}
	return s
}

func engineName(driver string) string {
	switch strings.ToLower(driver) {
	case "postgres", "postgresql":
		return "postgresql"
	case "sqlite", "sqlite3":
		return "sqlite"
	case "mariadb":
		return "mysql"
	case "":
		return "unknown"
	}
	return strings.ToLower(driver)
}

func defaultSchema(driver string) string {
	switch strings.ToLower(driver) {
	case "postgres", "postgresql":
		return "public"
	}
	return "main"
}

// onDelete extracts "ON DELETE <ACTION>" from an FK definition, best effort.
func onDelete(def string) string {
	u := strings.ToUpper(def)
	i := strings.Index(u, "ON DELETE ")
	if i < 0 {
		return ""
	}
	rest := u[i+len("ON DELETE "):]
	for _, action := range []string{"CASCADE", "SET NULL", "SET DEFAULT", "RESTRICT", "NO ACTION"} {
		if strings.HasPrefix(rest, action) {
			return action
		}
	}
	return ""
}

// triggerFunction extracts the EXECUTE FUNCTION/PROCEDURE target from a
// trigger definition so the parser can emit the trigger→function Call edge.
func triggerFunction(def, defSchema string) string {
	l := strings.ToLower(def)
	for _, kw := range []string{"execute function ", "execute procedure "} {
		i := strings.Index(l, kw)
		if i < 0 {
			continue
		}
		rest := def[i+len(kw):]
		j := strings.IndexByte(rest, '(')
		if j < 0 {
			continue
		}
		name := strings.TrimSpace(rest[:j])
		if name == "" {
			continue
		}
		if !strings.Contains(name, ".") {
			name = defSchema + "." + name
		}
		return name
	}
	return ""
}
