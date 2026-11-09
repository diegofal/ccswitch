//go:build !windows

// Package dpapi is the vault on Windows; elsewhere New refuses.
package dpapi

import (
	"errors"

	"github.com/diegofal/ccswitch/internal/ports"
)

// New fails: DPAPI exists only on Windows.
func New(string) (ports.Vault, error) {
	return nil, errors.New("DPAPI vault: Windows only")
}
