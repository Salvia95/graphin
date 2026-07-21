package parse

import (
	pathpkg "path"
	"strings"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// extractPlain turns an anchor-less text file (config, SQL, docs…) into one
// file-kind node: ID = workspace-relative path, span = whole file. Every
// downstream stage (Tier-0 filename match, read_code slicing + truncation,
// merkle 2-Track, watcher increments) works on it unchanged; it just never
// grows graph edges.
func extractPlain(src []byte, res *FileResult) {
	base := pathpkg.Base(res.RelPath)
	if dir := pathpkg.Dir(res.RelPath); dir != "." && dir != "/" {
		res.Package = strings.ReplaceAll(dir, "/", ".")
	}
	res.Nodes = append(res.Nodes, Node{
		ID:          res.RelPath,
		DisplayName: base,
		SimpleName:  base,
		Kind:        nodeid.KindFile,
		StartByte:   0,
		EndByte:     uint32(len(src)),
		Hash:        res.FileHash,
	})
}
