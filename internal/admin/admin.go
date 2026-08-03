package admin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
	"github.com/Salvia95/graphin/internal/workspace"
)

// Serve runs the admin page at addr until ctx is canceled. addr must resolve
// to a loopback host — the page is strictly local. Returns after a graceful
// shutdown so callers can tear the workspace down safely afterwards.
func Serve(ctx context.Context, ws *workspace.Workspace, addr, version string, lg *obs.Logger) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("admin: bad address %q: %w", addr, err)
	}
	if !hostAllowed(host) {
		return fmt.Errorf("admin: refusing non-loopback address %q", addr)
	}
	s, err := NewServer(ws, version, lg)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("admin: listen %s: %w", addr, err)
	}
	lg.Event("admin_listen", map[string]any{"addr": ln.Addr().String()})

	hs := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = hs.Shutdown(sctx)
		case <-done:
		}
	}()
	if err := hs.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
