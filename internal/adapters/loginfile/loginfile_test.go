package loginfile_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/loginfile"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

// setup copies the fixtures into a temp folder laid out like ~/.claude and
// ~/.claude.json.
func setup(t *testing.T) (creds, config string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	creds = filepath.Join(dir, ".claude", ".credentials.json")
	config = filepath.Join(dir, ".claude.json")
	if err := os.WriteFile(creds, testkit.Fixture(t, "credentials.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, testkit.Fixture(t, "claude.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	return creds, config
}

func members(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRead(t *testing.T) {
	creds, config := setup(t)
	l, err := loginfile.New(creds, config).Read()
	if err != nil {
		t.Fatal(err)
	}
	email, err := l.Email()
	if err != nil || email != "old.user@example.com" {
		t.Fatalf("email %q, %v", email, err)
	}
}

func TestWritePatchesOnlyTheLoginMembers(t *testing.T) {
	creds, config := setup(t)
	oldCreds, oldConfig := read(t, creds), read(t, config)
	store := loginfile.New(creds, config)
	want := testkit.FakeLogin("new@example.com")

	if err := store.Write(want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !sameJSON(t, got.Credentials, want.Credentials) || !sameJSON(t, got.Profile, want.Profile) ||
		!sameJSON(t, got.OrganizationUUID, want.OrganizationUUID) {
		t.Fatal("the login read back is not the one written")
	}
	checkUntouched(t, oldCreds, read(t, creds), "claudeAiOauth", "organizationUuid")
	checkUntouched(t, oldConfig, read(t, config), "oauthAccount")
	if !bytes.Contains(read(t, creds), members(t, oldCreds)["mcpOAuth"]) {
		t.Fatal("mcpOAuth bytes changed")
	}
}

// sameJSON compares two JSON values ignoring layout: the patch indents what
// it inserts to match the file.
func sameJSON(t *testing.T, a, b []byte) bool {
	t.Helper()
	var ca, cb bytes.Buffer
	if err := json.Compact(&ca, a); err != nil {
		t.Fatal(err)
	}
	if err := json.Compact(&cb, b); err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}

func checkUntouched(t *testing.T, before, after []byte, written ...string) {
	t.Helper()
	b, a := members(t, before), members(t, after)
	for k, v := range b {
		if contains(written, k) {
			continue
		}
		if !bytes.Equal(a[k], v) {
			t.Errorf("member %q changed:\n%s\n→\n%s", k, v, a[k])
		}
	}
	if len(a) < len(b) {
		t.Errorf("members lost: %d → %d", len(b), len(a))
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestWriteKeepsModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits")
	}
	creds, config := setup(t)
	if err := os.Chmod(config, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := loginfile.New(creds, config).Write(testkit.FakeLogin("new@example.com")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{creds: 0o600, config: 0o640} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", filepath.Base(path), fi.Mode().Perm(), want)
		}
	}
}

func TestRejectedLoginLeavesBothFiles(t *testing.T) {
	creds, config := setup(t)
	oldCreds, oldConfig := read(t, creds), read(t, config)
	bad := testkit.FakeLogin("new@example.com")
	bad.Profile = nil // the credentials patch succeeds, the config one fails
	if err := loginfile.New(creds, config).Write(bad); err == nil {
		t.Fatal("want an error for a login without a profile")
	}
	bad = domain.Login{}
	if err := loginfile.New(creds, config).Write(bad); err == nil {
		t.Fatal("want an error for an empty login")
	}
	if !bytes.Equal(read(t, creds), oldCreds) || !bytes.Equal(read(t, config), oldConfig) {
		t.Fatal("a rejected write changed the files")
	}
}

func TestMissingFiles(t *testing.T) {
	creds, config := setup(t)
	missing := filepath.Join(t.TempDir(), "absent.json")
	cases := map[string]*loginfile.Store{
		"credentials": loginfile.New(missing, config),
		"config":      loginfile.New(creds, missing),
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := store.Read()
			if err == nil || !strings.Contains(err.Error(), "absent.json") {
				t.Fatalf("Read: %v, want an error naming the file", err)
			}
			err = store.Write(testkit.FakeLogin("new@example.com"))
			if err == nil || !strings.Contains(err.Error(), "absent.json") {
				t.Fatalf("Write: %v, want an error naming the file", err)
			}
		})
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("Write created a missing file")
	}
	if _, err := loginfile.New(missing, config).ModTime(); err == nil {
		t.Fatal("ModTime: want an error for a missing file")
	}
}

func TestReadRejectsBrokenFile(t *testing.T) {
	creds, config := setup(t)
	if err := os.WriteFile(creds, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loginfile.New(creds, config).Read(); err == nil {
		t.Fatal("want an error for a broken credentials file")
	}
}

func TestModTime(t *testing.T) {
	creds, config := setup(t)
	when := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(creds, when, when); err != nil {
		t.Fatal(err)
	}
	got, err := loginfile.New(creds, config).ModTime()
	if err != nil || !got.Equal(when) {
		t.Fatalf("ModTime %v, %v; want %v", got, err, when)
	}
}

func TestConfigFilePatchWritesOnlyWhenCalled(t *testing.T) {
	_, config := setup(t)
	old := read(t, config)
	c := loginfile.Config(config)
	got, err := c.Read()
	if err != nil || !bytes.Equal(got, old) {
		t.Fatalf("Read: %v", err)
	}
	write, err := c.Patch(testkit.FakeLogin("new@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(t, config), old) {
		t.Fatal("Patch wrote before its write was called")
	}
	if err := write(); err != nil {
		t.Fatal(err)
	}
	checkUntouched(t, old, read(t, config), "oauthAccount")
	if !strings.Contains(string(read(t, config)), "new@example.com") {
		t.Fatal("the profile was not written")
	}
	if _, err := loginfile.Config(filepath.Join(t.TempDir(), "absent.json")).Patch(testkit.FakeLogin("x@example.com")); err == nil {
		t.Fatal("Patch: want an error for a missing file")
	}
}
