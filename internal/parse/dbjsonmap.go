package parse

// format:"json" custom-mapping extractor: the agent declares a minimal
// selector mapping in the manifest and graphin walks the project-native JSON
// SSOT with it. Spans reuse dbValueSpans, so read_code returns the table's
// own JSON block from the SSOT.

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

type jsonHit struct {
	val      any
	path     string   // dbValueSpans key
	captures []string // '*' captured object keys, outermost first
}

func extractDBJSONMapped(src []byte, route *DBRoute, res *FileResult) bool {
	m := route.JSON.Mapping
	if m == nil || m.Tables == "" {
		return false
	}
	var root any
	if json.Unmarshal(src, &root) != nil {
		return false
	}
	spans, err := dbValueSpans(src)
	if err != nil {
		return false
	}
	ds := route.Datasource
	res.Package = "db." + ds

	for _, hit := range jsonSelect(root, "", nil, m.Tables) {
		sp, ok := spans[hit.path]
		if !ok {
			continue
		}
		name := jsonName(hit, m.TableName)
		if name == "" {
			continue
		}
		schema := route.DefaultSchema
		if i := strings.LastIndexByte(name, '.'); i > 0 { // "schema.table" 꼴
			schema, name = name[:i], name[i+1:]
		} else if len(hit.captures) >= 2 {
			schema = hit.captures[len(hit.captures)-2]
		}
		n := Node{
			ID:          nodeid.DBNode(ds, schema, name),
			DisplayName: nodeid.DBDisplay(ds, schema, name),
			SimpleName:  name,
			Kind:        nodeid.KindTable,
			Container:   schema,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(src[sp.start:sp.end]),
		}
		if m.Columns != "" {
			nameSel := m.ColumnName
			if nameSel == "" {
				nameSel = defaultLeafSel(m.Columns, "name")
			}
			typeSel := m.ColumnType
			if typeSel == "" {
				typeSel = "field:type"
			}
			for _, col := range jsonSelect(hit.val, hit.path, nil, m.Columns) {
				cn := jsonName(col, nameSel)
				if cn == "" {
					continue
				}
				ct, _ := jsonLeaf(col.val, typeSel)
				n.Params = append(n.Params, strings.TrimSpace(cn+" "+ct))
			}
		}
		if m.FKs != "" {
			refSel := m.FKRef
			if refSel == "" {
				refSel = "field:references"
			}
			for _, fk := range jsonSelect(hit.val, hit.path, nil, m.FKs) {
				ref, ok := jsonLeaf(fk.val, refSel)
				if !ok {
					continue
				}
				fqn, ok := normalizeDBRef(ds, ref)
				if !ok {
					continue
				}
				enforced := true
				if m.FKEnforced != "" {
					if b, ok := jsonLeafBool(fk.val, m.FKEnforced); ok {
						enforced = b
					}
				}
				if enforced {
					n.Supers = append(n.Supers, fqn)
				} else {
					n.LogicalRefs = append(n.LogicalRefs, fqn)
				}
			}
		}
		res.Nodes = append(res.Nodes, n)
	}
	return len(res.Nodes) > 0
}

// jsonSelect walks a selector ("a.b[]", "schemas.*.tables.*") from v,
// yielding hits with span paths and '*' captures. Object iteration is sorted
// for deterministic node order.
func jsonSelect(v any, basePath string, captures []string, sel string) []jsonHit {
	segs := strings.Split(sel, ".")
	hits := []jsonHit{{val: v, path: basePath, captures: captures}}
	for _, seg := range segs {
		arr := strings.HasSuffix(seg, "[]")
		key := strings.TrimSuffix(seg, "[]")
		var next []jsonHit
		for _, h := range hits {
			if key == "*" {
				obj, ok := h.val.(map[string]any)
				if !ok {
					continue
				}
				for _, k := range sortedKeys(obj) {
					next = append(next, jsonHit{
						val:      obj[k],
						path:     joinSpanPath(h.path, k),
						captures: append(append([]string{}, h.captures...), k),
					})
				}
				continue
			}
			obj, ok := h.val.(map[string]any)
			if !ok {
				continue
			}
			child, ok := obj[key]
			if !ok {
				continue
			}
			ch := jsonHit{val: child, path: joinSpanPath(h.path, key), captures: h.captures}
			if !arr {
				next = append(next, ch)
				continue
			}
			list, ok := child.([]any)
			if !ok {
				continue
			}
			for i, el := range list {
				next = append(next, jsonHit{
					val:      el,
					path:     joinSpanPath(ch.path, "#"+strconv.Itoa(i)),
					captures: h.captures,
				})
			}
		}
		hits = next
	}
	return hits
}

func joinSpanPath(base, seg string) string {
	if base == "" {
		return seg
	}
	return base + "/" + seg
}

// jsonName resolves a name getter for a hit: "key" reads the innermost '*'
// capture, "field:<f>" reads from the value, "" defaults to "key".
func jsonName(h jsonHit, sel string) string {
	if sel == "" || sel == "key" {
		if len(h.captures) == 0 {
			return ""
		}
		return h.captures[len(h.captures)-1]
	}
	s, _ := jsonLeaf(h.val, sel)
	return s
}

// defaultLeafSel: array collections default to field:name, keyed collections
// to the captured key.
func defaultLeafSel(collectionSel, field string) string {
	if strings.HasSuffix(collectionSel, "[]") {
		return "field:" + field
	}
	return "key"
}

// jsonLeaf evaluates "field:a.b" | "const:x" against a value.
func jsonLeaf(v any, sel string) (string, bool) {
	if c, ok := strings.CutPrefix(sel, "const:"); ok {
		return c, true
	}
	f, ok := strings.CutPrefix(sel, "field:")
	if !ok {
		return "", false
	}
	for _, seg := range strings.Split(f, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return "", false
		}
		if v, ok = obj[seg]; !ok {
			return "", false
		}
	}
	s, ok := v.(string)
	return s, ok
}

func jsonLeafBool(v any, sel string) (bool, bool) {
	f, ok := strings.CutPrefix(sel, "field:")
	if !ok {
		return false, false
	}
	for _, seg := range strings.Split(f, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return false, false
		}
		if v, ok = obj[seg]; !ok {
			return false, false
		}
	}
	b, ok := v.(bool)
	return b, ok
}
