// Package cli is the ccswitch command line: it parses the arguments, calls
// the core and prints the domain.State it returns. It holds no logic of its
// own (CLAUDE.md rule 6); every operation's rules live in internal/core.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/core"
	"github.com/diegofal/ccswitch/internal/domain"
)

// Exit codes.
const (
	exitOK    = 0
	exitError = 1 // the operation failed
	exitUsage = 2 // the command line is wrong
)

// Clock is the time; the CLI never calls time.Now, so tests pin it.
type Clock interface {
	Now() time.Time
}

// StateReader is the read side of the state store. `statusline` reads
// state.json through it directly: no Switcher, no lock, no network, so it
// answers in milliseconds even while the tray holds the lock.
type StateReader interface {
	LoadSettings() (domain.Settings, error)
	LoadState() (domain.State, error)
}

// Notifier posts a desktop notification. `watch` runs headless, with nobody
// looking at the terminal, so it says a failed round there too.
type Notifier interface {
	Notify(title, body string) error
}

// Deps are what the commands run on.
type Deps struct {
	Switcher *core.Switcher
	Clock    Clock
	Store    StateReader // for statusline only
	Notify   Notifier    // for watch only
	Version  string      // ccswitch's own, set at build time
	// After waits between watch rounds; nil means time.After.
	After func(time.Duration) <-chan time.Time
}

const usageText = `ccswitch holds several Claude Code logins, shows their limits and switches between them.

Usage: ccswitch <command>

  status                 accounts, limits and versions as last read
  check                  read every account's usage now, then show the status
  use <email>            make that account the live login
  undo                   switch back after the last switch, while that is offered
  add                    log in another account (opens the browser)
  remove <email>         forget an account (not the active one)
  pool <email> on|off    let auto-switch use the account, or not
  auto on|off            switch by itself when the active account reaches the threshold
  update                 install the latest Claude Code
  watch [--once]         poll on the settings' intervals until Ctrl-C
  statusline             one line for Claude Code's status line (reads the saved state only)
  version                ccswitch's version

<email> may be any unique beginning of the address.
`

type runner struct {
	ctx            context.Context
	d              Deps
	stdout, stderr *out
}

// Run runs one command; args exclude the program name. It returns the exit
// code: 0 ok, 1 the operation failed (or its output could not be written),
// 2 the command line is wrong.
func Run(ctx context.Context, args []string, deps Deps, stdout, stderr io.Writer) int {
	r := &runner{ctx: ctx, d: deps, stdout: &out{w: stdout}, stderr: &out{w: stderr}}
	code := r.run(args)
	if err := r.stdout.err; err != nil && code == exitOK {
		return r.fail(fmt.Errorf("write the output: %w", err))
	}
	return code
}

func (r *runner) run(args []string) int {
	if len(args) == 0 {
		return r.usage("no command")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		r.stdout.printf("%s", usageText)
		return exitOK
	case "version", "--version":
		return r.noArgs(cmd, rest, r.version)
	case "statusline":
		return r.noArgs(cmd, rest, r.statusLine)
	case "status":
		return r.noArgs(cmd, rest, r.status)
	case "watch":
		return r.watch(rest)
	}
	return r.operation(cmd, rest)
}

// operation runs the commands that change something through the Switcher.
func (r *runner) operation(cmd string, rest []string) int {
	sw := r.d.Switcher
	switch cmd {
	case "check":
		return r.noArgs(cmd, rest, func() int { return r.result(sw.Poll(r.ctx, true)) })
	case "update":
		return r.noArgs(cmd, rest, func() int { return r.result(sw.Update(r.ctx)) })
	case "undo":
		return r.noArgs(cmd, rest, func() int { return r.result(sw.Undo(r.ctx)) })
	case "add":
		return r.noArgs(cmd, rest, func() int { return r.result(sw.Add(r.ctx)) })
	case "use":
		return r.withEmail(cmd, rest, 1, func(email string) int { return r.result(sw.Switch(r.ctx, email)) })
	case "remove":
		return r.withEmail(cmd, rest, 1, func(email string) int { return r.result(sw.Remove(r.ctx, email)) })
	case "auto":
		if len(rest) != 1 {
			return r.usage("auto takes on or off")
		}
		on, ok := onOff(rest[0])
		if !ok {
			return r.usage("auto takes on or off, not " + rest[0])
		}
		return r.result(sw.SetAuto(r.ctx, on))
	case "pool":
		if len(rest) != 2 {
			return r.usage("pool takes an email and on or off")
		}
		in, ok := onOff(rest[1])
		if !ok {
			return r.usage("pool takes on or off, not " + rest[1])
		}
		return r.withEmail(cmd, rest, 2, func(email string) int { return r.result(sw.SetPool(r.ctx, email, in)) })
	}
	return r.usage("unknown command " + cmd)
}

func onOff(s string) (on, ok bool) {
	switch s {
	case "on":
		return true, true
	case "off":
		return false, true
	}
	return false, false
}

func (r *runner) usage(problem string) int {
	r.stderr.printf("ccswitch: %s\n\n%s", problem, usageText)
	return exitUsage
}

func (r *runner) noArgs(cmd string, rest []string, fn func() int) int {
	if len(rest) != 0 {
		return r.usage(cmd + " takes no arguments")
	}
	return fn()
}

func (r *runner) fail(err error) int {
	r.stderr.printf("ccswitch: %v\n", err)
	return exitError
}

// withEmail resolves rest[0] to one account's email and calls fn with it.
func (r *runner) withEmail(cmd string, rest []string, n int, fn func(string) int) int {
	if len(rest) != n {
		return r.usage(cmd + " takes an email")
	}
	st, err := r.d.Switcher.State(r.ctx)
	if err != nil {
		return r.fail(err)
	}
	email, err := match(st.Accounts, rest[0])
	if err != nil {
		return r.fail(err)
	}
	return fn(email)
}

// match finds the one account whose email is q or starts with q, ignoring
// case. The core takes exact emails; this is only typing comfort.
func match(accts []domain.AccountState, q string) (string, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if len(accts) == 0 {
		return "", errors.New("no accounts yet; `ccswitch check` captures the live login")
	}
	var all, hits []string
	for _, a := range accts {
		e := strings.ToLower(a.Email)
		if e == q {
			return a.Email, nil
		}
		all = append(all, a.Email)
		if q != "" && strings.HasPrefix(e, q) {
			hits = append(hits, a.Email)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", fmt.Errorf("no account matches %q; accounts: %s", q, strings.Join(all, ", "))
	}
	return "", fmt.Errorf("%q matches more than one account: %s", q, strings.Join(hits, ", "))
}

// result prints what an operation left and its error. Every Switcher
// operation has saved the state before returning, error or not; a state
// never saved (zero UpdatedAt) holds no facts to print.
func (r *runner) result(st domain.State, err error) int {
	if !st.UpdatedAt.IsZero() || err == nil {
		set, serr := r.d.Switcher.Settings()
		if serr != nil {
			err = errors.Join(err, serr)
			set = domain.DefaultSettings()
		}
		writeStatus(r.stdout, st, set, r.d.Clock.Now(), false)
	}
	if err != nil {
		return r.fail(err)
	}
	return exitOK
}

func (r *runner) version() int {
	v := r.d.Version
	if v == "" {
		v = "dev"
	}
	r.stdout.printf("ccswitch %s\n", v)
	return exitOK
}

func (r *runner) status() int {
	st, err := r.d.Switcher.State(r.ctx)
	if err != nil {
		return r.fail(err)
	}
	set, err := r.d.Switcher.Settings()
	if err != nil {
		return r.fail(err)
	}
	writeStatus(r.stdout, st, set, r.d.Clock.Now(), true)
	return exitOK
}

// statusLine prints core.StatusLine from the saved state. Claude Code shows
// whatever it prints, so a failure is printed there too, on stdout.
func (r *runner) statusLine() int {
	st, err := r.d.Store.LoadState()
	if err == nil {
		var set domain.Settings
		set, err = r.d.Store.LoadSettings()
		if err == nil {
			r.stdout.println(core.StatusLine(st, set, r.d.Clock.Now()))
			return exitOK
		}
	}
	r.stdout.printf("ccswitch: %v\n", err)
	return exitError
}
