package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixed(l *File) *File {
	at := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return at }
	return l
}

func TestPrintfAppendsTimestampedEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	l := fixed(New(dir))
	l.Printf("switched to %s", "a@x.com")
	l.Printf("update output:\n%s\n", "done")
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "ccswitch.log"))
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-11-09T10:00:00Z switched to a@x.com\n2026-11-09T10:00:00Z update output:\ndone\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if l.Path() != filepath.Join(dir, "ccswitch.log") {
		t.Errorf("Path = %q", l.Path())
	}
}

func TestRotatesAtOneMegabyte(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ccswitch.log")
	old := strings.Repeat("x", maxSize-40) + "\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := fixed(New(dir))
	l.Printf("short")
	if got, _ := os.ReadFile(path); string(got) != old+"2026-11-09T10:00:00Z short\n" {
		t.Fatalf("an entry that fits should append, file is %d bytes", len(got))
	}
	l.Printf("this one does not fit")
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "2026-11-09T10:00:00Z this one does not fit\n" {
		t.Errorf("new log: %q", got)
	}
	if got, _ := os.ReadFile(path + ".1"); !strings.HasPrefix(string(got), "xxx") {
		t.Error("the full log should have moved to .1, replacing the older one")
	}
}

func TestErrWhenTheLogCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l := New(file)
	l.Printf("lost")
	if l.Err() == nil {
		t.Error("folder is a file: want an error")
	}
	if err := os.Mkdir(filepath.Join(dir, "ccswitch.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	l = New(dir)
	l.Printf("lost")
	if l.Err() == nil {
		t.Error("log is a folder: want an error")
	}
}

func TestErrWhenRotationFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ccswitch.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", maxSize)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(path+".1", "in-the-way"), 0o700); err != nil {
		t.Fatal(err)
	}
	l := New(dir)
	l.Printf("lost")
	if err := l.Err(); err == nil || !strings.Contains(err.Error(), "rotate") {
		t.Errorf("got %v, want a rotate error", err)
	}
}
