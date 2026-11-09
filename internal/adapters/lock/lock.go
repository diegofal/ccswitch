// Package lock is ports.Lock as an OS file lock: one writer at a time across
// the tray and every CLI run (PLAN D12). The OS drops the lock when its
// holder exits, so a crash never leaves it stuck.
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/diegofal/ccswitch/internal/ports"
)

// poll is how often Acquire tries again while another process holds the lock.
const poll = 100 * time.Millisecond

// errBusy is what tryLock returns when another holder has the lock.
var errBusy = errors.New("lock busy")

// File is a lock on one file: <DataDir>/ccswitch.lock for operations,
// app.lock for the tray's single instance.
type File struct{ path string }

var _ ports.Lock = (*File)(nil)

// New returns the lock on path. The file and its folder are created on
// Acquire.
func New(path string) *File { return &File{path: path} }

// Acquire waits until it holds the lock, trying every 100 ms, or until ctx
// is done. release unlocks it; calling it again does nothing.
func (l *File) Acquire(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return nil, fmt.Errorf("lock %s: %w", l.path, err)
	}
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", l.path, err)
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		err := tryLock(f)
		if err == nil {
			var once sync.Once
			return func() {
				once.Do(func() {
					_ = unlock(f)
					_ = f.Close()
				})
			}, nil
		}
		if !errors.Is(err, errBusy) {
			_ = f.Close()
			return nil, fmt.Errorf("lock %s: %w", l.path, err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("another ccswitch is busy (lock %s): %w", l.path, ctx.Err())
		case <-t.C:
		}
	}
}
