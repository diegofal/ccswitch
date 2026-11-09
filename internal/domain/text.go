package domain

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// pct writes a percent the way every front end shows it: whole numbers bare,
// anything else with one decimal, so 99.6 never reads as "100".
func pct(p float64) string {
	if p == math.Trunc(p) {
		return strconv.FormatFloat(p, 'f', 0, 64)
	}
	return strconv.FormatFloat(math.Floor(p*10+1e-9)/10, 'f', 1, 64) // 1e-9: 0.7*10 may be 6.999…
}

// BarText draws percent as a bar of width cells ("▓▓▓▓░░░░░░"). Any use
// shows at least one cell and anything under 100 % leaves one empty, so the
// bar never says 0 or full when the number does not.
func BarText(percent float64, width int) string {
	if width <= 0 {
		return ""
	}
	n := int(math.Round(percent / 100 * float64(width)))
	switch {
	case percent <= 0:
		n = 0
	case percent >= 100:
		n = width
	case n == 0:
		n = 1
	case n >= width:
		n = width - 1
	}
	return strings.Repeat("▓", n) + strings.Repeat("░", width-n)
}

// ResetText is when a limit resets, in local time: "13:00" today,
// "Fri 18:00" within a week, else "14 Nov".
func ResetText(resetsAt, now time.Time) string {
	r, n := resetsAt.In(time.Local), now.In(time.Local)
	ry, rm, rd := r.Date()
	ny, nm, nd := n.Date()
	switch {
	case ry == ny && rm == nm && rd == nd:
		return r.Format("15:04")
	case r.After(n) && r.Sub(n) < 7*24*time.Hour:
		return r.Format("Mon 15:04")
	}
	return r.Format("2 Jan")
}
