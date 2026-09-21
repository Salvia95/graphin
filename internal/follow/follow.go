// Package follow lets a server that lost the workspace lock answer through
// the one that holds it.
//
// The lock is per workspace and only its holder may index: graph.Open
// truncates the delta log, so a second engine over the same directory is not
// a reader, it is a second writer. Two sessions in one directory used to mean
// one working graphin and one dead one. Here the holder (the leader) listens
// on a unix socket inside the workspace data dir, and every other server (a
// follower) forwards its tool calls there — one index, one embedding stack,
// the same answers in every session.
//
// The wire is newline-delimited JSON at the tool-handler level (tool + args →
// text + isError), not MCP: both ends are this binary, the MCP envelope of
// each session stays that session's business, and the first message can be a
// version handshake.
package follow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	sockName = "leader.sock"
	addrName = "leader.addr"

	// sun_path is 108 bytes on Linux and 104 on darwin, NUL included.
	maxSockPath = 100
)

var (
	// ErrNoLeader: nothing is listening — the lock holder predates this
	// package, is still coming up, or is gone.
	ErrNoLeader = errors.New("no leader is listening for this workspace")
	// ErrLeaderGone fails calls that were in flight when the leader went away.
	ErrLeaderGone = errors.New("the leader went away")
)

// IncompatibleError is a refused handshake: the leader runs a different
// major.minor. Forwarding anyway would answer one version's tool schema with
// another version's handler, and nothing downstream could tell.
type IncompatibleError struct {
	LeaderVersion string
	LeaderPID     int
}

func (e *IncompatibleError) Error() string {
	return fmt.Sprintf("the leader (pid %d) runs graphin %s, which this build cannot follow",
		e.LeaderPID, e.LeaderVersion)
}

// msg is every message on the wire; Type selects which fields matter.
type msg struct {
	Type string `json:"type"` // hello | call | cancel | result

	// hello, both directions
	Version string `json:"version,omitempty"`
	PID     int    `json:"pid,omitempty"`
	OK      bool   `json:"ok,omitempty"`
	Reason  string `json:"reason,omitempty"`

	// call / cancel / result
	ID      uint64          `json:"id,omitempty"`
	Tool    string          `json:"tool,omitempty"`
	Args    json.RawMessage `json:"args,omitempty"`
	Text    string          `json:"text,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
	Unknown bool            `json:"unknown,omitempty"` // result: the leader has no such tool
}

// sockPath picks where the leader listens for the data dir. Inside it when
// the path fits sun_path; otherwise under the runtime dir, named by the data
// dir's hash. Either way addrName in the data dir records the choice, so a
// follower never has to re-derive it.
func sockPath(dataDir string) string {
	p := filepath.Join(dataDir, sockName)
	if len(p) <= maxSockPath {
		return p
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "graphin-"+strconv.Itoa(os.Getuid()))
	} else {
		base = filepath.Join(base, "graphin")
	}
	sum := sha256.Sum256([]byte(dataDir))
	return filepath.Join(base, hex.EncodeToString(sum[:8])+".sock")
}

// Compatible reports whether two builds may share a socket: same major.minor.
// A version that does not parse (a dev build) only follows its exact self.
func Compatible(a, b string) bool {
	am, aok := majorMinor(a)
	bm, bok := majorMinor(b)
	if !aok || !bok {
		return a == b
	}
	return am == bm
}

func majorMinor(v string) (string, bool) {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 3 {
		return "", false
	}
	for _, p := range parts[:2] {
		if _, err := strconv.Atoi(p); err != nil {
			return "", false
		}
	}
	return parts[0] + "." + parts[1], true
}
