// Command ccswitch-app is the tray: the core built by internal/wiring, drawn
// by internal/tray/shell. A separate program from the CLI (cmd/ccswitch)
// because on Windows it is built windowed (-H windowsgui), so starting it at
// login opens no console, and a windowed program cannot print to a terminal.
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

	"github.com/diegofal/ccswitch/internal/adapters/execx"
	"github.com/diegofal/ccswitch/internal/adapters/lock"
	"github.com/diegofal/ccswitch/internal/adapters/notify"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/tray/shell"
	"github.com/diegofal/ccswitch/internal/wiring"
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

func run(ctx context.Context) error {
	dataDir, err := paths.DataDir()
	if err != nil {
		return err
	}
	release, err := single(ctx, dataDir)
	if err != nil {
		return err
	}
	defer release()
	cliPath, err := beside("ccswitch")
	if err != nil {
		return err
	}
	notifier := shell.NewNotifier()
	w, err := wiring.New(version, notifier)
	if err != nil {
		return err
	}
	w.Log.Printf("tray %s started; data %s, claude %s", version, w.DataDir, w.ClaudeDir)
	err = shell.Run(ctx, shell.Options{
		Switcher:     w.Switcher,
		Notifier:     notifier,
		LoginModTime: w.Live.ModTime,
		Start:        start,
		CLIPath:      cliPath,
		LogPath:      w.Log.Path(),
		Logf:         w.Log.Printf,
		Version:      version,
	})
	return errors.Join(err, w.Log.Err())
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
