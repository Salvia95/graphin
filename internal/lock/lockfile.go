// Package lock implements the single-writer workspace lock (§4): a lockfile
// carrying the owner PID, refreshed by an mtime heartbeat, with orphan
// detection and automatic steal.
package lock

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Salvia95/graphin/internal/obs"
)

// ErrHeld is returned when another live process owns the lock.
var ErrHeld = errors.New("lock held by another process")

// Options tunes timing; zero values select the spec defaults.
type Options struct {
	Heartbeat time.Duration // mtime touch interval (default 3s)
	Stale     time.Duration // owner considered orphaned beyond this (default 10s)
}

// Lock is a held workspace lock with a running heartbeat.
type Lock struct {
	path    string
	stop    chan struct{}
	done    chan struct{}
	release sync.Once
}

// Acquire takes the lock at path. A lock is live only when its recorded PID
// is alive AND its mtime is fresh; anything else is an orphan and is stolen.
func Acquire(path string, opt Options, lg *obs.Logger) (*Lock, error) {
	if opt.Heartbeat <= 0 {
		opt.Heartbeat = 3 * time.Second
	}
	if opt.Stale <= 0 {
		opt.Stale = 10 * time.Second
	}

	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Sync()
			_ = f.Close()
			return start(path, opt), nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		live, lerr := isLive(path, opt.Stale)
		if lerr != nil {
			if os.IsNotExist(lerr) {
				continue // vanished between checks: retry create
			}
			return nil, lerr
		}
		if live {
			return nil, ErrHeld
		}

		// Orphaned lock: replace atomically, then verify we won.
		tmp := fmt.Sprintf("%s.steal-%d", path, os.Getpid())
		if err := os.WriteFile(tmp, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
			return nil, err
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Remove(tmp)
			return nil, err
		}
		b, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
			return nil, ErrHeld // lost a steal race
		}
		lg.Event("lock_steal", map[string]any{"path": path})
		return start(path, opt), nil
	}
	return nil, ErrHeld
}

// isLive reports whether the lock at path belongs to a live owner: PID alive
// and mtime within stale (§4 교차 검증).
func isLive(path string, stale time.Duration) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if time.Since(fi.ModTime()) > stale {
		return false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return false, nil // corrupt lockfile counts as orphaned
	}
	return pidAlive(pid), nil
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}

func start(path string, opt Options) *Lock {
	l := &Lock{path: path, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(l.done)
		t := time.NewTicker(opt.Heartbeat)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				now := time.Now()
				_ = os.Chtimes(l.path, now, now)
			}
		}
	}()
	return l
}

// Release stops the heartbeat and removes the lockfile. Safe to call twice.
func (l *Lock) Release() error {
	var err error
	l.release.Do(func() {
		close(l.stop)
		<-l.done
		err = os.Remove(l.path)
	})
	return err
}
