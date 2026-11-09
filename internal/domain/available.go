package domain

import (
	"cmp"
	"slices"
	"time"
)

// FreeAt is when the account can be used again: now when every active
// limit is under threshold, else when the last limit at or over it resets.
// ok is false when there is nothing to go on: the read failed, there is no
// read yet, or a limit over threshold has no reset time.
func (u Usage) FreeAt(threshold float64, now time.Time) (time.Time, bool) {
	if u.Err != "" || u.ReadAt.IsZero() {
		return time.Time{}, false
	}
	free := now
	for _, l := range u.Limits {
		if !l.Active || l.Percent < threshold {
			continue
		}
		if l.ResetsAt.IsZero() {
			return time.Time{}, false
		}
		if l.ResetsAt.After(free) {
			free = l.ResetsAt
		}
	}
	return free, true
}

// ByAvailability is accts ordered by when each can be used again (FreeAt):
// the ones free now first, lowest binding percent first, as the auto-switch
// would pick them (PLAN D8); then the soonest to reset; the ones with no
// fact to go on last. Equal accounts keep their order; accts is not changed.
func ByAvailability(accts []AccountState, threshold float64, now time.Time) []AccountState {
	out := slices.Clone(accts)
	slices.SortStableFunc(out, func(a, b AccountState) int {
		fa, oka := a.Usage.FreeAt(threshold, now)
		fb, okb := b.Usage.FreeAt(threshold, now)
		switch {
		case oka != okb:
			if oka {
				return -1
			}
			return 1
		case !oka:
			return 0
		}
		if c := fa.Compare(fb); c != 0 {
			return c
		}
		la, _ := a.Usage.Binding()
		lb, _ := b.Usage.Binding()
		return cmp.Compare(la.Percent, lb.Percent)
	})
	return out
}
