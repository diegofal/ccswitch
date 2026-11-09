// Package claudecli is ports.Claude: the installed Claude Code binary, run
// unseen through execx. The binary is paths.ClaudeBin, looked up on every
// call so an install or a CCSWITCH_CLAUDE_BIN change is seen without a
// restart.
package claudecli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/diegofal/ccswitch/internal/adapters/execx"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/ports"
)

// CLI runs the claude program.
type CLI struct{}

var _ ports.Claude = (*CLI)(nil)

// New returns the Claude Code found by paths.ClaudeBin.
func New() *CLI { return &CLI{} }

// Version is the first line of `claude --version`, e.g. "2.1.300 (Claude Code)".
func (*CLI) Version(ctx context.Context) (string, error) {
	out, err := run(ctx, nil, "--version")
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(execx.Text(out), "\n")
	if line = strings.TrimSpace(line); line == "" {
		return "", errors.New("claude --version printed nothing")
	}
	return line, nil
}

// Update runs `claude update` and returns what it printed. A failed update
// is an error carrying that output.
func (*CLI) Update(ctx context.Context) (string, error) {
	out, err := run(ctx, nil, "update")
	return execx.Text(out), err
}

// authJSON is `claude auth status --json`.
type authJSON struct {
	LoggedIn         *bool  `json:"loggedIn"`
	Email            string `json:"email"`
	OrgName          string `json:"orgName"`
	SubscriptionType string `json:"subscriptionType"`
	ConfigDirectory  string `json:"configDirectory"`
}

// AuthStatus runs `claude auth status --json`, with CLAUDE_CONFIG_DIR set to
// configDir unless it is "".
func (*CLI) AuthStatus(ctx context.Context, configDir string) (ports.AuthStatus, error) {
	out, err := run(ctx, configEnv(configDir), "auth", "status", "--json")
	if err != nil {
		return ports.AuthStatus{}, err
	}
	var j authJSON
	if err := json.Unmarshal(out, &j); err != nil {
		return ports.AuthStatus{}, fmt.Errorf("claude auth status: unreadable output: %w", err)
	}
	if j.LoggedIn == nil {
		return ports.AuthStatus{}, errors.New("claude auth status: no loggedIn in its output")
	}
	return ports.AuthStatus{
		LoggedIn: *j.LoggedIn, Email: j.Email, OrgName: j.OrgName,
		Subscription: j.SubscriptionType, ConfigDir: j.ConfigDirectory,
	}, nil
}

// Login runs `claude auth login` in this terminal, with CLAUDE_CONFIG_DIR
// set to configDir unless it is "", and waits until the user is done.
func (*CLI) Login(ctx context.Context, configDir string) error {
	bin, err := paths.ClaudeBin()
	if err != nil {
		return err
	}
	return execx.RunInteractive(ctx, configEnv(configDir), bin, "auth", "login")
}

func configEnv(configDir string) []string {
	if configDir == "" {
		return nil
	}
	return []string{"CLAUDE_CONFIG_DIR=" + configDir}
}

func run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	bin, err := paths.ClaudeBin()
	if err != nil {
		return nil, err
	}
	out, err := execx.Run(ctx, "", env, bin, args...)
	if err != nil {
		return out, fmt.Errorf("claude %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
