package domain_test

import (
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

var (
	avNow = time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC)
	tue   = time.Date(2026, 11, 10, 4, 0, 0, 0, time.UTC)
	thu   = time.Date(2026, 11, 12, 17, 59, 0, 0, time.UTC)
	fri   = time.Date(2026, 11, 13, 15, 59, 0, 0, time.UTC)
)

func avLim(group string, pct float64, resets time.Time) domain.Limit {
	return domain.Limit{Group: group, Percent: pct, ResetsAt: resets, Active: true}
}

func readAt(limits ...domain.Limit) domain.Usage { return domain.Usage{ReadAt: avNow, Limits: limits} }

func TestFreeAt(t *testing.T) {
	cases := []struct {
		name   string
		u      domain.Usage
		want   time.Time
		wantOK bool
	}{
		{"under threshold: now", readAt(avLim("weekly", 27, fri)), avNow, true},
		{"over: when it resets", readAt(avLim("weekly", 99, tue)), tue, true},
		{"two over: the later reset", readAt(avLim("session", 96, tue), avLim("weekly", 97, thu)), thu, true},
		{"one over, one under: the over one", readAt(avLim("session", 10, tue), avLim("weekly", 97, thu)), thu, true},
		{"inactive limit ignored", readAt(domain.Limit{Group: "weekly", Percent: 99, ResetsAt: thu}), avNow, true},
		{"no active limit: now", readAt(), avNow, true},
		{"over with no reset time: unknown", readAt(avLim("weekly", 99, time.Time{})), time.Time{}, false},
		{"unreadable: unknown", domain.Unreadable(nil, avNow), time.Time{}, false},
		{"never read: unknown", domain.Usage{}, time.Time{}, false},
	}
	for _, c := range cases {
		got, ok := c.u.FreeAt(95, avNow)
		if ok != c.wantOK || !got.Equal(c.want) {
			t.Errorf("%s: FreeAt = %v, %v; want %v, %v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestByAvailability(t *testing.T) {
	in := []domain.AccountState{
		{Email: "unread@x", Usage: domain.Unreadable(nil, avNow)},
		{Email: "thu@x", Usage: readAt(avLim("weekly", 97, thu))},
		{Email: "low50@x", Usage: readAt(avLim("weekly", 50, fri))},
		{Email: "tue@x", Usage: readAt(avLim("weekly", 99, tue))},
		{Email: "low20@x", Usage: readAt(avLim("weekly", 20, fri))},
		{Email: "never@x"},
	}
	got := domain.ByAvailability(in, 95, avNow)
	want := []string{"low20@x", "low50@x", "tue@x", "thu@x", "unread@x", "never@x"}
	for i, a := range got {
		if a.Email != want[i] {
			t.Fatalf("order = %v, want %v", emails(got), want)
		}
	}
	if in[0].Email != "unread@x" {
		t.Errorf("input reordered: %v", emails(in))
	}
}

func emails(as []domain.AccountState) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Email)
	}
	return out
}
