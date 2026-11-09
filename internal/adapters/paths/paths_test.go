package paths

import (
	"errors"
	"path/filepath"
	"testing"
)

func fakeEnv(goos string, vars map[string]string, home string) env {
	return env{
		goos:   goos,
		getenv: func(k string) string { return vars[k] },
		home: func() (string, error) {
			if home == "" {
				return "", errors.New("no home")
			}
			return home, nil
		},
	}
}

func TestDataDir(t *testing.T) {
	home := filepath.Join("h", "ana")
	cases := []struct {
		name string
		e    env
		want string
	}{
		{"override wins", fakeEnv("windows", map[string]string{EnvDataDir: "x", "APPDATA": "ad"}, home), "x"},
		{"windows", fakeEnv("windows", map[string]string{"APPDATA": "ad"}, home), filepath.Join("ad", "ccswitch")},
		{"darwin", fakeEnv("darwin", nil, home), filepath.Join(home, "Library", "Application Support", "ccswitch")},
		{"linux xdg", fakeEnv("linux", map[string]string{"XDG_DATA_HOME": "xdg"}, home), filepath.Join("xdg", "ccswitch")},
		{"linux default", fakeEnv("linux", nil, home), filepath.Join(home, ".local", "share", "ccswitch")},
	}
	for _, c := range cases {
		got, err := c.e.dataDir()
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for _, e := range []env{fakeEnv("windows", nil, home), fakeEnv("darwin", nil, ""), fakeEnv("linux", nil, "")} {
		if got, err := e.dataDir(); err == nil {
			t.Errorf("%s with nothing set: got %q, want an error", e.goos, got)
		}
	}
}

func TestClaudeDir(t *testing.T) {
	if got, _ := fakeEnv("linux", map[string]string{EnvClaudeDir: "c"}, "h").claudeDir(); got != "c" {
		t.Errorf("override: got %q", got)
	}
	if got, _ := fakeEnv("windows", nil, "h").claudeDir(); got != filepath.Join("h", ".claude") {
		t.Errorf("default: got %q", got)
	}
	if _, err := fakeEnv("linux", nil, "").claudeDir(); err == nil {
		t.Error("no home: want an error")
	}
}

func TestConfigFiles(t *testing.T) {
	dir := filepath.Join("h", ".claude")
	if got, want := ClaudeConfigFileBeside(dir), filepath.Join("h", ".claude.json"); got != want {
		t.Errorf("beside: got %q, want %q", got, want)
	}
	if got, want := ClaudeConfigFileBeside(dir+string(filepath.Separator)), filepath.Join("h", ".claude.json"); got != want {
		t.Errorf("beside with trailing separator: got %q, want %q", got, want)
	}
	if got, want := ClaudeConfigFile(dir), filepath.Join(dir, ".claude.json"); got != want {
		t.Errorf("inside: got %q, want %q", got, want)
	}
}

func TestOverridesFromProcessEnv(t *testing.T) {
	t.Setenv(EnvDataDir, "d")
	t.Setenv(EnvClaudeDir, "c")
	t.Setenv(EnvClaudeBin, "fake-claude")
	if got, err := DataDir(); err != nil || got != "d" {
		t.Errorf("DataDir: %q, %v", got, err)
	}
	if got, err := ClaudeDir(); err != nil || got != "c" {
		t.Errorf("ClaudeDir: %q, %v", got, err)
	}
	if got, err := ClaudeBin(); err != nil || got != "fake-claude" {
		t.Errorf("ClaudeBin: %q, %v", got, err)
	}
}

func TestClaudeBinNotOnPath(t *testing.T) {
	t.Setenv(EnvClaudeBin, "")
	t.Setenv("PATH", t.TempDir())
	if got, err := ClaudeBin(); err == nil {
		t.Errorf("got %q, want an error naming %s", got, EnvClaudeBin)
	}
}
