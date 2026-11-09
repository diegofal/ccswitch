package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// TestMain pins the local zone: notification texts show reset times locally.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("ART", -3*60*60)
	os.Exit(m.Run())
}

var (
	ctx = context.Background()
	t0  = testkit.T0
	// weekly resets Wed 11 Nov 12:00 UTC = 09:00 ART.
	weeklyResets = t0.Add(48 * time.Hour)
)

func usage(pct float64) domain.Usage {
	return domain.Usage{ReadAt: t0, Limits: []domain.Limit{{
		Kind: "weekly_all", Group: "weekly", Percent: pct, Severity: "normal", ResetsAt: weeklyResets, Active: true,
	}}}
}

func access(email string) string  { return "fake-access-" + email }
func refresh(email string) string { return "fake-refresh-" + email }

func account(t *testing.T, email string) domain.Account {
	t.Helper()
	a, err := domain.AccountFromLogin(testkit.FakeLogin(email), t0.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func withTokens(t *testing.T, a domain.Account, tk domain.Tokens) domain.Account {
	t.Helper()
	a, err := a.WithTokens(tk)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// lockCheckStore is the state store that also records whether the lock was
// held whenever the state was saved.
type lockCheckStore struct {
	*testkit.FakeStateStore
	lock          *testkit.FakeLock
	unlockedSaves int
}

func (s *lockCheckStore) SaveState(v domain.State) error {
	if s.lock.Holders != 1 {
		s.unlockedSaves++
	}
	return s.FakeStateStore.SaveState(v)
}

type rig struct {
	t        *testing.T
	login    *testkit.FakeLoginStore
	vault    *testkit.FakeVault
	usage    *testkit.FakeUsageAPI
	claude   *testkit.FakeClaude
	releases *testkit.FakeReleases
	notify   *testkit.FakeNotifier
	clock    *testkit.FakeClock
	lock     *testkit.FakeLock
	store    *lockCheckStore
	log      *testkit.FakeLogger
	// newLogin is what a scratch login (Add) leaves in its folder.
	newLogin    domain.Login
	scratchDirs []string
	authFrozen  bool
	deps        core.Deps
	sw          *core.Switcher
}

// newRig has a vault with the given accounts, the first one live and
// active, every account's usage at 10 %, and claude auth status following
// the live login.
func newRig(t *testing.T, emails ...string) *rig {
	t.Helper()
	r := &rig{
		t:        t,
		login:    &testkit.FakeLoginStore{},
		vault:    &testkit.FakeVault{},
		usage:    &testkit.FakeUsageAPI{Usages: map[string]domain.Usage{}, Errs: map[string]error{}},
		releases: &testkit.FakeReleases{Version: "2.1.319"},
		notify:   &testkit.FakeNotifier{},
		clock:    &testkit.FakeClock{T: t0},
		lock:     &testkit.FakeLock{},
		log:      &testkit.FakeLogger{},
	}
	r.claude = &testkit.FakeClaude{VersionOut: "2.1.300 (Claude Code)", Auth: map[string]ports.AuthStatus{}}
	r.store = &lockCheckStore{FakeStateStore: &testkit.FakeStateStore{}, lock: r.lock}
	for i, e := range emails {
		r.vault.Accounts = append(r.vault.Accounts, account(t, e))
		r.usage.Usages[access(e)] = usage(10)
		if i == 0 {
			r.login.Login = testkit.FakeLogin(e)
			r.claude.Auth[""] = ports.AuthStatus{LoggedIn: true, Email: e}
		}
	}
	r.deps = core.Deps{
		Login: authFollows{r.login, r}, Vault: r.vault, Usage: r.usage, Claude: r.claude, Releases: r.releases,
		Notify: r.notify, Clock: r.clock, Lock: r.lock, Store: r.store, Log: r.log,
		ScratchDir: func() (string, error) {
			d := filepath.Join(t.TempDir(), "login-scratch")
			r.scratchDirs = append(r.scratchDirs, d)
			return d, os.Mkdir(d, 0o700)
		},
		LoginStoreAt: func(dir string) ports.LoginStore {
			if _, err := os.Stat(filepath.Join(dir, ".credentials.json")); err != nil {
				return &testkit.FakeLoginStore{ReadErr: err}
			}
			return &testkit.FakeLoginStore{Login: r.newLogin}
		},
	}
	r.sw = core.New(r.deps)
	return r
}

// authFollows is the live login store, and makes `claude auth status`
// report the email of the login last written, as the real CLI would.
type authFollows struct {
	*testkit.FakeLoginStore
	r *rig
}

func (a authFollows) Write(l domain.Login) error {
	if err := a.FakeLoginStore.Write(l); err != nil {
		return err
	}
	if !a.r.authFrozen {
		email, _ := l.Email()
		a.r.claude.Auth[""] = ports.AuthStatus{LoggedIn: true, Email: email}
	}
	return nil
}

// freezeAuth makes `claude auth status` keep reporting what it reports now.
func (r *rig) freezeAuth() { r.authFrozen = true }

func (r *rig) settings(f func(*domain.Settings)) {
	s := domain.DefaultSettings()
	f(&s)
	r.store.Settings = &s
}

func (r *rig) vaultAccount(email string) domain.Account {
	r.t.Helper()
	for _, a := range r.vault.Accounts {
		if a.Email == email {
			return a
		}
	}
	r.t.Fatalf("no %s in the vault", email)
	return domain.Account{}
}

func stateAccount(t *testing.T, st domain.State, email string) domain.AccountState {
	t.Helper()
	for _, a := range st.Accounts {
		if a.Email == email {
			return a
		}
	}
	t.Fatalf("no %s in the state: %+v", email, st.Accounts)
	return domain.AccountState{}
}

func (r *rig) calls(prefix string) []string {
	var out []string
	for _, c := range r.usage.Calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func (r *rig) mustNoError(st domain.State, err error) domain.State {
	r.t.Helper()
	if err != nil {
		r.t.Fatalf("unexpected error: %v", err)
	}
	if st.LastError != "" {
		r.t.Fatalf("LastError = %q", st.LastError)
	}
	return st
}

// mustFail checks the error is returned, in LastError and saved.
func (r *rig) mustFail(st domain.State, err error, want string) {
	r.t.Helper()
	if err == nil {
		r.t.Fatalf("want an error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		r.t.Errorf("error = %q, want it to contain %q", err, want)
	}
	if st.LastError != err.Error() || r.store.State.LastError != err.Error() {
		r.t.Errorf("LastError returned %q, saved %q; want %q", st.LastError, r.store.State.LastError, err)
	}
}

func (r *rig) sent(substr string) int {
	n := 0
	for _, s := range r.notify.Sent {
		if strings.Contains(s, substr) {
			n++
		}
	}
	return n
}
