// Package atomicfile replaces a file so a reader sees the old bytes or the
// new ones, never a half-written file: Claude Code may read its login at any
// moment, and a crash mid-write must not lose it.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Write puts data at path through a temp file in the same folder (so the
// rename stays on one volume), synced to disk before the rename, with perm
// as the file's mode.
func Write(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), perm)
	}
	if err == nil {
		err = rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// rename retries briefly: on Windows a reader that has the target open (the
// statusline reads state.json without the lock) makes the rename fail for
// the moment it takes to read.
func rename(from, to string) error {
	var err error
	for range 10 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}
