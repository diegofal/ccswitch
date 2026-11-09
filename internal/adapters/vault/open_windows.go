//go:build windows

package vault

import (
	"github.com/diegofal/ccswitch/internal/adapters/vault/dpapi"
	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the DPAPI-sealed vault in dataDir.
func Open(dataDir string) (ports.Vault, error) {
	return dpapi.New(dataDir)
}
