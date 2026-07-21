package mcp

import "strings"

var (
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

// EscapeAttr escapes s for use inside a double-quoted XML attribute value.
func EscapeAttr(s string) string { return attrEscaper.Replace(s) }

// EscapeText escapes s for use as XML character data.
func EscapeText(s string) string { return textEscaper.Replace(s) }

// WriteCDATA writes s wrapped in CDATA, splitting the section whenever "]]>"
// occurs so the payload always round-trips.
func WriteCDATA(sb *strings.Builder, s string) {
	sb.WriteString("<![CDATA[")
	for {
		i := strings.Index(s, "]]>")
		if i < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:i+2]) // up to and including "]]"
		sb.WriteString("]]><![CDATA[")
		s = s[i+2:] // continue from ">"
	}
	sb.WriteString("]]>")
}
