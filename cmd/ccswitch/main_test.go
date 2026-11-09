package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/adapters/statefile"
	"github.com/diegofal/ccswitch/internal/domain"
)

// statusline runs from Claude Code on every prompt: through the real
// wiring and the real state file it must answer in under 50 ms, which it
// can only do without the lock, the vault and the network.
func TestStatusLineThroughTheRealWiringIsFast(t *testing.T) {
	data := t.TempDir()
	t.Setenv(paths.EnvDataDir, data)
	t.Setenv(paths.EnvClaudeDir, t.TempDir())
	now := time.Now()
	st := domain.State{
		Active: "ana@example.com",
		Accounts: []domain.AccountState{{Email: "ana@example.com", InPool: true, Usage: domain.Usage{
			ReadAt: now, Limits: []domain.Limit{{Kind: "weekly_all", Group: "weekly", Percent: 9, Severity: "normal",
				ResetsAt: now.Add(48 * time.Hour), Active: true}},
		}}},
		Installed: "2.1.300", Latest: "2.1.319", UpdatedAt: now,
	}
	if err := statefile.New(data).SaveState(st); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	start := time.Now()
	code := run(context.Background(), []string{"statusline"}, &out, &errOut)
	elapsed := time.Since(start)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
	if want := "ana@example · weekly 9% · Claude Code 2.1.300 (2.1.319 available)\n"; out.String() != want {
		t.Errorf("statusline %q, want %q", out.String(), want)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("statusline took %v, must be under 50 ms", elapsed)
	}
}

func TestVersionIsTheBuildVersion(t *testing.T) {
	t.Setenv(paths.EnvDataDir, t.TempDir())
	t.Setenv(paths.EnvClaudeDir, t.TempDir())
	version = "v9.9.9"
	t.Cleanup(func() { version = "dev" })
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"version"}, &out, &errOut); code != 0 || out.String() != "ccswitch v9.9.9\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
}
