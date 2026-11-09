//go:build !windows

package vault

import (
	"errors"

	"github.com/diegofal/ccswitch/internal/ports"
)

// NewDPAPI fails: DPAPI exists only on Windows.
func NewDPAPI(string) (ports.Vault, error) {
	return nil, errors.New("DPAPI vault: Windows only")
}
