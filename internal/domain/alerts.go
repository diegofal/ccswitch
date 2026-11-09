package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Alert is one notification to post. Key identifies it so it is sent once.
type Alert struct {
	Key, Email, Kind, Message string
	Threshold                 float64
}

// Thresholds with a meaning of their own in alert keys.
const (
	reachedThreshold   = 100
	availableThreshold = 0
	// A limit that was this full and is now under availableBelow has reset.
	availableFrom  = 95
	availableBelow = 50
)

// Alerts returns the alerts for cur that are not in fired (PLAN D7), without
// changing fired. For every active limit: each threshold it is at or over,
// "limit reached" (key threshold 100) when it is reached, and "available
// again" (key threshold 0) when the same limit was at 95 % or more in prev
// and is under 50 % now. Keys carry the limit's reset time, so each fires
// once per window. An unreadable cur gives no alerts: the red state says it.
func Alerts(email string, prev, cur Usage, thresholds []float64, fired map[string]bool) []Alert {
	if cur.Err != "" {
		return nil
	}
	var out []Alert
	sent := map[string]bool{}
	add := func(l Limit, threshold float64, msg string) {
		key := alertKey(email, l, threshold)
		if fired[key] || sent[key] {
			return
		}
		sent[key] = true
		out = append(out, Alert{Key: key, Email: email, Kind: l.Kind, Message: msg, Threshold: threshold})
	}
	for _, l := range cur.Limits {
		if !l.Active {
			continue
		}
		resets := alertResetText(l.ResetsAt, cur.ReadAt)
		for _, t := range thresholds {
			if l.Percent >= t && !(t == reachedThreshold && l.reached()) {
				add(l, t, fmt.Sprintf("%s: %s limit at %s %% (resets %s)", email, label(l), pct(l.Percent), resets))
			}
		}
		if l.reached() {
			add(l, reachedThreshold, fmt.Sprintf("%s: %s limit reached (resets %s)", email, label(l), resets))
		}
		if p, ok := sameLimit(prev, l); ok && p.Percent >= availableFrom && l.Percent < availableBelow {
			add(l, availableThreshold, fmt.Sprintf("%s is available again (%s at %s %%)", email, label(l), pct(l.Percent)))
		}
	}
	return out
}

// limitID tells limits apart: two weekly_scoped rows differ only by scope.
func limitID(l Limit) string {
	if l.Scope == "" {
		return l.Kind
	}
	return l.Kind + ":" + l.Scope
}

func label(l Limit) string {
	if l.Scope == "" {
		return l.Group
	}
	return l.Group + " (" + l.Scope + ")"
}

func sameLimit(u Usage, l Limit) (Limit, bool) {
	for _, p := range u.Limits {
		if limitID(p) == limitID(l) {
			return p, true
		}
	}
	return Limit{}, false
}

// alertKey is "<email>|<kind>|<resets_at RFC3339>|<threshold>". The reset
// time is rounded to the minute: the endpoint reports it to the microsecond
// and the digits differ between rows of the same window, so they may differ
// between reads too, and an alert must not fire twice for that.
func alertKey(email string, l Limit, threshold float64) string {
	resets := "none" // a limit without a window: its key is never pruned
	if !l.ResetsAt.IsZero() {
		resets = l.ResetsAt.UTC().Round(time.Minute).Format(time.RFC3339)
	}
	return strings.Join([]string{email, limitID(l), resets, strconv.FormatFloat(threshold, 'f', -1, 64)}, "|")
}

// alertResetText is "HH:MM" local, or "Mon HH:MM" when more than 24 h away.
func alertResetText(resetsAt, now time.Time) string {
	if resetsAt.IsZero() {
		return "—"
	}
	r := resetsAt.In(time.Local)
	if resetsAt.Sub(now) > 24*time.Hour {
		return r.Format("Mon 15:04")
	}
	return r.Format("15:04")
}

// UnreadableAlert is the alert for an account whose read failed when the one
// before it had not (or there was none). A failed read is a critical state:
// the icon turns red and this says why, once. The failures that follow and
// the recovery give nothing; the numbers say that.
func UnreadableAlert(email string, prev, cur Usage) (Alert, bool) {
	if cur.Err == "" || prev.Err != "" {
		return Alert{}, false
	}
	return Alert{Key: email + "|unreadable", Email: email, Threshold: -1,
		Message: fmt.Sprintf("%s: usage could not be read (%s)", email, cur.Err)}, true
}
