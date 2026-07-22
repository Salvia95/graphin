package parse

// Format registry for manifest-routed SSOT files. Each extractor turns one
// file into parse.Node lists under the route's datasource; everything
// downstream (graph, merkle, lexical, semantic, MCP) is format-agnostic.

type dbExtractor func(src []byte, route *DBRoute, res *FileResult) bool

var dbFormats = map[string]dbExtractor{
	"sql":    extractDBSQL,
	"schema": extractDBPrisma,
	"json":   extractDBJSONRouted,
}

// extractDBRouted dispatches by route format. A fatal extraction failure
// degrades the file to a Partial plain node — same posture as a broken
// inline snapshot: stay searchable, never vanish.
func extractDBRouted(src []byte, route *DBRoute, res *FileResult) {
	if route.Format == "graphindb" {
		// explicitly routed inline snapshot: the suffix convention governs.
		extractDBSchema(src, res)
		return
	}
	ex := dbFormats[route.Format]
	if ex != nil && ex(src, route, res) {
		return
	}
	res.Nodes = res.Nodes[:0]
	res.Package = ""
	res.Partial = true
	extractPlain(src, res)
}

// extractDBJSONRouted picks the preset or the mapping DSL.
func extractDBJSONRouted(src []byte, route *DBRoute, res *FileResult) bool {
	if route.JSON == nil {
		return false
	}
	if route.JSON.Preset == "tbls" {
		return extractDBTbls(src, route, res)
	}
	if route.JSON.Mapping != nil {
		return extractDBJSONMapped(src, route, res)
	}
	return false
}
