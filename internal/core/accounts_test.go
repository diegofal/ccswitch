package core_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// loginLeaves makes the scratch login leave l in its folder, and records
// whether the lock was held while the user was logging in.
func (r *rig) loginLeaves(l domain.Login, heldDuringLogin *bool) {
	r.newLogin = l
	r.claude.OnLogin = func(dir string) error {
		*heldDuringLogin = r.lock.Holders > 0
		return os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte("{}"), 0o600)
	}
}

func TestAddNewAccount(t *testing.T) {
	r := newRig(t, "a@x")
	var held bool
	r.loginLeaves(testkit.FakeLogin("b@x"), &held)
	r.usage.Usages[access("b@x")] = usage(33)

	st := r.mustNoError(r.sw.Add(ctx))

	if len(r.scratchDirs) != 1 || !slices.Contains(r.claude.Calls, "login "+r.scratchDirs[0]) {
		t.Fatalf("scratch %v, claude calls %v", r.scratchDirs, r.claude.Calls)
	}
	if _, err := os.Stat(r.scratchDirs[0]); !os.IsNotExist(err) {
		t.Errorf("the scratch folder must be removed: %v", err)
	}
	if held {
		t.Error("the lock must not be held while the user logs in in the browser")
	}
	b := r.vaultAccount("b@x")
	if !b.InPool || !bytes.Equal(b.Login.Credentials, testkit.FakeLogin("b@x").Credentials) {
		t.Errorf("vault b = %+v", b)
	}
	if st.Active != "a@x" || len(r.login.Writes) != 0 {
		t.Errorf("add must not touch the live login: active %q, writes %d", st.Active, len(r.login.Writes))
	}
	if l, _ := stateAccount(t, st, "b@x").Usage.Binding(); l.Percent != 33 {
		t.Errorf("b's usage must be read right away: %+v", stateAccount(t, st, "b@x").Usage)
	}
	if r.lock.Acquired != 1 || r.store.unlockedSaves != 0 {
		t.Errorf("lock acquired %d, unlocked saves %d", r.lock.Acquired, r.store.unlockedSaves)
	}
}

func TestAddSameEmailReplacesTheLogin(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	r.vault.Accounts[1].InPool = false
	again := withTokens(t, account(t, "b@x"), domain.Tokens{Access: "fake-access-b2", Refresh: "fake-refresh-b2", ExpiresAt: t0.Add(12 * time.Hour)}).Login
	var held bool
	r.loginLeaves(again, &held)
	r.usage.Usages["fake-access-b2"] = usage(1)

	r.mustNoError(r.sw.Add(ctx))

	if len(r.vault.Accounts) != 2 {
		t.Fatalf("vault = %d accounts, want 2", len(r.vault.Accounts))
	}
	b := r.vaultAccount("b@x")
	if !bytes.Equal(b.Login.Credentials, again.Credentials) || b.InPool {
		t.Errorf("b must have the new login and keep its pool choice: in pool %v", b.InPool)
	}
}

func TestAddFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *rig)
		want  string
	}{
		{"login fails", func(r *rig) { r.claude.LoginErr = errors.New("cancelled") }, "cancelled"},
		{"nothing left in the scratch folder", func(r *rig) {}, "read the new login"},
		{"bad login", func(r *rig) {
			var held bool
			r.loginLeaves(domain.Login{Profile: []byte(`{"emailAddress":"c@x"}`), Credentials: []byte(`[]`)}, &held)
		}, "claudeAiOauth"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x")
			c.setup(r)
			st, err := r.sw.Add(ctx)
			r.mustFail(st, err, c.want)
			if len(r.vault.Accounts) != 1 {
				t.Errorf("vault = %d accounts", len(r.vault.Accounts))
			}
			if _, err := os.Stat(r.scratchDirs[0]); !os.IsNotExist(err) {
				t.Errorf("the scratch folder must be removed: %v", err)
			}
		})
	}
}

func TestAddScratchFolderFailure(t *testing.T) {
	r := newRig(t, "a@x")
	r.deps.ScratchDir = func() (string, error) { return "", errors.New("no space") }
	r.sw = core.New(r.deps)
	st, err := r.sw.Add(ctx)
	r.mustFail(st, err, "no space")
	if len(r.claude.Calls) != 0 {
		t.Errorf("no login without a scratch folder: %v", r.claude.Calls)
	}
}

func TestSetPool(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	st := r.mustNoError(r.sw.SetPool(ctx, "B@x", false))
	if r.vaultAccount("b@x").InPool || stateAccount(t, st, "b@x").InPool {
		t.Error("b must be out of the pool in the vault and the state")
	}
	st, err := r.sw.SetPool(ctx, "z@x", true)
	r.mustFail(st, err, "no account z@x")
}

func TestRemove(t *testing.T) {
	r := newRig(t, "a@x", "b@x", "c@x")
	r.mustNoError(r.sw.Switch(ctx, "b@x")) // undo offers a@x

	st, err := r.sw.Remove(ctx, "b@x")
	r.mustFail(st, err, "active")

	st = r.mustNoError(r.sw.Remove(ctx, "a@x"))
	if len(r.vault.Accounts) != 2 || len(st.Accounts) != 2 || st.Undo != nil {
		t.Errorf("vault %d, state %d, undo %+v", len(r.vault.Accounts), len(st.Accounts), st.Undo)
	}
	st, err = r.sw.Remove(ctx, "a@x")
	r.mustFail(st, err, "no account a@x")
}

func TestSetAuto(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.SetAuto(ctx, true))
	if r.store.Settings == nil || !r.store.Settings.Auto || r.store.Settings.ThresholdPercent != 95 {
		t.Fatalf("settings = %+v", r.store.Settings)
	}
	set, err := r.sw.Settings()
	if err != nil || !set.Auto {
		t.Errorf("Settings() = %+v, %v", set, err)
	}
	r.store.SaveErr = errors.New("read-only")
	if _, err := r.sw.SetAuto(ctx, false); err == nil {
		t.Error("want the save error")
	}
}

func TestStateReadsWithoutTheLock(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.Poll(ctx, true))
	r.lock.Err = errors.New("held by the tray")
	st, err := r.sw.State(ctx)
	if err != nil || st.Active != "a@x" {
		t.Errorf("State = %+v, %v", st, err)
	}
}

func TestMaxAge(t *testing.T) {
	s := domain.DefaultSettings()
	if got := core.MaxAge(s); got != 45*time.Minute {
		t.Errorf("MaxAge = %v", got)
	}
}

func TestAddWithoutALiveLogin(t *testing.T) {
	r := newRig(t)
	r.login.ReadErr = errors.New("not logged in")
	var held bool
	r.loginLeaves(testkit.FakeLogin("b@x"), &held)

	st, err := r.sw.Add(ctx)

	r.mustFail(st, err, "not logged in")
	if len(r.vault.Accounts) != 1 || len(r.usage.Calls) != 0 {
		t.Errorf("the account is added, unread: vault %d, calls %v", len(r.vault.Accounts), r.usage.Calls)
	}
}
