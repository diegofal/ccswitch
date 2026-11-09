// Package logfile is ports.Logger: ccswitch.log in the app data folder, one
// timestamped entry per Printf. At 1 MB the file moves to ccswitch.log.1
// (replacing the previous one) and a new one starts, so the log never grows
// past about 2 MB. Tray and CLI both append to it; each entry opens, appends
// and closes the file, so neither holds it.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegofal/ccswitch/internal/ports"
)

// Name is the log's file name in the data folder.
const Name = "ccswitch.log"

// maxSize is the size at which the log rotates.
const maxSize = 1 << 20

// File appends to <dir>/ccswitch.log.
type File struct {
	path string
	now  func() time.Time

	mu  sync.Mutex
	err error
}

var _ ports.Logger = (*File)(nil)

// New returns the log in dir (normally paths.DataDir()); the folder is
// created on the first entry.
func New(dir string) *File { return &File{path: filepath.Join(dir, Name), now: time.Now} }

// Path is the log file, for "Open log".
func (l *File) Path() string { return l.path }

// Printf appends one entry: the local time, then the message.
func (l *File) Printf(format string, args ...any) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	entry := l.now().Format(time.RFC3339) + " " + msg + "\n"
	l.mu.Lock()
	defer l.mu.Unlock()
	l.err = l.write(entry)
}

// Err is the error of the last entry, nil when it was written. Printf
// cannot return one, so a front end asks here to show a log it cannot write.
func (l *File) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

func (l *File) write(entry string) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("log: %w", err)
	}
	if fi, err := os.Stat(l.path); err == nil && fi.Size()+int64(len(entry)) > maxSize {
		if err := os.Rename(l.path, l.path+".1"); err != nil {
			return fmt.Errorf("log: rotate: %w", err)
		}
	}
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("log: %w", err)
	}
	if _, err := f.WriteString(entry); err != nil {
		_ = f.Close()
		return fmt.Errorf("log: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("log: %w", err)
	}
	return nil
}
