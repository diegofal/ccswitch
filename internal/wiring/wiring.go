// Package wiring builds the core on the real adapters, once for both
// programs: the CLI (cmd/ccswitch) and the tray (cmd/ccswitch-app). Every
// folder comes from internal/adapters/paths, so CCSWITCH_DATA_DIR,
// CCSWITCH_CLAUDE_DIR and CCSWITCH_CLAUDE_BIN redirect it to a test copy.
// Nothing here reads a file or the network, so `ccswitch statusline` stays
// fast; the claude binary is looked up per call.
package wiring

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/claudecli"
	"github.com/diegofal/ccswitch/internal/adapters/lock"
	"github.com/diegofal/ccswitch/internal/adapters/logfile"
	"github.com/diegofal/ccswitch/internal/adapters/login"
	"github.com/diegofal/ccswitch/internal/adapters/npmreg"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/adapters/statefile"
	"github.com/diegofal/ccswitch/internal/adapters/usageapi"
	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Clock is the wall clock.
type Clock struct{}

// Now is time.Now.
func (Clock) Now() time.Time { return time.Now() }

// Real is the core on the real adapters, and the pieces of it a front end
// also uses on its own.
type Real struct {
	Switcher           *core.Switcher
	DataDir, ClaudeDir string
	Store              *statefile.Store
	Log                *logfile.File
	Live               ports.LoginStore // the live login, whose mtime the tray watches
}

// New wires the core for version, posting notifications through notifier:
// the CLI's desktop notifier or the tray's own.
func New(version string, notifier ports.Notifier) (Real, error) {
	dataDir, err := paths.DataDir()
	if err != nil {
		return Real{}, err
	}
	claudeDir, err := paths.ClaudeDir()
	if err != nil {
		return Real{}, err
	}
	v, err := vault.Open(dataDir)
	if err != nil {
		return Real{}, fmt.Errorf("open the vault: %w", err)
	}
	r := Real{
		DataDir:   dataDir,
		ClaudeDir: claudeDir,
		Store:     statefile.New(dataDir),
		Log:       logfile.New(dataDir),
		Live:      login.Open(claudeDir, paths.ClaudeConfigFileBeside(claudeDir)),
	}
	r.Switcher = core.New(core.Deps{
		Login:    r.Live,
		Vault:    v,
		Usage:    usageapi.New(version, Clock{}),
		Claude:   claudecli.New(),
		Releases: npmreg.New("", version),
		Notify:   notifier,
		Clock:    Clock{},
		Lock:     lock.New(filepath.Join(dataDir, "ccswitch.lock")),
		Store:    r.Store,
		Log:      r.Log,
		ScratchDir: func() (string, error) {
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				return "", err
			}
			return os.MkdirTemp(dataDir, "login-")
		},
		LoginStoreAt: func(dir string) ports.LoginStore {
			return login.Open(dir, paths.ClaudeConfigFile(dir))
		},
	})
	return r, nil
}
