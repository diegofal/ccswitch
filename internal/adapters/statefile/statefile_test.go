package statefile

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

func TestSettingsDefaultWhenAbsent(t *testing.T) {
	got, err := New(t.TempDir()).LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, domain.DefaultSettings()) {
		t.Errorf("got %+v, want the defaults", got)
	}
}

func TestSettingsRoundTripWithDurationsAsText(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet")
	s := New(dir)
	want := domain.Settings{
		Auto: true, ThresholdPercent: 90, AlertThresholds: []float64{70, 90},
		PollActive: 90 * time.Second, PollInactive: 20 * time.Minute, VersionEvery: 12 * time.Hour, UndoWindow: 5 * time.Minute,
	}
	if err := s.SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`"pollActive": "1m30s"`, `"undoWindow": "5m0s"`, `"auto": true`} {
		if !strings.Contains(string(raw), s) {
			t.Errorf("settings.json lacks %s:\n%s", s, raw)
		}
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("mode: %v, %v", fi.Mode(), err)
		}
	}
}

func TestSettingsMissingKeysTakeDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "settings.json", `{"auto": true, "pollActive": "30s"}`)
	got, err := New(dir).LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	want := domain.DefaultSettings()
	want.Auto, want.PollActive = true, 30*time.Second
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSettingsUnreadableIsAnError(t *testing.T) {
	for name, body := range map[string]string{
		"bad json":      `{"auto": tru`,
		"bad duration":  `{"pollActive": "soon"}`,
		"zero duration": `{"undoWindow": "0s"}`,
	} {
		dir := t.TempDir()
		write(t, dir, "settings.json", body)
		if _, err := New(dir).LoadSettings(); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestStateZeroWhenAbsentAndRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	got, err := s.LoadState()
	if err != nil || !reflect.DeepEqual(got, domain.State{}) {
		t.Fatalf("absent: got %+v, %v", got, err)
	}
	at := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	want := domain.State{
		Active: "a@x.com",
		Accounts: []domain.AccountState{{Email: "a@x.com", Plan: "max", InPool: true, Usage: domain.Usage{
			ReadAt: at, Limits: []domain.Limit{{Kind: "session", Group: "5h", Percent: 42, Severity: "normal", ResetsAt: at.Add(time.Hour), Active: true}},
		}}, {Email: "b@x.com", Usage: domain.Usage{Err: "unreadable"}}},
		Fired:     map[string]bool{"k": true},
		Undo:      &domain.Undo{From: "b@x.com", To: "a@x.com", Until: at.Add(10 * time.Minute)},
		Installed: "2.1.300", Latest: "2.1.319", VersionReadAt: at, UpdatedAt: at, LastError: "boom",
	}
	if err := s.SaveState(want); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveState(want); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, err = s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	left, _ := filepath.Glob(filepath.Join(s.dir, "*.tmp"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestStateUnreadableIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "state.json", `[`)
	if _, err := New(dir).LoadState(); err == nil {
		t.Error("want an error")
	}
}

func TestWriteFailsWhenFolderIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	write(t, filepath.Dir(file), "file", "x")
	if err := New(file).SaveState(domain.State{}); err == nil {
		t.Error("want an error")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "state.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir).LoadState(); err == nil {
		t.Error("state.json a folder: want an error")
	}
	if err := New(dir).SaveState(domain.State{}); err == nil {
		t.Error("renaming over a folder: want an error")
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
