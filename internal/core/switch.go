package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/diegofal/ccswitch/internal/domain"
)

// Switch makes email the live login (PLAN D9): capture (so the live tokens
// are snapshotted first), refresh the target if its access token expired,
// write its whole login, then verify with a usage read on its token and
// `claude auth status`. A failed verify writes the previous login back. A
// switch offers Undo for Settings.UndoWindow. Switching to the active
// account does nothing.
func (sw *Switcher) Switch(ctx context.Context, email string) (domain.State, error) {
	email = normalize(email)
	return sw.run(ctx, func(s *session) error {
		switched, err := sw.switchTo(s, email, true)
		if err != nil || !switched {
			return err
		}
		sw.notify(s, "Switched account", "Switched to "+email)
		return nil
	})
}

// Undo switches back to the account the last switch left, while the undo
// window is open. It offers no undo of its own.
func (sw *Switcher) Undo(ctx context.Context) (domain.State, error) {
	return sw.run(ctx, func(s *session) error {
		u := s.st.Undo
		if u == nil || !s.now.Before(u.Until) {
			return errors.New("nothing to undo")
		}
		if _, err := sw.switchTo(s, u.From, false); err != nil {
			return err
		}
		sw.notify(s, "Switched account", "Switched back to "+u.From)
		return nil
	})
}

// switchTo reports whether the live login changed.
func (sw *Switcher) switchTo(s *session, email string, offerUndo bool) (bool, error) {
	if err := sw.capture(s); err != nil {
		return false, err
	}
	from, prev := s.st.Active, s.live
	if email == from {
		return false, nil
	}
	i := s.find(email)
	if i < 0 {
		return false, fmt.Errorf("no account %s", email)
	}
	if s.accts[i].Expired(s.now) {
		if err := sw.refresh(s, i); err != nil {
			return false, fmt.Errorf("switch to %s: %w", email, err)
		}
	}
	target := s.accts[i]
	if err := sw.d.Login.Write(target.Login); err != nil {
		return false, sw.restore(prev, fmt.Errorf("write the login of %s: %w", email, err))
	}
	if err := sw.verify(s, target); err != nil {
		return false, sw.restore(prev, fmt.Errorf("switch to %s not verified: %w", email, err))
	}
	s.st.Active, s.live = email, target.Login
	s.st.Undo = nil
	if offerUndo {
		s.st.Undo = &domain.Undo{From: from, To: email, Until: s.now.Add(s.set.UndoWindow)}
	}
	sw.d.Log.Printf("switched from %s to %s", from, email)
	return true, nil
}

// verify checks the switch took: the target's token reads usage, and Claude
// Code itself reports the target's email. The read is kept as its usage.
func (sw *Switcher) verify(s *session, a domain.Account) error {
	u, err := sw.fetch(s, a)
	if err != nil {
		return fmt.Errorf("usage read: %w", err)
	}
	status, err := sw.d.Claude.AuthStatus(s.ctx, "")
	if err != nil {
		return fmt.Errorf("claude auth status: %w", err)
	}
	if normalize(status.Email) != a.Email {
		return fmt.Errorf("claude auth status reports %q", status.Email)
	}
	s.usage[a.Email], s.fetched[a.Email] = u, true
	return nil
}

func (sw *Switcher) restore(prev domain.Login, cause error) error {
	if err := sw.d.Login.Write(prev); err != nil {
		return errors.Join(cause, fmt.Errorf("restore the previous login: %w", err))
	}
	sw.d.Log.Printf("%v; previous login restored", cause)
	return fmt.Errorf("%w; previous login restored", cause)
}
