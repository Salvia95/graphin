package parse

// format:"json" preset:"tbls" — parses a tbls `schema.json` SSOT directly
// (no dbimport conversion step). Root-level relations, VIEW discrimination
// and nested triggers don't fit the mapping DSL, so this preset is code.
// The struct mirrors internal/dbimport's defensive tbls subset; the two
// stay separate on purpose (converter vs extractor concerns).

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

type dbTblsFile struct {
	Driver struct {
		Name string `json:"name"`
	} `json:"driver"`
	Tables []struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Def     string `json:"def"`
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
		Constraints []struct {
			Type            string   `json:"type"`
			ReferencedTable string   `json:"referenced_table"`
			Columns         []string `json:"columns"`
		} `json:"constraints"`
		Triggers []struct {
			Name string `json:"name"`
			Def  string `json:"def"`
		} `json:"triggers"`
	} `json:"tables"`
	Relations []struct {
		Table       string `json:"table"`
		ParentTable string `json:"parent_table"`
		Virtual     bool   `json:"virtual"`
	} `json:"relations"`
	Functions []struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Type      string `json:"type"`
	} `json:"functions"`
}

func extractDBTbls(src []byte, route *DBRoute, res *FileResult) bool {
	var f dbTblsFile
	if json.Unmarshal(src, &f) != nil || len(f.Tables) == 0 {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := route.Datasource
	res.Package = "db." + ds
	defSchema := route.DefaultSchema

	split := func(name string) (schema, simple string) {
		if i := strings.IndexByte(name, '.'); i > 0 {
			return name[:i], name[i+1:]
		}
		return defSchema, name
	}
	tableFQN := func(name string) string {
		schema, simple := split(name)
		return nodeid.DBNode(ds, schema, simple)
	}

	// root relations are the FK source of truth when present (virtual →
	// enforced:false), keyed by child table name as written.
	type rel struct {
		target  string
		virtual bool
	}
	relByTable := map[string][]rel{}
	for _, r := range f.Relations {
		relByTable[r.Table] = append(relByTable[r.Table], rel{target: tableFQN(r.ParentTable), virtual: r.Virtual})
	}

	for i, t := range f.Tables {
		sp, ok := spans["tables/#"+strconv.Itoa(i)]
		if !ok {
			continue
		}
		schema, simple := split(t.Name)
		kind := nodeid.KindTable
		if strings.Contains(strings.ToUpper(t.Type), "VIEW") {
			kind = nodeid.KindView
		}
		n := Node{
			ID:          nodeid.DBNode(ds, schema, simple),
			DisplayName: nodeid.DBDisplay(ds, schema, simple),
			SimpleName:  simple,
			Kind:        kind,
			Container:   schema,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(src[sp.start:sp.end]),
		}
		if kind == nodeid.KindView {
			for _, tok := range dbDefTokens(t.Def) {
				n.Calls = append(n.Calls, Call{Name: tok, Args: -1})
			}
		} else {
			for _, c := range t.Columns {
				n.Params = append(n.Params, strings.TrimSpace(c.Name+" "+c.Type))
			}
			rels := relByTable[t.Name]
			if rels == nil {
				rels = relByTable[simple]
			}
			if len(rels) > 0 {
				for _, r := range rels {
					if r.virtual {
						n.LogicalRefs = append(n.LogicalRefs, r.target)
					} else {
						n.Supers = append(n.Supers, r.target)
					}
				}
			} else {
				for _, c := range t.Constraints {
					if strings.EqualFold(c.Type, "FOREIGN KEY") && c.ReferencedTable != "" {
						n.Supers = append(n.Supers, tableFQN(c.ReferencedTable))
					}
				}
			}
		}
		res.Nodes = append(res.Nodes, n)

		myFQN := n.ID
		for j, trg := range t.Triggers {
			if trg.Name == "" || kind != nodeid.KindTable {
				continue
			}
			tsp, ok := spans["tables/#"+strconv.Itoa(i)+"/triggers/#"+strconv.Itoa(j)]
			if !ok {
				continue
			}
			tn := Node{
				ID:          myFQN + "." + trg.Name,
				DisplayName: nodeid.DBDisplay(ds, schema, simple) + "." + trg.Name,
				SimpleName:  trg.Name,
				Kind:        nodeid.KindTrigger,
				Container:   schema + "." + simple,
				StartByte:   tsp.start,
				EndByte:     tsp.end,
				Hash:        blake3.Sum256(src[tsp.start:tsp.end]),
				Supers:      []string{myFQN},
			}
			if fn := dbTblsTriggerFn(trg.Def); fn != "" {
				fnSchema, fnSimple := split(fn)
				tn.Supers = append(tn.Supers, nodeid.DBNode(ds, fnSchema, fnSimple))
			}
			res.Nodes = append(res.Nodes, tn)
		}
	}

	for i, fn := range f.Functions {
		sp, ok := spans["functions/#"+strconv.Itoa(i)]
		if !ok {
			continue
		}
		schema, simple := split(fn.Name)
		kind := nodeid.KindDBFunction
		if strings.Contains(strings.ToUpper(fn.Type), "PROCEDURE") {
			kind = nodeid.KindProcedure
		}
		n := Node{
			ID:          nodeid.DBNode(ds, schema, simple),
			DisplayName: nodeid.DBDisplay(ds, schema, simple),
			SimpleName:  simple,
			Kind:        kind,
			Container:   schema,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(src[sp.start:sp.end]),
		}
		if fn.Arguments != "" {
			n.Params = []string{fn.Arguments}
		}
		res.Nodes = append(res.Nodes, n)
	}
	return len(res.Nodes) > 0
}

// dbTblsTriggerFn extracts the EXECUTE FUNCTION/PROCEDURE target from a
// trigger definition (best effort, lower-cased match).
func dbTblsTriggerFn(def string) string {
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
		if name := strings.TrimSpace(rest[:j]); name != "" {
			return name
		}
	}
	return ""
}
