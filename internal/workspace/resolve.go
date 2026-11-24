package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HasNode reports whether id is an indexed node.
func (w *Workspace) HasNode(id string) bool {
	_, ok := w.nodeMeta(id)
	return ok
}

// NodeAtFileLine returns the narrowest node enclosing a 1-based line of an
// indexed file — the node an agent means when it says "src/order.go:120".
// It reads the file because node spans are bytes and the caller speaks
// lines; a line past the end of the file resolves to nothing.
func (w *Workspace) NodeAtFileLine(rel string, line int) (string, bool) {
	if line < 1 || !w.HasNode(rel) {
		return "", false
	}
	src, err := os.ReadFile(filepath.Join(w.Root, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	start := 0
	for i := 1; i < line; i++ {
		j := bytes.IndexByte(src[start:], '\n')
		if j < 0 {
			return "", false
		}
		start += j + 1
	}
	if start >= len(src) && line > 1 {
		return "", false
	}
	end := len(src)
	if j := bytes.IndexByte(src[start:], '\n'); j >= 0 {
		end = start + j
	}
	return NodeAtLine(w.NodesInFiles([]string{rel})[rel], uint32(start), uint32(end))
}

// NodesNamed returns the ids whose qualified name ends in q, compared without
// parameter lists and redefinition suffixes: "OrderService.cancel" finds
// "com.shop.OrderService.cancel(String)". q must hold at least one dot — a
// bare name is the symbol table's job, and scanning every node for it would
// be slower and say the same thing. Path-shaped ids (files, markdown
// sections) are never matched: their names are not dotted members.
func (w *Workspace) NodesNamed(q string) []string {
	q = strings.TrimSpace(q)
	if !strings.Contains(q, ".") {
		return nil
	}
	withParams := strings.Contains(q, "(")
	w.nodesMu.RLock()
	var out []string
	for id := range w.nodes {
		if strings.Contains(id, "/") {
			continue
		}
		cand := id
		if !withParams {
			cand = stripMemberSuffix(id)
		}
		if cand == q || strings.HasSuffix(cand, "."+q) {
			out = append(out, id)
		}
	}
	w.nodesMu.RUnlock()
	sort.Strings(out)
	return out
}

// stripMemberSuffix drops a Java/Kotlin parameter list and a Python "#n"
// redefinition ordinal, leaving the dotted qualified name.
func stripMemberSuffix(id string) string {
	if i := strings.IndexByte(id, '('); i >= 0 {
		id = id[:i]
	}
	if i := strings.LastIndexByte(id, '#'); i >= 0 {
		id = id[:i]
	}
	return id
}
