// Package paths is the one place that knows where things live: the app data
// folder, Claude Code's folder and config file, and the claude binary. It is
// the only package that reads HOME, USERPROFILE, APPDATA or XDG_*
// (internal/arch_test.go), so the CCSWITCH_* overrides redirect every other
// package and no test can reach the real ~/.claude or vault.
package paths

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// The overrides every binary honours (CLAUDE.md rule 1).
const (
	EnvDataDir   = "CCSWITCH_DATA_DIR"
	EnvClaudeDir = "CCSWITCH_CLAUDE_DIR"
	EnvClaudeBin = "CCSWITCH_CLAUDE_BIN"
)

const app = "ccswitch"

// env is what the lookups read; tests replace it to try every OS.
type env struct {
	goos   string
	getenv func(string) string
	home   func() (string, error)
}

func osEnv() env { return env{goos: runtime.GOOS, getenv: os.Getenv, home: os.UserHomeDir} }

// DataDir is ccswitch's own folder: CCSWITCH_DATA_DIR when set, else
// %APPDATA%\ccswitch, ~/Library/Application Support/ccswitch, or
// $XDG_DATA_HOME/ccswitch (~/.local/share/ccswitch). It is not created here.
func DataDir() (string, error) { return osEnv().dataDir() }

// ClaudeDir is Claude Code's folder: CCSWITCH_CLAUDE_DIR when set, else ~/.claude.
func ClaudeDir() (string, error) { return osEnv().claudeDir() }

// HomeClaudeDir is ~/.claude, ignoring CCSWITCH_CLAUDE_DIR: the folder
// Claude Code uses when CLAUDE_CONFIG_DIR is unset. On macOS it decides the
// keychain item's name (login.ServiceFor).
func HomeClaudeDir() (string, error) { return osEnv().homeClaudeDir() }

// ClaudeConfigFile is the .claude.json inside dir: where Claude Code keeps
// its config when CLAUDE_CONFIG_DIR=dir (the scratch folder of an add).
func ClaudeConfigFile(dir string) string { return filepath.Join(dir, ".claude.json") }

// ClaudeConfigFileBeside is the .claude.json next to claudeDir: ~/.claude.json
// for ~/.claude, where Claude Code keeps it when CLAUDE_CONFIG_DIR is unset.
func ClaudeConfigFileBeside(claudeDir string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(claudeDir)), ".claude.json")
}

// ClaudeBin is the claude program: CCSWITCH_CLAUDE_BIN when set, else
// "claude" found on PATH.
func ClaudeBin() (string, error) {
	if bin := os.Getenv(EnvClaudeBin); bin != "" {
		return bin, nil
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude not found on PATH (install Claude Code or set %s): %w", EnvClaudeBin, err)
	}
	return bin, nil
}

func (e env) dataDir() (string, error) {
	if dir := e.getenv(EnvDataDir); dir != "" {
		return dir, nil
	}
	switch e.goos {
	case "windows":
		if dir := e.getenv("APPDATA"); dir != "" {
			return filepath.Join(dir, app), nil
		}
		return "", errors.New("APPDATA is not set; set " + EnvDataDir)
	case "darwin":
		home, err := e.homeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", app), nil
	}
	if dir := e.getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, app), nil
	}
	home, err := e.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", app), nil
}

func (e env) claudeDir() (string, error) {
	if dir := e.getenv(EnvClaudeDir); dir != "" {
		return dir, nil
	}
	return e.homeClaudeDir()
}

func (e env) homeClaudeDir() (string, error) {
	home, err := e.homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

func (e env) homeDir() (string, error) {
	home, err := e.home()
	if err != nil {
		return "", fmt.Errorf("home folder: %w", err)
	}
	return home, nil
}
