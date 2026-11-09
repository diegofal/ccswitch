package testkit

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// The fakes are safe for concurrent use (the core may poll accounts in
// parallel); tests read their exported fields once the call under test has
// returned.

var (
	_ ports.LoginStore = (*FakeLoginStore)(nil)
	_ ports.Vault      = (*FakeVault)(nil)
	_ ports.UsageAPI   = (*FakeUsageAPI)(nil)
	_ ports.Claude     = (*FakeClaude)(nil)
	_ ports.Releases   = (*FakeReleases)(nil)
	_ ports.Notifier   = (*FakeNotifier)(nil)
	_ ports.Clock      = (*FakeClock)(nil)
	_ ports.Lock       = (*FakeLock)(nil)
	_ ports.StateStore = (*FakeStateStore)(nil)
	_ ports.Logger     = (*FakeLogger)(nil)
)

// FakeClock is a clock that moves only when told.
type FakeClock struct {
	mu sync.Mutex
	T  time.Time
}

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.T
}

// Advance moves the clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.T = c.T.Add(d)
}

// FakeLoginStore is the live login. Write replaces Login, records it and
// moves Mod one second on, as a real write changes the file's mtime.
type FakeLoginStore struct {
	mu                        sync.Mutex
	Login                     domain.Login
	Mod                       time.Time
	ReadErr, WriteErr, ModErr error
	Writes                    []domain.Login
}

func (s *FakeLoginStore) Read() (domain.Login, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ReadErr != nil {
		return domain.Login{}, s.ReadErr
	}
	return s.Login, nil
}

func (s *FakeLoginStore) Write(l domain.Login) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.WriteErr != nil {
		return s.WriteErr
	}
	s.Login = l
	s.Writes = append(s.Writes, l)
	s.Mod = s.Mod.Add(time.Second)
	return nil
}

func (s *FakeLoginStore) ModTime() (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ModErr != nil {
		return time.Time{}, s.ModErr
	}
	return s.Mod, nil
}

// FakeVault holds accounts in memory; Load and Save copy the slice.
type FakeVault struct {
	mu               sync.Mutex
	Accounts         []domain.Account
	LoadErr, SaveErr error
	Saves            int
}

func (v *FakeVault) Load() ([]domain.Account, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.LoadErr != nil {
		return nil, v.LoadErr
	}
	return slices.Clone(v.Accounts), nil
}

func (v *FakeVault) Save(a []domain.Account) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.SaveErr != nil {
		return v.SaveErr
	}
	v.Accounts = slices.Clone(a)
	v.Saves++
	return nil
}

// FakeUsageAPI answers from scripts. Usages is keyed by access token; Errs by
// access token (Fetch) or refresh token (Refresh). Calls records
// "fetch <token>" and "refresh <token>" (fixture tokens are fake).
type FakeUsageAPI struct {
	mu            sync.Mutex
	Usages        map[string]domain.Usage
	Errs          map[string]error
	RefreshResult domain.Tokens
	Calls         []string
}

func (f *FakeUsageAPI) Fetch(ctx context.Context, accessToken string) (domain.Usage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "fetch "+accessToken)
	if err := ctx.Err(); err != nil {
		return domain.Usage{}, err
	}
	if err := f.Errs[accessToken]; err != nil {
		return domain.Usage{}, err
	}
	u, ok := f.Usages[accessToken]
	if !ok {
		return domain.Usage{}, fmt.Errorf("fake usage api: no usage scripted for %s", accessToken)
	}
	return u, nil
}

func (f *FakeUsageAPI) Refresh(ctx context.Context, refreshToken string) (domain.Tokens, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, "refresh "+refreshToken)
	if err := ctx.Err(); err != nil {
		return domain.Tokens{}, err
	}
	if err := f.Errs[refreshToken]; err != nil {
		return domain.Tokens{}, err
	}
	if f.RefreshResult.Access == "" {
		return domain.Tokens{}, errors.New("fake usage api: no refresh scripted")
	}
	return f.RefreshResult, nil
}

// FakeClaude is a scripted Claude Code. A successful Update sets VersionOut
// to UpdatedVersion when that is set. Auth is keyed by config dir ("" =
// default). OnLogin, when set, runs on Login (e.g. to fill a scratch dir).
// Calls records "version", "update", "auth status <dir>", "login <dir>".
type FakeClaude struct {
	mu             sync.Mutex
	VersionOut     string
	VersionErr     error
	UpdateOut      string
	UpdateErr      error
	UpdatedVersion string
	Auth           map[string]ports.AuthStatus
	AuthErr        error
	LoginErr       error
	OnLogin        func(configDir string) error
	Calls          []string
}

func (c *FakeClaude) record(call string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Calls = append(c.Calls, call)
}

func (c *FakeClaude) Version(context.Context) (string, error) {
	c.record("version")
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.VersionOut, c.VersionErr
}

func (c *FakeClaude) Update(context.Context) (string, error) {
	c.record("update")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.UpdateErr == nil && c.UpdatedVersion != "" {
		c.VersionOut = c.UpdatedVersion
	}
	return c.UpdateOut, c.UpdateErr
}

func (c *FakeClaude) AuthStatus(_ context.Context, configDir string) (ports.AuthStatus, error) {
	c.record("auth status " + configDir)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.AuthErr != nil {
		return ports.AuthStatus{}, c.AuthErr
	}
	return c.Auth[configDir], nil
}

func (c *FakeClaude) Login(_ context.Context, configDir string) error {
	c.record("login " + configDir)
	c.mu.Lock()
	onLogin, err := c.OnLogin, c.LoginErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if onLogin != nil {
		return onLogin(configDir)
	}
	return nil
}

// FakeReleases returns Version, or Err.
type FakeReleases struct {
	mu      sync.Mutex
	Version string
	Err     error
	Calls   int
}

func (r *FakeReleases) Latest(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Calls++
	return r.Version, r.Err
}

// FakeNotifier records "title: body" for every notification it posts.
type FakeNotifier struct {
	mu   sync.Mutex
	Sent []string
	Err  error
}

func (n *FakeNotifier) Notify(title, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Err != nil {
		return n.Err
	}
	n.Sent = append(n.Sent, title+": "+body)
	return nil
}

// FakeLock is a non-reentrant lock: acquiring it while held fails, which is
// how a test catches an operation that takes the lock twice.
type FakeLock struct {
	mu       sync.Mutex
	Holders  int
	Acquired int
	Err      error
}

func (l *FakeLock) Acquire(ctx context.Context) (func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.Err != nil {
		return nil, l.Err
	}
	if l.Holders > 0 {
		return nil, errors.New("fake lock: acquired while already held")
	}
	l.Holders++
	l.Acquired++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.Holders--
		})
	}, nil
}

// FakeStateStore keeps settings and state in memory. Settings nil means no
// settings file yet (LoadSettings returns the defaults). Load and Save copy,
// so a test sees only what was saved.
type FakeStateStore struct {
	mu                   sync.Mutex
	Settings             *domain.Settings
	State                domain.State
	LoadErr, SaveErr     error
	SettingsSaves, Saves int
}

func (s *FakeStateStore) LoadSettings() (domain.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LoadErr != nil {
		return domain.Settings{}, s.LoadErr
	}
	if s.Settings == nil {
		return domain.DefaultSettings(), nil
	}
	v := *s.Settings
	v.AlertThresholds = slices.Clone(v.AlertThresholds)
	return v, nil
}

func (s *FakeStateStore) SaveSettings(v domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	v.AlertThresholds = slices.Clone(v.AlertThresholds)
	s.Settings = &v
	s.SettingsSaves++
	return nil
}

func (s *FakeStateStore) LoadState() (domain.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.LoadErr != nil {
		return domain.State{}, s.LoadErr
	}
	return cloneState(s.State), nil
}

func (s *FakeStateStore) SaveState(v domain.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SaveErr != nil {
		return s.SaveErr
	}
	s.State = cloneState(v)
	s.Saves++
	return nil
}

func cloneState(s domain.State) domain.State {
	s.Accounts = slices.Clone(s.Accounts)
	for i := range s.Accounts {
		s.Accounts[i].Usage.Limits = slices.Clone(s.Accounts[i].Usage.Limits)
	}
	s.Fired = maps.Clone(s.Fired)
	if s.Undo != nil {
		u := *s.Undo
		s.Undo = &u
	}
	return s
}

// FakeLogger collects every line.
type FakeLogger struct {
	mu    sync.Mutex
	Lines []string
}

func (l *FakeLogger) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Lines = append(l.Lines, fmt.Sprintf(format, args...))
}
