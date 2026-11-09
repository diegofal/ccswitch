package testkit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestFakeLogin(t *testing.T) {
	l := testkit.FakeLogin("b@example.com")
	a, err := domain.AccountFromLogin(l, testkit.T0)
	if err != nil {
		t.Fatal(err)
	}
	access, _ := a.AccessToken()
	refresh, _ := a.RefreshToken()
	exp, _ := a.ExpiresAt()
	rexp, _ := a.RefreshExpiresAt()
	if a.Email != "b@example.com" || access != "fake-access-b@example.com" || refresh != "fake-refresh-b@example.com" ||
		!exp.Equal(testkit.T0.Add(12*time.Hour)) || !rexp.Equal(testkit.T0.Add(28*24*time.Hour)) || a.Plan != "Max 20x" {
		t.Errorf("FakeLogin account = %+v (%s, %s, %v, %v)", a, access, refresh, exp, rexp)
	}
	creds, err := domain.PatchCredentialsFile(testkit.Fixture(t, "credentials.json"), l)
	if err != nil {
		t.Fatal(err)
	}
	config, err := domain.PatchConfigFile(testkit.Fixture(t, "claude.json"), l)
	if err != nil {
		t.Fatal(err)
	}
	back, err := domain.LoginFromFiles(creds, config)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := back.Email(); e != "b@example.com" {
		t.Errorf("FakeLogin does not round-trip through the files: %q", e)
	}
}

func TestFakeLockIsNotReentrant(t *testing.T) {
	var l testkit.FakeLock
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Acquire(context.Background()); err == nil {
		t.Error("second Acquire while held must fail")
	}
	release()
	release()
	if l.Holders != 0 || l.Acquired != 1 {
		t.Errorf("Holders = %d, Acquired = %d", l.Holders, l.Acquired)
	}
	if _, err := l.Acquire(context.Background()); err != nil {
		t.Errorf("Acquire after release: %v", err)
	}
}

func TestFakeUsageAPI(t *testing.T) {
	boom := errors.New("HTTP 401")
	f := testkit.FakeUsageAPI{
		Usages: map[string]domain.Usage{"fake-access-a": {ReadAt: testkit.T0}},
		Errs:   map[string]error{"fake-access-b": boom},
	}
	ctx := context.Background()
	if u, err := f.Fetch(ctx, "fake-access-a"); err != nil || !u.ReadAt.Equal(testkit.T0) {
		t.Errorf("scripted fetch = %+v, %v", u, err)
	}
	if _, err := f.Fetch(ctx, "fake-access-b"); !errors.Is(err, boom) {
		t.Errorf("scripted error = %v", err)
	}
	if _, err := f.Fetch(ctx, "fake-access-c"); err == nil {
		t.Error("unscripted token must fail")
	}
	if _, err := f.Refresh(ctx, "fake-refresh-a"); err == nil {
		t.Error("unscripted refresh must fail")
	}
	if len(f.Calls) != 4 || f.Calls[3] != "refresh fake-refresh-a" {
		t.Errorf("Calls = %v", f.Calls)
	}
}

func TestFakeStateStoreCopies(t *testing.T) {
	var s testkit.FakeStateStore
	if got, _ := s.LoadSettings(); got.ThresholdPercent != 95 {
		t.Errorf("absent settings must be the defaults, got %+v", got)
	}
	st := domain.State{Fired: map[string]bool{"k": true}, Undo: &domain.Undo{From: "a"}}
	if err := s.SaveState(st); err != nil {
		t.Fatal(err)
	}
	st.Fired["other"] = true
	st.Undo.From = "changed"
	got, _ := s.LoadState()
	if len(got.Fired) != 1 || got.Undo.From != "a" {
		t.Errorf("SaveState kept a reference to the caller's state: %+v", got)
	}
}

func TestFakeClaudeUpdateChangesVersion(t *testing.T) {
	c := testkit.FakeClaude{VersionOut: "2.1.300 (Claude Code)", UpdatedVersion: "2.1.319 (Claude Code)"}
	ctx := context.Background()
	if _, err := c.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Version(ctx); v != "2.1.319 (Claude Code)" {
		t.Errorf("Version after Update = %q", v)
	}
	var dirs []string
	c.OnLogin = func(dir string) error { dirs = append(dirs, dir); return nil }
	if err := c.Login(ctx, "scratch"); err != nil || len(dirs) != 1 {
		t.Errorf("Login did not run OnLogin: %v, %v", err, dirs)
	}
}
