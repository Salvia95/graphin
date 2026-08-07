// Package graph is the heuristic graph engine (§2.1.3): FlatBuffers forward
// shards read zero-copy through refcounted mmaps, a global reverse index fed
// by an append-only tombstone delta log, and confidence-scored edges.
package graph

import (
	"github.com/Salvia95/graphin/internal/graph/fbsgen"
)

// EdgeType mirrors fbsgen.EdgeType.
type EdgeType = fbsgen.EdgeType

const (
	EdgeImport     = fbsgen.EdgeTypeImport
	EdgeExtends    = fbsgen.EdgeTypeExtends
	EdgeImplements = fbsgen.EdgeTypeImplements
	EdgeCall       = fbsgen.EdgeTypeCall
	EdgeReference  = fbsgen.EdgeTypeReference
	EdgeForeignKey = fbsgen.EdgeTypeForeignKey // graphindb: 테이블 → 참조 테이블
	EdgeContains   = fbsgen.EdgeTypeContains   // markdown: 파일 → 섹션, 섹션 → 하위 섹션
)

// EdgeTypeName renders the §3.3 type attribute.
func EdgeTypeName(t EdgeType) string {
	switch t {
	case EdgeImport:
		return "import"
	case EdgeExtends:
		return "extends"
	case EdgeImplements:
		return "implements"
	case EdgeCall:
		return "call"
	case EdgeForeignKey:
		return "foreign_key"
	case EdgeContains:
		return "contains"
	default:
		return "reference"
	}
}

// Edge is one outgoing (uses) edge.
type Edge struct {
	TargetID   string
	Type       EdgeType
	Confidence float32
}

// RevEdge is one incoming (used_by) edge.
type RevEdge struct {
	SourceID   string
	Type       EdgeType
	Confidence float32
}
