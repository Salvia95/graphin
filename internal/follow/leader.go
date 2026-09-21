package follow

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/Salvia95/graphin/internal/obs"
)

// CallFunc runs one tool on the leader. known=false means no such tool.
type CallFunc func(ctx context.Context, tool string, args json.RawMessage) (text string, isErr, known bool)

// Leader is the listening side. Only the lock holder may create one: Listen
// removes whatever socket is already at the path, which is only safe for the
// process the lock says is alone.
type Leader struct {
	ln      net.Listener
	path    string
	addr    string
	version string
	call    CallFunc
	log     *obs.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

// Listen starts serving followers of dataDir.
func Listen(dataDir, version string, call CallFunc, lg *obs.Logger) (*Leader, error) {
	path := sockPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path) // a crashed leader's leftover; we hold the lock
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	addr := filepath.Join(dataDir, addrName)
	if err := os.WriteFile(addr, []byte(path+"\n"), 0o600); err != nil {
		ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &Leader{ln: ln, path: path, addr: addr, version: version, call: call, log: lg,
		ctx: ctx, cancel: cancel, conns: map[net.Conn]struct{}{}}
	l.wg.Add(1)
	go l.accept()
	lg.Event("leader_listen", map[string]any{"socket": path})
	return l, nil
}

func (l *Leader) accept() {
	defer l.wg.Done()
	for {
		c, err := l.ln.Accept()
		if err != nil {
			return // closed
		}
		l.mu.Lock()
		if l.closed { // accepted while Close was sweeping: nobody would hang it up
			l.mu.Unlock()
			c.Close()
			return
		}
		l.conns[c] = struct{}{}
		l.mu.Unlock()
		l.wg.Add(1)
		go l.serve(c)
	}
}

func (l *Leader) serve(c net.Conn) {
	defer l.wg.Done()
	// Every call of this follower dies with its connection: a session that
	// ended has nobody left to read the answer.
	ctx, cancel := context.WithCancel(l.ctx)
	var calls sync.WaitGroup
	defer func() {
		cancel()
		calls.Wait()
		c.Close()
		l.mu.Lock()
		delete(l.conns, c)
		l.mu.Unlock()
	}()

	dec := json.NewDecoder(c)
	enc := json.NewEncoder(c)
	var wmu sync.Mutex
	send := func(m *msg) {
		wmu.Lock()
		defer wmu.Unlock()
		_ = enc.Encode(m)
	}

	var hello msg
	if err := dec.Decode(&hello); err != nil || hello.Type != "hello" {
		return
	}
	ok := Compatible(hello.Version, l.version)
	reply := &msg{Type: "hello", Version: l.version, PID: os.Getpid(), OK: ok}
	if !ok {
		reply.Reason = "version"
	}
	send(reply)
	l.log.Event("follower_hello", map[string]any{
		"pid": hello.PID, "version": hello.Version, "accepted": ok})
	if !ok {
		return
	}

	var cmu sync.Mutex
	cancels := map[uint64]context.CancelFunc{}
	for {
		var m msg
		if err := dec.Decode(&m); err != nil {
			return
		}
		switch m.Type {
		case "call":
			cctx, ccancel := context.WithCancel(ctx)
			cmu.Lock()
			cancels[m.ID] = ccancel
			cmu.Unlock()
			calls.Add(1)
			go func(m msg) {
				defer calls.Done()
				defer func() {
					cmu.Lock()
					delete(cancels, m.ID)
					cmu.Unlock()
					ccancel()
				}()
				res := &msg{Type: "result", ID: m.ID}
				func() {
					defer func() {
						if r := recover(); r != nil {
							l.log.Event("tool_panic", map[string]any{"via": "follower", "tool": m.Tool})
							res.Text, res.IsError = "internal error", true
						}
					}()
					text, isErr, known := l.call(cctx, m.Tool, m.Args)
					res.Text, res.IsError, res.Unknown = text, isErr, !known
				}()
				send(res)
			}(m)
		case "cancel":
			cmu.Lock()
			if f, ok := cancels[m.ID]; ok {
				f()
			}
			cmu.Unlock()
		}
	}
}

// Close stops listening, drops every follower and removes the socket. The
// followers see EOF, which is their signal to contend for the lock.
func (l *Leader) Close() error {
	l.cancel()
	err := l.ln.Close()
	l.mu.Lock()
	l.closed = true
	for c := range l.conns {
		c.Close()
	}
	l.mu.Unlock()
	l.wg.Wait()
	_ = os.Remove(l.path)
	_ = os.Remove(l.addr)
	return err
}
