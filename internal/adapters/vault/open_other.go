//go:build !windows && !darwin

package vault

import (
	"github.com/diegofal/ccswitch/internal/ports"
)

// Open returns the plain JSON vault (mode 0600) in dataDir, as Claude Code
// itself keeps its login on Linux.
func Open(dataDir string) (ports.Vault, error) {
	return NewFile(dataDir), nil
}
