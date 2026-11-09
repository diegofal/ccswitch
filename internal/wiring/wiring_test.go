package wiring_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/testkit"
	"github.com/diegofal/ccswitch/internal/wiring"
)

// Both programs get the same core on the folders the CCSWITCH_* overrides
// name; building it reads no file and calls no network.
func TestNewUsesTheOverriddenFolders(t *testing.T) {
	data, claude := t.TempDir(), t.TempDir()
	t.Setenv(paths.EnvDataDir, data)
	t.Setenv(paths.EnvClaudeDir, claude)
	w, err := wiring.New("v1.2.3", &testkit.FakeNotifier{})
	if err != nil {
		t.Fatal(err)
	}
	if w.DataDir != data || w.ClaudeDir != claude {
		t.Errorf("folders = %q, %q; want %q, %q", w.DataDir, w.ClaudeDir, data, claude)
	}
	if w.Log.Path() != filepath.Join(data, "ccswitch.log") {
		t.Errorf("log at %q", w.Log.Path())
	}
	st, err := w.Switcher.State(context.Background())
	if err != nil || len(st.Accounts) != 0 {
		t.Errorf("fresh state = %+v, %v", st, err)
	}
	if _, err := w.Live.ModTime(); err == nil {
		t.Error("ModTime of an empty Claude folder: want an error")
	}
}
