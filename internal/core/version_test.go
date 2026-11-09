package core_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCheckVersion(t *testing.T) {
	r := newRig(t, "a@x")
	st := r.mustNoError(r.sw.CheckVersion(ctx))
	if st.Installed != "2.1.300" || st.Latest != "2.1.319" || !st.VersionReadAt.Equal(t0) {
		t.Errorf("installed %q latest %q read %v", st.Installed, st.Latest, st.VersionReadAt)
	}
}

func TestCheckVersionLatestFailureKeepsInstalled(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.CheckVersion(ctx))
	r.clock.Advance(time.Hour)
	r.claude.VersionOut = "2.1.319 (Claude Code)"
	r.releases.Err = errors.New("registry down")

	st, err := r.sw.CheckVersion(ctx)

	r.mustFail(st, err, "registry down")
	if st.Installed != "2.1.319" || !st.VersionReadAt.Equal(t0) {
		t.Errorf("installed %q, read at %v (a failed check must not count as a check)", st.Installed, st.VersionReadAt)
	}
}

func TestCheckVersionUnreadable(t *testing.T) {
	for name, setup := range map[string]func(r *rig){
		"version fails":     func(r *rig) { r.claude.VersionErr = errors.New("claude not found") },
		"version garbage":   func(r *rig) { r.claude.VersionOut = "command not found" },
		"latest is garbage": func(r *rig) { r.releases.Version = "<html>" },
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, "a@x")
			setup(r)
			st, err := r.sw.CheckVersion(ctx)
			if err == nil || st.LastError == "" || !st.VersionReadAt.IsZero() {
				t.Errorf("err %v, LastError %q, read at %v", err, st.LastError, st.VersionReadAt)
			}
		})
	}
}

func TestPollChecksTheVersionEverySixHours(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.Poll(ctx, true))
	r.clock.Advance(6*time.Hour - time.Minute)
	r.mustNoError(r.sw.Poll(ctx, true))
	if r.releases.Calls != 1 {
		t.Errorf("within 6 h: %d checks", r.releases.Calls)
	}
	r.clock.Advance(time.Minute)
	r.mustNoError(r.sw.Poll(ctx, true))
	if r.releases.Calls != 2 {
		t.Errorf("after 6 h: %d checks", r.releases.Calls)
	}
}

func TestPollVersionFailureIsShownAndRetried(t *testing.T) {
	r := newRig(t, "a@x")
	r.releases.Err = errors.New("registry down")
	st, err := r.sw.Poll(ctx, true)
	r.mustFail(st, err, "registry down")
	if u := stateAccount(t, st, "a@x").Usage; u.Err != "" || len(u.Limits) != 1 {
		t.Errorf("a version failure must not stop the usage read: %+v", u)
	}
	r.releases.Err = nil
	r.clock.Advance(2 * time.Minute)
	st = r.mustNoError(r.sw.Poll(ctx, false))
	if st.Latest != "2.1.319" {
		t.Errorf("Latest = %q", st.Latest)
	}
}

func TestUpdateSuccess(t *testing.T) {
	r := newRig(t, "a@x")
	r.claude.UpdateOut = "Updating to 2.1.319...\nDone\n"
	r.claude.UpdatedVersion = "2.1.319 (Claude Code)"

	st := r.mustNoError(r.sw.Update(ctx))

	if st.Installed != "2.1.319" {
		t.Errorf("Installed = %q", st.Installed)
	}
	if !slices.Equal(r.claude.Calls, []string{"update", "version"}) {
		t.Errorf("calls = %v", r.claude.Calls)
	}
	if r.sent("Claude Code 2.1.319 installed — new sessions use it") != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
	if !strings.Contains(strings.Join(r.log.Lines, "\n"), "Updating to 2.1.319") {
		t.Errorf("the update output must reach the log: %v", r.log.Lines)
	}
	if r.lock.Acquired != 1 || r.store.unlockedSaves != 0 {
		t.Errorf("lock acquired %d", r.lock.Acquired)
	}
}

func TestUpdateFailure(t *testing.T) {
	r := newRig(t, "a@x")
	r.claude.UpdateOut = "EACCES"
	r.claude.UpdateErr = errors.New("exit status 1")

	st, err := r.sw.Update(ctx)

	r.mustFail(st, err, "exit status 1")
	if r.sent("Claude Code update failed") != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
	if !strings.Contains(strings.Join(r.log.Lines, "\n"), "EACCES") {
		t.Errorf("the failed update's output must reach the log: %v", r.log.Lines)
	}
}

func TestUpdateVersionUnreadableAfterwards(t *testing.T) {
	r := newRig(t, "a@x")
	r.claude.VersionErr = errors.New("claude not found")
	st, err := r.sw.Update(ctx)
	r.mustFail(st, err, "claude not found")
	if r.sent("Claude Code update failed") != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
}
