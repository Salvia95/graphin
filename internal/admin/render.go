package admin

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strconv"

	"github.com/Salvia95/graphin/internal/nodeid"
)

// funcMap holds the helpers shared by every page and partial.
func funcMap() template.FuncMap {
	return template.FuncMap{
		"simple": nodeid.Simple,
		"conf":   func(c float32) string { return strconv.FormatFloat(float64(c), 'f', 2, 32) },
		"confClass": func(c float32) string {
			switch {
			case c >= 0.999:
				return "conf-certain"
			case c >= 0.95:
				return "conf-samepkg"
			case c >= 0.90:
				return "conf-imported"
			default:
				return "conf-global"
			}
		},
		"comma":      comma,
		"bytesHuman": bytesHuman,
	}
}

func comma(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return s
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func bytesHuman(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

// loadTemplates builds one template set per page (layout + partials + page)
// and one partials-only set for htmx fragment responses.
func loadTemplates() (map[string]*template.Template, error) {
	partials, err := fs.Glob(assets, "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	pages, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	out := make(map[string]*template.Template, len(pages)+1)
	for _, p := range pages {
		files := append([]string{"templates/layout.html", p}, partials...)
		t, err := template.New("layout.html").Funcs(funcMap()).ParseFS(assets, files...)
		if err != nil {
			return nil, err
		}
		out[path.Base(p)] = t
	}
	if len(partials) > 0 {
		t, err := template.New("partials").Funcs(funcMap()).ParseFS(assets, partials...)
		if err != nil {
			return nil, err
		}
		out["_partials"] = t
	}
	return out, nil
}

// renderPage executes one full page into a buffer first so template errors
// become a clean 500 instead of a half-written body.
func (s *Server) renderPage(w http.ResponseWriter, page string, data any) {
	t, ok := s.tmpl[page]
	if !ok {
		http.Error(w, "unknown page "+page, http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		s.log.Event("admin_render_error", map[string]any{"page": page, "error": err.Error()})
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// renderPartial executes one named partial; code lets pollers send htmx's
// 286 stop signal.
func (s *Server) renderPartial(w http.ResponseWriter, name string, code int, data any) {
	t := s.tmpl["_partials"]
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		s.log.Event("admin_render_error", map[string]any{"partial": name, "error": err.Error()})
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if code != http.StatusOK {
		w.WriteHeader(code)
	}
	_, _ = buf.WriteTo(w)
}
