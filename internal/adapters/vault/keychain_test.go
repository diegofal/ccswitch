package vault_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitError) ExitCode() int { return int(e) }

// fakeKeychain is a fake `security` holding one generic password.
type fakeKeychain struct {
	data   *string
	calls  [][]string
	setErr error
}

func (k *fakeKeychain) run(_ context.Context, args ...string) ([]byte, error) {
	k.calls = append(k.calls, args)
	switch args[0] {
	case "find-generic-password":
		if k.data == nil {
			return nil, exitError(44)
		}
		return []byte(*k.data + "\n"), nil
	case "add-generic-password":
		if k.setErr != nil {
			return nil, k.setErr
		}
		d := args[len(args)-1]
		k.data = &d
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected %v", args)
}

func keychainAccount(t *testing.T, email string) domain.Account {
	t.Helper()
	a, err := domain.AccountFromLogin(testkit.FakeLogin(email), testkit.T0)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestEmptyKeychainIsNoAccounts(t *testing.T) {
	k := &fakeKeychain{}
	got, err := vault.NewKeychain("/data", k.run).Load()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load %v, %v; want an empty list", got, err)
	}
}

func TestRoundTripInOneItem(t *testing.T) {
	k := &fakeKeychain{}
	v := vault.NewKeychain("/data", k.run)
	if err := v.Save([]domain.Account{keychainAccount(t, "a@example.com"), keychainAccount(t, "b@example.com")}); err != nil {
		t.Fatal(err)
	}
	got, err := v.Load()
	if err != nil || len(got) != 2 || got[1].Email != "b@example.com" {
		t.Fatalf("Load %v, %v", got, err)
	}
	for _, c := range k.calls {
		joined := strings.Join(c, " ")
		if !strings.Contains(joined, "-s ccswitch-accounts -a /data") {
			t.Fatalf("call %q is not on the item ccswitch-accounts / the data folder", c)
		}
	}
}

func TestSaveNilIsEmptyList(t *testing.T) {
	k := &fakeKeychain{}
	if err := vault.NewKeychain("/data", k.run).Save(nil); err != nil || k.data == nil || *k.data != "[]" {
		t.Fatalf("Save(nil): %v, stored %v", err, k.data)
	}
}

func TestKeychainFailures(t *testing.T) {
	broken := "{not json"
	if _, err := vault.NewKeychain("/data", (&fakeKeychain{data: &broken}).run).Load(); err == nil {
		t.Fatal("Load: want an error for a broken item")
	}
	k := &fakeKeychain{setErr: exitError(51)}
	err := vault.NewKeychain("/data", k.run).Save([]domain.Account{keychainAccount(t, "a@example.com")})
	if err == nil || strings.Contains(err.Error(), "fake-refresh") {
		t.Fatalf("Save: %v, want an error without the tokens", err)
	}
	failing := func(context.Context, ...string) ([]byte, error) { return nil, exitError(36) }
	if _, err := vault.NewKeychain("/data", failing).Load(); err == nil {
		t.Fatal("Load: want an error when security fails")
	}
}
