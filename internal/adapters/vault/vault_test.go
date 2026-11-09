//go:build !darwin

package vault_test

import (
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// On macOS Open is the real keychain, which tests never touch
// (open_darwin_test.go checks the choice only).
func TestOpenRoundTrip(t *testing.T) {
	v, err := vault.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, err := domain.AccountFromLogin(testkit.FakeLogin("a@example.com"), testkit.T0)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Save([]domain.Account{a}); err != nil {
		t.Fatal(err)
	}
	got, err := v.Load()
	if err != nil || len(got) != 1 || got[0].Email != "a@example.com" {
		t.Fatalf("Load %v, %v", got, err)
	}
}
