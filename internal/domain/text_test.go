package domain_test

import (
	"os"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

// TestMain pins the local zone: reset times are shown in local time.
func TestMain(m *testing.M) {
	time.Local = time.FixedZone("ART", -3*60*60)
	os.Exit(m.Run())
}

func TestBarText(t *testing.T) {
	cases := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 10, "░░░░░░░░░░"},
		{1, 10, "▓░░░░░░░░░"},
		{41, 10, "▓▓▓▓░░░░░░"},
		{78, 10, "▓▓▓▓▓▓▓▓░░"},
		{96, 10, "▓▓▓▓▓▓▓▓▓░"},
		{100, 10, "▓▓▓▓▓▓▓▓▓▓"},
		{130, 10, "▓▓▓▓▓▓▓▓▓▓"},
		{-5, 4, "░░░░"},
		{50, 0, ""},
		{50, -1, ""},
	}
	for _, c := range cases {
		if got := domain.BarText(c.pct, c.width); got != c.want {
			t.Errorf("BarText(%v, %d) = %q, want %q", c.pct, c.width, got, c.want)
		}
	}
}

func TestResetText(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) // Mon 09:00 local
	cases := []struct {
		name   string
		resets time.Time
		want   string
	}{
		{"later today", time.Date(2026, 11, 9, 16, 0, 0, 0, time.UTC), "13:00"},
		{"today, local day not UTC day", time.Date(2026, 11, 10, 2, 59, 0, 0, time.UTC), "23:59"},
		{"tomorrow", time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC), "Tue 00:00"},
		{"within a week", time.Date(2026, 11, 13, 21, 0, 0, 0, time.UTC), "Fri 18:00"},
		{"a week or more", time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC), "16 Nov"},
		{"past", time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), "2 Nov"},
		{"no window", time.Time{}, "—"},
	}
	for _, c := range cases {
		if got := domain.ResetText(c.resets, now); got != c.want {
			t.Errorf("%s: ResetText = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPercentText(t *testing.T) {
	for p, want := range map[float64]string{0: "0", 78: "78", 99.69: "99.6", 0.7: "0.7", 100: "100"} {
		if got := domain.PercentText(p); got != want {
			t.Errorf("PercentText(%v) = %q, want %q", p, got, want)
		}
	}
}

func TestResetInText(t *testing.T) {
	now := time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC) // Mon 19:00 local
	if got := domain.ResetInText(time.Date(2026, 11, 10, 4, 0, 0, 0, time.UTC), now); got != "in 6h 0m · Tue 01:00" {
		t.Errorf("ResetInText = %q", got)
	}
	if got := domain.ResetInText(time.Time{}, now); got != "—" {
		t.Errorf("ResetInText(no window) = %q, want —", got)
	}
}

func TestBar(t *testing.T) {
	if got := domain.Bar(41, 10, "▰", "▱"); got != "▰▰▰▰▱▱▱▱▱▱" {
		t.Errorf("Bar = %q", got)
	}
}

func TestUntilText(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"days and hours", 4*24*time.Hour + 22*time.Hour + 59*time.Minute, "in 4d 22h"},
		{"exactly a day", 24 * time.Hour, "in 1d 0h"},
		{"hours and minutes", 5*time.Hour + 12*time.Minute, "in 5h 12m"},
		{"seconds round up", 5*time.Hour + 12*time.Minute + 30*time.Second, "in 5h 13m"},
		{"minutes", 42*time.Minute + 10*time.Second, "in 43m"},
		{"seconds count as a minute", 20 * time.Second, "in 1m"},
		{"just under an hour", 59*time.Minute + 30*time.Second, "in 1h 0m"},
		{"due", 0, "now"},
		{"past", -time.Hour, "now"},
	}
	for _, c := range cases {
		if got := domain.UntilText(now.Add(c.in), now); got != c.want {
			t.Errorf("%s: UntilText = %q, want %q", c.name, got, c.want)
		}
	}
	if got := domain.UntilText(time.Time{}, now); got != "" {
		t.Errorf("no window: UntilText = %q, want empty", got)
	}
}
