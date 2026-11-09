// Claude Code's live login on macOS (KeychainStore): the credentials JSON is
// a generic password in the login keychain, the profile is the same
// .claude.json as on the other systems (ConfigFile). It builds on every OS;
// `security` is injected. Untested on a real Mac (PLAN V5).

package login

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Service is the keychain item Claude Code keeps its credentials in when
// CLAUDE_CONFIG_DIR is unset.
const Service = "Claude Code-credentials"

var _ ports.LoginStore = (*KeychainStore)(nil)

// KeychainStore is the keychain item plus the config file.
type KeychainStore struct {
	item   keychainitem.Item
	config ConfigFile
}

// NewKeychain returns the store over the keychain item (service, account) and
// the config file at configPath, calling `security` through run.
func NewKeychain(service, account, configPath string, run keychainitem.Runner) *KeychainStore {
	return &KeychainStore{
		item:   keychainitem.Item{Service: service, Account: account, Run: run},
		config: Config(configPath),
	}
}

// ServiceFor is the item name Claude Code uses for the folder claudeDir:
// Service for its default folder homeClaudeDir (~/.claude), else Service
// plus "-" and the first 8 hex digits of sha256(claudeDir), as it does when
// CLAUDE_CONFIG_DIR=claudeDir (the scratch folder of an add). A test copy
// under CCSWITCH_CLAUDE_DIR therefore never reaches the real item.
func ServiceFor(claudeDir, homeClaudeDir string) string {
	if claudeDir == homeClaudeDir {
		return Service
	}
	sum := sha256.Sum256([]byte(claudeDir))
	return Service + "-" + hex.EncodeToString(sum[:])[:8]
}

// CurrentUser is the item's account as Claude Code sets it: $USER, else the
// OS user name; "" when neither is known (every call then fails).
func CurrentUser() string {
	if name := os.Getenv("USER"); name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// Read returns the login held in the keychain item and the config file.
func (s *KeychainStore) Read() (domain.Login, error) {
	creds, err := s.item.Get()
	if err != nil {
		return domain.Login{}, err
	}
	config, err := s.config.Read()
	if err != nil {
		return domain.Login{}, err
	}
	return domain.LoginFromFiles(creds, config)
}

// Write patches l into the item and the file. Both patches are made before
// either is replaced, and the item goes first: a failed keychain write leaves
// both as they were.
func (s *KeychainStore) Write(l domain.Login) error {
	creds, err := s.item.Get()
	if err != nil {
		return err
	}
	newCreds, err := domain.PatchCredentialsFile(creds, l)
	if err != nil {
		return err
	}
	writeConfig, err := s.config.Patch(l)
	if err != nil {
		return err
	}
	if err := s.item.Set(newCreds); err != nil {
		return err
	}
	return writeConfig()
}

// ModTime is the keychain item's modification date.
func (s *KeychainStore) ModTime() (time.Time, error) { return s.item.ModTime() }
