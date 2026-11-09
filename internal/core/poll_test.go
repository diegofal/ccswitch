package core_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

func TestPollReadsEveryAccount(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.usage.Usages[access("b@x")] = usage(41)

	st := r.mustNoError(r.sw.Poll(ctx, false))

	if len(st.Accounts) != 2 || st.Active != "a@x" {
		t.Fatalf("state = %+v", st)
	}
	b := stateAccount(t, st, "b@x").Usage
	if l, _ := b.Binding(); l.Percent != 41 || !b.ReadAt.Equal(t0) {
		t.Errorf("b usage = %+v", b)
	}
	if !st.UpdatedAt.Equal(t0) {
		t.Errorf("UpdatedAt = %v", st.UpdatedAt)
	}
}

func TestPollReadsOnlyWhatIsDue(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.mustNoError(r.sw.Poll(ctx, false))
	r.usage.Calls = nil

	r.clock.Advance(2 * time.Minute) // active due, inactive not
	r.mustNoError(r.sw.Poll(ctx, false))
	if !slices.Equal(r.usage.Calls, []string{"fetch " + access("a@x")}) {
		t.Errorf("after 2 min: %v", r.usage.Calls)
	}

	r.usage.Calls = nil
	r.clock.Advance(13 * time.Minute) // both due
	r.mustNoError(r.sw.Poll(ctx, false))
	if len(r.calls("fetch")) != 2 {
		t.Errorf("after 15 min: %v", r.usage.Calls)
	}

	r.usage.Calls = nil
	r.mustNoError(r.sw.Poll(ctx, true))
	if len(r.calls("fetch")) != 2 {
		t.Errorf("full poll: %v", r.usage.Calls)
	}
}

func TestPollUnreadableUsageIsRedAndNeverSwitches(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.settings(func(s *domain.Settings) { s.Auto = true })
	r.usage.Errs[access("a@x")] = errors.New("HTTP 500")

	st := r.mustNoError(r.sw.Poll(ctx, true))

	if u := stateAccount(t, st, "a@x").Usage; u.Err != "HTTP 500" || !u.ReadAt.Equal(t0) || len(u.Limits) != 0 {
		t.Errorf("a usage = %+v", u)
	}
	if len(r.login.Writes) != 0 || st.Active != "a@x" {
		t.Error("unreadable usage must not switch")
	}
}

func TestPollUnreadableKeepsNoStaleNumbers(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.Poll(ctx, true))
	r.usage.Errs[access("a@x")] = errors.New("timeout")
	st := r.mustNoError(r.sw.Poll(ctx, true))
	if u := stateAccount(t, st, "a@x").Usage; len(u.Limits) != 0 || u.Err == "" {
		t.Errorf("a failed read must replace the old numbers: %+v", u)
	}
}

func TestActiveAccountIsNeverRefreshed(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.clock.Advance(13 * time.Hour)
	now := r.clock.Now()
	r.usage.RefreshResult = domain.Tokens{Access: "fake-access-new", Refresh: "fake-refresh-new", ExpiresAt: now.Add(12 * time.Hour)}
	r.usage.Usages["fake-access-new"] = usage(5)
	r.usage.Errs[access("a@x")] = ports.ErrUnauthorized

	st := r.mustNoError(r.sw.Poll(ctx, true))

	if got := r.calls("refresh"); !slices.Equal(got, []string{"refresh " + refresh("b@x")}) {
		t.Errorf("refreshes = %v, want only the inactive b", got)
	}
	if u := stateAccount(t, st, "a@x").Usage; u.Err == "" {
		t.Error("a refused active token must show unreadable, not be refreshed")
	}
	if u := stateAccount(t, st, "b@x").Usage; u.Err != "" {
		t.Errorf("b usage = %+v", u)
	}
	if r.vault.Saves == 0 {
		t.Error("refreshed tokens must be saved to the vault")
	}
	for _, l := range r.log.Lines {
		if strings.Contains(l, "fake-access") || strings.Contains(l, "fake-refresh") {
			t.Errorf("a token reached the log: %q", l)
		}
	}
}

func TestInactiveRefreshedWhenItsRefreshTokenDiesWithinAWeek(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	// b's refresh token dies at t0+28d; its access token is still good.
	r.clock.T = t0.Add(21*24*time.Hour + time.Hour)
	now := r.clock.Now()
	for i, a := range r.vault.Accounts {
		r.vault.Accounts[i] = withTokens(t, a, domain.Tokens{Access: access(a.Email), Refresh: refresh(a.Email), ExpiresAt: now.Add(6 * time.Hour)})
	}
	r.usage.RefreshResult = domain.Tokens{Access: "fake-access-new", Refresh: "fake-refresh-new", ExpiresAt: now.Add(12 * time.Hour)}
	r.usage.Usages["fake-access-new"] = usage(5)

	r.mustNoError(r.sw.Poll(ctx, true))
	if got := r.calls("refresh"); !slices.Equal(got, []string{"refresh " + refresh("b@x")}) {
		t.Fatalf("refreshes = %v, want b's once", got)
	}

	// The response carries no new refresh-token expiry, so the stored one
	// is still inside the week: once refreshed, it must not repeat.
	r.clock.Advance(15 * time.Minute)
	r.mustNoError(r.sw.Poll(ctx, true))
	if got := r.calls("refresh"); len(got) != 1 {
		t.Errorf("refreshes = %v, want no repeat", got)
	}
}

func TestInactiveUnauthorizedRefreshesOnceAndRetries(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.usage.Errs[access("b@x")] = fmt.Errorf("usage: %w", ports.ErrUnauthorized)
	r.usage.RefreshResult = domain.Tokens{Access: "fake-access-new", Refresh: "fake-refresh-new", ExpiresAt: t0.Add(12 * time.Hour)}
	r.usage.Usages["fake-access-new"] = usage(7)

	st := r.mustNoError(r.sw.Poll(ctx, true))

	want := []string{"fetch " + access("a@x"), "fetch " + access("b@x"), "refresh " + refresh("b@x"), "fetch fake-access-new"}
	if !slices.Equal(r.usage.Calls, want) {
		t.Errorf("calls = %v, want %v", r.usage.Calls, want)
	}
	if l, _ := stateAccount(t, st, "b@x").Usage.Binding(); l.Percent != 7 {
		t.Errorf("b usage = %+v", stateAccount(t, st, "b@x").Usage)
	}
}

func TestRefreshFailures(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(r *rig)
		usageErr  string
		lastError string
	}{
		{"refresh refused", func(r *rig) { r.usage.Errs[refresh("b@x")] = fmt.Errorf("token: %w", ports.ErrUnauthorized) }, "login expired — add it again", ""},
		{"refresh fails", func(r *rig) { r.usage.Errs[refresh("b@x")] = errors.New("HTTP 503") }, "HTTP 503", ""},
		{"incomplete tokens", func(r *rig) { r.usage.RefreshResult = domain.Tokens{Access: "only-access"} }, "incomplete", "incomplete"},
		{"vault save fails", func(r *rig) {
			r.usage.RefreshResult = domain.Tokens{Access: "n", Refresh: "m", ExpiresAt: t0.Add(time.Hour)}
			r.vault.SaveErr = errors.New("disk full")
		}, "disk full", "disk full"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x", "b@x")
			r.usage.Errs[access("b@x")] = ports.ErrUnauthorized
			c.setup(r)

			st, err := r.sw.Poll(ctx, true)

			if u := stateAccount(t, st, "b@x").Usage; !strings.Contains(u.Err, c.usageErr) {
				t.Errorf("b usage error = %q, want %q", u.Err, c.usageErr)
			}
			if c.lastError == "" {
				r.mustNoError(st, err)
			} else {
				r.mustFail(st, err, c.lastError)
			}
		})
	}
}

func TestPollCaptureFailureStopsThePoll(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.login.ReadErr = errors.New("locked")
	st, err := r.sw.Poll(ctx, true)
	r.mustFail(st, err, "locked")
	if len(r.usage.Calls) != 0 {
		t.Errorf("without the live login nothing is read: %v", r.usage.Calls)
	}
}

func TestAlertsSentOnce(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.usage.Usages[access("a@x")] = usage(85)

	st := r.mustNoError(r.sw.Poll(ctx, true))
	if len(r.notify.Sent) != 1 || r.sent("a@x: weekly limit at 85 %") != 1 {
		t.Fatalf("sent = %v", r.notify.Sent)
	}
	if len(st.Fired) != 1 {
		t.Errorf("Fired = %v", st.Fired)
	}

	r.clock.Advance(15 * time.Minute)
	r.mustNoError(r.sw.Poll(ctx, true))
	if len(r.notify.Sent) != 1 {
		t.Errorf("an alert must fire once per window: %v", r.notify.Sent)
	}

	// The window resets: the key is pruned and a new window alerts again.
	r.clock.T = weeklyResets.Add(time.Minute)
	next := usage(85)
	next.Limits[0].ResetsAt = weeklyResets.Add(7 * 24 * time.Hour)
	r.usage.Usages[access("a@x")] = next
	st = r.mustNoError(r.sw.Poll(ctx, true))
	if len(r.notify.Sent) != 2 || len(st.Fired) != 1 {
		t.Errorf("sent %v, fired %v", r.notify.Sent, st.Fired)
	}
}

func TestAlertNotifyFailureRetriesNextPoll(t *testing.T) {
	r := newRig(t, "a@x")
	r.usage.Usages[access("a@x")] = usage(85)
	r.notify.Err = errors.New("no notification service")
	st, err := r.sw.Poll(ctx, true)
	r.mustFail(st, err, "no notification service")
	if len(st.Fired) != 0 {
		t.Error("an alert that was not posted is not fired")
	}
	r.notify.Err = nil
	r.clock.Advance(2 * time.Minute)
	r.mustNoError(r.sw.Poll(ctx, true))
	if len(r.notify.Sent) != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
}

func TestAutoSwitchFiresOnceAndNotifies(t *testing.T) {
	r := newRig(t, "a@x", "b@x", "c@x")
	r.settings(func(s *domain.Settings) { s.Auto = true; s.AlertThresholds = nil })
	r.usage.Usages[access("a@x")] = usage(96)
	r.usage.Usages[access("b@x")] = usage(30)
	r.usage.Usages[access("c@x")] = usage(20)

	st := r.mustNoError(r.sw.Poll(ctx, true))

	if st.Active != "c@x" || len(r.login.Writes) != 1 {
		t.Fatalf("active %q, writes %d", st.Active, len(r.login.Writes))
	}
	if r.sent("Switched to c@x · a@x weekly resets at Wed 09:00") != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
	if st.Undo == nil || st.Undo.From != "a@x" {
		t.Errorf("Undo = %+v", st.Undo)
	}

	r.clock.Advance(2 * time.Minute)
	st = r.mustNoError(r.sw.Poll(ctx, false))
	r.clock.Advance(15 * time.Minute)
	st = r.mustNoError(r.sw.Poll(ctx, false))
	if st.Active != "c@x" || len(r.login.Writes) != 1 || r.sent("Switched") != 1 {
		t.Errorf("auto-switch must fire once: active %q, writes %d, sent %v", st.Active, len(r.login.Writes), r.notify.Sent)
	}
}

func TestAutoOffNeverSwitches(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.usage.Usages[access("a@x")] = usage(100)
	st := r.mustNoError(r.sw.Poll(ctx, true))
	if st.Active != "a@x" || len(r.login.Writes) != 0 {
		t.Errorf("active %q, writes %d", st.Active, len(r.login.Writes))
	}
}

func TestAutoRereadsCandidatesBeforeDeciding(t *testing.T) {
	r := newRig(t, "a@x", "b@x", "c@x")
	r.settings(func(s *domain.Settings) { s.Auto = true })
	r.mustNoError(r.sw.Poll(ctx, true))

	// Five minutes on, a is due (2 min) and over threshold; b and c are not
	// due (15 min) but are read again so the decision rests on fresh numbers.
	r.clock.Advance(5 * time.Minute)
	r.usage.Calls = nil
	r.usage.Usages[access("a@x")] = usage(97)
	r.usage.Usages[access("b@x")] = usage(99) // b filled up since the last read
	r.vault.Accounts[2].InPool = false        // c out of the pool: not read

	st := r.mustNoError(r.sw.Poll(ctx, false))

	want := []string{"fetch " + access("a@x"), "fetch " + access("b@x")}
	if !slices.Equal(r.usage.Calls, want) {
		t.Errorf("calls = %v, want %v", r.usage.Calls, want)
	}
	if st.Active != "a@x" {
		t.Errorf("b is over threshold now; no switch, active %q", st.Active)
	}
	if r.sent("no account to switch to") != 1 {
		t.Errorf("staying put is said once: %v", r.notify.Sent)
	}
	r.clock.Advance(2 * time.Minute)
	r.mustNoError(r.sw.Poll(ctx, false))
	if r.sent("no account to switch to") != 1 {
		t.Errorf("staying put is said once per window: %v", r.notify.Sent)
	}
}

func TestAutoDoesNotRereadUnderThreshold(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.settings(func(s *domain.Settings) { s.Auto = true })
	r.mustNoError(r.sw.Poll(ctx, true))
	r.clock.Advance(2 * time.Minute)
	r.usage.Calls = nil
	r.mustNoError(r.sw.Poll(ctx, false))
	if !slices.Equal(r.usage.Calls, []string{"fetch " + access("a@x")}) {
		t.Errorf("calls = %v", r.usage.Calls)
	}
}

// The decision's staleness limit is 3 × the inactive poll interval: with
// PollInactive 1 min and PollActive 10 min, the active account's last read
// is reused (not due) and judged by its age.
func TestAutoStalenessIsThreeInactiveIntervals(t *testing.T) {
	for _, c := range []struct {
		age        time.Duration
		wantSwitch bool
	}{{2 * time.Minute, true}, {3 * time.Minute, true}, {3*time.Minute + time.Second, false}} {
		t.Run(c.age.String(), func(t *testing.T) {
			r := newRig(t, "a@x", "b@x")
			r.settings(func(s *domain.Settings) { s.Auto = true; s.PollActive = 10 * time.Minute; s.PollInactive = time.Minute })
			r.usage.Usages[access("a@x")] = usage(96)
			r.mustNoError(r.sw.Poll(ctx, true)) // a over threshold, b at 10: switches right away
			if len(r.login.Writes) != 1 {
				t.Fatalf("setup: want an immediate switch")
			}
			// Back to a by hand, then let a's last read age.
			r.mustNoError(r.sw.Switch(ctx, "a@x"))
			r.usage.Calls = nil
			r.clock.Advance(c.age)

			r.mustNoError(r.sw.Poll(ctx, false))

			switched := len(r.login.Writes) == 3
			if switched != c.wantSwitch {
				t.Errorf("a read %v ago: switched %v, want %v (calls %v)", c.age, switched, c.wantSwitch, r.usage.Calls)
			}
		})
	}
}

func TestAutoSwitchFailureIsShown(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.settings(func(s *domain.Settings) { s.Auto = true })
	r.usage.Usages[access("a@x")] = usage(96)
	r.freezeAuth()
	st, err := r.sw.Poll(ctx, true)
	r.mustFail(st, err, "auto-switch to b@x")
	if st.Active != "a@x" {
		t.Errorf("Active = %q", st.Active)
	}
}

func TestAlertTitles(t *testing.T) {
	r := newRig(t, "a@x")
	r.settings(func(s *domain.Settings) { s.AlertThresholds = nil })
	r.usage.Usages[access("a@x")] = usage(100)
	r.mustNoError(r.sw.Poll(ctx, true))
	r.usage.Usages[access("a@x")] = usage(3)
	r.clock.Advance(2 * time.Minute)
	r.mustNoError(r.sw.Poll(ctx, false))
	want := []string{"Limit reached: a@x: weekly limit reached (resets Wed 09:00)", "Available again: a@x is available again (weekly at 3 %)"}
	if !slices.Equal(r.notify.Sent, want) {
		t.Errorf("sent %q, want %q", r.notify.Sent, want)
	}
}
