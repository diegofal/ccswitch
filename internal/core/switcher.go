// Package core is what ccswitch does: capture the live login, poll usage,
// alert, switch (by hand or automatically), undo, add and remove accounts,
// check and update Claude Code. Front ends call these and render the
// domain.State they return; they hold no logic of their own.
//
// Every operation except State and Settings holds the file lock while it
// works and saves the state before returning, with LastError set to the
// error it returns ("" on success). The one exception is a lock it cannot
// get: then another process owns the files and nothing is written.
package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Deps are the Switcher's ports.
type Deps struct {
	Login    ports.LoginStore // the live login (~/.claude)
	Vault    ports.Vault
	Usage    ports.UsageAPI
	Claude   ports.Claude
	Releases ports.Releases
	Notify   ports.Notifier
	Clock    ports.Clock
	Lock     ports.Lock
	Store    ports.StateStore
	Log      ports.Logger

	// ScratchDir creates and returns a fresh, empty folder under the data
	// folder, where Add runs `claude auth login`. Add removes it.
	ScratchDir func() (string, error)
	// LoginStoreAt opens the login a scratch CLAUDE_CONFIG_DIR holds; both
	// .credentials.json and .claude.json live inside dir.
	LoginStoreAt func(dir string) ports.LoginStore
}

// Switcher runs ccswitch's operations over its ports.
type Switcher struct{ d Deps }

// New returns a Switcher over d.
func New(d Deps) *Switcher { return &Switcher{d: d} }

// MaxAge is how old a usage read may be and still count as current: three
// inactive poll intervals (PLAN D6). Older reads are stale: shown "as of",
// never a switch target.
func MaxAge(s domain.Settings) time.Duration { return 3 * s.PollInactive }

// errLoginExpired marks an account whose refresh token is refused.
var errLoginExpired = errors.New("login expired — add it again")

// session is one locked operation: settings, state and vault are loaded
// once at the start and written once at the end.
type session struct {
	ctx      context.Context
	now      time.Time
	set      domain.Settings
	st       domain.State
	accts    []domain.Account
	usage    map[string]domain.Usage // by email; written back into st.Accounts
	fetched  map[string]bool         // usage read during this operation
	live     domain.Login            // as last captured
	loaded   bool                    // accts came from the vault
	dirty    bool                    // accts must be saved
	problems []error                 // failures that did not stop the operation
}

func (s *session) problem(err error) { s.problems = append(s.problems, err) }

func (s *session) find(email string) int {
	for i, a := range s.accts {
		if a.Email == email {
			return i
		}
	}
	return -1
}

func normalize(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// run is every locked operation: lock, load, fn, save.
func (sw *Switcher) run(ctx context.Context, fn func(*session) error) (domain.State, error) {
	release, err := sw.d.Lock.Acquire(ctx)
	if err != nil {
		st, lerr := sw.d.Store.LoadState()
		return st, errors.Join(fmt.Errorf("wait for the ccswitch lock: %w", err), lerr)
	}
	defer release()
	st, err := sw.d.Store.LoadState()
	if err != nil {
		return st, fmt.Errorf("load state: %w", err)
	}
	s := &session{ctx: ctx, now: sw.d.Clock.Now(), st: st, usage: map[string]domain.Usage{}, fetched: map[string]bool{}}
	for _, a := range st.Accounts {
		s.usage[a.Email] = a.Usage
	}
	err = sw.load(s)
	if err == nil {
		err = fn(s)
	}
	if s.dirty {
		if verr := sw.d.Vault.Save(s.accts); verr != nil {
			err = errors.Join(err, fmt.Errorf("save the vault: %w", verr))
		}
	}
	err = errors.Join(append([]error{err}, s.problems...)...)
	s.finish(err)
	if serr := sw.d.Store.SaveState(s.st); serr != nil {
		err = errors.Join(err, fmt.Errorf("save state: %w", serr))
	}
	return s.st, err
}

func (sw *Switcher) load(s *session) error {
	set, err := sw.d.Store.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	s.set = set
	accts, err := sw.d.Vault.Load()
	if err != nil {
		return fmt.Errorf("load the vault: %w", err)
	}
	s.accts, s.loaded = accts, true
	return nil
}

// finish writes the session back into the state.
func (s *session) finish(err error) {
	if s.loaded {
		out := make([]domain.AccountState, len(s.accts))
		for i, a := range s.accts {
			out[i] = domain.AccountState{Email: a.Email, OrgName: a.OrgName, Plan: a.Plan, InPool: a.InPool, Usage: s.usage[a.Email]}
		}
		s.st.Accounts = out
	}
	if s.st.Undo != nil && !s.now.Before(s.st.Undo.Until) {
		s.st.Undo = nil
	}
	s.st.LastError = ""
	if err != nil {
		s.st.LastError = err.Error()
	}
	s.st.UpdatedAt = s.now
}

// notify posts a notification; a failure is the operation's problem.
func (sw *Switcher) notify(s *session, title, body string) bool {
	if err := sw.d.Notify.Notify(title, body); err != nil {
		s.problem(fmt.Errorf("notify: %w", err))
		return false
	}
	return true
}

// notifyOnce posts a notification unless key was already sent, and records
// key once it is.
func (sw *Switcher) notifyOnce(s *session, key, title, body string) {
	if s.st.Fired[key] {
		return
	}
	if sw.notify(s, title, body) {
		if s.st.Fired == nil {
			s.st.Fired = map[string]bool{}
		}
		s.st.Fired[key] = true
	}
}

// State is the last saved state. It takes no lock: front ends render from
// it at any time.
func (sw *Switcher) State(context.Context) (domain.State, error) {
	st, err := sw.d.Store.LoadState()
	if err != nil {
		return st, fmt.Errorf("load state: %w", err)
	}
	return st, nil
}

// Settings are the saved settings, or the defaults.
func (sw *Switcher) Settings() (domain.Settings, error) {
	set, err := sw.d.Store.LoadSettings()
	if err != nil {
		return set, fmt.Errorf("load settings: %w", err)
	}
	return set, nil
}

// SetAuto turns the auto-switch on or off.
func (sw *Switcher) SetAuto(ctx context.Context, on bool) (domain.State, error) {
	return sw.run(ctx, func(s *session) error {
		s.set.Auto = on
		if err := sw.d.Store.SaveSettings(s.set); err != nil {
			return fmt.Errorf("save settings: %w", err)
		}
		return nil
	})
}
