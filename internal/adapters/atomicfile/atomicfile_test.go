package atomicfile_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/atomicfile"
)

func TestWriteReplacesAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new" {
		t.Fatalf("got %q, %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(path)
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
		}
	}
}

func TestWriteMissingFolder(t *testing.T) {
	err := atomicfile.Write(filepath.Join(t.TempDir(), "nope", "f.json"), []byte("x"), 0o600)
	if err == nil {
		t.Fatal("want an error for a missing folder")
	}
}

func TestWriteOverDirectoryFailsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.Write(target, []byte("x"), 0o600); err == nil {
		t.Fatal("want an error renaming over a non-empty folder")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

// On Windows a reader holding the target open (the statusline reads
// state.json without the lock) makes a rename fail until it closes; Write
// waits it out.
func TestWriteWaitsForAReaderToClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = reader.Close()
		close(closed)
	}()
	err = atomicfile.Write(path, []byte("new"), 0o600)
	<-closed
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("got %q", got)
	}
}
