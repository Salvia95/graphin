package workspace

import (
	"sync/atomic"

	"github.com/llls2542/graphin/internal/mcp"
)

// Phase models staged availability (§2.1.1): lexical search opens before the
// semantic model finishes warming up.
type Phase int32

const (
	PhaseNotBootstrapped Phase = iota
	PhaseIndexing
	PhaseLexicalReady
	PhaseSemanticReady
)

// FSM tracks the workspace lifecycle. All methods are safe for concurrent use.
type FSM struct {
	phase    atomic.Int32
	progress atomic.Int32
}

func (f *FSM) Set(p Phase)   { f.phase.Store(int32(p)) }
func (f *FSM) Phase() Phase  { return Phase(f.phase.Load()) }

// SetProgress clamps p to 0..100.
func (f *FSM) SetProgress(p int) {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	f.progress.Store(int32(p))
}

// Status renders the current lifecycle as a §3.0 system status block.
func (f *FSM) Status() mcp.Status {
	st := mcp.Status{Progress: int(f.progress.Load())}
	switch f.Phase() {
	case PhaseNotBootstrapped:
		st.State = "not_bootstrapped"
	case PhaseIndexing:
		st.State = "indexing"
	case PhaseLexicalReady:
		st.State = "indexing" // semantic warmup still pending
		st.LexicalReady = true
	case PhaseSemanticReady:
		st.State = "ready"
		st.Progress = 100
		st.LexicalReady = true
		st.SemanticReady = true
	}
	return st
}
