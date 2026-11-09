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

// Row is one menu row. An enabled Label is drawn in full colour but does
// nothing when clicked: account rows are, so they do not read as greyed out.
type Row struct {
	Kind    Kind
	Text    string
	Enabled bool
	Checked bool
	Action  string
	Arg     string
	Mark    Mark // the small picture left of the text, if any
}

// Mark is a row's picture (MarkPNG): an account's level as a dot, a
// limit's percent as a ring like the tray icon's.
type Mark struct {
	Shape   string // MarkDot, MarkRing, or "" for none
	Level   string
	Percent float64 // MarkRing only
}

// Mark shapes.
const (
	MarkDot  = "dot"
	MarkRing = "ring"
)

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
// the active account and the others, the soonest usable again first
// (domain.ByAvailability), each with its active limits, the
// auto-switch and undo, adding an account, the Claude Code version, and
// the app's own items.
func MenuModel(st domain.State, set domain.Settings, now time.Time, maxAge time.Duration, ui UI) []Row {
	var rows []Row
	if st.LastError != "" {
		rows = append(rows, label("⚠ "+cut(st.LastError, maxErrRunes)), sep())
	}
	if len(st.Accounts) == 0 {
		rows = append(rows, label("No account yet"))
	}
	a, hasActive := active(st)
	if hasActive {
		rows = append(rows, accountRows(a, true, now, maxAge)...)
	}
	var others []domain.AccountState
	for _, a := range st.Accounts {
		if a.Email != st.Active {
			others = append(others, a)
		}
	}
	if hasActive && len(others) > 0 {
		rows = append(rows, sep())
	}
	for _, a := range domain.ByAvailability(others, set.ThresholdPercent, now) {
		rows = append(rows, accountRows(a, false, now, maxAge)...)
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
	head := a.Email
	for _, s := range []string{a.Plan, orgName(a)} {
		if s != "" {
			head += " · " + s
		}
	}
	if active {
		head += " · in use"
	}
	if !a.InPool {
		head += " · not in auto-switch"
	}
	percent, level := usageLevel(a.Usage, now, maxAge)
	rows := []Row{bright(head, Mark{Shape: MarkDot, Level: level, Percent: percent})}
	rows = append(rows, usageRows(a.Usage, now, maxAge)...)
	if !active {
		rows = append(rows, action(indent+"Use this account", ActUse, a.Email))
	}
	return rows
}

// orgName is the account's organisation, unless it is the default one
// Anthropic names after the email, which would only repeat it.
func orgName(a domain.AccountState) string {
	if a.OrgName == a.Email+"'s Organization" {
		return ""
	}
	return a.OrgName
}

// usageRows are an account's usage rows: one per active limit, or the
// reason there is none. A stale read says when it was taken, so an old
// number never passes for a current one.
func usageRows(u domain.Usage, now time.Time, maxAge time.Duration) []Row {
	switch {
	case u.Err != "":
		return []Row{bright(indent+"unreadable: "+u.Err, Mark{})}
	case u.ReadAt.IsZero():
		return []Row{bright(indent+"usage not read yet", Mark{})}
	}
	fresh := u.Fresh(now, maxAge)
	asOf := ""
	if !fresh {
		asOf = "  (as of " + clock(u.ReadAt) + ")"
	}
	var out []Row
	for _, l := range u.Limits {
		if l.Active {
			text := indent + limitLabel(l) + "  " + domain.Bar(l.Percent, barWidth, "▰", "▱") + "  " + domain.PercentText(l.Percent) + " %  resets " + domain.ResetInText(l.ResetsAt, now) + asOf
			out = append(out, bright(text, Mark{Shape: MarkRing, Level: levelOf(l.Percent, fresh), Percent: l.Percent}))
		}
	}
	if len(out) == 0 {
		return []Row{bright(indent+"no active limit", Mark{})}
	}
	return out
}

func versionRows(st domain.State, ui UI) []Row {
	if st.Installed == "" {
		return []Row{label("Claude Code version not read yet")}
	}
	head := "Claude Code " + st.Installed
	if st.Latest == "" {
		return []Row{label(head + " · latest version not read yet")}
	}
	if c, err := domain.CompareVersion(st.Installed, st.Latest); err != nil || c >= 0 {
		return []Row{label(head + " · up to date" + checked(st))}
	}
	update := action(indent+"Update now", ActUpdate, "")
	if ui.Updating {
		update.Text, update.Enabled = indent+"Updating…", false
	}
	return []Row{label(head + " · " + st.Latest + " available" + checked(st)), update}
}

// checked is when the latest version was last looked up.
func checked(st domain.State) string {
	if st.VersionReadAt.IsZero() {
		return ""
	}
	return " (checked " + clock(st.VersionReadAt) + ")"
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
	return usageLevel(a.Usage, now, maxAge)
}

// usageLevel is an account's binding percent and its level.
func usageLevel(u domain.Usage, now time.Time, maxAge time.Duration) (float64, string) {
	if u.Err != "" {
		return 0, LevelUnreadable
	}
	b, found := u.Binding()
	if !found {
		b.Percent = 0
	}
	return b.Percent, levelOf(b.Percent, u.Fresh(now, maxAge))
}

// levelOf is a percent's level; any read that is not current is grey.
func levelOf(percent float64, fresh bool) string {
	switch {
	case !fresh:
		return LevelStale
	case percent >= alertAt:
		return LevelAlert
	case percent >= warnAt:
		return LevelWarn
	}
	return LevelOK
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

func bright(text string, m Mark) Row { return Row{Kind: Label, Text: text, Enabled: true, Mark: m} }

func action(text, act, arg string) Row {
	return Row{Kind: Action, Text: text, Enabled: true, Action: act, Arg: arg}
}

func sep() Row { return Row{Kind: Separator} }
