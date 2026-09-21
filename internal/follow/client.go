package follow

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const dialTimeout = 2 * time.Second

// Client is a follower's connection to the leader.
type Client struct {
	conn      net.Conn
	enc       *json.Encoder
	LeaderPID int
	LeaderVer string

	wmu sync.Mutex // serialises writes

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]chan msg
	gone    bool
	done    chan struct{}
}

// Dial connects to the leader of dataDir and shakes hands. ErrNoLeader when
// nothing answers; *IncompatibleError when the leader refuses this version.
func Dial(dataDir, version string) (*Client, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, addrName))
	if err != nil {
		return nil, ErrNoLeader
	}
	conn, err := net.DialTimeout("unix", strings.TrimSpace(string(raw)), dialTimeout)
	if err != nil {
		return nil, ErrNoLeader
	}
	c := &Client{conn: conn, enc: json.NewEncoder(conn),
		pending: map[uint64]chan msg{}, done: make(chan struct{})}
	dec := json.NewDecoder(conn)

	_ = conn.SetDeadline(time.Now().Add(dialTimeout))
	if err := c.enc.Encode(&msg{Type: "hello", Version: version, PID: os.Getpid()}); err != nil {
		conn.Close()
		return nil, ErrNoLeader
	}
	var hello msg
	if err := dec.Decode(&hello); err != nil || hello.Type != "hello" {
		conn.Close()
		return nil, ErrNoLeader
	}
	_ = conn.SetDeadline(time.Time{})
	if !hello.OK {
		conn.Close()
		return nil, &IncompatibleError{LeaderVersion: hello.Version, LeaderPID: hello.PID}
	}
	c.LeaderPID, c.LeaderVer = hello.PID, hello.Version
	go c.read(dec)
	return c, nil
}

func (c *Client) read(dec *json.Decoder) {
	for {
		var m msg
		if err := dec.Decode(&m); err != nil {
			break
		}
		if m.Type != "result" {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[m.ID]
		delete(c.pending, m.ID)
		c.mu.Unlock()
		if ok {
			ch <- m
		}
	}
	c.mu.Lock()
	c.gone = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
	c.conn.Close()
	close(c.done)
}

// Done is closed once the leader is gone — the moment to contend for the lock.
func (c *Client) Done() <-chan struct{} { return c.done }

// Call runs one tool on the leader. known=false means the leader has no such
// tool. ErrLeaderGone when the connection died before the answer; ctx.Err()
// when the caller gave up, in which case the leader is told to stop.
func (c *Client) Call(ctx context.Context, tool string, args json.RawMessage) (text string, isErr, known bool, err error) {
	ch := make(chan msg, 1)
	c.mu.Lock()
	if c.gone {
		c.mu.Unlock()
		return "", false, false, ErrLeaderGone
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.send(&msg{Type: "call", ID: id, Tool: tool, Args: args}); err != nil {
		c.forget(id)
		return "", false, false, ErrLeaderGone
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return "", false, false, ErrLeaderGone
		}
		return m.Text, m.IsError, !m.Unknown, nil
	case <-ctx.Done():
		c.forget(id)
		_ = c.send(&msg{Type: "cancel", ID: id})
		return "", false, false, ctx.Err()
	}
}

func (c *Client) send(m *msg) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.enc.Encode(m)
}

func (c *Client) forget(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Close hangs up. The leader cancels whatever this follower still had running.
func (c *Client) Close() error { return c.conn.Close() }
