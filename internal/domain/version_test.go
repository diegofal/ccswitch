package domain_test

import (
	"testing"

	"github.com/diegofal/ccswitch/internal/domain"
)

func TestParseVersion(t *testing.T) {
	good := map[string][3]int{
		"2.1.300":                 {2, 1, 300},
		"2.1.300 (Claude Code)":   {2, 1, 300},
		"  2.1.319\n":             {2, 1, 319},
		"10.0.0":                  {10, 0, 0},
		"2.1.300  (Claude Code) ": {2, 1, 300},
	}
	for s, want := range good {
		if got, err := domain.ParseVersion(s); err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	for _, s := range []string{"", "2.1", "2.1.300.1", "v2.1.300", "2.x.1", "2.1.-1", "2.1.300-beta", "Claude Code 2.1.300"} {
		if got, err := domain.ParseVersion(s); err == nil {
			t.Errorf("ParseVersion(%q) = %v; want error", s, got)
		}
	}
}

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.1.300", "2.1.319", -1},
		{"2.1.319", "2.1.300 (Claude Code)", 1},
		{"2.1.300 (Claude Code)", "2.1.300", 0},
		{"2.10.0", "2.9.99", 1},
		{"1.9.9", "2.0.0", -1},
	}
	for _, c := range cases {
		if got, err := domain.CompareVersion(c.a, c.b); err != nil || got != c.want {
			t.Errorf("CompareVersion(%q, %q) = %d, %v; want %d", c.a, c.b, got, err, c.want)
		}
	}
	if _, err := domain.CompareVersion("x", "2.1.0"); err == nil {
		t.Error("malformed a: want error")
	}
	if _, err := domain.CompareVersion("2.1.0", "x"); err == nil {
		t.Error("malformed b: want error")
	}
}
