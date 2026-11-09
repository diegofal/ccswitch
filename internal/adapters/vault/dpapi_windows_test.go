//go:build windows

package vault_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestRoundTripSealed(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.NewDPAPI(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := domain.AccountFromLogin(testkit.FakeLogin("a@example.com"), testkit.T0)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Account{a}
	if err := v.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, vault.Name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fake-refresh") || strings.Contains(string(raw), "a@example.com") {
		t.Fatal("the vault file holds account data in the clear")
	}
	got, err := v.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: %v", err)
	}
}

func TestUnsealRefusesForeignBytes(t *testing.T) {
	dir := t.TempDir()
	v, err := vault.NewDPAPI(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"", `[]`} {
		if err := os.WriteFile(filepath.Join(dir, vault.Name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Load(); err == nil {
			t.Fatalf("Load of %q: want an error", content)
		}
	}
}
