//go:build !darwin

package login_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/login"
	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/testkit"
)

func TestOpenReadsTheFolder(t *testing.T) {
	dir := t.TempDir()
	config := paths.ClaudeConfigFile(dir)
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), testkit.Fixture(t, "credentials.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, testkit.Fixture(t, "claude.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := login.Open(dir, config).Read()
	if err != nil {
		t.Fatal(err)
	}
	if email, _ := l.Email(); email != "old.user@example.com" {
		t.Fatalf("email %q", email)
	}
}
