package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Limit is one row of the usage endpoint's limits[]. Nothing is hard-coded
// to a kind: every active row is shown and alerted on (PLAN D6).
type Limit struct {
	Kind, Group, Scope string // Scope = scope.model.display_name or ""
	Percent            float64
	Severity           string // "normal" or anything else
	ResetsAt           time.Time
	Active             bool // is_active
}

// Usage is one read of an account's limits.
type Usage struct {
	ReadAt time.Time
	Limits []Limit
	Err    string // non-empty = unreadable; Limits then empty
}

// limitRow mirrors a limits[] row with pointers, so a missing member is an
// error instead of a zero value shown as a fact.
type limitRow struct {
	Kind     *string  `json:"kind"`
	Group    *string  `json:"group"`
	Percent  *float64 `json:"percent"`
	Severity *string  `json:"severity"`
	ResetsAt *string  `json:"resets_at"`
	IsActive *bool    `json:"is_active"`
	Scope    *struct {
		Model *struct {
			DisplayName string `json:"display_name"`
		} `json:"model"`
	} `json:"scope"`
}

// ParseUsage reads the usage endpoint's body. Only limits[] is used; the
// endpoint is undocumented, so a body without a complete limits[] is an
// error, never an account at 0 %.
func ParseUsage(body []byte, readAt time.Time) (Usage, error) {
	var doc struct {
		Limits *[]json.RawMessage `json:"limits"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Usage{}, fmt.Errorf("usage: %w", err)
	}
	if doc.Limits == nil || len(*doc.Limits) == 0 {
		return Usage{}, errors.New("usage: no limits in the response")
	}
	u := Usage{ReadAt: readAt}
	for i, raw := range *doc.Limits {
		l, err := parseLimit(raw)
		if err != nil {
			return Usage{}, fmt.Errorf("usage: limits[%d]: %w", i, err)
		}
		u.Limits = append(u.Limits, l)
	}
	return u, nil
}

func parseLimit(raw json.RawMessage) (Limit, error) {
	var r limitRow
	if err := json.Unmarshal(raw, &r); err != nil {
		return Limit{}, err
	}
	switch {
	case r.Kind == nil:
		return Limit{}, errors.New("no kind")
	case r.Group == nil:
		return Limit{}, errors.New("no group")
	case r.Percent == nil:
		return Limit{}, errors.New("no percent")
	case r.Severity == nil:
		return Limit{}, errors.New("no severity")
	case r.IsActive == nil:
		return Limit{}, errors.New("no is_active")
	}
	// resets_at is null on the session row while no 5-hour window is open
	// (the row is then at 0 % and not active): a limit without a reset
	// time, not an unreadable response.
	var resets time.Time
	if r.ResetsAt != nil {
		var err error
		if resets, err = time.Parse(time.RFC3339Nano, *r.ResetsAt); err != nil {
			return Limit{}, fmt.Errorf("resets_at: %w", err)
		}
	}
	l := Limit{Kind: *r.Kind, Group: *r.Group, Percent: *r.Percent, Severity: *r.Severity, ResetsAt: resets, Active: *r.IsActive}
	if r.Scope != nil && r.Scope.Model != nil {
		l.Scope = r.Scope.Model.DisplayName
	}
	return l, nil
}

// Unreadable is the usage of an account whose read failed: shown red, never
// a switch target.
func Unreadable(err error, readAt time.Time) Usage {
	msg := "usage unreadable"
	if err != nil {
		msg = err.Error()
	}
	return Usage{ReadAt: readAt, Err: msg}
}

// Binding is the active limit with the highest percent: the one that stops
// the account first. On a tie the one resetting later wins, since it holds
// the account longer.
func (u Usage) Binding() (Limit, bool) {
	var best Limit
	found := false
	for _, l := range u.Limits {
		if !l.Active {
			continue
		}
		if !found || l.Percent > best.Percent || (l.Percent == best.Percent && l.ResetsAt.After(best.ResetsAt)) {
			best, found = l, true
		}
	}
	return best, found
}

// Fresh reports whether the usage was read without error at most maxAge ago.
func (u Usage) Fresh(now time.Time, maxAge time.Duration) bool {
	return u.Err == "" && !u.ReadAt.IsZero() && now.Sub(u.ReadAt) <= maxAge
}

// reached reports whether a limit is used up. Only the percent counts: the
// endpoint's severity turns to "warning" well before 100 % (seen at 78 %),
// so it is shown but never read as "reached".
func (l Limit) reached() bool {
	return l.Percent >= 100
}

// Reached reports whether any active limit is reached.
func (u Usage) Reached() bool {
	for _, l := range u.Limits {
		if l.Active && l.reached() {
			return true
		}
	}
	return false
}
