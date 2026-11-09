// Command ccswitch is the command line: the core built by internal/wiring,
// driven by internal/cli. A console program, apart from the tray
// (cmd/ccswitch-app), so its output reaches the terminal.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/diegofal/ccswitch/internal/adapters/logfile"
	"github.com/diegofal/ccswitch/internal/adapters/notify"
	"github.com/diegofal/ccswitch/internal/cli"
	"github.com/diegofal/ccswitch/internal/wiring"
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

// wire builds the core (internal/wiring) and the CLI's own pieces.
func wire() (cli.Deps, *logfile.File, error) {
	desktop := notify.New()
	w, err := wiring.New(version, desktop)
	if err != nil {
		return cli.Deps{}, nil, err
	}
	return cli.Deps{Switcher: w.Switcher, Clock: wiring.Clock{}, Store: w.Store, Notify: desktop, Version: version}, w.Log, nil
}
