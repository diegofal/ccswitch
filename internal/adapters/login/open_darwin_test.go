//go:build darwin

package login_test

import (
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/login"
)

// Only the choice: a Read here would reach the real keychain.
func TestOpenIsTheKeychain(t *testing.T) {
	if s, ok := login.Open(t.TempDir(), "x.json").(*login.KeychainStore); !ok {
		t.Fatalf("Open is %T, want *login.KeychainStore", s)
	}
}
