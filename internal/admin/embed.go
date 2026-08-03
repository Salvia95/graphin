package admin

import (
	"embed"
	"io/fs"
	"net/http"
)

// assets embeds the admin UI — templates plus vendored static files — so the
// page works offline from the single graphin binary.
//
//go:embed templates static
var assets embed.FS

func staticHandler() http.Handler {
	sub, _ := fs.Sub(assets, "static")
	return http.FileServerFS(sub)
}
