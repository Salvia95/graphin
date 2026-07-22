package parse

// format:"schema" extractor — Prisma schema subset (schema/graphindb.md):
// model/view blocks become table/view nodes; scalar fields fold into Params
// (honoring @map); @relation(fields:…) on the owning side becomes an FK ref;
// @@map/@@schema override table name and schema. enum, composite type,
// datasource and generator blocks are skipped (v1 제외 확정).

import (
	"strings"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func extractDBPrisma(src []byte, route *DBRoute, res *FileResult) bool {
	ds := route.Datasource
	res.Package = "db." + ds
	blocks := prismaBlocks(src)

	// pass 1: model name → physical (schema, table). Relation fields refer
	// to model names, so targets resolve through this map.
	type modelInfo struct{ schema, table string }
	models := map[string]modelInfo{}
	for _, b := range blocks {
		if b.kind != "model" && b.kind != "view" {
			continue
		}
		table := prismaBlockAttr(b.body, "@@map")
		if table == "" {
			table = b.name
		}
		schema := prismaBlockAttr(b.body, "@@schema")
		if schema == "" {
			schema = route.DefaultSchema
		}
		models[b.name] = modelInfo{schema: schema, table: table}
	}

	for _, b := range blocks {
		if b.kind != "model" && b.kind != "view" {
			continue
		}
		mi := models[b.name]
		kind := nodeid.KindTable
		if b.kind == "view" {
			kind = nodeid.KindView
		}
		n := Node{
			ID:          nodeid.DBNode(ds, mi.schema, mi.table),
			DisplayName: nodeid.DBDisplay(ds, mi.schema, mi.table),
			SimpleName:  mi.table,
			Kind:        kind,
			Container:   mi.schema,
			StartByte:   b.start,
			EndByte:     b.end,
			Hash:        blake3.Sum256(src[b.start:b.end]),
			Aliases:     prismaAliases(b.name, mi.table),
		}
		for _, line := range strings.Split(string(b.body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "@@") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 || !isPrismaIdent(fields[0]) {
				continue
			}
			fieldName, fieldType := fields[0], fields[1]
			rest := strings.Join(fields[2:], " ")
			baseType := strings.TrimSuffix(strings.TrimSuffix(fieldType, "?"), "[]")
			if target, isRel := models[baseType]; isRel {
				// owning side only: @relation(fields:[…]) declares the FK
				// column; back-relations and implicit m2m carry no FK.
				if strings.Contains(rest, "@relation") && strings.Contains(rest, "fields:") {
					n.Supers = append(n.Supers, nodeid.DBNode(ds, target.schema, target.table))
				}
				continue
			}
			col := prismaCallString(rest, "@map")
			if col == "" {
				col = fieldName
			}
			n.Params = append(n.Params, col+" "+fieldType)
		}
		res.Nodes = append(res.Nodes, n)
	}
	return len(res.Nodes) > 0
}

// prismaAliases exposes the model name and its client-property (lcfirst)
// form as extra resolver lookup names so `prisma.<member>` client refs land
// on the table node (Phase 7a).
func prismaAliases(model, table string) []string {
	var out []string
	if model != table {
		out = append(out, model)
	}
	if lc := lcFirst(model); lc != model && lc != table {
		out = append(out, lc)
	}
	return out
}

func lcFirst(s string) string {
	if s == "" || (s[0] < 'A' || s[0] > 'Z') {
		return s
	}
	return string(s[0]+('a'-'A')) + s[1:]
}

type prismaBlock struct {
	kind, name string
	body       []byte
	start, end uint32
}

// prismaBlocks scans top-level `<kind> <Name> { … }` blocks with string and
// line-comment awareness. Unknown top-level lines are skipped whole.
func prismaBlocks(src []byte) []prismaBlock {
	var out []prismaBlock
	i, n := 0, len(src)
	for i < n {
		// skip whitespace and comments
		c := src[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		start := i
		kind, j := prismaWord(src, i)
		if kind == "" {
			i++
			continue
		}
		switch kind {
		case "model", "view", "enum", "datasource", "generator", "type":
		default: // 알 수 없는 최상위 구문: 그 줄을 통째로 건너뛴다
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		name, j2 := prismaWord(src, skipPrismaWS(src, j))
		open := skipPrismaWS(src, j2)
		if name == "" || open >= n || src[open] != '{' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		depth := 0
		k := open
		for k < n {
			switch src[k] {
			case '{':
				depth++
				k++
			case '}':
				depth--
				k++
				if depth == 0 {
					goto closed
				}
			case '"':
				k = skipSQLQuote(src, k, '"')
			case '/':
				if k+1 < n && src[k+1] == '/' {
					for k < n && src[k] != '\n' {
						k++
					}
				} else {
					k++
				}
			default:
				k++
			}
		}
	closed:
		out = append(out, prismaBlock{
			kind: kind, name: name,
			body:  src[open+1 : max(open+1, k-1)],
			start: uint32(start), end: uint32(k),
		})
		i = k
	}
	return out
}

func prismaWord(src []byte, i int) (string, int) {
	j := i
	for j < len(src) {
		c := src[j]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (j > i && c >= '0' && c <= '9') {
			j++
			continue
		}
		break
	}
	return string(src[i:j]), j
}

func skipPrismaWS(src []byte, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

func isPrismaIdent(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// prismaBlockAttr reads the first string argument of a block attribute like
// @@map("job_posting") anywhere in the body.
func prismaBlockAttr(body []byte, attr string) string {
	return prismaCallString(string(body), attr)
}

// prismaCallString extracts the first "…" literal inside attr(…): used for
// @@map, @@schema and field-level @map.
func prismaCallString(s, attr string) string {
	i := strings.Index(s, attr+"(")
	if i < 0 {
		return ""
	}
	rest := s[i+len(attr)+1:]
	q := strings.IndexByte(rest, '"')
	if q < 0 {
		return ""
	}
	end := strings.IndexByte(rest[q+1:], '"')
	if end < 0 {
		return ""
	}
	return rest[q+1 : q+1+end]
}
