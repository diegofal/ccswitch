package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/diegofal/ccswitch/internal/domain"
)

// Add logs a new account in (PLAN D3): `claude auth login` in a scratch
// CLAUDE_CONFIG_DIR, so the live login is untouched; the login is read from
// there and the folder removed before the account goes into the vault (in
// the pool, not active) and its usage is read once. The same email again
// replaces that account's login (D13). The lock is not held while the user
// logs in in the browser: that can take minutes and the tray keeps polling.
func (sw *Switcher) Add(ctx context.Context) (domain.State, error) {
	l, err := sw.loginInScratch(ctx)
	return sw.run(ctx, func(s *session) error {
		if l.Credentials == nil {
			return err
		}
		return errors.Join(err, sw.addLogin(s, l))
	})
}

// loginInScratch returns the login a scratch `claude auth login` left, and
// any error, including one removing the folder (it holds tokens). A login
// returned with an error is still good.
func (sw *Switcher) loginInScratch(ctx context.Context) (domain.Login, error) {
	dir, err := sw.d.ScratchDir()
	if err != nil {
		return domain.Login{}, fmt.Errorf("make a scratch folder: %w", err)
	}
	l, err := sw.harvest(ctx, dir)
	if rerr := os.RemoveAll(dir); rerr != nil {
		err = errors.Join(err, fmt.Errorf("remove the scratch folder: %w", rerr))
	}
	return l, err
}

func (sw *Switcher) harvest(ctx context.Context, dir string) (domain.Login, error) {
	if err := sw.d.Claude.Login(ctx, dir); err != nil {
		return domain.Login{}, fmt.Errorf("claude auth login: %w", err)
	}
	l, err := sw.d.LoginStoreAt(dir).Read()
	if err != nil {
		return domain.Login{}, fmt.Errorf("read the new login: %w", err)
	}
	return l, nil
}

// addLogin captures the live login first, so the active account is known
// and never refreshed by the usage read. Without a readable live login (a
// first account, or a broken one) the account is still added, unread.
func (sw *Switcher) addLogin(s *session, l domain.Login) error {
	captureErr := sw.capture(s)
	a, err := domain.AccountFromLogin(l, s.now)
	if err != nil {
		return errors.Join(captureErr, fmt.Errorf("the new login: %w", err))
	}
	i := s.find(a.Email)
	if i < 0 {
		s.accts = append(s.accts, a)
		i = len(s.accts) - 1
	} else {
		a.InPool = s.accts[i].InPool
		s.accts[i] = a
	}
	s.dirty = true
	sw.d.Log.Printf("added %s", a.Email)
	if captureErr != nil {
		return captureErr
	}
	sw.readUsage(s, i)
	return nil
}

// SetPool puts an account in or out of the auto-switch pool (PLAN D8).
func (sw *Switcher) SetPool(ctx context.Context, email string, in bool) (domain.State, error) {
	email = normalize(email)
	return sw.run(ctx, func(s *session) error {
		i := s.find(email)
		if i < 0 {
			return fmt.Errorf("no account %s", email)
		}
		s.accts[i].InPool = in
		s.dirty = true
		return nil
	})
}

// Remove deletes an account from the vault. The active account cannot be
// removed: its login is the live one.
func (sw *Switcher) Remove(ctx context.Context, email string) (domain.State, error) {
	email = normalize(email)
	return sw.run(ctx, func(s *session) error {
		if err := sw.capture(s); err != nil {
			return err
		}
		if email == s.st.Active {
			return fmt.Errorf("%s is the active account; switch to another one first", email)
		}
		i := s.find(email)
		if i < 0 {
			return fmt.Errorf("no account %s", email)
		}
		s.accts = slices.Delete(s.accts, i, i+1)
		delete(s.usage, email)
		s.dirty = true
		if u := s.st.Undo; u != nil && (u.From == email || u.To == email) {
			s.st.Undo = nil
		}
		sw.d.Log.Printf("removed %s", email)
		return nil
	})
}
