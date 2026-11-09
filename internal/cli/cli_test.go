package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/cli"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestStatusTable(t *testing.T) {
	r := newRig(t, "a@x.com")
	session := domain.Limit{Kind: "session", Group: "session", Percent: 50, Severity: "normal", ResetsAt: t0.Add(time.Hour), Active: false}
	fresh := usage(1, t0.Add(-time.Minute))
	fresh.Limits = append(fresh.Limits, session)
	opus := usage(99.6, t0.Add(-2*time.Hour))
	opus.Limits = append(opus.Limits, domain.Limit{Kind: "weekly_opus", Group: "weekly", Scope: "Opus", Percent: 100,
		Severity: "warning", ResetsAt: t0.Add(30 * 24 * time.Hour), Active: true})
	r.store.State = domain.State{
		Active: "a@x.com",
		Accounts: []domain.AccountState{
			{Email: "a@x.com", Plan: "Max 20x", InPool: true, Usage: fresh},
			{Email: "b@y.com", Plan: "Pro", InPool: false, Usage: opus},
			{Email: "c@z.com", InPool: true, Usage: domain.Unreadable(errors.New("login expired — add it again"), t0)},
			{Email: "d@w.com", InPool: true},
			{Email: "e@v.com", InPool: true, Usage: domain.Usage{ReadAt: t0}},
		},
		Installed: "2.1.300", Latest: "2.1.319",
		Undo:      &domain.Undo{From: "b@y.com", To: "a@x.com", Until: t0.Add(5 * time.Minute)},
		LastError: "claude auth status: boom",
		UpdatedAt: t0,
	}
	code, out, errOut := r.run("status")
	want := `* a@x.com · Max 20x
    weekly  ▓░░░░░░░░░     1%  resets Wed 09:00
  b@y.com · Pro · not in the auto-switch pool
    weekly         ▓▓▓▓▓▓▓▓▓░  99.6%  resets Wed 09:00  as of 07:00
    weekly (Opus)  ▓▓▓▓▓▓▓▓▓▓   100%  resets 9 Dec  as of 07:00
  c@z.com
    unreadable: login expired — add it again (tried 09:00)
  d@w.com
    usage not read yet
  e@v.com
    no active limits

Auto-switch off (would switch at 95 %)
Claude Code 2.1.300 · 2.1.319 available (` + "`ccswitch update`" + `)
` + "`ccswitch undo`" + ` switches back to b@y.com until 09:05
Last error: claude auth status: boom
`
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if out != want {
		t.Errorf("status:\n%s\nwant:\n%s", out, want)
	}
	if r.lock.Acquired != 0 || len(r.usage.Calls) != 0 || len(r.claude.Calls) != 0 {
		t.Errorf("status must only read the state: lock %d, usage %v, claude %v", r.lock.Acquired, r.usage.Calls, r.claude.Calls)
	}
}

func TestStatusFooter(t *testing.T) {
	cases := []struct {
		name  string
		st    domain.State
		auto  bool
		wants []string
		not   []string
	}{
		{name: "nothing read", wants: []string{"No accounts yet", "Claude Code version not read yet", "Auto-switch off"}},
		{name: "auto on, latest", auto: true, st: domain.State{Installed: "2.1.319", Latest: "2.1.319"},
			wants: []string{"Auto-switch on at 95 %", "Claude Code 2.1.319 (latest)"}},
		{name: "latest unknown", st: domain.State{Installed: "2.1.300"}, wants: []string{"Claude Code 2.1.300\n"}},
		{name: "undo expired", st: domain.State{Undo: &domain.Undo{From: "b@y.com", Until: t0}}, not: []string{"undo"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			r.store.State = c.st
			s := domain.DefaultSettings()
			s.Auto = c.auto
			r.store.Settings = &s
			code, out, _ := r.run("status")
			if code != 0 {
				t.Fatalf("code %d", code)
			}
			for _, w := range c.wants {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in:\n%s", w, out)
				}
			}
			for _, n := range c.not {
				if strings.Contains(out, n) {
					t.Errorf("unexpected %q in:\n%s", n, out)
				}
			}
		})
	}
}

func TestStatusLoadErrors(t *testing.T) {
	r := newRig(t)
	r.store.LoadErr = errors.New("disk gone")
	code, out, errOut := r.run("status")
	if code != 1 || out != "" || !strings.Contains(errOut, "disk gone") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestCheckReadsEveryAccountAndPrintsTheStatus(t *testing.T) {
	r := newRig(t, "a@x.com", "b@y.com")
	code, out, errOut := r.run("check")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if got := strings.Join(r.usage.Calls, ","); got != "fetch "+access("a@x.com")+",fetch "+access("b@y.com") {
		t.Errorf("usage calls %s", got)
	}
	for _, w := range []string{"* a@x.com · Max 20x", "  b@y.com · Max 20x", "10%", "Claude Code 2.1.300 · 2.1.319 available"} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
	if strings.Contains(out, "fake-access") || strings.Contains(out, "fake-refresh") {
		t.Errorf("a token reached the output:\n%s", out)
	}
}

func TestUseMatchesAPrefix(t *testing.T) {
	cases := []struct {
		name, arg, want string
	}{
		{"exact", "bob@y.com", "bob@y.com"},
		{"prefix", "bo", "bob@y.com"},
		{"case", "BOB@", "bob@y.com"},
		{"exact beats a longer address", "carol@y.co", "carol@y.co"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x.com", "bob@y.com", "carol@y.co", "carol@y.com")
			r.capture()
			code, out, errOut := r.run("use", c.arg)
			if code != 0 {
				t.Fatalf("code %d, stderr %q", code, errOut)
			}
			if !strings.Contains(out, "* "+c.want+" ·") {
				t.Errorf("%s not active:\n%s", c.want, out)
			}
			if !strings.Contains(out, "`ccswitch undo` switches back to a@x.com until 09:10") {
				t.Errorf("no undo offer:\n%s", out)
			}
			if got, _ := r.login.Login.Email(); got != c.want {
				t.Errorf("live login %s", got)
			}
		})
	}
}

func TestUseMatchFailures(t *testing.T) {
	cases := []struct {
		name, arg string
		capture   bool
		want      string
	}{
		{"no state yet", "a", false, "no accounts yet; `ccswitch check` captures the live login"},
		{"ambiguous", "b", true, `"b" matches more than one account: bob@y.com, bea@y.com`},
		{"none", "zed", true, `no account matches "zed"; accounts: a@x.com, bob@y.com, bea@y.com`},
		{"empty", " ", true, `no account matches ""`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x.com", "bob@y.com", "bea@y.com")
			if c.capture {
				r.capture()
			}
			code, out, errOut := r.run("use", c.arg)
			if code != 1 || out != "" || !strings.Contains(errOut, c.want) {
				t.Errorf("code %d, stdout %q, stderr %q; want %q", code, out, errOut, c.want)
			}
			if len(r.login.Writes) != 0 || r.lock.Acquired != 0 {
				t.Errorf("nothing may run: writes %d, lock %d", len(r.login.Writes), r.lock.Acquired)
			}
		})
	}
}

func TestUseStateLoadError(t *testing.T) {
	r := newRig(t, "a@x.com")
	r.store.LoadErr = errors.New("disk gone")
	if code, _, errOut := r.run("use", "a"); code != 1 || !strings.Contains(errOut, "disk gone") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestUseFailurePrintsTheStateAndTheError(t *testing.T) {
	r := newRig(t, "a@x.com", "b@y.com")
	r.capture()
	r.usage.Errs[access("b@y.com")] = errors.New("HTTP 500")
	code, out, errOut := r.run("use", "b")
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(out, "* a@x.com") {
		t.Errorf("a@x.com must still be active:\n%s", out)
	}
	if strings.Contains(out, "Last error") {
		t.Errorf("the error goes to stderr only:\n%s", out)
	}
	if !strings.Contains(errOut, "switch to b@y.com not verified") || !strings.Contains(errOut, "previous login restored") {
		t.Errorf("stderr %q", errOut)
	}
}

func TestOperationThatSavedNothingPrintsOnlyTheError(t *testing.T) {
	r := newRig(t, "a@x.com")
	r.lock.Err = errors.New("held by the tray")
	code, out, errOut := r.run("check")
	if code != 1 || out != "" || !strings.Contains(errOut, "held by the tray") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestSettingsErrorAfterAnOperation(t *testing.T) {
	r := newRig(t, "a@x.com")
	r.capture()
	r.lock.Err = errors.New("held")
	r.store.LoadErr = errors.New("settings gone")
	// The state the failed lock returns is unreadable too: nothing printed.
	code, out, errOut := r.run("undo")
	if code != 1 || out != "" || !strings.Contains(errOut, "held") || !strings.Contains(errOut, "settings gone") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestOperations(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		code  int
		check func(t *testing.T, r *rig, out, errOut string)
	}{
		{"auto on", []string{"auto", "on"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if !r.store.Settings.Auto || !strings.Contains(out, "Auto-switch on at 95 %") {
				t.Errorf("auto not on:\n%s", out)
			}
		}},
		{"auto off", []string{"auto", "off"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if r.store.Settings == nil || r.store.Settings.Auto || !strings.Contains(out, "Auto-switch off") {
				t.Errorf("auto not off:\n%s", out)
			}
		}},
		{"pool off", []string{"pool", "b", "off"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if !strings.Contains(out, "b@y.com · Max 20x · not in the auto-switch pool") {
				t.Errorf("b still in the pool:\n%s", out)
			}
		}},
		{"pool on", []string{"pool", "b@y.com", "on"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if strings.Contains(out, "not in the auto-switch pool") {
				t.Errorf("b out of the pool:\n%s", out)
			}
		}},
		{"remove", []string{"remove", "b"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if strings.Contains(out, "b@y.com") || len(r.vault.Accounts) != 1 {
				t.Errorf("b not removed:\n%s", out)
			}
		}},
		{"remove the active", []string{"remove", "a"}, 1, func(t *testing.T, r *rig, out, errOut string) {
			if !strings.Contains(errOut, "a@x.com is the active account") || !strings.Contains(out, "* a@x.com") {
				t.Errorf("stdout %q, stderr %q", out, errOut)
			}
		}},
		{"undo with nothing", []string{"undo"}, 1, func(t *testing.T, _ *rig, _, errOut string) {
			if !strings.Contains(errOut, "nothing to undo") {
				t.Errorf("stderr %q", errOut)
			}
		}},
		{"update", []string{"update"}, 0, func(t *testing.T, r *rig, out, _ string) {
			if !strings.Contains(out, "Claude Code 2.1.319 (latest)") || !strings.Contains(r.notify.Sent[0], "2.1.319") {
				t.Errorf("not updated:\n%s\n%v", out, r.notify.Sent)
			}
		}},
		{"update fails", []string{"update"}, 1, func(t *testing.T, r *rig, _, errOut string) {
			if !strings.Contains(errOut, "no network") {
				t.Errorf("stderr %q", errOut)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "a@x.com", "b@y.com")
			r.capture()
			r.claude.UpdatedVersion = "2.1.319"
			if c.name == "update fails" {
				r.claude.UpdateErr = errors.New("no network")
			}
			code, out, errOut := r.run(c.args...)
			if code != c.code {
				t.Fatalf("code %d, want %d; stderr %q", code, c.code, errOut)
			}
			c.check(t, r, out, errOut)
		})
	}
}

func TestUndoAfterUse(t *testing.T) {
	r := newRig(t, "a@x.com", "b@y.com")
	r.capture()
	if code, _, errOut := r.run("use", "b"); code != 0 {
		t.Fatalf("use: %s", errOut)
	}
	code, out, errOut := r.run("undo")
	if code != 0 || !strings.Contains(out, "* a@x.com") || strings.Contains(out, "ccswitch undo") {
		t.Errorf("code %d, stdout:\n%s\nstderr %q", code, out, errOut)
	}
}

func TestAdd(t *testing.T) {
	r := newRig(t, "a@x.com")
	r.capture()
	r.newLogin = testkit.FakeLogin("new@z.com")
	code, out, errOut := r.run("add")
	if code != 0 || !strings.Contains(out, "  new@z.com · Max 20x") || !strings.Contains(out, "* a@x.com") {
		t.Errorf("code %d, stdout:\n%s\nstderr %q", code, out, errOut)
	}
	if r.claude.Calls[0][:6] != "login " {
		t.Errorf("claude calls %v", r.claude.Calls)
	}
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		nil,
		{"frobnicate"},
		{"status", "extra"},
		{"statusline", "extra"},
		{"version", "x"},
		{"check", "now"},
		{"use"},
		{"use", "a", "b"},
		{"remove"},
		{"auto"},
		{"auto", "maybe"},
		{"pool", "a"},
		{"pool", "a", "maybe"},
		{"watch", "--forever"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			r := newRig(t, "a@x.com")
			code, out, errOut := r.run(args...)
			if code != 2 || out != "" || !strings.Contains(errOut, "Usage: ccswitch <command>") {
				t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
			if r.lock.Acquired != 0 {
				t.Error("a usage error must run nothing")
			}
		})
	}
}

func TestHelpAndVersion(t *testing.T) {
	r := newRig(t)
	for _, h := range []string{"help", "-h", "--help"} {
		if code, out, _ := r.run(h); code != 0 || !strings.Contains(out, "statusline") {
			t.Errorf("%s: code %d, %q", h, code, out)
		}
	}
	if code, out, _ := r.run("version"); code != 0 || out != "ccswitch v1.2.3\n" {
		t.Errorf("version: code %d, %q", code, out)
	}
	r.deps.Version = ""
	if _, out, _ := r.run("--version"); out != "ccswitch dev\n" {
		t.Errorf("version without a build version: %q", out)
	}
}

func TestStatusLineReadsOnlyTheSavedState(t *testing.T) {
	r := newRig(t, "ana@example.com")
	r.capture()
	// The Switcher's own reads would fail from here on, and its lock is
	// taken: statusline must touch neither.
	saved := &testkit.FakeStateStore{State: r.store.State}
	r.deps.Store = saved
	r.store.LoadErr = errors.New("switcher store used")
	release, err := r.lock.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	code, out, errOut := r.run("statusline")
	elapsed := time.Since(start)
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if want := "ana@example · weekly 10% · Claude Code 2.1.300 (2.1.319 available)\n"; out != want {
		t.Errorf("statusline %q, want %q", out, want)
	}
	if r.lock.Acquired != 1 || len(r.usage.Calls) != 0 || len(r.claude.Calls) != 0 || r.releases.Calls != 0 {
		t.Errorf("statusline touched the switcher: lock %d, usage %v, claude %v", r.lock.Acquired, r.usage.Calls, r.claude.Calls)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("statusline took %v, must be under 50 ms", elapsed)
	}
}

func TestStatusLineFailureIsPrinted(t *testing.T) {
	for _, settingsOnly := range []bool{false, true} {
		r := newRig(t)
		bad := &failingSettings{FakeStateStore: &testkit.FakeStateStore{}}
		if !settingsOnly {
			bad.LoadErr = errors.New("state.json unreadable")
		}
		r.deps.Store = bad
		code, out, _ := r.run("statusline")
		if code != 1 || !strings.HasPrefix(out, "ccswitch: ") || !strings.Contains(out, "unreadable") {
			t.Errorf("settingsOnly=%v: code %d, stdout %q", settingsOnly, code, out)
		}
	}
}

type failingSettings struct{ *testkit.FakeStateStore }

func (failingSettings) LoadSettings() (domain.Settings, error) {
	return domain.Settings{}, errors.New("settings.json unreadable")
}

type brokenPipe struct{}

func (brokenPipe) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestOutputWriteFailureIsAnError(t *testing.T) {
	r := newRig(t, "a@x.com")
	var errOut strings.Builder
	code := cli.Run(context.Background(), []string{"status"}, r.deps, brokenPipe{}, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), "write the output: pipe closed") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}
