// Package tray renders ccswitch's state as a tray menu, icon and tooltip.
// Everything here is a pure function of domain.State; the Wails glue in
// internal/tray/shell only draws the rows and calls the core.
package tray

import (
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

// Kind is what a menu row is.
type Kind int

// The row kinds.
const (
	Label     Kind = iota // a fact, not clickable
	Action                // a clickable item
	Check                 // a checkbox
	Separator             // a line
)

// Actions a row can carry; the shell maps each to a core call.
const (
	ActUse       = "use" // Arg = the account's email
	ActAuto      = "auto"
	ActUndo      = "undo"
	ActAdd       = "add"
	ActUpdate    = "update"
	ActRefresh   = "refresh"
	ActOpenLog   = "log"
	ActAutostart = "autostart"
	ActQuit      = "quit"
)

// Row is one menu row.
type Row struct {
	Kind    Kind
	Text    string
	Enabled bool
	Checked bool
	Action  string
	Arg     string
}

// UI is what the menu shows that is not in domain.State: the tray's own
// running work and the autostart registration.
type UI struct {
	Updating     bool   // `claude update` is running
	Autostart    bool   // registered to start at login
	AutostartErr string // the registration could not be read
}

// Icon levels.
const (
	LevelOK         = "ok"         // green
	LevelWarn       = "warn"       // amber, ≥ 80 %
	LevelAlert      = "alert"      // red, ≥ 95 %
	LevelUnreadable = "unreadable" // red with a dot: the read failed
	LevelStale      = "stale"      // grey: no current read (PLAN D6)
)

const (
	warnAt      = 80
	alertAt     = 95
	barWidth    = 10
	indent      = "    "
	maxErrRunes = 90
)

// MenuModel is the tray menu for st (PLAN §4): the last error first, then
// each account with its active limits, the auto-switch and undo, adding an
// account, the Claude Code version, and the app's own items.
func MenuModel(st domain.State, set domain.Settings, now time.Time, maxAge time.Duration, ui UI) []Row {
	var rows []Row
	if st.LastError != "" {
		rows = append(rows, label("⚠ "+cut(st.LastError, maxErrRunes)), sep())
	}
	if len(st.Accounts) == 0 {
		rows = append(rows, label("No account yet"))
	}
	for _, a := range st.Accounts {
		rows = append(rows, accountRows(a, a.Email == st.Active, now, maxAge)...)
	}
	rows = append(rows, sep(),
		Row{Kind: Check, Text: "Auto-switch at " + domain.PercentText(set.ThresholdPercent) + " %", Enabled: true, Checked: set.Auto, Action: ActAuto})
	if st.Undo != nil && now.Before(st.Undo.Until) {
		rows = append(rows, action("Undo → "+st.Undo.From, ActUndo, ""))
	}
	rows = append(rows, action("Add account…", ActAdd, ""), sep())
	rows = append(rows, versionRows(st, ui)...)
	rows = append(rows, sep(), action("Refresh now", ActRefresh, ""), action("Open log", ActOpenLog, ""), autostartRow(ui), action("Quit", ActQuit, ""))
	return rows
}

func accountRows(a domain.AccountState, active bool, now time.Time, maxAge time.Duration) []Row {
	head := "○ " + a.Email
	if active {
		head = "● " + a.Email
	}
	for _, s := range []string{a.Plan, a.OrgName} {
		if s != "" {
			head += " · " + s
		}
	}
	if !a.InPool {
		head += " · not in auto-switch"
	}
	rows := []Row{label(head)}
	for _, line := range usageLines(a.Usage, now, maxAge) {
		rows = append(rows, label(indent+line))
	}
	if !active {
		rows = append(rows, action(indent+"Use this account", ActUse, a.Email))
	}
	return rows
}

// usageLines are an account's usage rows: one per active limit, or the
// reason there is none. A stale read says when it was taken, so an old
// number never passes for a current one.
func usageLines(u domain.Usage, now time.Time, maxAge time.Duration) []string {
	switch {
	case u.Err != "":
		return []string{"unreadable: " + u.Err}
	case u.ReadAt.IsZero():
		return []string{"usage not read yet"}
	}
	asOf := ""
	if !u.Fresh(now, maxAge) {
		asOf = "  (as of " + clock(u.ReadAt) + ")"
	}
	var out []string
	for _, l := range u.Limits {
		if l.Active {
			out = append(out, limitLabel(l)+"  "+domain.BarText(l.Percent, barWidth)+"  "+domain.PercentText(l.Percent)+" %  resets "+domain.ResetText(l.ResetsAt, now)+asOf)
		}
	}
	if len(out) == 0 {
		return []string{"no active limit"}
	}
	return out
}

func versionRows(st domain.State, ui UI) []Row {
	if st.Installed == "" {
		return []Row{label("Claude Code version not read yet")}
	}
	if c, err := domain.CompareVersion(st.Installed, st.Latest); err != nil || c >= 0 {
		return []Row{label("Claude Code " + st.Installed)}
	}
	update := action(indent+"Update now", ActUpdate, "")
	if ui.Updating {
		update.Text, update.Enabled = indent+"Updating…", false
	}
	return []Row{label("Claude Code " + st.Installed + " · " + st.Latest + " available"), update}
}

func autostartRow(ui UI) Row {
	r := Row{Kind: Check, Text: "Start at login", Enabled: true, Checked: ui.Autostart, Action: ActAutostart}
	if ui.AutostartErr != "" {
		r.Text, r.Enabled, r.Checked = r.Text+": "+ui.AutostartErr, false, false
	}
	return r
}

// IconSpec is what the tray icon shows: the active account's binding
// percent and its level. A failed read is red; no current read is grey.
func IconSpec(st domain.State, maxAge time.Duration, now time.Time) (percent float64, level string) {
	a, ok := active(st)
	if !ok {
		if st.LastError != "" {
			return 0, LevelUnreadable
		}
		return 0, LevelStale
	}
	u := a.Usage
	if u.Err != "" {
		return 0, LevelUnreadable
	}
	b, found := u.Binding()
	switch {
	case !u.Fresh(now, maxAge):
		return b.Percent, LevelStale
	case !found:
		return 0, LevelOK
	case b.Percent >= alertAt:
		return b.Percent, LevelAlert
	case b.Percent >= warnAt:
		return b.Percent, LevelWarn
	}
	return b.Percent, LevelOK
}

// TooltipText is the tray tooltip: the active account and its binding
// limit.
func TooltipText(st domain.State, maxAge time.Duration, now time.Time) string {
	a, ok := active(st)
	if !ok {
		return "ccswitch · no account yet"
	}
	head := "ccswitch · " + a.Email + " · "
	u := a.Usage
	switch {
	case u.Err != "":
		return head + "usage unreadable"
	case u.ReadAt.IsZero():
		return head + "usage not read yet"
	}
	b, found := u.Binding()
	if !found {
		return head + "no active limit"
	}
	text := head + limitLabel(b) + " " + domain.PercentText(b.Percent) + " %"
	if !u.Fresh(now, maxAge) {
		text += " (as of " + clock(u.ReadAt) + ")"
	}
	return text
}

func active(st domain.State) (domain.AccountState, bool) {
	for _, a := range st.Accounts {
		if st.Active != "" && a.Email == st.Active {
			return a, true
		}
	}
	return domain.AccountState{}, false
}

func limitLabel(l domain.Limit) string {
	if l.Scope == "" {
		return l.Group
	}
	return l.Group + " " + l.Scope
}

func clock(t time.Time) string { return t.In(time.Local).Format("15:04") }

// cut shortens s to n runes plus "…": a menu row as wide as a long error
// would push the menu off the screen; the log has the whole text.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " …"
}

func label(text string) Row { return Row{Kind: Label, Text: text} }

func action(text, act, arg string) Row {
	return Row{Kind: Action, Text: text, Enabled: true, Action: act, Arg: arg}
}

func sep() Row { return Row{Kind: Separator} }
