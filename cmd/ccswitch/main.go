// Command ccswitch is the command line: the real adapters wired into the
// core and the CLI. Every folder comes from internal/adapters/paths, so
// CCSWITCH_DATA_DIR, CCSWITCH_CLAUDE_DIR and CCSWITCH_CLAUDE_BIN redirect it
// to a test copy.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/claudecli"
	"github.com/diegofal/ccswitch/internal/adapters/lock"
	"github.com/diegofal/ccswitch/internal/adapters/logfile"
	"github.com/diegofal/ccswitch/internal/adapters/loginfile"
	"github.com/diegofal/ccswitch/internal/adapters/notify"
	"github.com/diegofal/ccswitch/internal/adapters/npmreg"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/adapters/statefile"
	"github.com/diegofal/ccswitch/internal/adapters/usageapi"
	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/cli"
	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/ports"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	deps, log, err := wire()
	if err != nil {
		fmt.Fprintf(stderr, "ccswitch: %v\n", err) //nolint:errcheck // stderr is the last resort
		return 1
	}
	code := cli.Run(ctx, args, deps, stdout, stderr)
	if err := log.Err(); err != nil {
		fmt.Fprintf(stderr, "ccswitch: write the log: %v\n", err) //nolint:errcheck // stderr is the last resort
	}
	return code
}

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

// wire builds the adapters. Nothing here reads a file or the network, so
// `statusline` stays fast; the claude binary is looked up per call.
func wire() (cli.Deps, *logfile.File, error) {
	dataDir, err := paths.DataDir()
	if err != nil {
		return cli.Deps{}, nil, err
	}
	claudeDir, err := paths.ClaudeDir()
	if err != nil {
		return cli.Deps{}, nil, err
	}
	v, err := vault.Open(dataDir)
	if err != nil {
		return cli.Deps{}, nil, fmt.Errorf("open the vault: %w", err)
	}
	store := statefile.New(dataDir)
	log := logfile.New(dataDir)
	desktop := notify.New()
	sw := core.New(core.Deps{
		Login:    loginfile.New(filepath.Join(claudeDir, ".credentials.json"), paths.ClaudeConfigFileBeside(claudeDir)),
		Vault:    v,
		Usage:    usageapi.New(version, clock{}),
		Claude:   claudecli.New(),
		Releases: npmreg.New("", version),
		Notify:   desktop,
		Clock:    clock{},
		Lock:     lock.New(filepath.Join(dataDir, "ccswitch.lock")),
		Store:    store,
		Log:      log,
		ScratchDir: func() (string, error) {
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				return "", err
			}
			return os.MkdirTemp(dataDir, "login-")
		},
		LoginStoreAt: func(dir string) ports.LoginStore {
			return loginfile.New(filepath.Join(dir, ".credentials.json"), paths.ClaudeConfigFile(dir))
		},
	})
	return cli.Deps{Switcher: sw, Clock: clock{}, Store: store, Notify: desktop, Version: version}, log, nil
}
