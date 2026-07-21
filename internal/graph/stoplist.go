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
	// JS/TS 초빈출: 프로미스 체인, 배열/객체 내장, 이벤트 관용구
	"then": true, "catch": true, "finally": true, "map": true, "filter": true,
	"foreach": true, "reduce": true, "push": true, "pop": true, "slice": true,
	"splice": true, "concat": true, "join": true, "split": true, "find": true,
	"includes": true, "indexof": true, "keys": true, "values": true,
	"entries": true, "assign": true, "stringify": true, "parse": true,
	"require": true, "resolve": true, "reject": true, "fetch": true,
	"on": true, "off": true, "emit": true, "send": true, "test": true,
	"render": true, "settimeout": true, "setinterval": true,
}

// Stoplisted reports whether name is too common for global matching.
func Stoplisted(name string) bool {
	return stoplist[strings.ToLower(name)]
}
