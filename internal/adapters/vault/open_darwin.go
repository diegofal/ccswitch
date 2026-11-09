//go:build darwin

package vault

import (
	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the vault in the login keychain, one item per data folder.
func Open(dataDir string) (ports.Vault, error) {
	return NewKeychain(dataDir, keychainitem.Security), nil
}
