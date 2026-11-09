//go:build darwin

package vault_test

import (
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault"
)

// Only the choice: a Load or Save here would reach the real keychain.
func TestOpenIsTheKeychain(t *testing.T) {
	v, err := vault.Open(t.TempDir())
	if _, ok := v.(*vault.KeychainVault); err != nil || !ok {
		t.Fatalf("Open %T, %v; want *vault.KeychainVault", v, err)
	}
}
