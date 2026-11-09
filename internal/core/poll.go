package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// refreshAhead is how long before its refresh token dies an inactive
// account is refreshed even if its access token is still good (PLAN D2).
const refreshAhead = 7 * 24 * time.Hour

// Poll is one round of the poll loop (PLAN §4): capture the live login, read
// the usage of every account that is due (the active one every PollActive,
// the others every PollInactive; full = all of them now), post new alerts,
// auto-switch when on, and check the Claude Code version every VersionEvery.
// An account whose read fails shows its error as its usage; that is not an
// error of the poll.
func (sw *Switcher) Poll(ctx context.Context, full bool) (domain.State, error) {
	return sw.run(ctx, func(s *session) error {
		if err := sw.capture(s); err != nil {
			return err
		}
		prev := maps.Clone(s.usage)
		for i := range s.accts {
			if full || s.due(s.accts[i].Email) {
				sw.readUsage(s, i)
			}
		}
		sw.alert(s, prev)
		if s.set.Auto {
			sw.auto(s)
		}
		if s.st.VersionReadAt.IsZero() || s.now.Sub(s.st.VersionReadAt) >= s.set.VersionEvery {
			if err := sw.checkVersion(s); err != nil {
				s.problem(err)
			}
		}
		return nil
	})
}

func (s *session) due(email string) bool {
	u, ok := s.usage[email]
	if !ok || u.ReadAt.IsZero() {
		return true
	}
	every := s.set.PollInactive
	if email == s.st.Active {
		every = s.set.PollActive
	}
	return s.now.Sub(u.ReadAt) >= every
}

// readUsage reads account i's usage into the session, refreshing an
// inactive account's tokens when they are expired, its refresh token is
// about to die, or the endpoint refuses its token. The active account is
// never refreshed: Claude Code owns its tokens (PLAN D2).
func (sw *Switcher) readUsage(s *session, i int) {
	email := s.accts[i].Email
	active := email == s.st.Active
	s.fetched[email] = true
	if !active && needsRefresh(s.accts[i], s.now) {
		if err := sw.refresh(s, i); err != nil {
			s.usage[email] = domain.Unreadable(err, s.now)
			return
		}
	}
	u, err := sw.fetch(s, s.accts[i])
	if errors.Is(err, ports.ErrUnauthorized) && !active {
		if rerr := sw.refresh(s, i); rerr != nil {
			s.usage[email] = domain.Unreadable(rerr, s.now)
			return
		}
		u, err = sw.fetch(s, s.accts[i])
	}
	if err != nil {
		u = domain.Unreadable(err, s.now)
	}
	s.usage[email] = u
}

func (sw *Switcher) fetch(s *session, a domain.Account) (domain.Usage, error) {
	token, err := a.AccessToken()
	if err != nil {
		return domain.Usage{}, err
	}
	u, err := sw.d.Usage.Fetch(s.ctx, token)
	if err != nil {
		return domain.Usage{}, err
	}
	u.ReadAt = s.now
	return u, nil
}

func needsRefresh(a domain.Account, now time.Time) bool {
	if a.Expired(now) {
		return true
	}
	dies, err := a.RefreshExpiresAt()
	if err != nil || dies.IsZero() {
		return false
	}
	// The refresh response does not say when the new refresh token dies, so
	// the stored date outlives a refresh. CapturedAt (set on every refresh)
	// tells whether the refresh token was renewed since that date's week
	// began.
	from := dies.Add(-refreshAhead)
	return !now.Before(from) && a.CapturedAt.Before(from)
}

// refresh renews inactive account i's tokens and saves them to the vault at
// once: the old refresh token is dead from here on. Callers make sure i is
// not the active account.
func (sw *Switcher) refresh(s *session, i int) error {
	a := s.accts[i]
	rt, err := a.RefreshToken()
	if err != nil {
		return err
	}
	t, err := sw.d.Usage.Refresh(s.ctx, rt)
	if errors.Is(err, ports.ErrUnauthorized) {
		sw.d.Log.Printf("refresh of %s refused: %v", a.Email, err)
		return errLoginExpired
	}
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	na, err := a.WithTokens(t)
	if err != nil {
		err = fmt.Errorf("refresh %s: %w", a.Email, err)
		s.problem(err)
		return err
	}
	na.CapturedAt = s.now
	s.accts[i] = na
	if err := sw.d.Vault.Save(s.accts); err != nil {
		s.dirty = true
		err = fmt.Errorf("save the refreshed tokens of %s: %w", a.Email, err)
		s.problem(err)
		return err
	}
	sw.d.Log.Printf("refreshed the tokens of %s", a.Email)
	return nil
}

// alert posts every alert not sent yet (PLAN D7), says once when a read
// starts failing, and forgets the keys of windows that have reset.
func (sw *Switcher) alert(s *session, prev map[string]domain.Usage) {
	for _, a := range s.accts {
		for _, al := range domain.Alerts(a.Email, prev[a.Email], s.usage[a.Email], s.set.AlertThresholds, s.st.Fired) {
			sw.notifyOnce(s, al.Key, alertTitle(al), al.Message)
		}
		if al, ok := domain.UnreadableAlert(a.Email, prev[a.Email], s.usage[a.Email]); ok {
			sw.d.Log.Printf("%s", al.Message)
			sw.notify(s, "Usage unreadable", al.Message)
		}
	}
	s.st.Prune(s.now)
}

func alertTitle(a domain.Alert) string {
	switch a.Threshold {
	case 100:
		return "Limit reached"
	case 0:
		return "Available again"
	}
	return "Usage warning"
}

// auto is the auto-switch (PLAN D8). Only when the active account is at or
// over the threshold are the other in-pool accounts read again, so the
// decision never rests on a number up to PollInactive old.
func (sw *Switcher) auto(s *session) {
	from := s.st.Active
	cur, over := s.binding(from)
	if over {
		for i, a := range s.accts {
			if a.Email != from && a.InPool && !s.fetched[a.Email] {
				sw.readUsage(s, i)
			}
		}
	}
	var others []domain.Candidate
	for _, a := range s.accts {
		others = append(others, domain.Candidate{Email: a.Email, InPool: a.InPool, Usage: s.usage[a.Email]})
	}
	active := domain.Candidate{Email: from, Usage: s.usage[from]}
	d := domain.Decide(active, others, s.set.ThresholdPercent, s.now, MaxAge(s.set))
	if !d.Switch {
		if over {
			sw.d.Log.Printf("auto-switch: %s", d.Reason)
			// Said once per window of the binding limit, not every poll.
			key := from + "|auto-stay|" + cur.ResetsAt.UTC().Round(time.Minute).Format(time.RFC3339) + "|stay"
			sw.notifyOnce(s, key, "Auto-switch", d.Reason)
		}
		return
	}
	if _, err := sw.switchTo(s, d.To, true); err != nil {
		s.problem(fmt.Errorf("auto-switch to %s: %w", d.To, err))
		return
	}
	sw.d.Log.Printf("auto-switch: %s", d.Reason)
	sw.notify(s, "Switched account", fmt.Sprintf("Switched to %s · %s %s resets at %s",
		d.To, from, label(cur), domain.ResetText(cur.ResetsAt, s.now)))
}

// binding is the email's binding limit, and whether it is readable and at
// or over the auto-switch threshold.
func (s *session) binding(email string) (domain.Limit, bool) {
	u := s.usage[email]
	l, ok := u.Binding()
	return l, ok && u.Err == "" && l.Percent >= s.set.ThresholdPercent
}

func label(l domain.Limit) string {
	if l.Scope == "" {
		return l.Group
	}
	return l.Group + " " + l.Scope
}
