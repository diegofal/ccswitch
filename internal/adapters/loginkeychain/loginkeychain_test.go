package loginkeychain_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/keychainitem"
	"github.com/diegofal/ccswitch/internal/adapters/loginkeychain"
	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/testkit"
)

type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitError) ExitCode() int { return int(e) }

// keychain is a fake `security` holding generic passwords in memory.
type keychain struct {
	items   map[string]string // service + "/" + account → data
	setErr  error
	written int
}

func flag(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func (k *keychain) run(_ context.Context, args ...string) ([]byte, error) {
	key := flag(args, "-s") + "/" + flag(args, "-a")
	switch args[0] {
	case "find-generic-password":
		data, ok := k.items[key]
		if !ok {
			return nil, exitError(44)
		}
		if args[len(args)-1] == "-w" {
			return []byte(data + "\n"), nil
		}
		return []byte(`    "mdat"<timedate>=0x32303236313130393132333435365A00  "20261109123456Z\000"` + "\n"), nil
	case "add-generic-password":
		if k.setErr != nil {
			return nil, k.setErr
		}
		k.items[key] = flag(args, "-w")
		k.written++
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected %v", args)
}

const user = "someone"

func setup(t *testing.T) (*keychain, string) {
	t.Helper()
	config := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(config, testkit.Fixture(t, "claude.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := &keychain{items: map[string]string{
		loginkeychain.Service + "/" + user: string(testkit.Fixture(t, "credentials.json")),
	}}
	return k, config
}

func store(k *keychain, config string) *loginkeychain.Store {
	return loginkeychain.New(loginkeychain.Service, user, config, k.run)
}

func TestRead(t *testing.T) {
	k, config := setup(t)
	l, err := store(k, config).Read()
	if err != nil {
		t.Fatal(err)
	}
	if email, _ := l.Email(); email != "old.user@example.com" {
		t.Fatalf("email %q", email)
	}
}

func members(t *testing.T, b []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWritePatchesTheItemAndTheConfig(t *testing.T) {
	k, config := setup(t)
	key := loginkeychain.Service + "/" + user
	oldCreds := k.items[key]
	s := store(k, config)

	if err := s.Write(testkit.FakeLogin("new@example.com")); err != nil {
		t.Fatal(err)
	}

	l, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if email, _ := l.Email(); email != "new@example.com" {
		t.Fatalf("email after write %q", email)
	}
	if !strings.Contains(k.items[key], "fake-access-new@example.com") {
		t.Fatal("the keychain item does not hold the new token")
	}
	if !bytes.Equal(members(t, []byte(k.items[key]))["mcpOAuth"], members(t, []byte(oldCreds))["mcpOAuth"]) {
		t.Fatal("mcpOAuth changed")
	}
}

func TestRejectedLoginWritesNothing(t *testing.T) {
	k, config := setup(t)
	oldConfig, _ := os.ReadFile(config)
	bad := testkit.FakeLogin("new@example.com")
	bad.Profile = nil // the credentials patch succeeds, the config one fails
	if err := store(k, config).Write(bad); err == nil {
		t.Fatal("want an error for a login without a profile")
	}
	if err := store(k, config).Write(domain.Login{}); err == nil {
		t.Fatal("want an error for an empty login")
	}
	newConfig, _ := os.ReadFile(config)
	if k.written != 0 || !bytes.Equal(oldConfig, newConfig) {
		t.Fatal("a rejected write changed the keychain or the config")
	}
}

func TestFailedKeychainWriteLeavesTheConfig(t *testing.T) {
	k, config := setup(t)
	k.setErr = exitError(1)
	oldConfig, _ := os.ReadFile(config)
	err := store(k, config).Write(testkit.FakeLogin("new@example.com"))
	if err == nil || strings.Contains(err.Error(), "fake-access") {
		t.Fatalf("err %v: want an error without the token", err)
	}
	newConfig, _ := os.ReadFile(config)
	if !bytes.Equal(oldConfig, newConfig) {
		t.Fatal("the config changed although the keychain write failed")
	}
}

func TestNotLoggedIn(t *testing.T) {
	k, config := setup(t)
	k.items = map[string]string{}
	s := store(k, config)
	if _, err := s.Read(); !errors.Is(err, keychainitem.ErrNotFound) || !strings.Contains(err.Error(), loginkeychain.Service) {
		t.Fatalf("Read: %v, want ErrNotFound naming the item", err)
	}
	if err := s.Write(testkit.FakeLogin("new@example.com")); !errors.Is(err, keychainitem.ErrNotFound) {
		t.Fatalf("Write: %v, want ErrNotFound", err)
	}
	if _, err := s.ModTime(); !errors.Is(err, keychainitem.ErrNotFound) {
		t.Fatalf("ModTime: %v, want ErrNotFound", err)
	}
}

func TestMissingConfig(t *testing.T) {
	k, _ := setup(t)
	s := store(k, filepath.Join(t.TempDir(), "absent.json"))
	if _, err := s.Read(); err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Fatalf("Read: %v, want an error naming the file", err)
	}
	if err := s.Write(testkit.FakeLogin("new@example.com")); err == nil || k.written != 0 {
		t.Fatalf("Write: %v, written %d; want an error and no keychain write", err, k.written)
	}
}

func TestModTime(t *testing.T) {
	k, config := setup(t)
	got, err := store(k, config).ModTime()
	if want := time.Date(2026, 11, 9, 12, 34, 56, 0, time.UTC); err != nil || !got.Equal(want) {
		t.Fatalf("ModTime %v, %v; want %v", got, err, want)
	}
}

func TestServiceFor(t *testing.T) {
	home := filepath.Join("Users", "someone", ".claude")
	if got := loginkeychain.ServiceFor(home, home); got != "Claude Code-credentials" {
		t.Fatalf("default folder: %q", got)
	}
	// Claude Code with CLAUDE_CONFIG_DIR=dir names the item with the first
	// 8 hex digits of sha256(dir).
	if got := loginkeychain.ServiceFor("/tmp/scratch", home); got != "Claude Code-credentials-e549f2e8" {
		t.Fatalf("scratch folder: %q", got)
	}
}

func TestCurrentUser(t *testing.T) {
	t.Setenv("USER", "from-env")
	if got := loginkeychain.CurrentUser(); got != "from-env" {
		t.Fatalf("CurrentUser %q", got)
	}
	t.Setenv("USER", "")
	if got := loginkeychain.CurrentUser(); got == "" {
		t.Fatal("CurrentUser empty without USER")
	}
}
