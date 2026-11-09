package core_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestCaptureNewLogin(t *testing.T) {
	r := newRig(t)
	r.login.Login = testkit.FakeLogin("A@x.com")

	st := r.mustNoError(r.sw.Capture(ctx))

	if st.Active != "a@x.com" {
		t.Errorf("Active = %q", st.Active)
	}
	if len(r.vault.Accounts) != 1 || !r.vault.Accounts[0].InPool || !r.vault.Accounts[0].CapturedAt.Equal(t0) {
		t.Fatalf("vault = %+v", r.vault.Accounts)
	}
	if a := stateAccount(t, st, "a@x.com"); a.Plan != "Max 20x" || !a.InPool {
		t.Errorf("state account = %+v", a)
	}
	if r.store.Saves != 1 || r.store.State.Active != "a@x.com" {
		t.Errorf("state not saved: %d saves, %+v", r.store.Saves, r.store.State)
	}
	if r.lock.Acquired != 1 || r.lock.Holders != 0 || r.store.unlockedSaves != 0 {
		t.Errorf("lock acquired %d, held %d, unlocked saves %d", r.lock.Acquired, r.lock.Holders, r.store.unlockedSaves)
	}
}

func TestCaptureSnapshotsChangedLogin(t *testing.T) {
	r := newRig(t, "a@x", "b@x")
	live := withTokens(t, account(t, "a@x"), domain.Tokens{Access: "fake-access-a2", Refresh: "fake-refresh-a2", ExpiresAt: t0.Add(11 * time.Hour)}).Login
	live.OrganizationUUID = json.RawMessage(`"fake-org-moved"`)
	r.login.Login = live

	r.mustNoError(r.sw.Capture(ctx))

	got := r.vaultAccount("a@x")
	if !bytes.Equal(got.Login.Credentials, live.Credentials) || string(got.Login.OrganizationUUID) != `"fake-org-moved"` {
		t.Errorf("vault login not the live one: %s %s", got.Login.Credentials, got.Login.OrganizationUUID)
	}
	if !bytes.Equal(r.vaultAccount("b@x").Login.Credentials, testkit.FakeLogin("b@x").Credentials) {
		t.Error("other accounts must stay as they were")
	}
	if len(r.login.Writes) != 0 {
		t.Error("capture must never write the live login")
	}
}

func TestCaptureKeepsPoolChoiceOnSnapshot(t *testing.T) {
	r := newRig(t, "a@x")
	r.vault.Accounts[0].InPool = false
	r.login.Login = withTokens(t, account(t, "a@x"), domain.Tokens{Access: "x", Refresh: "y", ExpiresAt: t0.Add(time.Hour)}).Login

	r.mustNoError(r.sw.Capture(ctx))

	if r.vaultAccount("a@x").InPool {
		t.Error("a snapshot must not put an account back in the pool")
	}
}

func TestCaptureUnchangedLoginDoesNotSaveTheVault(t *testing.T) {
	r := newRig(t, "a@x")
	r.mustNoError(r.sw.Capture(ctx))
	if r.vault.Saves != 0 {
		t.Errorf("vault saved %d times for an unchanged login", r.vault.Saves)
	}
}

func TestCaptureReadErrorIsShown(t *testing.T) {
	r := newRig(t, "a@x")
	r.login.ReadErr = errors.New("no such file")
	st, err := r.sw.Capture(ctx)
	r.mustFail(st, err, "no such file")
	if stateAccount(t, st, "a@x").Email != "a@x" {
		t.Error("a failed capture must keep the accounts in the state")
	}

	r.login.ReadErr = nil
	r.login.Login = domain.Login{Profile: json.RawMessage(`{"emailAddress":""}`)}
	st, err = r.sw.Capture(ctx)
	r.mustFail(st, err, "no emailAddress")

	r.login.Login = domain.Login{Profile: json.RawMessage(`{"emailAddress":"n@x"}`), Credentials: json.RawMessage(`[]`)}
	st, err = r.sw.Capture(ctx)
	r.mustFail(st, err, "capture n@x")
}

func TestErrorClearedOnSuccess(t *testing.T) {
	r := newRig(t, "a@x")
	r.login.ReadErr = errors.New("boom")
	if _, err := r.sw.Capture(ctx); err == nil {
		t.Fatal("want an error")
	}
	r.login.ReadErr = nil
	r.mustNoError(r.sw.Capture(ctx))
	if r.store.State.LastError != "" {
		t.Errorf("saved LastError = %q", r.store.State.LastError)
	}
}

func TestOperationsFailWithoutTheLockAndSaveNothing(t *testing.T) {
	r := newRig(t, "a@x")
	r.lock.Err = errors.New("busy")
	_, err := r.sw.Capture(ctx)
	if err == nil || r.store.Saves != 0 || r.vault.Saves != 0 {
		t.Errorf("err %v, %d state saves, %d vault saves", err, r.store.Saves, r.vault.Saves)
	}
}

func TestLoadErrors(t *testing.T) {
	r := newRig(t, "a@x")
	r.store.LoadErr = errors.New("state.json: bad")
	if _, err := r.sw.Poll(ctx, true); err == nil || r.store.Saves != 0 {
		t.Errorf("an unreadable state must not be overwritten: err %v, saves %d", err, r.store.Saves)
	}

	r = newRig(t, "a@x")
	r.mustNoError(r.sw.Poll(ctx, true))
	r.vault.LoadErr = errors.New("accounts.json: bad")
	st, err := r.sw.Poll(ctx, true)
	r.mustFail(st, err, "accounts.json: bad")
	if len(st.Accounts) != 1 {
		t.Error("an unreadable vault must not empty the state's accounts")
	}
}

func TestSaveErrors(t *testing.T) {
	r := newRig(t)
	r.login.Login = testkit.FakeLogin("a@x")
	r.vault.SaveErr = errors.New("disk full")
	st, err := r.sw.Capture(ctx)
	r.mustFail(st, err, "disk full")

	r = newRig(t, "a@x")
	r.store.SaveErr = errors.New("read-only")
	if _, err := r.sw.Capture(ctx); err == nil {
		t.Error("a state that cannot be saved is an error")
	}
}
