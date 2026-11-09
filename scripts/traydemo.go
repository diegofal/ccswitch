//go:build ignore

// traydemo runs the real tray on made-up accounts, for screenshots and UI
// work: every port is a testkit fake, so it reads no real login, touches no
// vault or ~/.claude, and makes no network call. The menu draws with the real
// time, so the made-up limits are read "now" and reset at fixed distances
// from the start. Usage: go run -ldflags "-H windowsgui" scripts/traydemo.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
	"github.com/diegofal/ccswitch/internal/tray/shell"
)

// demo is one made-up account. Every address is in a domain reserved for
// examples (RFC 2606).
type demo struct {
	email, plan, org string
	inPool           bool
	limits           []domain.Limit
}

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

func weekly(pct float64, resets time.Time) domain.Limit {
	return domain.Limit{Kind: "weekly_all", Group: "weekly", Percent: pct, Severity: "normal", ResetsAt: resets, Active: true}
}

func session(pct float64, resets time.Time) domain.Limit {
	return domain.Limit{Kind: "session", Group: "session", Percent: pct, Severity: "normal", ResetsAt: resets, Active: true}
}

func main() {
	now := time.Now().Truncate(time.Minute)
	in := func(d time.Duration) time.Time { return now.Add(d) }
	const h, day = time.Hour, 24 * time.Hour
	demos := []demo{
		{"ana@example.com", "Max 20x", "", true, []domain.Limit{session(12, in(3*h+40*time.Minute)), weekly(27, in(4*day+22*h))}},
		{"dev@example.org", "Max 5x", "", true, []domain.Limit{weekly(94, in(3*day+17*h))}},
		{"home@example.net", "Pro", "", true, []domain.Limit{weekly(99, in(5*h+19*time.Minute))}},
		{"seat@example.com", "Team", "Example Co", false, []domain.Limit{weekly(97, in(2*day+19*h))}},
	}

	vault := &testkit.FakeVault{}
	usage := &testkit.FakeUsageAPI{Usages: map[string]domain.Usage{}}
	ms := func(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }
	for _, d := range demos {
		l := testkit.FakeLogin(d.email)
		// FakeLogin's tokens expire 12 h and 28 days after testkit.T0; move
		// that to now, or the inactive accounts would need a refresh.
		l.Credentials = json.RawMessage(strings.NewReplacer(
			ms(testkit.T0.Add(12*h)), ms(in(12*h)),
			ms(testkit.T0.Add(28*day)), ms(in(28*day)),
		).Replace(string(l.Credentials)))
		l.Profile = json.RawMessage(strings.NewReplacer(
			`"planDisplayName":"Max 20x"`, `"planDisplayName":"`+d.plan+`"`,
			`"organizationName":"Fake Org"`, `"organizationName":"`+d.org+`"`,
		).Replace(string(l.Profile)))
		a, err := domain.AccountFromLogin(l, now)
		if err != nil {
			fail(err)
		}
		a.InPool = d.inPool
		vault.Accounts = append(vault.Accounts, a)
		usage.Usages["fake-access-"+d.email] = domain.Usage{ReadAt: now, Limits: d.limits}
	}
	live := &testkit.FakeLoginStore{Login: vault.Accounts[0].Login, Mod: now}
	notifier := shell.NewNotifier()
	sw := core.New(core.Deps{
		Login:      live,
		Vault:      vault,
		Usage:      usage,
		Claude:     &testkit.FakeClaude{VersionOut: "2.1.300", UpdatedVersion: "2.1.321"},
		Releases:   &testkit.FakeReleases{Version: "2.1.321"},
		Notify:     &testkit.FakeNotifier{}, // no desktop toasts for made-up limits
		Clock:      clock{},
		Lock:       &testkit.FakeLock{},
		Store:      &testkit.FakeStateStore{},
		Log:        &testkit.FakeLogger{},
		ScratchDir: func() (string, error) { return "", errors.New("traydemo: no adding accounts") },
	})
	logf := func(format string, args ...any) { fmt.Fprintf(os.Stderr, "traydemo: "+format+"\n", args...) }
	err := shell.Run(context.Background(), shell.Options{
		Switcher:     sw,
		Notifier:     notifier,
		LoginModTime: live.ModTime,
		Start:        func(string, ...string) error { return errors.New("traydemo: nothing to start") },
		LogPath:      os.DevNull,
		Logf:         logf,
		Version:      "demo",
	})
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "traydemo:", err)
	os.Exit(1)
}
