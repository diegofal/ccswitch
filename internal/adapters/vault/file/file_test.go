package file_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/vault/file"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func accounts(t *testing.T) []domain.Account {
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

func TestEmptyWhenAbsent(t *testing.T) {
	got, err := file.New(t.TempDir()).Load()
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load %v, %v; want an empty list", got, err)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet")
	v := file.New(dir)
	want := accounts(t)
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
		fi, err := os.Stat(filepath.Join(dir, file.Name))
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
	v := file.NewSealed(dir, flip, flip)
	want := accounts(t)
	if err := v.Save(want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, file.Name))
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
	if _, err := file.New(dir).Load(); err == nil {
		t.Fatal("reading a sealed vault as plain JSON must fail")
	}
}

func TestSealErrors(t *testing.T) {
	dir := t.TempDir()
	boom := func([]byte) ([]byte, error) { return nil, errors.New("boom") }
	if err := file.NewSealed(dir, boom, boom).Save(accounts(t)); err == nil {
		t.Fatal("Save: want the seal error")
	}
	if err := file.New(dir).Save(accounts(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := file.NewSealed(dir, boom, boom).Load(); err == nil {
		t.Fatal("Load: want the unseal error")
	}
}

func TestBrokenFileNamesNoToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, file.Name), []byte(`[{"Login":{"Credentials":fake-refresh-x}}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := file.New(dir).Load()
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
	if err := file.New(filepath.Join(blocker, "sub")).Save(accounts(t)); err == nil {
		t.Fatal("Save: want an error when the folder cannot be made")
	}
	if err := os.Mkdir(filepath.Join(dir, file.Name), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := file.New(dir).Load(); err == nil {
		t.Fatal("Load: want an error when accounts.json is a folder")
	}
}
