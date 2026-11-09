// Package notify is ports.Notifier for the CLI and headless runs (`ccswitch
// watch`): a Windows toast through the WinRT API (go-toast, no PowerShell),
// `osascript` on macOS, `notify-send` on Linux. The tray posts through the
// Wails notifications service instead. Log is the notifier that only writes
// to ccswitch.log, for tests and machines without a desktop.
package notify

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/execx"
	"github.com/diegofal/ccswitch/internal/ports"
)

// AppID is the name Windows shows on ccswitch's toasts.
const AppID = "ccswitch"

// timeout bounds how long a notification command may take.
const timeout = 10 * time.Second

// Desktop shows notifications with the OS's own mechanism.
type Desktop struct {
	goos  string
	run   func(ctx context.Context, name string, args ...string) error
	toast func(title, body string) error
}

var _ ports.Notifier = (*Desktop)(nil)

// New returns the desktop notifier for this OS.
func New() *Desktop {
	return &Desktop{goos: runtime.GOOS, run: runHidden, toast: toast}
}

func runHidden(ctx context.Context, name string, args ...string) error {
	_, err := execx.Run(ctx, "", nil, name, args...)
	return err
}

// Notify shows title and body.
func (d *Desktop) Notify(title, body string) error {
	if d.goos == "windows" {
		if err := d.toast(title, body); err != nil {
			return fmt.Errorf("notify %q: %w", title, err)
		}
		return nil
	}
	name, args, err := command(d.goos, title, body)
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := d.run(ctx, name, args...); err != nil {
		return fmt.Errorf("notify %q: %w", title, err)
	}
	return nil
}

// command builds the command that shows a notification on goos. The texts
// go as arguments, never inside a script, so no quoting can break them.
func command(goos, title, body string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "osascript", []string{
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run",
			title, body,
		}, nil
	case "linux":
		return "notify-send", []string{"--app-name=" + AppID, "--", title, body}, nil
	}
	return "", nil, fmt.Errorf("no desktop notifications on %s", goos)
}

// Log is the notifier that writes each notification to a Logger.
type Log struct{ l ports.Logger }

var _ ports.Notifier = (*Log)(nil)

// NewLog returns the notifier writing to l.
func NewLog(l ports.Logger) *Log { return &Log{l: l} }

// Notify logs title and body.
func (n *Log) Notify(title, body string) error {
	n.l.Printf("notification: %s · %s", title, body)
	return nil
}
