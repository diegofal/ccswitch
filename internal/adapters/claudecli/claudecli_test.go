package claudecli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/adapters/paths"
	"github.com/diegofal/ccswitch/internal/ports"
)

// fakeBin is testdata/fakeclaude built once for the package's tests,
// unstripped (Defender quarantines tiny stripped binaries).
var fakeBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ccswitch-fakeclaude-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeBin = filepath.Join(dir, "claude")
	if runtime.GOOS == "windows" {
		fakeBin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", fakeBin, "./testdata/fakeclaude").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build fake claude: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func useFake(t *testing.T, mode string) {
	t.Helper()
	t.Setenv(paths.EnvClaudeBin, fakeBin)
	t.Setenv("FAKE_CLAUDE_MODE", mode)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
}

func TestVersionIsTheTrimmedFirstLine(t *testing.T) {
	useFake(t, "")
	got, err := New().Version(context.Background())
	if err != nil || got != "2.1.300 (Claude Code)" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestVersionEmptyIsAnError(t *testing.T) {
	useFake(t, "empty")
	if got, err := New().Version(context.Background()); err == nil {
		t.Errorf("got %q, want an error", got)
	}
}

func TestUpdateReturnsOutput(t *testing.T) {
	useFake(t, "")
	got, err := New().Update(context.Background())
	if err != nil || !strings.HasSuffix(got, "Successfully updated to 2.1.319") {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestUpdateFailureCarriesOutput(t *testing.T) {
	useFake(t, "fail")
	got, err := New().Update(context.Background())
	if err == nil || !strings.Contains(err.Error(), "EACCES: permission denied") || !strings.HasPrefix(err.Error(), "claude update: ") {
		t.Errorf("want an error carrying the output, got %v", err)
	}
	if !strings.Contains(got, "EACCES") {
		t.Errorf("output %q should be returned too", got)
	}
}

func TestAuthStatus(t *testing.T) {
	useFake(t, "")
	scratch := t.TempDir()
	cases := map[string]string{"": "", scratch: scratch}
	for dir, wantDir := range cases {
		got, err := New().AuthStatus(context.Background(), dir)
		want := ports.AuthStatus{LoggedIn: true, Email: "a@x.com", OrgName: "Org", Subscription: "max", ConfigDir: wantDir}
		if err != nil || got != want {
			t.Errorf("dir %q: got %+v, %v; want %+v", dir, got, err, want)
		}
	}
}

func TestAuthStatusUnreadable(t *testing.T) {
	for _, mode := range []string{"badjson", "noflag"} {
		useFake(t, mode)
		if got, err := New().AuthStatus(context.Background(), ""); err == nil {
			t.Errorf("%s: got %+v, want an error", mode, got)
		}
	}
}

func TestLoginRunsWithConfigDir(t *testing.T) {
	useFake(t, "")
	scratch := t.TempDir()
	if err := New().Login(context.Background(), scratch); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(scratch, "login-ran")); err != nil {
		t.Errorf("login did not run with CLAUDE_CONFIG_DIR=%s: %v", scratch, err)
	}
	useFake(t, "fail")
	if err := New().Login(context.Background(), scratch); err == nil {
		t.Error("failed login: want an error")
	}
}

func TestNoClaudeInstalled(t *testing.T) {
	t.Setenv(paths.EnvClaudeBin, "")
	t.Setenv("PATH", t.TempDir())
	c := New()
	if _, err := c.Version(context.Background()); err == nil {
		t.Error("Version: want an error")
	}
	if _, err := c.AuthStatus(context.Background(), ""); err == nil {
		t.Error("AuthStatus: want an error")
	}
	if err := c.Login(context.Background(), ""); err == nil {
		t.Error("Login: want an error")
	}
}
