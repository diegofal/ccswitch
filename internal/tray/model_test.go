package tray

import (
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

func TestMain(m *testing.M) {
	time.Local = time.FixedZone("ART", -3*60*60)
	m.Run()
}

// t0 is Monday 9 Nov 2026, 12:00 in Buenos Aires.
var t0 = time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)

const maxAge = 45 * time.Minute

func limit(group, scope string, percent float64, resets time.Time) domain.Limit {
	return domain.Limit{Kind: group, Group: group, Scope: scope, Percent: percent, Severity: "normal", ResetsAt: resets, Active: true}
}

func read(at time.Time, limits ...domain.Limit) domain.Usage {
	return domain.Usage{ReadAt: at, Limits: limits}
}

var (
	saturday = time.Date(2026, 11, 14, 21, 0, 0, 0, time.UTC) // Sat 18:00 ART
	at13     = time.Date(2026, 11, 9, 16, 0, 0, 0, time.UTC)  // today 13:00 ART
)

// dump writes rows one per line: "---" separator, "[x] "/"[ ] " checkbox,
// "> " action, "  " label, "+ " label in full colour (then its picture); a disabled action or checkbox ends in " (off)",
// and an action's argument follows "=>".
func dump(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		switch r.Kind {
		case Separator:
			b.WriteString("---")
		case Label:
			lead := "  "
			if r.Enabled {
				lead = "+ "
			}
			b.WriteString(lead + markText(r.Mark) + r.Text)
		case Action:
			b.WriteString("> " + r.Text + " [" + r.Action)
			if r.Arg != "" {
				b.WriteString(" " + r.Arg)
			}
			b.WriteString("]")
		case Check:
			mark := "[ ] "
			if r.Checked {
				mark = "[x] "
			}
			b.WriteString(mark + r.Text + " [" + r.Action + "]")
		}
		if r.Kind != Separator && r.Kind != Label && !r.Enabled {
			b.WriteString(" (off)")
		}
		if r.Kind == Label && r.Action != "" {
			b.WriteString(" (label with an action!)")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// markText is a row's picture as "(dot ok) " or "(ring ok 78) ".
func markText(m Mark) string {
	switch m.Shape {
	case "":
		return ""
	case MarkRing:
		return "(" + m.Shape + " " + m.Level + " " + domain.PercentText(m.Percent) + ") "
	}
	return "(" + m.Shape + " " + m.Level + ") "
}

func twoAccounts() domain.State {
	return domain.State{
		Active: "ana@example.com",
		Accounts: []domain.AccountState{
			{Email: "ana@example.com", Plan: "Max 20x", InPool: true, Usage: read(t0, limit("weekly", "", 1, saturday))},
			{Email: "personal@x.com", Plan: "Pro", InPool: true, Usage: read(t0,
				limit("session", "", 78, at13),
				limit("weekly", "Opus", 41.37, saturday),
				domain.Limit{Group: "monthly", Percent: 3, ResetsAt: saturday, Active: false})},
		},
		Installed: "2.1.300",
		Latest:    "2.1.319",
	}
}

func TestMenuModelFullMenu(t *testing.T) {
	want := `+ (dot ok) ana@example.com · Max 20x · in use
+ (ring ok 1)     weekly  ▰▱▱▱▱▱▱▱▱▱  1 %  resets in 5d 6h · Sat 18:00
---
+ (dot ok) personal@x.com · Pro
+ (ring ok 78)     session  ▰▰▰▰▰▰▰▰▱▱  78 %  resets in 1h 0m · 13:00
+ (ring ok 41.3)     weekly Opus  ▰▰▰▰▱▱▱▱▱▱  41.3 %  resets in 5d 6h · Sat 18:00
>     Use this account [use personal@x.com]
---
[ ] Auto-switch at 95 % [auto]
> Add account… [add]
---
  Claude Code 2.1.300 · 2.1.319 available
>     Update now [update]
---
> Refresh now [refresh]
> Open log [log]
[ ] Start at login [autostart]
> Quit [quit]
`
	got := dump(MenuModel(twoAccounts(), domain.DefaultSettings(), t0, maxAge, UI{}))
	if got != want {
		t.Errorf("menu =\n%s\nwant\n%s", got, want)
	}
}

func TestMenuModelLimitRowsAreIndented(t *testing.T) {
	for _, r := range MenuModel(twoAccounts(), domain.DefaultSettings(), t0, maxAge, UI{}) {
		if strings.Contains(r.Text, "resets") && !strings.HasPrefix(r.Text, indent) {
			t.Errorf("limit row %q is not indented under its account", r.Text)
		}
	}
}

// section returns the dumped lines from the first one containing from up to
// (not including) the next separator.
func section(t *testing.T, menu, from string) string {
	t.Helper()
	i := strings.Index(menu, from)
	if i < 0 {
		t.Fatalf("no %q in menu:\n%s", from, menu)
	}
	i = strings.LastIndex(menu[:i], "\n") + 1 // from the start of that line
	rest := menu[i:]
	if j := strings.Index(rest, "---"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestMenuModelAccounts(t *testing.T) {
	stale := twoAccounts()
	stale.Accounts[1].Usage.ReadAt = t0.Add(-46 * time.Minute) // 11:14 ART
	unreadable := twoAccounts()
	unreadable.Accounts[1].Usage = domain.Unreadable(errFake("usage: HTTP 500"), t0)
	never := twoAccounts()
	never.Accounts[1].Usage = domain.Usage{}
	noActive := twoAccounts()
	noActive.Accounts[1].Usage = read(t0, domain.Limit{Group: "weekly", Percent: 3, ResetsAt: saturday})
	outOfPool := twoAccounts()
	outOfPool.Accounts[1].InPool = false
	outOfPool.Accounts[1].OrgName = "Example Co"
	noPlan := twoAccounts()
	noPlan.Accounts[1].Plan = ""
	defaultOrg := twoAccounts()
	defaultOrg.Accounts[1].OrgName = "personal@x.com's Organization"
	noReset := twoAccounts()
	noReset.Accounts[1].Usage = read(t0, limit("weekly", "", 3, time.Time{}))
	activeUnreadable := twoAccounts()
	activeUnreadable.Accounts[0].Usage = domain.Unreadable(nil, t0)

	cases := []struct {
		name string
		st   domain.State
		from string
		want string
	}{
		{"stale rows say as of, in grey", stale, "personal@x.com · Pro", `+ (dot stale) personal@x.com · Pro
+ (ring stale 78)     session  ▰▰▰▰▰▰▰▰▱▱  78 %  resets in 1h 0m · 13:00  (as of 11:14)
+ (ring stale 41.3)     weekly Opus  ▰▰▰▰▱▱▱▱▱▱  41.3 %  resets in 5d 6h · Sat 18:00  (as of 11:14)
>     Use this account [use personal@x.com]
`},
		{"failed read is shown", unreadable, "personal@x.com · Pro", `+ (dot unreadable) personal@x.com · Pro
+     unreadable: usage: HTTP 500
>     Use this account [use personal@x.com]
`},
		{"never read", never, "personal@x.com · Pro", `+ (dot stale) personal@x.com · Pro
+     usage not read yet
>     Use this account [use personal@x.com]
`},
		{"no active limit", noActive, "personal@x.com · Pro", `+ (dot ok) personal@x.com · Pro
+     no active limit
>     Use this account [use personal@x.com]
`},
		{"out of pool, with org", outOfPool, "personal@x.com · Pro", "+ (dot ok) personal@x.com · Pro · Example Co · not in auto-switch\n"},
		{"default org name is not repeated", defaultOrg, "personal@x.com · Pro", "+ (dot ok) personal@x.com · Pro\n"},
		{"no plan", noPlan, "personal@x.com", "+ (dot ok) personal@x.com\n"},
		{"reset with no time", noReset, "personal@x.com · Pro", `+ (dot ok) personal@x.com · Pro
+ (ring ok 3)     weekly  ▰▱▱▱▱▱▱▱▱▱  3 %  resets —
`},
		{"active unreadable", activeUnreadable, "ana@example.com", `+ (dot unreadable) ana@example.com · Max 20x · in use
+     unreadable: usage unreadable
`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			menu := dump(MenuModel(c.st, domain.DefaultSettings(), t0, maxAge, UI{}))
			if got := section(t, menu, c.from); !strings.HasPrefix(got, c.want) {
				t.Errorf("section =\n%s\nwant prefix\n%s", got, c.want)
			}
		})
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func TestMenuModelNoAccount(t *testing.T) {
	menu := dump(MenuModel(domain.State{}, domain.DefaultSettings(), t0, maxAge, UI{}))
	if !strings.HasPrefix(menu, "  No account yet\n---\n[ ] Auto-switch at 95 % [auto]\n> Add account… [add]\n---\n  Claude Code version not read yet\n---\n") {
		t.Errorf("menu =\n%s", menu)
	}
}

func TestMenuModelLastErrorFirst(t *testing.T) {
	st := twoAccounts()
	st.LastError = "read the live login: open .credentials.json: no such file"
	menu := dump(MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{}))
	if !strings.HasPrefix(menu, "  ⚠ read the live login: open .credentials.json: no such file\n---\n+ (dot ok) ana") {
		t.Errorf("menu =\n%s", menu)
	}
}

func TestMenuModelLongErrorIsCut(t *testing.T) {
	st := domain.State{LastError: strings.Repeat("é", 200)}
	rows := MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{})
	text := []rune(rows[0].Text)
	if len(text) != len([]rune("⚠ "))+maxErrRunes+len([]rune(" …")) || string(text[len(text)-1]) != "…" {
		t.Errorf("error row has %d runes, ends %q", len(text), string(text[len(text)-1]))
	}
}

func TestMenuModelAutoSwitchAndUndo(t *testing.T) {
	set := domain.DefaultSettings()
	set.Auto = true
	set.ThresholdPercent = 90.5
	st := twoAccounts()
	st.Undo = &domain.Undo{From: "personal@x.com", To: "ana@example.com", Until: t0.Add(time.Minute)}
	got := section(t, dump(MenuModel(st, set, t0, maxAge, UI{})), "[x] Auto")
	want := "[x] Auto-switch at 90.5 % [auto]\n> Undo → personal@x.com [undo]\n> Add account… [add]\n"
	if got != want {
		t.Errorf("section =\n%s\nwant\n%s", got, want)
	}

	st.Undo.Until = t0
	if menu := dump(MenuModel(st, set, t0, maxAge, UI{})); strings.Contains(menu, "Undo") {
		t.Errorf("undo shown after its window:\n%s", menu)
	}
}

func TestMenuModelVersion(t *testing.T) {
	cases := []struct {
		name              string
		installed, latest string
		ui                UI
		want              string
	}{
		{"update available", "2.1.300", "2.1.319", UI{}, "  Claude Code 2.1.300 · 2.1.319 available\n>     Update now [update]\n"},
		{"updating", "2.1.300", "2.1.319", UI{Updating: true}, "  Claude Code 2.1.300 · 2.1.319 available\n>     Updating… [update] (off)\n"},
		{"up to date", "2.1.319", "2.1.319", UI{}, "  Claude Code 2.1.319 · up to date\n"},
		{"newer than latest", "2.1.320", "2.1.319", UI{}, "  Claude Code 2.1.320 · up to date\n"},
		{"latest unknown", "2.1.300", "", UI{}, "  Claude Code 2.1.300 · latest version not read yet\n"},
		{"installed unreadable", "", "2.1.319", UI{}, "  Claude Code version not read yet\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := twoAccounts()
			st.Installed, st.Latest = c.installed, c.latest
			got := section(t, dump(MenuModel(st, domain.DefaultSettings(), t0, maxAge, c.ui)), "  Claude Code")
			if got != c.want {
				t.Errorf("section =\n%s\nwant\n%s", got, c.want)
			}
		})
	}
}

func TestMenuModelAutostart(t *testing.T) {
	cases := []struct {
		ui   UI
		want string
	}{
		{UI{Autostart: true}, "[x] Start at login [autostart]\n"},
		{UI{AutostartErr: "access denied"}, "[ ] Start at login: access denied [autostart] (off)\n"},
	}
	for _, c := range cases {
		got := section(t, dump(MenuModel(twoAccounts(), domain.DefaultSettings(), t0, maxAge, c.ui)), "Start at login")
		if !strings.HasPrefix(got, c.want) {
			t.Errorf("autostart row = %q, want %q", got, c.want)
		}
	}
}

func TestIconSpec(t *testing.T) {
	withActive := func(u domain.Usage) domain.State {
		st := twoAccounts()
		st.Accounts[0].Usage = u
		return st
	}
	cases := []struct {
		name    string
		st      domain.State
		percent float64
		level   string
	}{
		{"ok", withActive(read(t0, limit("weekly", "", 1, saturday))), 1, LevelOK},
		{"just under warn", withActive(read(t0, limit("weekly", "", 79.9, saturday))), 79.9, LevelOK},
		{"warn at 80", withActive(read(t0, limit("weekly", "", 80, saturday))), 80, LevelWarn},
		{"binding row wins", withActive(read(t0, limit("weekly", "", 10, saturday), limit("session", "", 94, at13))), 94, LevelWarn},
		{"alert at 95", withActive(read(t0, limit("weekly", "", 95, saturday))), 95, LevelAlert},
		{"reached", withActive(read(t0, limit("weekly", "", 100, saturday))), 100, LevelAlert},
		{"unreadable", withActive(domain.Unreadable(nil, t0)), 0, LevelUnreadable},
		{"stale", withActive(read(t0.Add(-time.Hour), limit("weekly", "", 50, saturday))), 50, LevelStale},
		{"never read", withActive(domain.Usage{}), 0, LevelStale},
		{"no active limit", withActive(read(t0)), 0, LevelOK},
		{"no account yet", domain.State{}, 0, LevelStale},
		{"no account, read failed", domain.State{LastError: "read the live login: boom"}, 0, LevelUnreadable},
		{"active not in accounts", domain.State{Active: "ghost@x.com"}, 0, LevelStale},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, level := IconSpec(c.st, maxAge, t0)
			if p != c.percent || level != c.level {
				t.Errorf("IconSpec = %v, %q; want %v, %q", p, level, c.percent, c.level)
			}
		})
	}
}

func TestTooltipText(t *testing.T) {
	stale := twoAccounts()
	stale.Accounts[0].Usage.ReadAt = t0.Add(-time.Hour)
	unreadable := twoAccounts()
	unreadable.Accounts[0].Usage = domain.Unreadable(nil, t0)
	never := twoAccounts()
	never.Accounts[0].Usage = domain.Usage{}
	noLimit := twoAccounts()
	noLimit.Accounts[0].Usage = read(t0)
	scoped := twoAccounts()
	scoped.Accounts[0].Usage = read(t0, limit("weekly", "Opus", 99.69, saturday))
	cases := []struct {
		name string
		st   domain.State
		want string
	}{
		{"fresh", twoAccounts(), "ccswitch · ana@example.com · weekly 1 %"},
		{"scoped", scoped, "ccswitch · ana@example.com · weekly Opus 99.6 %"},
		{"stale", stale, "ccswitch · ana@example.com · weekly 1 % (as of 11:00)"},
		{"unreadable", unreadable, "ccswitch · ana@example.com · usage unreadable"},
		{"never read", never, "ccswitch · ana@example.com · usage not read yet"},
		{"no active limit", noLimit, "ccswitch · ana@example.com · no active limit"},
		{"no account", domain.State{}, "ccswitch · no account yet"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TooltipText(c.st, maxAge, t0); got != c.want {
				t.Errorf("TooltipText = %q, want %q", got, c.want)
			}
		})
	}
}

func TestMenuModelActiveAccountFirst(t *testing.T) {
	st := twoAccounts()
	st.Active = "personal@x.com"
	menu := dump(MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{}))
	if !strings.HasPrefix(menu, "+ (dot ok) personal@x.com · Pro · in use\n") || !strings.Contains(menu, "---\n+ (dot ok) ana@example.com · Max 20x\n") {
		t.Errorf("menu =\n%s", menu)
	}
}

func TestMenuModelVersionSaysWhenChecked(t *testing.T) {
	st := twoAccounts()
	st.Installed, st.Latest, st.VersionReadAt = "2.1.321", "2.1.321", time.Date(2026, 11, 9, 21, 13, 0, 0, time.UTC)
	got := section(t, dump(MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{})), "  Claude Code")
	if want := "  Claude Code 2.1.321 · up to date (checked 18:13)\n"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
	st.Installed = "2.1.300"
	got = section(t, dump(MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{})), "  Claude Code")
	if want := "  Claude Code 2.1.300 · 2.1.321 available (checked 18:13)\n>     Update now [update]\n"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
}

// The others come soonest usable first: under the threshold (lowest
// percent first), then over it by reset time, then the unreadable.
func TestMenuModelOthersBySoonestAvailable(t *testing.T) {
	tue := time.Date(2026, 11, 10, 4, 0, 0, 0, time.UTC)
	thu := time.Date(2026, 11, 12, 17, 59, 0, 0, time.UTC)
	st := domain.State{
		Active: "work@x.com",
		Accounts: []domain.AccountState{
			{Email: "work@x.com", InPool: true, Usage: read(t0, limit("weekly", "", 27, saturday))},
			{Email: "broken@x.com", InPool: true, Usage: domain.Unreadable(nil, t0)},
			{Email: "thu@x.com", InPool: true, Usage: read(t0, limit("weekly", "", 97, thu))},
			{Email: "tue@x.com", InPool: true, Usage: read(t0, limit("weekly", "", 99, tue))},
			{Email: "fresh@x.com", InPool: true, Usage: read(t0, limit("weekly", "", 12, saturday))},
		},
	}
	var order []string
	for _, r := range MenuModel(st, domain.DefaultSettings(), t0, maxAge, UI{}) {
		if r.Mark.Shape == MarkDot {
			order = append(order, strings.Fields(r.Text)[0])
		}
	}
	want := []string{"work@x.com", "fresh@x.com", "tue@x.com", "thu@x.com", "broken@x.com"}
	if strings.Join(order, " ") != strings.Join(want, " ") {
		t.Errorf("order = %v, want %v", order, want)
	}
}
