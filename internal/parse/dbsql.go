package parse

// format:"sql" extractor — a tolerant, in-house DDL scanner for *state-style*
// SQL files (schema dumps, flyway baselines). It never replays migration
// history (설계 확정: 이력 폴드 비채택). Recognized statements become nodes
// whose span is the statement text, so read_code returns real DDL; anything
// unrecognized is skipped, and structurally broken CREATEs mark Partial.
//
// tree-sitter를 쓰지 않는 이유: 방언(Oracle, Postgres RLS 등)에서 강한
// 문법은 ERROR로 무너지지만, 탐색용 추출은 관용적 스캐너가 더 오래 버틴다.

import (
	"bytes"
	"strings"

	"github.com/zeebo/blake3"

	"github.com/Salvia95/graphin/internal/nodeid"
)

func extractDBSQL(src []byte, route *DBRoute, res *FileResult) bool {
	ds := route.Datasource
	res.Package = "db." + ds

	tableIdx := map[string]int{}      // table FQN → res.Nodes index (ALTER 병합)
	alterMix := map[string][][]byte{} // FQN → 병합된 ALTER 문 원문 (해시 혼합)

	for _, sp := range splitSQLStatements(src) {
		stmt := src[sp.start:sp.end]
		s := &sqlScan{src: stmt}
		w, _, ok := s.word()
		if !ok {
			continue
		}
		switch strings.ToUpper(w) {
		case "CREATE":
			sqlCreate(s, stmt, sp, ds, route.DefaultSchema, res, tableIdx)
		case "ALTER":
			sqlAlterTable(s, stmt, ds, route.DefaultSchema, res, tableIdx, alterMix)
		}
	}

	// ALTER로 FK가 붙은 테이블은 해시에 ALTER 원문을 섞는다: ALTER만 바뀌어도
	// merkle 2-Track이 해당 테이블을 Changed로 분류해 엣지를 재해석한다.
	for fqn, extras := range alterMix {
		i := tableIdx[fqn]
		h := blake3.New()
		_, _ = h.Write(src[res.Nodes[i].StartByte:res.Nodes[i].EndByte])
		for _, e := range extras {
			_, _ = h.Write(e)
		}
		sum := h.Sum(nil)
		copy(res.Nodes[i].Hash[:], sum)
	}
	return len(res.Nodes) > 0
}

// sqlCreate handles one CREATE statement after the CREATE keyword.
func sqlCreate(s *sqlScan, stmt []byte, sp dbSpan, ds, defSchema string, res *FileResult, tableIdx map[string]int) {
	// modifiers before the object keyword: OR REPLACE, TEMP, UNLOGGED,
	// MATERIALIZED, GLOBAL/LOCAL...
	obj := ""
	for {
		w, _, ok := s.word()
		if !ok {
			return
		}
		switch u := strings.ToUpper(w); u {
		case "OR", "REPLACE", "TEMP", "TEMPORARY", "UNLOGGED", "GLOBAL", "LOCAL", "MATERIALIZED":
			continue
		default:
			obj = u
		}
		break
	}

	switch obj {
	case "TABLE":
		s.skipIfNotExists()
		fqn, simple, schema, ok := s.qualifiedFQN(ds, defSchema)
		if !ok {
			res.Partial = true
			return
		}
		body, ok := s.parenBlock()
		if !ok {
			res.Partial = true
			return
		}
		n := Node{
			ID:          fqn,
			DisplayName: nodeid.DBDisplay(ds, schema, simple),
			SimpleName:  simple,
			Kind:        nodeid.KindTable,
			Container:   schema,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(stmt),
		}
		for _, entry := range splitTopComma(body) {
			sqlTableEntry(entry, ds, defSchema, &n)
		}
		tableIdx[fqn] = len(res.Nodes)
		res.Nodes = append(res.Nodes, n)

	case "VIEW":
		s.skipIfNotExists()
		fqn, simple, schema, ok := s.qualifiedFQN(ds, defSchema)
		if !ok {
			res.Partial = true
			return
		}
		n := dbStmtNode(fqn, ds, schema, simple, nodeid.KindView, sp, stmt)
		for _, tok := range dbDefTokens(string(stmt)) {
			n.Calls = append(n.Calls, Call{Name: tok, Args: -1})
		}
		res.Nodes = append(res.Nodes, n)

	case "FUNCTION", "PROCEDURE":
		s.skipIfNotExists()
		fqn, simple, schema, ok := s.qualifiedFQN(ds, defSchema)
		if !ok {
			res.Partial = true
			return
		}
		kind := nodeid.KindDBFunction
		if obj == "PROCEDURE" {
			kind = nodeid.KindProcedure
		}
		n := dbStmtNode(fqn, ds, schema, simple, kind, sp, stmt)
		if args, ok := s.parenBlock(); ok {
			if a := strings.Join(strings.Fields(string(args)), " "); a != "" {
				n.Params = []string{a}
			}
		}
		for _, tok := range dbDefTokens(string(stmt)) {
			n.Calls = append(n.Calls, Call{Name: tok, Args: -1})
		}
		res.Nodes = append(res.Nodes, n)

	case "TRIGGER":
		name, _, ok := s.word()
		if !ok {
			res.Partial = true
			return
		}
		tableFQN, tSchema, tSimple := "", "", ""
		fnFQN := ""
		for {
			w, ok := s.nextWord()
			if !ok {
				break
			}
			switch strings.ToUpper(w) {
			case "ON":
				if tableFQN == "" {
					tableFQN, tSimple, tSchema, _ = s.qualifiedFQN(ds, defSchema)
				}
			case "EXECUTE":
				w2, _, ok2 := s.word() // FUNCTION | PROCEDURE
				if u := strings.ToUpper(w2); ok2 && (u == "FUNCTION" || u == "PROCEDURE") {
					fnFQN, _, _, _ = s.qualifiedFQN(ds, defSchema)
				}
			}
		}
		if tableFQN == "" {
			res.Partial = true
			return
		}
		n := Node{
			ID:          tableFQN + "." + name,
			DisplayName: nodeid.DBDisplay(ds, tSchema, tSimple) + "." + name,
			SimpleName:  name,
			Kind:        nodeid.KindTrigger,
			Container:   tSchema + "." + tSimple,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(stmt),
			Supers:      []string{tableFQN},
		}
		if fnFQN != "" {
			n.Supers = append(n.Supers, fnFQN)
		}
		res.Nodes = append(res.Nodes, n)

	case "POLICY":
		pname, _, ok := s.word()
		if !ok {
			res.Partial = true
			return
		}
		if w, _, ok := s.word(); !ok || strings.ToUpper(w) != "ON" {
			res.Partial = true
			return
		}
		tableFQN, tSimple, tSchema, ok := s.qualifiedFQN(ds, defSchema)
		if !ok {
			res.Partial = true
			return
		}
		param := pname
		for { // optional FOR SELECT|INSERT|UPDATE|DELETE|ALL
			w, ok := s.nextWord()
			if !ok {
				break
			}
			if strings.ToUpper(w) == "FOR" {
				if cmd, _, ok := s.word(); ok {
					param += " " + strings.ToUpper(cmd)
				}
				break
			}
		}
		// SQL 소스의 RLS는 정책문당 1노드 (인라인 사이드카의 테이블당 번들과
		// 구분되는 ID 층: <table>.rls.<policy>).
		res.Nodes = append(res.Nodes, Node{
			ID:          tableFQN + ".rls." + pname,
			DisplayName: nodeid.DBDisplay(ds, tSchema, tSimple) + ".rls." + pname,
			SimpleName:  tSimple,
			Kind:        nodeid.KindRLSPolicy,
			Container:   tSchema + "." + tSimple,
			StartByte:   sp.start,
			EndByte:     sp.end,
			Hash:        blake3.Sum256(stmt),
			Supers:      []string{tableFQN},
			Params:      []string{param},
		})
	}
	// INDEX, TYPE, EXTENSION, SEQUENCE… → 무시 (스팬 비할당, 스펙 명시)
}

// sqlAlterTable merges ALTER TABLE … ADD [CONSTRAINT x] FOREIGN KEY … into
// the table node declared earlier in the same file. Tables declared elsewhere
// are skipped (스펙: 상태형 DDL은 자기완결 파일 단위).
func sqlAlterTable(s *sqlScan, stmt []byte, ds, defSchema string, res *FileResult, tableIdx map[string]int, alterMix map[string][][]byte) {
	if w, _, ok := s.word(); !ok || strings.ToUpper(w) != "TABLE" {
		return
	}
	for { // IF EXISTS / ONLY
		save := s.pos
		w, _, ok := s.word()
		if !ok {
			return
		}
		if u := strings.ToUpper(w); u == "IF" || u == "EXISTS" || u == "ONLY" {
			continue
		}
		s.pos = save
		break
	}
	fqn, _, _, ok := s.qualifiedFQN(ds, defSchema)
	if !ok {
		return
	}
	idx, declared := tableIdx[fqn]
	if !declared {
		return
	}
	for {
		w, ok := s.nextWord()
		if !ok {
			return
		}
		if strings.ToUpper(w) != "REFERENCES" {
			continue
		}
		target, _, _, ok := s.qualifiedFQN(ds, defSchema)
		if !ok {
			return
		}
		s.parenBlock() // optional (cols)
		res.Nodes[idx].Supers = append(res.Nodes[idx].Supers, target)
		alterMix[fqn] = append(alterMix[fqn], stmt)
	}
}

// sqlTableEntry parses one top-level entry of a CREATE TABLE body: either a
// column definition (→ Params "name type" + inline REFERENCES FK) or a
// table-level constraint (→ FOREIGN KEY만 취한다).
func sqlTableEntry(entry []byte, ds, defSchema string, n *Node) {
	s := &sqlScan{src: entry}
	w, quoted, ok := s.word()
	if !ok {
		return
	}
	upper := strings.ToUpper(w)
	if !quoted {
		switch upper {
		case "CONSTRAINT":
			s.word() // constraint name
			w2, _, ok2 := s.word()
			if !ok2 || strings.ToUpper(w2) != "FOREIGN" {
				return
			}
			upper = "FOREIGN"
		case "PRIMARY", "UNIQUE", "CHECK", "EXCLUDE", "LIKE":
			return
		}
		if upper == "FOREIGN" {
			s.word()       // KEY
			s.parenBlock() // (cols)
			if w3, _, ok3 := s.word(); ok3 && strings.ToUpper(w3) == "REFERENCES" {
				if target, _, _, ok4 := s.qualifiedFQN(ds, defSchema); ok4 {
					n.Supers = append(n.Supers, target)
				}
			}
			return
		}
	}

	// column: name then raw type text up to the first attribute keyword.
	colName := w
	typeStart := -1
	typeEnd := -1
	for {
		s.skip()
		if s.pos >= len(s.src) {
			break
		}
		if s.src[s.pos] == '(' || s.src[s.pos] == '[' { // varchar(200), bigint[]
			if typeStart < 0 {
				break
			}
			if s.src[s.pos] == '(' {
				s.parenBlock()
			} else {
				s.pos++
				if s.pos < len(s.src) && s.src[s.pos] == ']' {
					s.pos++
				}
			}
			typeEnd = s.pos
			continue
		}
		save := s.pos
		tok, tokQuoted, ok := s.word()
		if !ok {
			break
		}
		u := strings.ToUpper(tok)
		if !tokQuoted && sqlColStop[u] {
			s.pos = save
			break
		}
		if typeStart < 0 {
			typeStart = save
		}
		typeEnd = s.pos
	}
	colType := ""
	if typeStart >= 0 && typeEnd > typeStart {
		colType = strings.Join(strings.Fields(string(s.src[typeStart:typeEnd])), " ")
	}
	n.Params = append(n.Params, strings.TrimSpace(colName+" "+colType))

	// inline REFERENCES (남은 속성부에서)
	for {
		w2, ok2 := s.nextWord()
		if !ok2 {
			return
		}
		if strings.ToUpper(w2) == "REFERENCES" {
			if target, _, _, ok3 := s.qualifiedFQN(ds, defSchema); ok3 {
				n.Supers = append(n.Supers, target)
			}
			return
		}
	}
}

// sqlColStop terminates the type-text capture of a column definition.
var sqlColStop = map[string]bool{
	"NOT": true, "NULL": true, "DEFAULT": true, "PRIMARY": true,
	"REFERENCES": true, "UNIQUE": true, "CHECK": true, "CONSTRAINT": true,
	"GENERATED": true, "COLLATE": true,
}

func dbStmtNode(fqn, ds, schema, simple, kind string, sp dbSpan, stmt []byte) Node {
	return Node{
		ID:          fqn,
		DisplayName: nodeid.DBDisplay(ds, schema, simple),
		SimpleName:  simple,
		Kind:        kind,
		Container:   schema,
		StartByte:   sp.start,
		EndByte:     sp.end,
		Hash:        blake3.Sum256(stmt),
	}
}

// ---- statement splitter ----

// splitSQLStatements splits on top-level ';', honoring 'strings' (with ”
// escapes), "quoted idents", `backticks`, line/block comments and Postgres
// $tag$ dollar quotes. Statement spans start at the first significant byte
// (leading comments excluded) and exclude the terminator.
func splitSQLStatements(src []byte) []dbSpan {
	var out []dbSpan
	n := len(src)
	start := -1
	emit := func(end int) {
		for end > start && isSQLSpace(src[end-1]) {
			end--
		}
		if start >= 0 && end > start {
			out = append(out, dbSpan{start: uint32(start), end: uint32(end)})
		}
		start = -1
	}
	for i := 0; i < n; {
		c := src[i]
		switch {
		case c == '-' && i+1 < n && src[i+1] == '-':
			for i < n && src[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			i += 2
			for i+1 < n && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i = min(i+2, n)
		case c == '\'' || c == '"' || c == '`':
			if start < 0 {
				start = i
			}
			i = skipSQLQuote(src, i, c)
		case c == '$':
			if start < 0 {
				start = i
			}
			i = skipSQLDollar(src, i)
		case c == ';':
			emit(i)
			i++
		case isSQLSpace(c):
			i++
		default:
			if start < 0 {
				start = i
			}
			i++
		}
	}
	emit(n)
	return out
}

func isSQLSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// skipSQLQuote returns the index just past the closing quote; ” doubles
// escape inside single quotes.
func skipSQLQuote(src []byte, i int, q byte) int {
	n := len(src)
	i++
	for i < n {
		if src[i] == q {
			if q == '\'' && i+1 < n && src[i+1] == q {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return n
}

// skipSQLDollar skips a $tag$ … $tag$ block; a lone '$' advances one byte.
func skipSQLDollar(src []byte, i int) int {
	n := len(src)
	j := i + 1
	for j < n && (isSQLIdentByte(src[j])) {
		j++
	}
	if j >= n || src[j] != '$' {
		return i + 1
	}
	tag := src[i : j+1]
	if k := bytes.Index(src[j+1:], tag); k >= 0 {
		return j + 1 + k + len(tag)
	}
	return n
}

func isSQLIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ---- statement-local token scanner ----

type sqlScan struct {
	src []byte
	pos int
}

func (s *sqlScan) skip() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if isSQLSpace(c) {
			s.pos++
			continue
		}
		if c == '-' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '-' {
			for s.pos < len(s.src) && s.src[s.pos] != '\n' {
				s.pos++
			}
			continue
		}
		if c == '/' && s.pos+1 < len(s.src) && s.src[s.pos+1] == '*' {
			s.pos += 2
			for s.pos+1 < len(s.src) && !(s.src[s.pos] == '*' && s.src[s.pos+1] == '/') {
				s.pos++
			}
			s.pos = min(s.pos+2, len(s.src))
			continue
		}
		break
	}
}

// word reads one identifier/keyword. Unquoted words fold to lower case
// (Postgres semantics); quoted identifiers keep their exact text.
func (s *sqlScan) word() (string, bool, bool) {
	s.skip()
	if s.pos >= len(s.src) {
		return "", false, false
	}
	c := s.src[s.pos]
	if c == '"' || c == '`' {
		end := skipSQLQuote(s.src, s.pos, c)
		text := string(s.src[s.pos+1 : max(s.pos+1, end-1)])
		s.pos = end
		return text, true, true
	}
	if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		j := s.pos + 1
		for j < len(s.src) && (isSQLIdentByte(s.src[j]) || s.src[j] == '$') {
			j++
		}
		text := strings.ToLower(string(s.src[s.pos:j]))
		s.pos = j
		return text, false, true
	}
	return "", false, false
}

// skipIfNotExists consumes an optional IF NOT EXISTS clause.
func (s *sqlScan) skipIfNotExists() {
	save := s.pos
	for _, want := range []string{"IF", "NOT", "EXISTS"} {
		w, _, ok := s.word()
		if !ok || strings.ToUpper(w) != want {
			s.pos = save
			return
		}
	}
}

// nextWord scans past punctuation to the next word (false at EOF). Loops
// that search for a keyword mid-statement must use this, not word(): word()
// stops at '(' and would end the search at the first paren.
func (s *sqlScan) nextWord() (string, bool) {
	for {
		if w, _, ok := s.word(); ok {
			return w, true
		}
		s.skip()
		if s.pos >= len(s.src) {
			return "", false
		}
		s.pos++
	}
}

func (s *sqlScan) eat(c byte) bool {
	s.skip()
	if s.pos < len(s.src) && s.src[s.pos] == c {
		s.pos++
		return true
	}
	return false
}

// qualifiedFQN reads [schema.]name and returns the node FQN under ds.
func (s *sqlScan) qualifiedFQN(ds, defSchema string) (fqn, simple, schema string, ok bool) {
	w, _, wok := s.word()
	if !wok {
		return "", "", "", false
	}
	parts := []string{w}
	for s.eat('.') {
		w2, _, ok2 := s.word()
		if !ok2 {
			break
		}
		parts = append(parts, w2)
	}
	simple = parts[len(parts)-1]
	schema = defSchema
	if len(parts) >= 2 {
		schema = parts[len(parts)-2]
	}
	return nodeid.DBNode(ds, schema, simple), simple, schema, true
}

// parenBlock consumes a balanced ( … ) and returns the inner bytes.
func (s *sqlScan) parenBlock() ([]byte, bool) {
	s.skip()
	if s.pos >= len(s.src) || s.src[s.pos] != '(' {
		return nil, false
	}
	depth := 0
	start := s.pos + 1
	for i := s.pos; i < len(s.src); {
		switch c := s.src[i]; c {
		case '(':
			depth++
			i++
		case ')':
			depth--
			if depth == 0 {
				s.pos = i + 1
				return s.src[start:i], true
			}
			i++
		case '\'', '"', '`':
			i = skipSQLQuote(s.src, i, c)
		case '-':
			if i+1 < len(s.src) && s.src[i+1] == '-' {
				for i < len(s.src) && s.src[i] != '\n' {
					i++
				}
			} else {
				i++
			}
		default:
			i++
		}
	}
	return nil, false
}

// splitTopComma splits a table body at top-level commas.
func splitTopComma(body []byte) [][]byte {
	var out [][]byte
	depth, start := 0, 0
	for i := 0; i < len(body); {
		switch c := body[i]; c {
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
		case '\'', '"', '`':
			i = skipSQLQuote(body, i, c)
		case ',':
			if depth == 0 {
				out = append(out, body[start:i])
				start = i + 1
			}
			i++
		default:
			i++
		}
	}
	if start < len(body) {
		out = append(out, body[start:])
	}
	return out
}
