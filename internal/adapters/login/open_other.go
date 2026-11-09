//go:build !darwin

package login

import (
	"path/filepath"

	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the login of the Claude Code folder claudeDir: its
// .credentials.json and the config file at configPath.
func Open(claudeDir, configPath string) ports.LoginStore {
	return NewFile(filepath.Join(claudeDir, ".credentials.json"), configPath)
}
