// Package scan enumerates indexable source files under a workspace, honoring
// the §2.4 filter rules: gitignore syntax, default build-dir excludes,
// symlink and oversized-file skipping.
package scan

import (
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/Salvia95/graphin/internal/ignore"
	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/parse"
)

// DefaultMaxFileSize: larger single sources are treated as generated code.
const DefaultMaxFileSize = 1 << 20 // 1MB

// DefaultExcludedDirs are pruned regardless of ignore files (§2.4).
var DefaultExcludedDirs = map[string]bool{
	"build": true, "out": true, "target": true, ".gradle": true,
	"__pycache__": true, "venv": true, ".venv": true, "node_modules": true,
	"dist": true, ".next": true, "coverage": true,
	".git": true, ".graphin": true,
}

// ExcludedBasenames are machine-generated lock files: text by extension but
// pure noise for navigation.
var ExcludedBasenames = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"go.sum": true, "Cargo.lock": true, "poetry.lock": true, "uv.lock": true,
	"gradle.lockfile": true, "composer.lock": true, "Gemfile.lock": true,
}

// FileInfo is one indexable source file.
type FileInfo struct {
	RelPath string // "/"-separated, relative to root
	AbsPath string
	Size    int64
}

// Result carries the file list plus the composed ignore matcher, which the
// watcher reuses to filter events.
type Result struct {
	Files   []FileInfo
	Matcher *ignore.Matcher
}

// Walk scans root. Nested .gitignore files and .graphin/ignore are honored.
func Walk(root string, lg *obs.Logger) (*Result, error) {
	m := ignore.NewMatcher()
	if b, err := os.ReadFile(filepath.Join(root, ".graphin", "ignore")); err == nil {
		m.AddFile("", b)
	}
	res := &Result{Matcher: m}

	var walk func(dirRel string) error
	walk = func(dirRel string) error {
		dirAbs := filepath.Join(root, filepath.FromSlash(dirRel))
		if b, err := os.ReadFile(filepath.Join(dirAbs, ".gitignore")); err == nil {
			m.AddFile(dirRel, b)
		}
		entries, err := os.ReadDir(dirAbs) // sorted by name: deterministic
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := e.Name()
			rel := pathpkg.Join(dirRel, name)
			if e.Type()&fs.ModeSymlink != 0 {
				lg.Event("skip_symlink", map[string]any{"path": rel})
				continue
			}
			if e.IsDir() {
				if DefaultExcludedDirs[name] || m.Ignored(rel, true) {
					continue
				}
				if err := walk(rel); err != nil {
					return err
				}
				continue
			}
			if parse.DetectLanguage(name) == parse.LangUnknown || ExcludedBasenames[name] {
				continue
			}
			if m.Ignored(rel, false) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.Size() > DefaultMaxFileSize {
				lg.Event("skip_large_file", map[string]any{"path": rel, "bytes": info.Size()})
				continue
			}
			res.Files = append(res.Files, FileInfo{
				RelPath: rel,
				AbsPath: filepath.Join(dirAbs, name),
				Size:    info.Size(),
			})
		}
		return nil
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	return res, nil
}

// Indexable reports whether a single path (e.g. from a watcher event) should
// be indexed under the same rules Walk applies.
func Indexable(root, relPath string, m *ignore.Matcher) bool {
	segs := splitPath(relPath)
	for _, s := range segs[:max(0, len(segs)-1)] {
		if DefaultExcludedDirs[s] {
			return false
		}
	}
	if parse.DetectLanguage(relPath) == parse.LangUnknown {
		return false
	}
	if len(segs) > 0 && ExcludedBasenames[segs[len(segs)-1]] {
		return false
	}
	if m != nil && m.Ignored(relPath, false) {
		return false
	}
	return true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
