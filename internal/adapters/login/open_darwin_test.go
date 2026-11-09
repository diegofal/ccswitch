//go:build darwin

package login_test

import (
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/login"
	"github.com/diegofal/ccswitch/internal/adapters/loginkeychain"
)

// Only the choice: a Read here would reach the real keychain.
func TestOpenIsTheKeychain(t *testing.T) {
	if s, ok := login.Open(t.TempDir(), "x.json").(*loginkeychain.Store); !ok {
		t.Fatalf("Open is %T, want *loginkeychain.Store", s)
	}
}
