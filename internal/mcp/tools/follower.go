package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Salvia95/graphin/internal/follow"
	"github.com/Salvia95/graphin/internal/mcp"
	"github.com/Salvia95/graphin/internal/workspace"
)

// takeoverWait bounds how long a call rides out a leader change. A leader
// that exits releases the lock at once; one that was killed holds it until
// the heartbeat goes stale (lock.Options default: 10s).
const (
	takeoverWait = 12 * time.Second
	takeoverPoll = 250 * time.Millisecond
)

// rerunnable are the tools a follower may send again after the leader died
// holding the first attempt: they read, or repeating them changes nothing.
// Everything else may have written before the answer was lost.
var rerunnable = map[string]bool{
	"bootstrap_workspace": true,
	"search_hybrid":       true,
	"search_keyword":      true,
	"explore_graph":       true,
	"read_code":           true,
	"diagnose_index":      true,
	"wiki_preflight":      true,
	"wiki_resolve":        true,
}

// follower routes the tool table of a server that may not hold the workspace
// lock. There is one rule: a server that is not bootstrapped asks whether a
// leader is listening, and answers through it if so. Everything else falls
// out of that — a fresh workspace has no leader and takes the local path, a
// second session finds one and forwards, and when the leader goes away the
// next look finds nobody and the local path takes the lock: promotion is not
// a separate mechanism.
type follower struct {
	ws      *workspace.Workspace
	version string

	mu     sync.Mutex
	client *follow.Client
}

// Follow makes the tool table work from whichever side of the workspace lock
// this server ends up on: holding it, the server listens and runs the calls
// of the sessions that lost it; having lost it, every tool answers through
// the holder. Call after Register.
func Follow(reg *mcp.Registry, ws *workspace.Workspace, version string) {
	f := &follower{ws: ws, version: version}
	ws.OnLeader(func(dataDir string) (io.Closer, error) {
		return follow.Listen(dataDir, version, func(ctx context.Context, tool string, args json.RawMessage) (string, bool, bool) {
			t, ok := reg.Get(tool)
			if !ok {
				return "", false, false
			}
			text, isErr := t.Handler(ctx, args)
			return text, isErr, true
		}, ws.Log)
	})
	for _, t := range reg.List() {
		wrapped := *t
		wrapped.Handler = f.wrap(t.Name, t.Handler)
		reg.Register(&wrapped)
	}
}

func (f *follower) wrap(name string, local mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (string, bool) {
		deadline := time.Now().Add(takeoverWait)
		followed := false // this call has seen a leader, so a miss is a takeover, not a fresh tree
		for {
			if f.ws.Bootstrapped() {
				return local(ctx, args)
			}
			c, err := f.leader()
			var inc *follow.IncompatibleError
			if errors.As(err, &inc) {
				st := f.ws.FSM.Status()
				return mcp.ErrorXML(mcp.ErrLockHeld, fmt.Sprintf(
					"another session (pid %d) holds this workspace's index and runs graphin %s; "+
						"this server is %s and cannot answer through it. Restart the older session "+
						"to bring both onto one version.", inc.LeaderPID, inc.LeaderVersion, f.version), &st), true
			}
			if c != nil {
				followed = true
				text, isErr, known, cerr := c.Call(ctx, name, args)
				switch {
				case cerr == nil && known:
					return text, isErr
				case cerr == nil:
					// Same major.minor, so this should not happen; if it does,
					// the local handler's own guard gives the honest answer.
					return local(ctx, args)
				case ctx.Err() != nil:
					return "", true // cancelled: the transport sends nothing anyway
				case errors.Is(cerr, follow.ErrLeaderGone) && !rerunnable[name]:
					st := f.ws.FSM.Status()
					return mcp.ErrorXML(mcp.ErrLeaderChanged,
						"the session that holds this workspace's index ended while running this call, "+
							"so it may or may not have taken effect. Check before repeating it.", &st), true
				}
				f.drop(c)
			} else if f.ws.EnsureBootstrapped(ctx, "tool") {
				continue // we hold the lock now: leader
			} else if !followed {
				// No leader and no lock to take: never indexed, or held by
				// a server that predates the socket. The local guard says which.
				return local(ctx, args)
			}
			if time.Now().After(deadline) {
				return local(ctx, args)
			}
			select {
			case <-ctx.Done():
				return "", true
			case <-time.After(takeoverPoll):
			}
		}
	}
}

// leader returns the live connection, dialling if there is none. (nil, nil)
// means nobody is listening.
func (f *follower) leader() (*follow.Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.client != nil {
		select {
		case <-f.client.Done():
			f.client = nil
		default:
			return f.client, nil
		}
	}
	c, err := follow.Dial(f.ws.Dir, f.version)
	if err != nil {
		if errors.Is(err, follow.ErrNoLeader) {
			return nil, nil
		}
		return nil, err
	}
	f.client = c
	f.ws.Log.Event("follow_connected", map[string]any{"leader_pid": c.LeaderPID, "leader_version": c.LeaderVer})
	go f.succeed(c)
	return c, nil
}

func (f *follower) drop(c *follow.Client) {
	f.mu.Lock()
	if f.client == c {
		f.client = nil
	}
	f.mu.Unlock()
	c.Close()
}

// succeed waits for the leader to go away and then contends for the lock, so
// the index keeps a watcher even if no tool call arrives to trigger it. Losing
// is fine: whoever won is listening, and the next call finds them.
func (f *follower) succeed(c *follow.Client) {
	<-c.Done()
	f.drop(c)
	f.ws.Log.Event("follow_leader_gone", map[string]any{"leader_pid": c.LeaderPID})
	deadline := time.Now().Add(takeoverWait)
	for time.Now().Before(deadline) {
		if f.ws.EnsureBootstrapped(context.Background(), "succession") {
			return
		}
		if l, _ := f.leader(); l != nil {
			return
		}
		time.Sleep(takeoverPoll)
	}
}
