package core_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestSwitchSuccess(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	// Claude Code refreshed a's tokens since the vault last saw them.
	refreshed := withTokens(t, account(t, "a@x"), domain.Tokens{Access: "fake-access-a2", Refresh: "fake-refresh-a2", ExpiresAt: t0.Add(12 * time.Hour)}).Login
	r.login.Login = refreshed

	st := r.mustNoError(r.sw.Switch(ctx, " B@x "))

	if len(r.login.Writes) != 1 || !bytes.Equal(r.login.Writes[0].Credentials, testkit.FakeLogin("b@x").Credentials) ||
		!bytes.Equal(r.login.Writes[0].OrganizationUUID, testkit.FakeLogin("b@x").OrganizationUUID) ||
		!bytes.Equal(r.login.Writes[0].Profile, testkit.FakeLogin("b@x").Profile) {
		t.Fatalf("live login writes = %d, want b's whole login once", len(r.login.Writes))
	}
	if !bytes.Equal(r.vaultAccount("a@x").Login.Credentials, refreshed.Credentials) {
		t.Error("a's live tokens must be snapshotted into the vault before the switch")
	}
	if st.Active != "b@x" {
		t.Errorf("Active = %q", st.Active)
	}
	want := domain.Undo{From: "a@x", To: "b@x", Until: t0.Add(10 * time.Minute)}
	if st.Undo == nil || *st.Undo != want {
		t.Errorf("Undo = %+v, want %+v", st.Undo, want)
	}
	if !slices.Contains(r.usage.Calls, "fetch "+access("b@x")) || !slices.Contains(r.claude.Calls, "auth status ") {
		t.Errorf("verify must read b's usage and claude auth status: %v %v", r.usage.Calls, r.claude.Calls)
	}
	if u := stateAccount(t, st, "b@x").Usage; u.Err != "" || !u.ReadAt.Equal(t0) {
		t.Errorf("b's usage from the verify read = %+v", u)
	}
	if r.sent("Switched to b@x") != 1 {
		t.Errorf("notifications = %v", r.notify.Sent)
	}
	if len(r.calls("refresh")) != 0 {
		t.Errorf("no refresh expected: %v", r.usage.Calls)
	}
}

func TestSwitchToTheActiveAccountDoesNothing(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	st := r.mustNoError(r.sw.Switch(ctx, "a@x"))
	if len(r.login.Writes) != 0 || st.Undo != nil || len(r.notify.Sent) != 0 {
		t.Errorf("writes %d, undo %+v, sent %v", len(r.login.Writes), st.Undo, r.notify.Sent)
	}
}

func TestSwitchUnknownAccount(t *testing.T) {
	r := newRig(t, "a@x")
	st, err := r.sw.Switch(ctx, "z@x")
	r.mustFail(st, err, "no account z@x")
}

func TestSwitchWithExpiredTargetRefreshesExactlyOnce(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.clock.Advance(13 * time.Hour) // every access token has expired
	now := r.clock.Now()
	r.usage.RefreshResult = domain.Tokens{Access: "fake-access-b2", Refresh: "fake-refresh-b2", ExpiresAt: now.Add(12 * time.Hour)}
	r.usage.Usages["fake-access-b2"] = usage(5)

	r.mustNoError(r.sw.Switch(ctx, "b@x"))

	if got := r.calls("refresh"); !slices.Equal(got, []string{"refresh " + refresh("b@x")}) {
		t.Errorf("refreshes = %v, want b's once (never a's, the live one)", got)
	}
	b := r.vaultAccount("b@x")
	if tok, _ := b.AccessToken(); tok != "fake-access-b2" || !b.CapturedAt.Equal(now) {
		t.Errorf("vault b token %q captured %v", tok, b.CapturedAt)
	}
	if tok, _ := (domain.Account{Login: r.login.Login}).AccessToken(); tok != "fake-access-b2" {
		t.Errorf("live token after switch = %q", tok)
	}
}

func TestSwitchRefreshFailure(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.clock.Advance(13 * time.Hour)
	r.usage.Errs[refresh("b@x")] = errors.New("HTTP 500")
	st, err := r.sw.Switch(ctx, "b@x")
	r.mustFail(st, err, "HTTP 500")
	if len(r.login.Writes) != 0 || st.Active != "a@x" {
		t.Errorf("a failed refresh must not touch the live login: %d writes, active %q", len(r.login.Writes), st.Active)
	}
}

func TestSwitchVerifyFailsRestoresThePreviousLogin(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *rig)
		want  string
	}{
		{"usage refused", func(r *rig) { r.usage.Errs[access("b@x")] = ports.ErrUnauthorized }, "unauthorized"},
		{"auth status says another email", func(r *rig) { r.freezeAuth() }, `reports "a@x"`},
		{"auth status fails", func(r *rig) { r.claude.AuthErr = errors.New("claude not found") }, "claude not found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x", "b@x")
			c.setup(r)
			prev := r.login.Login

			st, err := r.sw.Switch(ctx, "b@x")

			r.mustFail(st, err, c.want)
			if len(r.login.Writes) != 2 || !bytes.Equal(r.login.Login.Credentials, prev.Credentials) {
				t.Errorf("writes = %d; the previous login must be written back", len(r.login.Writes))
			}
			if st.Active != "a@x" || st.Undo != nil || len(r.notify.Sent) != 0 {
				t.Errorf("active %q, undo %+v, sent %v", st.Active, st.Undo, r.notify.Sent)
			}
		})
	}
}

func TestSwitchWriteFailure(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.login.WriteErr = errors.New("access denied")
	st, err := r.sw.Switch(ctx, "b@x")
	r.mustFail(st, err, "restore the previous login")
	if st.Active != "a@x" {
		t.Errorf("Active = %q", st.Active)
	}
}

func TestUndo(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.mustNoError(r.sw.Switch(ctx, "b@x"))
	r.clock.Advance(9 * time.Minute)

	st := r.mustNoError(r.sw.Undo(ctx))

	if st.Active != "a@x" || st.Undo != nil {
		t.Errorf("after undo: active %q, undo %+v", st.Active, st.Undo)
	}
	if r.sent("Switched back to a@x") != 1 {
		t.Errorf("sent %v", r.notify.Sent)
	}
	st, err := r.sw.Undo(ctx)
	r.mustFail(st, err, "nothing to undo")
}

func TestUndoOutsideTheWindow(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.mustNoError(r.sw.Switch(ctx, "b@x"))
	r.clock.Advance(10 * time.Minute)

	st, err := r.sw.Undo(ctx)

	r.mustFail(st, err, "nothing to undo")
	if st.Active != "b@x" || len(r.login.Writes) != 1 || st.Undo != nil {
		t.Errorf("active %q, writes %d, undo %+v", st.Active, len(r.login.Writes), st.Undo)
	}
}

func TestUndoFailureKeepsTheUndo(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.mustNoError(r.sw.Switch(ctx, "b@x"))
	r.usage.Errs[access("a@x")] = errors.New("offline")
	st, err := r.sw.Undo(ctx)
	r.mustFail(st, err, "offline")
	if st.Undo == nil || st.Active != "b@x" {
		t.Errorf("a failed undo keeps the offer: undo %+v active %q", st.Undo, st.Active)
	}
}
