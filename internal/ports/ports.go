// Package ports are the interfaces between the core and the outside world.
// Each has a fake in internal/testkit and a real adapter in internal/adapters.
package ports

import (
	"context"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

// LoginStore is Claude Code's live login: ~/.claude/.credentials.json and
// ~/.claude.json (the macOS Keychain item instead of the first, PLAN F1).
type LoginStore interface {
	Read() (domain.Login, error)
	Write(domain.Login) error    // patches the two files atomically, mode kept
	ModTime() (time.Time, error) // of the credentials file; tray polls this
}

// Vault holds every account's login, encrypted where the OS allows (PLAN D4).
type Vault interface {
	Load() ([]domain.Account, error)
	Save([]domain.Account) error
}

// UsageAPI is the usage endpoint and the token refresh (PLAN F3, F4).
type UsageAPI interface {
	// Fetch returns transport and HTTP errors as error; a 200 with a body
	// ParseUsage rejects is an error too.
	Fetch(ctx context.Context, accessToken string) (domain.Usage, error)
	Refresh(ctx context.Context, refreshToken string) (domain.Tokens, error)
}

// AuthStatus is `claude auth status --json`.
type AuthStatus struct {
	LoggedIn                                bool
	Email, OrgName, Subscription, ConfigDir string
}

// Claude is the installed Claude Code binary.
type Claude interface {
	Version(ctx context.Context) (string, error)
	Update(ctx context.Context) (string, error)                           // returns the command's output
	AuthStatus(ctx context.Context, configDir string) (AuthStatus, error) // configDir "" = default
	Login(ctx context.Context, configDir string) error                    // interactive `claude auth login`, stdio inherited
}

// Releases tells the latest published Claude Code version.
type Releases interface {
	Latest(ctx context.Context) (string, error)
}

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(title, body string) error
}

// Clock is the time; the core never calls time.Now.
type Clock interface {
	Now() time.Time
}

// Lock is the one-writer-at-a-time file lock shared by tray and CLI
// (PLAN D12).
type Lock interface {
	Acquire(ctx context.Context) (release func(), err error)
}

// StateStore is settings.json and state.json in the app data folder.
type StateStore interface {
	LoadSettings() (domain.Settings, error) // defaults when the file is absent
	SaveSettings(domain.Settings) error
	LoadState() (domain.State, error) // zero State when absent
	SaveState(domain.State) error     // atomic write
}

// Logger writes to ccswitch.log. Tokens never reach it (CLAUDE.md rule 2).
type Logger interface {
	Printf(format string, args ...any)
}
