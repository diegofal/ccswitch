package vault_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func fileAccounts(t *testing.T) []domain.Account {
	t.Helper()
	var out []domain.Account
	for _, email := range []string{"a@example.com", "b@example.com"} {
		a, err := domain.AccountFromLogin(testkit.FakeLogin(email), testkit.T0)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	out[1].InPool = false
	return out
}

func TestFileEmptyWhenAbsent(t *testing.T) {
	got, err := vault.NewFile(t.TempDir()).Load()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load %v, %v; want an empty list", got, err)
	}
}

func TestFileRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet")
	v := vault.NewFile(dir)
	want := fileAccounts(t)
	if err := v.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := v.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\n%+v\nwant\n%+v", got, want)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, vault.Name))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v, %v; want 0600", fi.Mode().Perm(), err)
		}
	}
	if err := v.Save(nil); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Load(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("after saving none: %v, %v", got, err)
	}
}

func TestSealed(t *testing.T) {
	dir := t.TempDir()
	flip := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i, c := range b {
			out[i] = ^c
		}
		return out, nil
	}
	v := vault.NewSealedFile(dir, flip, flip)
	want := fileAccounts(t)
	if err := v.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, vault.Name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "fake-refresh") {
		t.Fatal("sealed vault holds a token in the clear")
	}
	got, err := v.Load()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("sealed round trip: %v", err)
	}
	if _, err := vault.NewFile(dir).Load(); err == nil {
		t.Fatal("reading a sealed vault as plain JSON must fail")
	}
}

func TestSealErrors(t *testing.T) {
	dir := t.TempDir()
	boom := func([]byte) ([]byte, error) { return nil, errors.New("boom") }
	if err := vault.NewSealedFile(dir, boom, boom).Save(fileAccounts(t)); err == nil {
		t.Fatal("Save: want the seal error")
	}
	if err := vault.NewFile(dir).Save(fileAccounts(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.NewSealedFile(dir, boom, boom).Load(); err == nil {
		t.Fatal("Load: want the unseal error")
	}
}

func TestBrokenFileNamesNoToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, vault.Name), []byte(`[{"Login":{"Credentials":fake-refresh-x}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := vault.NewFile(dir).Load()
	if err == nil || strings.Contains(err.Error(), "fake-refresh") {
		t.Fatalf("error %v: want one without the token", err)
	}
}

func TestUnusableFolder(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := vault.NewFile(filepath.Join(blocker, "sub")).Save(fileAccounts(t)); err == nil {
		t.Fatal("Save: want an error when the folder cannot be made")
	}
	if err := os.Mkdir(filepath.Join(dir, vault.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.NewFile(dir).Load(); err == nil {
		t.Fatal("Load: want an error when accounts.json is a folder")
	}
}
