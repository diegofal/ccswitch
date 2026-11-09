package domain

import (
	"fmt"
	"strings"
	"time"
)

// Candidate is an account as the auto-switch sees it.
type Candidate struct {
	Email  string
	InPool bool
	Usage  Usage
}

// Decision is the auto-switch's answer; Reason is a short English sentence
// in every case, shown when nothing happens too.
type Decision struct {
	Switch bool
	To     string
	Reason string
}

// Decide is the auto-switch rule (PLAN D8). When the active account's
// binding limit is at or over threshold, switch to the in-pool account with
// fresh usage and the lowest binding percent under threshold (tie: the one
// whose binding limit resets first). The target is under the threshold by
// construction, so the rule cannot flap. An active account whose usage is
// unreadable or stale never triggers a switch: there is no fact to act on.
func Decide(active Candidate, others []Candidate, threshold float64, now time.Time, maxAge time.Duration) Decision {
	switch {
	case active.Usage.Err != "":
		return Decision{Reason: "Active account usage is unreadable."}
	case !active.Usage.Fresh(now, maxAge):
		return Decision{Reason: "Active account usage is stale."}
	}
	cur, ok := active.Usage.Binding()
	if !ok {
		return Decision{Reason: "Active account has no active limit."}
	}
	if cur.Percent < threshold {
		return Decision{Reason: fmt.Sprintf("Active account at %s %%, under threshold %s %%.", pct(cur.Percent), pct(threshold))}
	}
	head := fmt.Sprintf("Active account at %s %%", pct(cur.Percent))
	s := scanTargets(active.Email, others, threshold, now, maxAge)
	switch {
	case s.best != nil:
		return Decision{Switch: true, To: s.best.Email, Reason: fmt.Sprintf("%s, switching to %s at %s %%.", head, s.best.Email, pct(s.bestLimit.Percent))}
	case s.seen == 0:
		return Decision{Reason: head + ", no other account."}
	}
	return Decision{Reason: fmt.Sprintf("%s, no account to switch to: %s.", head, s.why())}
}

// targets is the outcome of looking at every other account: the best one,
// and why the rest were passed over.
type targets struct {
	best                               *Candidate
	bestLimit                          Limit
	seen, over, stale, outOfPool, none int
}

func scanTargets(activeEmail string, others []Candidate, threshold float64, now time.Time, maxAge time.Duration) targets {
	var s targets
	for i := range others {
		c := &others[i]
		if c.Email == activeEmail {
			continue
		}
		s.seen++
		if !c.InPool {
			s.outOfPool++
			continue
		}
		if !c.Usage.Fresh(now, maxAge) {
			s.stale++
			continue
		}
		b, ok := c.Usage.Binding()
		switch {
		case !ok:
			s.none++
		case b.Percent >= threshold:
			s.over++
		case s.best == nil || b.Percent < s.bestLimit.Percent || (b.Percent == s.bestLimit.Percent && b.ResetsAt.Before(s.bestLimit.ResetsAt)):
			s.best, s.bestLimit = c, b
		}
	}
	return s
}

func (s targets) why() string {
	var why []string
	for _, n := range []struct {
		count int
		what  string
	}{{s.over, "over threshold"}, {s.stale, "stale or unreadable"}, {s.outOfPool, "out of pool"}, {s.none, "with no active limit"}} {
		if n.count > 0 {
			why = append(why, fmt.Sprintf("%d %s", n.count, n.what))
		}
	}
	return strings.Join(why, ", ")
}
