package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

func TestWatchOnce(t *testing.T) {
	r := newRig(t, "a@x.com", "b@y.com")
	code, out, errOut := r.run("watch", "--once")
	if code != 0 || errOut != "" || !strings.Contains(out, "* a@x.com") || !strings.Contains(out, "10%") {
		t.Errorf("code %d, stdout:\n%s\nstderr %q", code, out, errOut)
	}
	if len(r.usage.Calls) != 2 || r.lock.Acquired != 1 {
		t.Errorf("one round reading both accounts: usage %v, lock %d", r.usage.Calls, r.lock.Acquired)
	}
}

// watchScript drives the watch loop: before round i+2 it runs between[i];
// once they are spent it cancels the context.
type watchScript struct {
	r       *rig
	cancel  context.CancelFunc
	between []func()
	waits   []time.Duration
}

func (s *watchScript) after(d time.Duration) <-chan time.Time {
	s.waits = append(s.waits, d)
	ch := make(chan time.Time, 1)
	if len(s.waits) > len(s.between) {
		s.cancel()
		return ch
	}
	s.between[len(s.waits)-1]()
	s.r.clock.Advance(d)
	ch <- s.r.clock.Now()
	return ch
}

func runWatch(t *testing.T, r *rig, between ...func()) (s *watchScript, code int, out, errOut string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s = &watchScript{r: r, cancel: cancel, between: between}
	r.deps.After = s.after
	code, out, errOut = r.runCtx(ctx, "watch")
	return s, code, out, errOut
}

func TestWatchLoopNotifiesEachNewFailureOnce(t *testing.T) {
	r := newRig(t, "a@x.com")
	broken := errors.New("credentials unreadable")
	r.login.ReadErr = broken
	s, code, out, errOut := runWatch(t, r,
		func() {},                           // round 2 fails the same way
		func() { r.login.ReadErr = nil },    // round 3 works
		func() { r.login.ReadErr = broken }, // round 4 fails again
	)
	if code != 0 {
		t.Fatalf("Ctrl-C is a normal exit, got %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if lines[0] != "Watching; Ctrl-C stops." || len(lines) != 5 {
		t.Fatalf("one line per round:\n%s", out)
	}
	if lines[1] != "09:00  no account" {
		t.Errorf("round 1: %q", lines[1])
	}
	if lines[3] != "09:04  a@x · weekly 10% · Claude Code 2.1.300 (2.1.319 available)" {
		t.Errorf("round 3: %q", lines[3])
	}
	if n := strings.Count(errOut, "credentials unreadable"); n != 3 {
		t.Errorf("every failed round is printed, got %d:\n%s", n, errOut)
	}
	var mine []string
	for _, m := range r.notify.Sent {
		if strings.HasPrefix(m, "ccswitch: ") {
			mine = append(mine, m)
		}
	}
	if len(mine) != 2 {
		t.Errorf("notified once per new failure: %v", r.notify.Sent)
	}
	for _, w := range s.waits {
		if w != 2*time.Minute {
			t.Errorf("waits %v, want the active poll interval", s.waits)
		}
	}
}

func TestWatchNotifyFailureIsPrintedAndRetried(t *testing.T) {
	r := newRig(t, "a@x.com")
	r.login.ReadErr = errors.New("credentials unreadable")
	r.notify.Err = errors.New("no toast service")
	_, code, _, errOut := runWatch(t, r, func() {})
	if code != 0 || strings.Count(errOut, "ccswitch: notify: no toast service") != 2 {
		t.Errorf("code %d, stderr:\n%s", code, errOut)
	}
}

func TestWatchWithBrokenSettings(t *testing.T) {
	r := newRig(t, "a@x.com")
	zero := domain.Settings{} // PollActive 0 must not spin
	r.store.Settings = &zero
	s, code, _, errOut := runWatch(t, r)
	if code != 0 || errOut != "" || len(s.waits) != 1 || s.waits[0] != 2*time.Minute {
		t.Errorf("code %d, waits %v, stderr %q", code, s.waits, errOut)
	}
}

func TestWatchStopsWhenCancelled(t *testing.T) {
	r := newRig(t, "a@x.com")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, out, _ := r.runCtx(ctx, "watch")
	if code != 0 || out != "Watching; Ctrl-C stops.\n" {
		t.Errorf("code %d, stdout %q", code, out)
	}
}

func TestWatchUsesTimeAfterByDefault(t *testing.T) {
	r := newRig(t, "a@x.com")
	fast := domain.DefaultSettings()
	fast.PollActive = time.Millisecond
	r.store.Settings = &fast
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	code, out, _ := r.runCtx(ctx, "watch")
	if code != 0 || strings.Count(out, "a@x · weekly 10%") < 2 {
		t.Errorf("code %d, stdout:\n%s", code, out)
	}
}
