package core

import (
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

// StatusLine is the one line `ccswitch statusline` prints for Claude Code's
// status line: "ana@example · weekly 1% · Claude Code 2.1.300 (2.1.319
// available)". Every active limit of the active account is listed; a read
// older than MaxAge says "(as of HH:MM)" and a failed one says so, so no
// old number passes for a current one (CLAUDE.md rule 5).
func StatusLine(st domain.State, set domain.Settings, now time.Time) string {
	parts := []string{"no account"}
	if st.Active != "" {
		parts = append([]string{shortEmail(st.Active)}, usageParts(activeUsage(st), set, now)...)
	}
	if st.Installed != "" {
		v := "Claude Code " + st.Installed
		if c, err := domain.CompareVersion(st.Installed, st.Latest); err == nil && c < 0 {
			v += " (" + st.Latest + " available)"
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, " · ")
}

func activeUsage(st domain.State) domain.Usage {
	for _, a := range st.Accounts {
		if a.Email == st.Active {
			return a.Usage
		}
	}
	return domain.Usage{}
}

func usageParts(u domain.Usage, set domain.Settings, now time.Time) []string {
	switch {
	case u.Err != "":
		return []string{"usage unreadable"}
	case u.ReadAt.IsZero():
		return []string{"usage not read yet"}
	}
	var out []string
	for _, l := range u.Limits {
		if l.Active {
			out = append(out, label(l)+" "+domain.PercentText(l.Percent)+"%")
		}
	}
	if len(out) > 0 && !u.Fresh(now, MaxAge(set)) {
		out[len(out)-1] += " (as of " + u.ReadAt.In(time.Local).Format("15:04") + ")"
	}
	return out
}

// shortEmail is "local@first-label-of-domain": ana@example.com →
// ana@example.
func shortEmail(email string) string {
	local, domainPart, ok := strings.Cut(email, "@")
	if !ok {
		return email
	}
	first, _, _ := strings.Cut(domainPart, ".")
	return local + "@" + first
}
