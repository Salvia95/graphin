package graph

import "strings"

// stoplist suppresses global same-name matching for ultra-common member
// names (§2.1.3 used_by 폭발 방지). Scoped matches (same package, imported)
// are still allowed — §3.3's example expects a call edge onto "process".
var stoplist = map[string]bool{
	"process": true, "get": true, "set": true, "info": true, "run": true,
	"main": true, "init": true, "handle": true, "execute": true,
	"update": true, "create": true, "delete": true, "add": true,
	"remove": true, "close": true, "open": true, "start": true, "stop": true,
	"read": true, "write": true, "call": true, "apply": true, "invoke": true,
	"next": true, "size": true, "len": true, "str": true, "repr": true,
	"tostring": true, "equals": true, "hashcode": true, "__init__": true,
	"print": true, "println": true, "log": true, "debug": true, "error": true,
	"warn": true, "format": true, "of": true, "valueof": true, "build": true,
}

// Stoplisted reports whether name is too common for global matching.
func Stoplisted(name string) bool {
	return stoplist[strings.ToLower(name)]
}
