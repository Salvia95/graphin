package wiki

import (
	"bytes"
	"strings"
)

// Frontmatter is the machine-readable header of a wiki page: the leading
// block delimited by `---` lines at the very top of the file.
//
// This is a deliberate subset of YAML — scalars and flat lists, nothing
// nested — and the schema is designed to fit it rather than the other way
// around. graphin has no YAML dependency and gaining one to read its own
// files would be a poor trade: every nesting level a format allows is a
// level an author will eventually use, and then the parser has to be real.
//
// The one place the subset pinches is a field that wants pairs, such as
// "not to be confused with" (a term plus the reason). Those are written as
// one string with an em-dash separator and split at the point of use, which
// keeps the file readable and the parser trivial.
type Frontmatter struct {
	// Scalars and Lists are kept separate because the distinction is the
	// author's, not the reader's: `aliases: []` means "none" while a missing
	// key means "not applicable", and collapsing them loses that.
	Scalars map[string]string
	Lists   map[string][]string
}

// Get returns a scalar field, or "" when absent.
func (f Frontmatter) Get(key string) string { return f.Scalars[key] }

// List returns a list field. A scalar written where a list is expected reads
// as a one-element list: `roles: backend` and `roles: [backend]` mean the
// same thing to an author, so they mean the same thing here.
func (f Frontmatter) List(key string) []string {
	if v, ok := f.Lists[key]; ok {
		return v
	}
	if s, ok := f.Scalars[key]; ok && s != "" {
		return []string{s}
	}
	return nil
}

// splitFrontmatter separates the frontmatter block from the body. A file
// without a leading `---` has no frontmatter and is all body — that is not an
// error, because a wiki page can be a draft before it is a record.
func splitFrontmatter(src []byte) (fm []byte, body []byte) {
	if !bytes.HasPrefix(src, []byte("---\n")) && !bytes.HasPrefix(src, []byte("---\r\n")) {
		return nil, src
	}
	rest := src[bytes.IndexByte(src, '\n')+1:]
	// The closing delimiter is a line that is exactly `---`.
	off := 0
	for off <= len(rest) {
		nl := bytes.IndexByte(rest[off:], '\n')
		end := len(rest)
		if nl >= 0 {
			end = off + nl
		}
		line := strings.TrimRight(string(rest[off:end]), "\r")
		if line == "---" {
			if nl < 0 {
				return rest[:off], nil
			}
			return rest[:off], rest[end+1:]
		}
		if nl < 0 {
			break
		}
		off = end + 1
	}
	// Unterminated block: treat the whole file as body rather than guessing
	// where the header was meant to stop. A silently truncated header would
	// drop fields the author believed were recorded.
	return nil, src
}

// parseFrontmatter reads the scalar/list subset described on Frontmatter.
func parseFrontmatter(fm []byte) Frontmatter {
	out := Frontmatter{Scalars: map[string]string{}, Lists: map[string][]string{}}
	lines := strings.Split(string(fm), "\n")

	openList := "" // key whose block list we are currently collecting
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// A `- item` line continues the most recent `key:` with no value.
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			if openList == "" {
				continue // orphan item: no key to attach it to
			}
			item := unquote(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
			if item != "" {
				out.Lists[openList] = append(out.Lists[openList], item)
			}
			continue
		}

		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}

		switch {
		case value == "":
			// `key:` opens a block list. Record it as an empty list now so an
			// author's explicit "none" survives even with no items under it.
			openList = key
			if _, seen := out.Lists[key]; !seen {
				out.Lists[key] = []string{}
			}
		case strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]"):
			openList = ""
			out.Lists[key] = splitInline(value[1 : len(value)-1])
		default:
			openList = ""
			out.Scalars[key] = unquote(value)
		}
	}
	return out
}

// splitInline reads `[a, b, c]` contents. Empty brackets give an empty list,
// not nil: see the Scalars/Lists note on Frontmatter.
func splitInline(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if v := unquote(strings.TrimSpace(part)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
