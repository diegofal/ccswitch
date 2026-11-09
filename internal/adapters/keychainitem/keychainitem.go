// Package keychainitem is one generic password in the macOS login keychain,
// read and written through the `security` program, so ccswitch needs no cgo.
// The program is an injected Runner: tests fake its output on any OS.
package keychainitem

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/execx"
)

// ErrNotFound is returned, wrapped, when the keychain has no such item.
var ErrNotFound = errors.New("not found in the keychain")

// notFoundExit is security's exit status for errSecItemNotFound.
const notFoundExit = 44

// timeout bounds one `security` call: it never prompts for an item that
// `security` itself created, so a slow call is a stuck one.
const timeout = 30 * time.Second

// Runner runs `security` with args and returns its standard output. A
// failure's error has an ExitCode() int in its chain, as *exec.ExitError does.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Security runs the real /usr/bin/security.
var Security = Command("security")

// Command is the Runner for the program name, started through execx. Only
// standard output is returned; standard error goes into the error.
func Command(name string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		out, err := execx.Command(ctx, name, args...).Output()
		if err != nil {
			var stderr []byte
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				stderr = ee.Stderr
			}
			// args[0] is the subcommand; later args may hold the secret.
			return nil, fmt.Errorf("%s %s: %w: %s", name, args[0], err, execx.Text(stderr))
		}
		return out, nil
	}
}

// Item is the generic password with this service and account.
type Item struct {
	Service, Account string
	Run              Runner
}

// run calls `security <command...> -s <service> -a <account> <after...>`.
func (i Item) run(command []string, after ...string) ([]byte, error) {
	if i.Account == "" {
		// `-a ""` would match the service's item of any account.
		return nil, fmt.Errorf("keychain item %q: no account (user name unknown)", i.Service)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := append(append(command, "-s", i.Service, "-a", i.Account), after...)
	out, err := i.Run(ctx, args...)
	if err == nil {
		return out, nil
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) && coded.ExitCode() == notFoundExit {
		return nil, fmt.Errorf("keychain item %q (account %q): %w: %w", i.Service, i.Account, ErrNotFound, err)
	}
	return nil, fmt.Errorf("keychain item %q (account %q): %w", i.Service, i.Account, err)
}

// Get returns the item's data.
func (i Item) Get() ([]byte, error) {
	out, err := i.run([]string{"find-generic-password"}, "-w")
	if err != nil {
		return nil, err
	}
	return decode(out), nil
}

// Set creates the item or replaces its data. `security` takes the data as
// an argument, so `ps` can show it for the moment the command runs; Claude
// Code's own writes of its credentials do the same.
func (i Item) Set(data []byte) error {
	_, err := i.run([]string{"add-generic-password", "-U"}, "-w", string(data))
	return err
}

// ModTime is the item's modification date ("mdat").
func (i Item) ModTime() (time.Time, error) {
	out, err := i.run([]string{"find-generic-password"})
	if err != nil {
		return time.Time{}, err
	}
	t, err := mdat(out)
	if err != nil {
		return time.Time{}, fmt.Errorf("keychain item %q (account %q): %w", i.Service, i.Account, err)
	}
	return t, nil
}

// decode undoes `security -w`: it prints the data and a newline, or the
// data as hex when it has bytes it does not print as text (non-ASCII). JSON
// starts with { or [, never a hex digit, so the two cannot be confused.
func decode(out []byte) []byte {
	out = bytes.TrimSuffix(out, []byte("\n"))
	if b, err := hex.DecodeString(string(out)); err == nil && len(out) > 0 {
		return b
	}
	return out
}

// mdat reads the modification date out of the attribute listing, from the
// line `"mdat"<timedate>=0x3230…5A00  "20261109123456Z\000"`.
func mdat(out []byte) (time.Time, error) {
	const key = `"mdat"<timedate>=0x`
	for _, line := range strings.Split(string(out), "\n") {
		_, rest, ok := strings.Cut(line, key)
		if !ok {
			continue
		}
		hexDate, _, _ := strings.Cut(rest, " ")
		b, err := hex.DecodeString(hexDate)
		if err != nil {
			return time.Time{}, fmt.Errorf("modification date: %w", err)
		}
		t, err := time.Parse("20060102150405Z", strings.TrimRight(string(b), "\x00"))
		if err != nil {
			return time.Time{}, fmt.Errorf("modification date: %w", err)
		}
		return t, nil
	}
	return time.Time{}, errors.New("no modification date in the attributes")
}
