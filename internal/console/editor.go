package console

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Editors whose URL scheme is documented and stable enough to ship.
//
// The list is short on purpose. A link that silently does nothing is worse than
// no link — the reader clicks, nothing happens, and they learn the button is a
// lie. Anything not here is reachable by passing a template directly, which is
// also the honest answer for an editor whose handler this project cannot test.
var editorURLs = map[string]string{
	"vscode":   "vscode://file/{path}:{line}",
	"cursor":   "cursor://file/{path}:{line}",
	"windsurf": "windsurf://file/{path}:{line}",
	"zed":      "zed://file/{path}:{line}",
	"sublime":  "subl://open?url=file://{path}&line={line}",
}

// EditorNames lists what --editor accepts, for the usage line.
func EditorNames() []string {
	out := make([]string, 0, len(editorURLs))
	for k := range editorURLs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolveEditor turns the flag — or the environment — into a name and a URL
// template. Both are empty when nothing is known, and the interface then offers
// only the copy button.
func resolveEditor(flag string) (name, tmpl string) {
	switch {
	case strings.Contains(flag, "{path}"):
		// A raw template. Whoever passed it knows their editor better than a
		// table in this file does.
		return "custom", flag
	case flag != "":
		return flag, editorURLs[flag]
	}
	return detectEditor(getenv)
}

// getenv is indirected so the detection can be tested without touching the
// process environment.
var getenv = os.Getenv

// detectEditor guesses from what a terminal session says about itself.
//
// $EDITOR is consulted before $TERM_PROGRAM because it is the more specific
// claim: the VS Code forks all report TERM_PROGRAM=vscode, so that variable
// alone cannot tell one from another, while an $EDITOR of `cursor` can.
func detectEditor(env func(string) string) (string, string) {
	for _, key := range []string{"EDITOR", "VISUAL"} {
		if n := editorFromCommand(env(key)); n != "" {
			return n, editorURLs[n]
		}
	}
	switch strings.ToLower(env("TERM_PROGRAM")) {
	case "vscode":
		return "vscode", editorURLs["vscode"]
	case "zed":
		return "zed", editorURLs["zed"]
	}
	return "", ""
}

func editorFromCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	// $EDITOR is a command line, not a path: "code --wait" and
	// "/usr/bin/code-insiders -w" both mean VS Code.
	base := strings.ToLower(filepath.Base(strings.Fields(cmd)[0]))
	base = strings.TrimSuffix(base, ".exe")
	switch {
	case base == "code" || base == "code-insiders" || base == "codium":
		return "vscode"
	case base == "cursor":
		return "cursor"
	case base == "windsurf":
		return "windsurf"
	case base == "zed" || base == "zeditor":
		return "zed"
	case base == "subl" || base == "sublime_text":
		return "sublime"
	}
	return ""
}
