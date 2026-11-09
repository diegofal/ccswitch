package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestParseUsageRecordedResponse(t *testing.T) {
	u, err := domain.ParseUsage(testkit.Fixture(t, "usage-max20x.json"), t0)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Limit{
		{Kind: "weekly_all", Group: "weekly", Percent: 1, Severity: "normal", ResetsAt: mustTime(t, "2026-11-14T21:00:00.879290Z"), Active: true},
		{Kind: "weekly_scoped", Group: "weekly", Scope: "Fable", Percent: 1, Severity: "normal", ResetsAt: mustTime(t, "2026-11-14T21:00:00.879462Z")},
	}
	assertLimits(t, u, want)
	if !u.ReadAt.Equal(t0) || u.Err != "" {
		t.Errorf("ReadAt = %v, Err = %q", u.ReadAt, u.Err)
	}
}

func TestParseUsageSessionWarning(t *testing.T) {
	u, err := domain.ParseUsage(testkit.Fixture(t, "usage-session-warning.json"), t0)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Limit{
		{Kind: "session_all", Group: "session", Percent: 78, Severity: "warning", ResetsAt: mustTime(t, "2026-11-09T14:00:00.412Z"), Active: true},
		{Kind: "weekly_all", Group: "weekly", Percent: 41, Severity: "normal", ResetsAt: mustTime(t, "2026-11-13T18:00:00.412Z"), Active: true},
		{Kind: "weekly_scoped", Group: "weekly", Scope: "Opus", Percent: 12, Severity: "normal", ResetsAt: mustTime(t, "2026-11-13T18:00:00.412185Z")},
	}
	assertLimits(t, u, want)
}

func assertLimits(t *testing.T, u domain.Usage, want []domain.Limit) {
	t.Helper()
	if len(u.Limits) != len(want) {
		t.Fatalf("got %d limits, want %d: %+v", len(u.Limits), len(want), u.Limits)
	}
	for i, w := range want {
		g := u.Limits[i]
		if g.Kind != w.Kind || g.Group != w.Group || g.Scope != w.Scope || g.Percent != w.Percent ||
			g.Severity != w.Severity || !g.ResetsAt.Equal(w.ResetsAt) || g.Active != w.Active {
			t.Errorf("limit %d = %+v\nwant %+v", i, g, w)
		}
	}
}

func TestParseUsageRejectsWhatItCannotRead(t *testing.T) {
	row := func(drop string) string {
		parts := []string{`"kind":"k"`, `"group":"g"`, `"percent":5`, `"severity":"normal"`, `"resets_at":"2026-11-09T14:00:00Z"`, `"is_active":true`}
		var keep []string
		for _, p := range parts {
			if !strings.HasPrefix(p, `"`+drop+`"`) {
				keep = append(keep, p)
			}
		}
		return `{"limits":[{` + strings.Join(keep, ",") + `}]}`
	}
	cases := map[string]string{
		"not JSON":          `<html>`,
		"not an object":     `[]`,
		"no limits":         `{"five_hour":null}`,
		"limits null":       `{"limits":null}`,
		"limits empty":      `{"limits":[]}`,
		"limits not array":  `{"limits":{}}`,
		"row not an object": `{"limits":[3]}`,
		"no kind":           row("kind"),
		"no group":          row("group"),
		"no percent":        row("percent"),
		"no severity":       row("severity"),
		"no resets_at":      row("resets_at"),
		"no is_active":      row("is_active"),
		"percent a string":  `{"limits":[{"kind":"k","group":"g","percent":"5","severity":"normal","resets_at":"2026-11-09T14:00:00Z","is_active":true}]}`,
		"bad resets_at":     `{"limits":[{"kind":"k","group":"g","percent":5,"severity":"normal","resets_at":"tomorrow","is_active":true}]}`,
	}
	for name, body := range cases {
		u, err := domain.ParseUsage([]byte(body), t0)
		if err == nil {
			t.Errorf("%s: want error, got %+v", name, u)
		}
	}
	if _, err := domain.ParseUsage([]byte(row("")), t0); err != nil {
		t.Errorf("complete row: %v", err)
	}
}

func TestUnreadable(t *testing.T) {
	u := domain.Unreadable(errors.New("HTTP 401"), t0)
	if u.Err != "HTTP 401" || len(u.Limits) != 0 || !u.ReadAt.Equal(t0) {
		t.Errorf("Unreadable = %+v", u)
	}
	if u := domain.Unreadable(nil, t0); u.Err == "" {
		t.Error("Unreadable(nil) must still be unreadable")
	}
}

func lim(kind string, pct float64, active bool, resets time.Time) domain.Limit {
	return domain.Limit{Kind: kind, Group: "weekly", Percent: pct, Severity: "normal", ResetsAt: resets, Active: active}
}

func TestBinding(t *testing.T) {
	r1, r2 := t0.Add(time.Hour), t0.Add(48*time.Hour)
	cases := []struct {
		name   string
		limits []domain.Limit
		want   string
		ok     bool
	}{
		{"none", nil, "", false},
		{"only inactive", []domain.Limit{lim("a", 99, false, r1)}, "", false},
		{"highest active", []domain.Limit{lim("a", 10, true, r1), lim("b", 99, false, r1), lim("c", 40, true, r1)}, "c", true},
		{"tie: the one that holds longer", []domain.Limit{lim("a", 40, true, r1), lim("b", 40, true, r2)}, "b", true},
	}
	for _, c := range cases {
		got, ok := domain.Usage{Limits: c.limits}.Binding()
		if got.Kind != c.want || ok != c.ok {
			t.Errorf("%s: Binding = %q, %v; want %q, %v", c.name, got.Kind, ok, c.want, c.ok)
		}
	}
}

func TestFresh(t *testing.T) {
	cases := []struct {
		name string
		u    domain.Usage
		now  time.Time
		want bool
	}{
		{"just read", domain.Usage{ReadAt: t0}, t0, true},
		{"at max age", domain.Usage{ReadAt: t0}, t0.Add(6 * time.Minute), true},
		{"older", domain.Usage{ReadAt: t0}, t0.Add(6*time.Minute + time.Second), false},
		{"unreadable", domain.Usage{ReadAt: t0, Err: "x"}, t0, false},
		{"never read", domain.Usage{}, t0, false},
	}
	for _, c := range cases {
		if got := c.u.Fresh(c.now, 6*time.Minute); got != c.want {
			t.Errorf("%s: Fresh = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReached(t *testing.T) {
	warn := lim("a", 50, true, t0)
	warn.Severity = "warning"
	inactiveWarn := warn
	inactiveWarn.Active = false
	cases := []struct {
		name   string
		limits []domain.Limit
		want   bool
	}{
		{"under", []domain.Limit{lim("a", 99.9, true, t0)}, false},
		{"at 100", []domain.Limit{lim("a", 100, true, t0)}, true},
		{"inactive at 100", []domain.Limit{lim("a", 100, false, t0)}, false},
		{"severity not normal", []domain.Limit{warn}, true},
		{"inactive severity", []domain.Limit{inactiveWarn}, false},
	}
	for _, c := range cases {
		if got := (domain.Usage{Limits: c.limits}).Reached(); got != c.want {
			t.Errorf("%s: Reached = %v, want %v", c.name, got, c.want)
		}
	}
}
