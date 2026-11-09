package core

import (
	"bytes"
	"context"
	"fmt"

	"github.com/diegofal/ccswitch/internal/domain"
)

// Capture reads the live login and makes the vault agree with it (PLAN D2,
// D13): an unknown email becomes a new account in the pool; a known one whose
// login changed (Claude Code refreshed its tokens, or the org moved) is
// snapshotted into the vault. The live login is never written. State.Active
// is the live email.
func (sw *Switcher) Capture(ctx context.Context) (domain.State, error) {
	return sw.run(ctx, sw.capture)
}

func (sw *Switcher) capture(s *session) error {
	live, err := sw.d.Login.Read()
	if err != nil {
		return fmt.Errorf("read the live login: %w", err)
	}
	email, err := live.Email()
	if err != nil {
		return fmt.Errorf("read the live login: %w", err)
	}
	i := s.find(email)
	if i < 0 || !sameLogin(s.accts[i].Login, live) {
		a, err := domain.AccountFromLogin(live, s.now)
		if err != nil {
			return fmt.Errorf("capture %s: %w", email, err)
		}
		if i < 0 {
			s.accts = append(s.accts, a)
			sw.d.Log.Printf("captured a new login: %s", email)
		} else {
			a.InPool = s.accts[i].InPool
			s.accts[i] = a
		}
		s.dirty = true
	}
	s.st.Active, s.live = email, live
	return nil
}

// sameLogin compares the raw bytes: any change Claude Code made is kept.
func sameLogin(a, b domain.Login) bool {
	return bytes.Equal(a.Credentials, b.Credentials) && bytes.Equal(a.OrganizationUUID, b.OrganizationUUID) &&
		bytes.Equal(a.Profile, b.Profile)
}
