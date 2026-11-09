package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/cli"
	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// TestMain pins the local zone: the status shows times locally.
// T0 (12:00 UTC) is 09:00 here.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("ART", -3*60*60)
	os.Exit(m.Run())
}

var (
	t0 = testkit.T0
	// weekly resets Wed 11 Nov 12:00 UTC = 09:00 ART.
	weeklyResets = t0.Add(48 * time.Hour)
)

func usage(pct float64, readAt time.Time) domain.Usage {
	return domain.Usage{ReadAt: readAt, Limits: []domain.Limit{{
		Kind: "weekly_all", Group: "weekly", Percent: pct, Severity: "normal", ResetsAt: weeklyResets, Active: true,
	}}}
}

func access(email string) string { return "fake-access-" + email }

// rig is a Switcher over testkit fakes, and the CLI over it. The first
// email is the live, active login; every account reads 10 %.
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
	store    *testkit.FakeStateStore
	newLogin domain.Login // what `add` finds in the scratch folder
	deps     cli.Deps
}

func newRig(t *testing.T, emails ...string) *rig {
	t.Helper()
	r := &rig{
		t:        t,
		login:    &testkit.FakeLoginStore{},
		vault:    &testkit.FakeVault{},
		usage:    &testkit.FakeUsageAPI{Usages: map[string]domain.Usage{}, Errs: map[string]error{}},
		claude:   &testkit.FakeClaude{VersionOut: "2.1.300 (Claude Code)", Auth: map[string]ports.AuthStatus{}},
		releases: &testkit.FakeReleases{Version: "2.1.319"},
		notify:   &testkit.FakeNotifier{},
		clock:    &testkit.FakeClock{T: t0},
		lock:     &testkit.FakeLock{},
		store:    &testkit.FakeStateStore{},
	}
	for i, e := range emails {
		a, err := domain.AccountFromLogin(testkit.FakeLogin(e), t0.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		r.vault.Accounts = append(r.vault.Accounts, a)
		r.usage.Usages[access(e)] = usage(10, t0)
		if i == 0 {
			r.login.Login = testkit.FakeLogin(e)
			r.claude.Auth[""] = ports.AuthStatus{LoggedIn: true, Email: e}
		}
	}
	sw := core.New(core.Deps{
		Login: authFollows{r.login, r}, Vault: r.vault, Usage: r.usage, Claude: r.claude, Releases: r.releases,
		Notify: r.notify, Clock: r.clock, Lock: r.lock, Store: r.store, Log: &testkit.FakeLogger{},
		ScratchDir: func() (string, error) {
			d := filepath.Join(t.TempDir(), "login-scratch")
			return d, os.Mkdir(d, 0o700)
		},
		LoginStoreAt: func(string) ports.LoginStore { return &testkit.FakeLoginStore{Login: r.newLogin} },
	})
	r.deps = cli.Deps{Switcher: sw, Clock: r.clock, Store: r.store, Notify: r.notify, Version: "v1.2.3"}
	return r
}

// authFollows makes `claude auth status` report the email of the login
// last written, as the real CLI would.
type authFollows struct {
	*testkit.FakeLoginStore
	r *rig
}

func (a authFollows) Write(l domain.Login) error {
	if err := a.FakeLoginStore.Write(l); err != nil {
		return err
	}
	email, _ := l.Email()
	a.r.claude.Auth[""] = ports.AuthStatus{LoggedIn: true, Email: email}
	return nil
}

// capture runs one poll so the state knows every account, as after the
// first `ccswitch check`, and forgets the calls it made.
func (r *rig) capture() {
	r.t.Helper()
	if code, _, errOut := r.run("check"); code != 0 {
		r.t.Fatalf("check: %d %s", code, errOut)
	}
	r.usage.Calls, r.claude.Calls, r.notify.Sent = nil, nil, nil
	r.lock.Acquired, r.releases.Calls = 0, 0
}

func (r *rig) run(args ...string) (code int, stdout, stderr string) {
	return r.runCtx(context.Background(), args...)
}

func (r *rig) runCtx(ctx context.Context, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(ctx, args, r.deps, &out, &errOut)
	return code, out.String(), errOut.String()
}
