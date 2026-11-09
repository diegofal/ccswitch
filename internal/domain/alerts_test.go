package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

var thresholds = []float64{80, 95}

func usage(limits ...domain.Limit) domain.Usage {
	return domain.Usage{ReadAt: t0, Limits: limits}
}

// session is a session limit resetting at 14:00:00.412 UTC, 11:00 in the
// pinned local zone (UTC-3).
func session(pct float64) domain.Limit {
	return domain.Limit{Kind: "session_all", Group: "session", Percent: pct, Severity: "normal",
		ResetsAt: time.Date(2026, 11, 9, 14, 0, 0, 412000000, time.UTC), Active: true}
}

func keys(alerts []domain.Alert) []string {
	var k []string
	for _, a := range alerts {
		k = append(k, a.Key)
	}
	return k
}

const sessionKey = "a@x|session_all|2026-11-09T14:00:00Z|"

func TestAlertsCrossings(t *testing.T) {
	cases := []struct {
		name  string
		prev  domain.Usage
		cur   domain.Usage
		fired map[string]bool
		want  []string
	}{
		{"under every threshold", usage(session(10)), usage(session(79.9)), nil, nil},
		{"crosses 80", usage(session(70)), usage(session(85)), nil, []string{sessionKey + "80"}},
		{"over both regardless of prev", usage(session(99)), usage(session(96)), nil, []string{sessionKey + "80", sessionKey + "95"}},
		{"80 already fired", usage(), usage(session(96)), map[string]bool{sessionKey + "80": true}, []string{sessionKey + "95"}},
		{"all fired", usage(), usage(session(96)), map[string]bool{sessionKey + "80": true, sessionKey + "95": true}, nil},
		{"reached at 100", usage(), usage(session(100)), nil, []string{sessionKey + "80", sessionKey + "95", sessionKey + "100"}},
		{"inactive limit ignored", usage(), usage(domain.Limit{Kind: "x", Percent: 100, Severity: "normal"}), nil, nil},
		{"unreadable", usage(session(99)), domain.Unreadable(errors.New("HTTP 500"), t0), nil, nil},
		{"available again", usage(session(97)), usage(session(3)), nil, []string{sessionKey + "0"}},
		{"available again only below 50", usage(session(97)), usage(session(50)), nil, nil},
		{"available again only from 95", usage(session(94)), usage(session(1)), nil, nil},
		{"available again fired", usage(session(97)), usage(session(3)), map[string]bool{sessionKey + "0": true}, nil},
	}
	for _, c := range cases {
		fired := map[string]bool{}
		for k := range c.fired {
			fired[k] = true
		}
		got := keys(domain.Alerts("a@x", c.prev, c.cur, thresholds, fired))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: keys = %v, want %v", c.name, got, c.want)
		}
		if len(fired) != len(c.fired) {
			t.Errorf("%s: Alerts mutated fired", c.name)
		}
	}
}

func TestAlertsMessages(t *testing.T) {
	weekly := domain.Limit{Kind: "weekly_scoped", Group: "weekly", Scope: "Opus", Percent: 82.5, Severity: "normal",
		ResetsAt: time.Date(2026, 11, 13, 18, 0, 0, 0, time.UTC), Active: true}
	cases := []struct {
		name string
		prev domain.Usage
		cur  domain.Usage
		want []domain.Alert
	}{
		{"threshold, reset today", usage(), usage(session(81)), []domain.Alert{
			{Key: sessionKey + "80", Email: "a@x", Kind: "session_all", Threshold: 80, Message: "a@x: session limit at 81 % (resets 11:00)"},
		}},
		{"scoped, reset in days", usage(), usage(weekly), []domain.Alert{
			{Key: "a@x|weekly_scoped:Opus|2026-11-13T18:00:00Z|80", Email: "a@x", Kind: "weekly_scoped", Threshold: 80, Message: "a@x: weekly (Opus) limit at 82.5 % (resets Fri 15:00)"},
		}},
		{"reached by severity", usage(), func() domain.Usage {
			l := session(40)
			l.Severity = "blocked"
			return usage(l)
		}(), []domain.Alert{
			{Key: sessionKey + "100", Email: "a@x", Kind: "session_all", Threshold: 100, Message: "a@x: session limit reached (resets 11:00)"},
		}},
		{"available again", usage(session(100)), usage(session(0)), []domain.Alert{
			{Key: sessionKey + "0", Email: "a@x", Kind: "session_all", Threshold: 0, Message: "a@x is available again (session at 0 %)"},
		}},
	}
	for _, c := range cases {
		got := domain.Alerts("a@x", c.prev, c.cur, thresholds, nil)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

func TestAlertsThreshold100IsTheReachedAlert(t *testing.T) {
	got := domain.Alerts("a@x", usage(), usage(session(100)), []float64{100}, nil)
	if len(got) != 1 || got[0].Message != "a@x: session limit reached (resets 11:00)" {
		t.Errorf("Alerts = %+v, want one reached alert", got)
	}
}

func TestAlertsDistinguishScopedLimits(t *testing.T) {
	opus, sonnet := session(90), session(90)
	opus.Kind, opus.Scope = "weekly_scoped", "Opus"
	sonnet.Kind, sonnet.Scope = "weekly_scoped", "Sonnet"
	got := keys(domain.Alerts("a@x", usage(), usage(opus, sonnet), []float64{80}, nil))
	if len(got) != 2 || got[0] == got[1] {
		t.Errorf("two scoped limits must alert separately, got %v", got)
	}
}

func TestAlertsOnTheSessionWarningFixture(t *testing.T) {
	cur, err := domain.ParseUsage(testkit.Fixture(t, "usage-session-warning.json"), t0)
	if err != nil {
		t.Fatal(err)
	}
	// Severity "warning" counts as reached (PLAN D7: severity != normal).
	got := keys(domain.Alerts("a@x", domain.Usage{}, cur, thresholds, nil))
	want := []string{sessionKey + "100"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}
