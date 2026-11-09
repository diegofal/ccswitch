package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

const maxAge = 6 * time.Minute

func cand(email string, pct float64, resets time.Time) domain.Candidate {
	return domain.Candidate{Email: email, InPool: true, Usage: domain.Usage{ReadAt: t0, Limits: []domain.Limit{lim("weekly_all", pct, true, resets)}}}
}

func TestDecide(t *testing.T) {
	soon, later := t0.Add(time.Hour), t0.Add(48*time.Hour)
	active := cand("a@x", 96, soon)

	outOfPool := cand("c@x", 5, soon)
	outOfPool.InPool = false
	stale := cand("d@x", 5, soon)
	stale.Usage.ReadAt = t0.Add(-maxAge - time.Second)
	unreadable := domain.Candidate{Email: "e@x", InPool: true, Usage: domain.Unreadable(nil, t0)}
	noActive := domain.Candidate{Email: "f@x", InPool: true, Usage: domain.Usage{ReadAt: t0, Limits: []domain.Limit{lim("weekly_all", 5, false, soon)}}}
	activeStale := active
	activeStale.Usage.ReadAt = t0.Add(-time.Hour)
	activeUnreadable := domain.Candidate{Email: "a@x", InPool: true, Usage: domain.Unreadable(nil, t0)}
	activeNoLimit := noActive
	activeNoLimit.Email = "a@x"

	cases := []struct {
		name   string
		active domain.Candidate
		others []domain.Candidate
		to     string
		reason []string // words the reason must contain
	}{
		{"active under threshold", cand("a@x", 94.5, soon), []domain.Candidate{cand("b@x", 1, soon)}, "", []string{"94.5 %", "under threshold"}},
		{"active unreadable", activeUnreadable, []domain.Candidate{cand("b@x", 1, soon)}, "", []string{"unreadable"}},
		{"active stale", activeStale, []domain.Candidate{cand("b@x", 1, soon)}, "", []string{"stale"}},
		{"active without active limit", activeNoLimit, []domain.Candidate{cand("b@x", 1, soon)}, "", []string{"no active limit"}},
		{"lowest binding wins", active, []domain.Candidate{cand("b@x", 50, soon), cand("c@x", 20, later), cand("d@x", 30, soon)}, "c@x", []string{"96 %", "c@x", "20 %"}},
		{"tie: earliest reset", active, []domain.Candidate{cand("b@x", 20, later), cand("c@x", 20, soon)}, "c@x", []string{"c@x"}},
		{"at threshold counts as over", cand("a@x", 95, soon), []domain.Candidate{cand("b@x", 95, soon), cand("c@x", 94, soon)}, "c@x", []string{"c@x"}},
		{"active itself skipped", active, []domain.Candidate{cand("a@x", 1, soon)}, "", []string{"no other account"}},
		{"no other account", active, nil, "", []string{"no other account"}},
		{"all over", active, []domain.Candidate{cand("b@x", 97, soon), cand("c@x", 95, soon)}, "", []string{"2 over threshold"}},
		{"mixed", active, []domain.Candidate{cand("b@x", 99, soon), outOfPool, stale, unreadable, noActive}, "",
			[]string{"1 over threshold", "2 stale or unreadable", "1 out of pool", "1 with no active limit"}},
	}
	for _, c := range cases {
		d := domain.Decide(c.active, c.others, 95, t0, maxAge)
		if d.Switch != (c.to != "") || d.To != c.to {
			t.Errorf("%s: Decide = %+v, want switch to %q", c.name, d, c.to)
		}
		for _, w := range c.reason {
			if !strings.Contains(d.Reason, w) {
				t.Errorf("%s: reason %q lacks %q", c.name, d.Reason, w)
			}
		}
		if !strings.HasSuffix(d.Reason, ".") || strings.Contains(d.Reason, "\n") {
			t.Errorf("%s: reason %q is not one sentence", c.name, d.Reason)
		}
	}
}
