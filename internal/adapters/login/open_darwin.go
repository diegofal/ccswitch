//go:build darwin

package login

import (
	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
	"github.com/diegofal/ccswitch/internal/adapters/loginkeychain"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the login of the Claude Code folder claudeDir: the keychain
// item Claude Code uses for that folder, and the config file at configPath.
// claudeDir must be the exact string given to Claude Code as
// CLAUDE_CONFIG_DIR (the item's name hashes it).
func Open(claudeDir, configPath string) ports.LoginStore {
	// When home is unknown it is "", so no folder is taken for ~/.claude
	// and none reaches the real item.
	home, _ := paths.HomeClaudeDir()
	return loginkeychain.New(loginkeychain.ServiceFor(claudeDir, home), loginkeychain.CurrentUser(), configPath, keychainitem.Security)
}
