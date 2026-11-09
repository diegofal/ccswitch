//go:build !windows

package vault

import (
	"github.com/diegofal/ccswitch/internal/adapters/vault/file"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the plain JSON vault (mode 0600) in dataDir.
func Open(dataDir string) (ports.Vault, error) {
	return file.New(dataDir), nil
}
