// Command ccswitch-app is the tray app: the real adapters wired into the
// core and the tray shell. Every folder comes from internal/adapters/paths,
// so CCSWITCH_DATA_DIR, CCSWITCH_CLAUDE_DIR and CCSWITCH_CLAUDE_BIN redirect
// it to a test copy.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/claudecli"
	"github.com/diegofal/ccswitch/internal/adapters/execx"
	"github.com/diegofal/ccswitch/internal/adapters/lock"
	"github.com/diegofal/ccswitch/internal/adapters/logfile"
	"github.com/diegofal/ccswitch/internal/adapters/login"
	"github.com/diegofal/ccswitch/internal/adapters/notify"
	"github.com/diegofal/ccswitch/internal/adapters/npmreg"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/adapters/statefile"
	"github.com/diegofal/ccswitch/internal/adapters/usageapi"
	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/ports"
	"github.com/diegofal/ccswitch/internal/tray/shell"
)

// version is set at build time: -ldflags "-X main.version=…".
var version = "dev"

// instanceWait is how long a second copy waits for the first to let go of
// app.lock before giving up.
const instanceWait = time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx)
	stop()
	if err != nil {
		// The app has no console: the desktop notification is what the user
		// sees, stderr is for a terminal start.
		fmt.Fprintf(os.Stderr, "ccswitch-app: %v\n", err)
		_ = notify.New().Notify("ccswitch could not start", err.Error())
		os.Exit(1)
	}
}

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

func run(ctx context.Context) error {
	dataDir, err := paths.DataDir()
	if err != nil {
		return err
	}
	claudeDir, err := paths.ClaudeDir()
	if err != nil {
		return err
	}
	release, err := single(ctx, dataDir)
	if err != nil {
		return err
	}
	defer release()
	log := logfile.New(dataDir)
	v, err := vault.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open the vault: %w", err)
	}
	cliPath, err := beside("ccswitch")
	if err != nil {
		return err
	}
	liveLogin := login.Open(claudeDir, paths.ClaudeConfigFileBeside(claudeDir))
	notifier := shell.NewNotifier()
	sw := core.New(core.Deps{
		Login:    liveLogin,
		Vault:    v,
		Usage:    usageapi.New(version, clock{}),
		Claude:   claudecli.New(),
		Releases: npmreg.New("", version),
		Notify:   notifier,
		Clock:    clock{},
		Lock:     lock.New(filepath.Join(dataDir, "ccswitch.lock")),
		Store:    statefile.New(dataDir),
		Log:      log,
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
	log.Printf("tray %s started; data %s, claude %s", version, dataDir, claudeDir)
	err = shell.Run(ctx, shell.Options{
		Switcher:     sw,
		Notifier:     notifier,
		LoginModTime: liveLogin.ModTime,
		Start:        start,
		CLIPath:      cliPath,
		LogPath:      log.Path(),
		Logf:         log.Printf,
		Version:      version,
	})
	return errors.Join(err, log.Err())
}

// single holds app.lock while the tray runs, so a second copy exits instead
// of polling and switching beside the first.
func single(ctx context.Context, dataDir string) (func(), error) {
	ctx, cancel := context.WithTimeout(ctx, instanceWait)
	defer cancel()
	release, err := lock.New(filepath.Join(dataDir, "app.lock")).Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("ccswitch is already running (app.lock is held): %w", err)
	}
	return release, nil
}

// beside is the path of program name in the app's own folder.
func beside(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find the app's folder: %w", err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(exe), name), nil
}

// start runs a program without waiting for it: the terminal "Add account…"
// opens lives on its own.
func start(name string, args ...string) error {
	cmd := execx.Command(context.Background(), name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	go cmd.Wait() //nolint:errcheck // reaps the process; its outcome is the terminal's
	return nil
}
