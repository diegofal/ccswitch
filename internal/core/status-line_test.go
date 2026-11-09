package core_test

import (
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
)

func TestStatusLine(t *testing.T) {
	set := domain.DefaultSettings()
	two := usage(1)
	two.Limits = append(two.Limits,
		domain.Limit{Kind: "weekly_scoped", Group: "weekly", Scope: "Opus", Percent: 99.69, ResetsAt: weeklyResets, Active: true},
		domain.Limit{Kind: "session", Group: "session", Percent: 50, ResetsAt: weeklyResets, Active: false})
	st := func(u domain.Usage, installed, latest string) domain.State {
		return domain.State{
			Active:    "ana@example.com",
			Accounts:  []domain.AccountState{{Email: "other@x.com", Usage: usage(80)}, {Email: "ana@example.com", Usage: u}},
			Installed: installed, Latest: latest,
		}
	}
	cases := []struct {
		name string
		st   domain.State
		now  time.Time
		want string
	}{
		{"update available", st(usage(1), "2.1.300", "2.1.319"), t0,
			"ana@example · weekly 1% · Claude Code 2.1.300 (2.1.319 available)"},
		{"up to date, scoped limit", st(two, "2.1.319", "2.1.319"), t0,
			"ana@example · weekly 1% · weekly Opus 99.6% · Claude Code 2.1.319"},
		{"latest unknown", st(usage(1), "2.1.300", ""), t0,
			"ana@example · weekly 1% · Claude Code 2.1.300"},
		{"stale", st(usage(1), "", ""), t0.Add(46 * time.Minute),
			"ana@example · weekly 1% (as of 09:00)"},
		{"unreadable", st(domain.Unreadable(nil, t0), "", ""), t0,
			"ana@example · usage unreadable"},
		{"never read", st(domain.Usage{}, "", ""), t0,
			"ana@example · usage not read yet"},
		{"no account", domain.State{Installed: "2.1.300"}, t0,
			"no account · Claude Code 2.1.300"},
		{"email without domain", domain.State{Active: "local", Accounts: []domain.AccountState{{Email: "local", Usage: usage(5)}}}, t0,
			"local · weekly 5%"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := core.StatusLine(c.st, set, c.now); got != c.want {
				t.Errorf("StatusLine =\n  %q\nwant\n  %q", got, c.want)
			}
		})
	}
}
