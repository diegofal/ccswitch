package cli

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
)

const barWidth = 10

// writeStatus prints the state as a table:
//
//   - ana@example.com · Max 20x
//     weekly            ▓░░░░░░░░░    1%  resets Sat 18:00
//     home@example.net · Pro · not in the auto-switch pool
//     unreadable: login expired — add it again
//
// then the auto-switch setting, the Claude Code versions, the undo offer and,
// when lastError, the last operation's error. A read older than
// core.MaxAge carries "as of HH:MM" on every row, so an old number never
// reads as current.
func writeStatus(w *out, st domain.State, set domain.Settings, now time.Time, lastError bool) {
	if len(st.Accounts) == 0 {
		w.println("No accounts yet: `ccswitch check` captures the live login, `ccswitch add` adds another.")
	}
	for _, a := range st.Accounts {
		writeAccount(w, a, a.Email == st.Active, set, now)
	}
	w.println()
	if set.Auto {
		w.printf("Auto-switch on at %s %%\n", domain.PercentText(set.ThresholdPercent))
	} else {
		w.printf("Auto-switch off (would switch at %s %%)\n", domain.PercentText(set.ThresholdPercent))
	}
	w.println(versionsText(st))
	if u := st.Undo; u != nil && now.Before(u.Until) {
		w.printf("`ccswitch undo` switches back to %s until %s\n", u.From, clock(u.Until))
	}
	if lastError && st.LastError != "" {
		w.printf("Last error: %s\n", st.LastError)
	}
}

func writeAccount(w *out, a domain.AccountState, active bool, set domain.Settings, now time.Time) {
	head := []string{a.Email}
	if a.Plan != "" {
		head = append(head, a.Plan)
	}
	if !a.InPool {
		head = append(head, "not in the auto-switch pool")
	}
	marker := " "
	if active {
		marker = "*"
	}
	w.printf("%s %s\n", marker, strings.Join(head, " · "))

	u := a.Usage
	switch {
	case u.Err != "":
		w.printf("    unreadable: %s (tried %s)\n", u.Err, clock(u.ReadAt))
		return
	case u.ReadAt.IsZero():
		w.println("    usage not read yet")
		return
	}
	var rows []domain.Limit
	width := 0
	for _, l := range u.Limits {
		if l.Active {
			rows = append(rows, l)
			width = max(width, utf8.RuneCountInString(limitLabel(l)))
		}
	}
	if len(rows) == 0 {
		w.println("    no active limits")
	}
	stale := ""
	if !u.Fresh(now, core.MaxAge(set)) {
		stale = "  as of " + clock(u.ReadAt)
	}
	for _, l := range rows {
		w.printf("    %-*s  %s %5s%%  resets %s%s\n", width, limitLabel(l),
			domain.BarText(l.Percent, barWidth), domain.PercentText(l.Percent), domain.ResetText(l.ResetsAt, now), stale)
	}
}

// limitLabel is the limit's group, and its model when it has one:
// "weekly", "weekly (Opus)".
func limitLabel(l domain.Limit) string {
	if l.Scope == "" {
		return l.Group
	}
	return l.Group + " (" + l.Scope + ")"
}

func versionsText(st domain.State) string {
	if st.Installed == "" {
		return "Claude Code version not read yet"
	}
	s := "Claude Code " + st.Installed
	if st.Latest == "" {
		return s
	}
	if c, err := domain.CompareVersion(st.Installed, st.Latest); err == nil && c < 0 {
		return s + " · " + st.Latest + " available (`ccswitch update`)"
	}
	return s + " (latest)"
}

func clock(t time.Time) string { return t.In(time.Local).Format("15:04") }
