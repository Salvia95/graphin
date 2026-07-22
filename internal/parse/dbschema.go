package parse

// graphindb snapshot extractor (schema/graphindb.md v1). Snapshot files are
// committed to the repo by external generators (tbls/Atlas/dbimport/hand);
// graphin never connects to a live database. Tables, views, functions,
// procedures, RLS bundles and triggers become individual nodes whose byte
// spans point into the snapshot JSON, so merkle 2-Track, read_code slicing
// and BodyTokens indexing all apply unchanged.

import (
	"bytes"
	"encoding/json"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

const graphindbSuffix = ".graphindb.json"

// ---- snapshot shapes; unknown JSON fields are ignored (전방 호환) ----

type dbMeta struct {
	Datasource string `json:"datasource"`
}

type dbColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type dbForeignKey struct {
	References string `json:"references"`
	Enforced   *bool  `json:"enforced"` // nil/true: physical FK, false: logical
}

type dbTable struct {
	Columns     []dbColumn     `json:"columns"`
	ForeignKeys []dbForeignKey `json:"foreign_keys"`
}

type dbView struct {
	Definition string   `json:"definition"`
	References []string `json:"references"`
}

type dbRoutine struct {
	Args       string   `json:"args"`
	Definition string   `json:"definition"`
	References []string `json:"references"`
}

type dbPolicy struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type dbRLSTable struct {
	Enabled  bool       `json:"enabled"`
	Policies []dbPolicy `json:"policies"`
}

type dbTrigger struct {
	Name     string `json:"name"`
	Function string `json:"function"`
}

type dbMainSchema struct {
	Tables map[string]dbTable `json:"tables"`
	Views  map[string]dbView  `json:"views"`
}

type dbRoutineSchema struct {
	Functions  map[string]dbRoutine `json:"functions"`
	Procedures map[string]dbRoutine `json:"procedures"`
}

type dbMainFile struct {
	Meta    dbMeta                  `json:"_meta"`
	Schemas map[string]dbMainSchema `json:"schemas"`
}

type dbRoutineFile struct {
	Meta    dbMeta                     `json:"_meta"`
	Schemas map[string]dbRoutineSchema `json:"schemas"`
}

type dbRLSFile struct {
	Meta    dbMeta                           `json:"_meta"`
	Schemas map[string]map[string]dbRLSTable `json:"schemas"`
}

type dbTriggerFile struct {
	Meta    dbMeta                            `json:"_meta"`
	Schemas map[string]map[string][]dbTrigger `json:"schemas"`
}

// extractDBSchema dispatches one snapshot file by its section suffix. A file
// that fails to decode degrades to a plain file node with Partial set, so a
// broken snapshot stays searchable instead of vanishing from the index.
func extractDBSchema(src []byte, res *FileResult) {
	section, stem := dbSection(res.RelPath)
	ok := false
	switch section {
	case "":
		ok = extractDBMain(src, stem, res)
	case "functions":
		ok = extractDBRoutines(src, stem, res)
	case "rls":
		ok = extractDBRLS(src, stem, res)
	case "triggers":
		ok = extractDBTriggers(src, stem, res)
	}
	if !ok {
		res.Partial = true
		extractPlain(src, res)
	}
}

// dbSection splits "<name>[.<section>].graphindb.json" into section ("" for
// the main file) and the datasource fallback stem.
func dbSection(relPath string) (section, stem string) {
	stem = strings.TrimSuffix(pathpkg.Base(relPath), graphindbSuffix)
	for _, s := range []string{"functions", "rls", "triggers"} {
		if strings.HasSuffix(stem, "."+s) {
			return s, strings.TrimSuffix(stem, "."+s)
		}
	}
	return "", stem
}

// dbDatasource: _meta.datasource is authoritative, filename stem is the
// fallback (schema/graphindb.md 파일 규약).
func dbDatasource(meta dbMeta, stem string) string {
	if meta.Datasource != "" {
		return meta.Datasource
	}
	if stem == "" {
		return "unknown"
	}
	return stem
}

// normalizeDBRef canonicalizes "[db.<ds>.]<schema>.<name>[.<column>]" to the
// target node FQN "db.<ds>.<schema>.<name>", dropping a trailing column.
func normalizeDBRef(ds, ref string) (string, bool) {
	segs := strings.Split(strings.TrimSpace(ref), ".")
	if segs[0] == "db" {
		if len(segs) < 4 {
			return "", false
		}
		return strings.Join(segs[:4], "."), true
	}
	if len(segs) < 2 || segs[0] == "" {
		return "", false
	}
	return "db." + ds + "." + segs[0] + "." + segs[1], true
}

func extractDBMain(src []byte, stem string, res *FileResult) bool {
	var f dbMainFile
	if json.Unmarshal(src, &f) != nil || f.Schemas == nil {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := dbDatasource(f.Meta, stem)
	res.Package = "db." + ds
	for _, schema := range sortedKeys(f.Schemas) {
		sc := f.Schemas[schema]
		for _, tbl := range sortedKeys(sc.Tables) {
			sp, ok := spans["schemas/"+schema+"/tables/"+tbl]
			if !ok {
				continue
			}
			n := dbNode(src, sp, nodeid.KindTable, ds, schema, tbl)
			t := sc.Tables[tbl]
			for _, c := range t.Columns {
				n.Params = append(n.Params, strings.TrimSpace(c.Name+" "+c.Type))
			}
			for _, fk := range t.ForeignKeys {
				fqn, ok := normalizeDBRef(ds, fk.References)
				if !ok {
					continue
				}
				if fk.Enforced != nil && !*fk.Enforced {
					n.LogicalRefs = append(n.LogicalRefs, fqn)
				} else {
					n.Supers = append(n.Supers, fqn)
				}
			}
			res.Nodes = append(res.Nodes, n)
		}
		for _, vw := range sortedKeys(sc.Views) {
			sp, ok := spans["schemas/"+schema+"/views/"+vw]
			if !ok {
				continue
			}
			n := dbNode(src, sp, nodeid.KindView, ds, schema, vw)
			dbRefsOrDefTokens(&n, ds, sc.Views[vw].References, sc.Views[vw].Definition)
			res.Nodes = append(res.Nodes, n)
		}
	}
	return true
}

func extractDBRoutines(src []byte, stem string, res *FileResult) bool {
	var f dbRoutineFile
	if json.Unmarshal(src, &f) != nil || f.Schemas == nil {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := dbDatasource(f.Meta, stem)
	res.Package = "db." + ds
	for _, schema := range sortedKeys(f.Schemas) {
		sc := f.Schemas[schema]
		emit := func(section, kind string, routines map[string]dbRoutine) {
			for _, name := range sortedKeys(routines) {
				sp, ok := spans["schemas/"+schema+"/"+section+"/"+name]
				if !ok {
					continue
				}
				n := dbNode(src, sp, kind, ds, schema, name)
				r := routines[name]
				if r.Args != "" {
					n.Params = []string{r.Args}
				}
				dbRefsOrDefTokens(&n, ds, r.References, r.Definition)
				res.Nodes = append(res.Nodes, n)
			}
		}
		emit("functions", nodeid.KindDBFunction, sc.Functions)
		emit("procedures", nodeid.KindProcedure, sc.Procedures)
	}
	return true
}

func extractDBRLS(src []byte, stem string, res *FileResult) bool {
	var f dbRLSFile
	if json.Unmarshal(src, &f) != nil || f.Schemas == nil {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := dbDatasource(f.Meta, stem)
	res.Package = "db." + ds
	for _, schema := range sortedKeys(f.Schemas) {
		for _, tbl := range sortedKeys(f.Schemas[schema]) {
			rls := f.Schemas[schema][tbl]
			if !rls.Enabled && len(rls.Policies) == 0 {
				continue
			}
			sp, ok := spans["schemas/"+schema+"/"+tbl]
			if !ok {
				continue
			}
			tableFQN := nodeid.DBNode(ds, schema, tbl)
			n := Node{
				ID:          tableFQN + ".rls",
				DisplayName: nodeid.DBDisplay(ds, schema, tbl) + ".rls",
				SimpleName:  tbl, // Tier-0: 테이블명 검색에 정책 묶음도 걸린다
				Kind:        nodeid.KindRLSPolicy,
				Container:   schema + "." + tbl,
				StartByte:   sp.start,
				EndByte:     sp.end,
				Hash:        blake3.Sum256(src[sp.start:sp.end]),
				Supers:      []string{tableFQN},
			}
			for _, p := range rls.Policies {
				n.Params = append(n.Params, strings.TrimSpace(p.Name+" "+p.Command))
			}
			res.Nodes = append(res.Nodes, n)
		}
	}
	return true
}

func extractDBTriggers(src []byte, stem string, res *FileResult) bool {
	var f dbTriggerFile
	if json.Unmarshal(src, &f) != nil || f.Schemas == nil {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := dbDatasource(f.Meta, stem)
	res.Package = "db." + ds
	for _, schema := range sortedKeys(f.Schemas) {
		for _, tbl := range sortedKeys(f.Schemas[schema]) {
			tableFQN := nodeid.DBNode(ds, schema, tbl)
			for i, trg := range f.Schemas[schema][tbl] {
				if trg.Name == "" {
					continue
				}
				sp, ok := spans["schemas/"+schema+"/"+tbl+"/#"+strconv.Itoa(i)]
				if !ok {
					continue
				}
				n := Node{
					ID:          tableFQN + "." + trg.Name,
					DisplayName: nodeid.DBDisplay(ds, schema, tbl) + "." + trg.Name,
					SimpleName:  trg.Name,
					Kind:        nodeid.KindTrigger,
					Container:   schema + "." + tbl,
					StartByte:   sp.start,
					EndByte:     sp.end,
					Hash:        blake3.Sum256(src[sp.start:sp.end]),
					Supers:      []string{tableFQN},
				}
				if fqn, ok := normalizeDBRef(ds, trg.Function); ok {
					n.Supers = append(n.Supers, fqn)
				}
				res.Nodes = append(res.Nodes, n)
			}
		}
	}
	return true
}

// dbNode builds the common shell for schema-level objects (table, view,
// function, procedure) whose span is their JSON block in the snapshot.
func dbNode(src []byte, sp dbSpan, kind, ds, schema, name string) Node {
	return Node{
		ID:          nodeid.DBNode(ds, schema, name),
		DisplayName: nodeid.DBDisplay(ds, schema, name),
		SimpleName:  name,
		Kind:        kind,
		Container:   schema,
		StartByte:   sp.start,
		EndByte:     sp.end,
		Hash:        blake3.Sum256(src[sp.start:sp.end]),
	}
}

// dbRefsOrDefTokens fills reference material for view/function/procedure
// nodes: explicit references become Supers (resolved at conf 1.0); otherwise
// definition identifier tokens become Calls for the heuristic tier (명세:
// 명시가 항상 우선).
func dbRefsOrDefTokens(n *Node, ds string, refs []string, definition string) {
	if len(refs) > 0 {
		for _, r := range refs {
			if fqn, ok := normalizeDBRef(ds, r); ok {
				n.Supers = append(n.Supers, fqn)
			}
		}
		return
	}
	for _, tok := range dbDefTokens(definition) {
		n.Calls = append(n.Calls, Call{Name: tok, Args: -1})
	}
}

// maxDBDefTokens bounds heuristic candidates per definition.
const maxDBDefTokens = 32

// sqlStop drops SQL/PLpgSQL keywords from definition token extraction.
var sqlStop = map[string]bool{
	"select": true, "from": true, "where": true, "and": true, "not": true,
	"null": true, "insert": true, "update": true, "delete": true, "set": true,
	"into": true, "values": true, "join": true, "left": true, "right": true,
	"inner": true, "outer": true, "full": true, "cross": true, "using": true,
	"group": true, "order": true, "having": true, "limit": true, "offset": true,
	"distinct": true, "union": true, "all": true, "exists": true, "between": true,
	"like": true, "ilike": true, "case": true, "when": true, "then": true,
	"else": true, "end": true, "begin": true, "declare": true, "return": true,
	"returns": true, "returning": true, "new": true, "old": true, "for": true,
	"each": true, "row": true, "execute": true, "function": true, "procedure": true,
	"trigger": true, "language": true, "security": true, "definer": true,
	"invoker": true, "create": true, "replace": true, "alter": true, "drop": true,
	"table": true, "view": true, "index": true, "primary": true, "key": true,
	"foreign": true, "references": true, "constraint": true, "unique": true,
	"check": true, "default": true, "cascade": true, "restrict": true,
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"coalesce": true, "nullif": true, "now": true, "true": true, "false": true,
	"with": true, "recursive": true, "integer": true, "bigint": true,
	"boolean": true, "text": true, "varchar": true, "date": true,
	"timestamptz": true, "timestamp": true, "interval": true,
}

// dbDefTokens pulls identifier-looking tokens (≥3 chars, keyword-filtered,
// order-preserving unique) from a definition body as heuristic edge
// candidates. Matching against actual table defs happens at resolve time.
func dbDefTokens(def string) []string {
	lower := strings.ToLower(def)
	seen := map[string]bool{}
	var out []string
	for i := 0; i < len(lower) && len(out) < maxDBDefTokens; {
		c := lower[i]
		if c != '_' && (c < 'a' || c > 'z') {
			i++
			continue
		}
		j := i + 1
		for j < len(lower) {
			d := lower[j]
			if d != '_' && (d < 'a' || d > 'z') && (d < '0' || d > '9') {
				break
			}
			j++
		}
		if tok := lower[i:j]; len(tok) >= 3 && !sqlStop[tok] && !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
		i = j
	}
	return out
}

// ---- byte-span walker ----

type dbSpan struct{ start, end uint32 }

// dbValueSpans records the exact byte span of every JSON object/array value,
// keyed by "/"-joined path ("schemas/public/tables/job_posting", array
// elements as "#<i>"). Spans feed node offsets so merkle subtree hashing and
// read_code slicing work per-object, not per-file.
func dbValueSpans(src []byte) (map[string]dbSpan, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	spans := map[string]dbSpan{}
	var value func(path string) error
	value = func(path string) error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		d, isDelim := tok.(json.Delim)
		if !isDelim {
			return nil // scalar: no span needed
		}
		start := uint32(dec.InputOffset() - 1) // InputOffset is just past the 1-byte delim
		join := func(seg string) string {
			if path == "" {
				return seg
			}
			return path + "/" + seg
		}
		if d == '{' {
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key, _ := keyTok.(string)
				if err := value(join(key)); err != nil {
					return err
				}
			}
		} else {
			for i := 0; dec.More(); i++ {
				if err := value(join("#" + strconv.Itoa(i))); err != nil {
					return err
				}
			}
		}
		if _, err := dec.Token(); err != nil { // consume closing delim
			return err
		}
		spans[path] = dbSpan{start: start, end: uint32(dec.InputOffset())}
		return nil
	}
	if err := value(""); err != nil {
		return nil, err
	}
	return spans, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
