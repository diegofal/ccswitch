package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecondHolderWaitsForRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "ccswitch.lock")
	release, err := New(path).Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r, err := New(path).Acquire(ctx)
		if err == nil {
			r()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("second holder got the lock while the first held it (err %v)", err)
	case <-time.After(400 * time.Millisecond):
	}
	release()
	release() // a second release does nothing
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("second holder after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second holder still waiting after release")
	}
}

func TestAcquireGivesUpAtDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ccswitch.lock")
	release, err := New(path).Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := New(path).Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the deadline", err)
	}
	if d := time.Since(start); d < 200*time.Millisecond {
		t.Errorf("gave up after %v, before the deadline", d)
	}
}

func TestAcquireFailsWhenPathUnusable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(filepath.Join(file, "ccswitch.lock")).Acquire(context.Background()); err == nil {
		t.Error("folder is a file: want an error")
	}
	if _, err := New(dir).Acquire(context.Background()); err == nil {
		t.Error("lock path is a folder: want an error")
	}
}
