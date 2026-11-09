// Package statefile keeps settings.json and state.json in the app data
// folder (ports.StateStore). Writes are atomic: a reader such as
// `ccswitch statusline`, which takes no lock, sees the old file or the new
// one, never half of one.
package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
	"github.com/diegofal/ccswitch/internal/ports"
)

const (
	settingsName = "settings.json"
	stateName    = "state.json"
)

// Store is settings.json and state.json in one folder.
type Store struct{ dir string }

var _ ports.StateStore = (*Store)(nil)

// New returns the store in dir (normally paths.DataDir()). The folder is
// created on the first save.
func New(dir string) *Store { return &Store{dir: dir} }

// settingsJSON is settings.json: durations as text ("2m") so the file reads
// and edits by hand.
type settingsJSON struct {
	Auto             bool      `json:"auto"`
	ThresholdPercent float64   `json:"thresholdPercent"`
	AlertThresholds  []float64 `json:"alertThresholds"`
	PollActive       string    `json:"pollActive"`
	PollInactive     string    `json:"pollInactive"`
	VersionEvery     string    `json:"versionEvery"`
	UndoWindow       string    `json:"undoWindow"`
}

func toJSON(s domain.Settings) settingsJSON {
	return settingsJSON{
		Auto:             s.Auto,
		ThresholdPercent: s.ThresholdPercent,
		AlertThresholds:  s.AlertThresholds,
		PollActive:       s.PollActive.String(),
		PollInactive:     s.PollInactive.String(),
		VersionEvery:     s.VersionEvery.String(),
		UndoWindow:       s.UndoWindow.String(),
	}
}

func (j settingsJSON) settings() (domain.Settings, error) {
	s := domain.Settings{Auto: j.Auto, ThresholdPercent: j.ThresholdPercent, AlertThresholds: j.AlertThresholds}
	for _, d := range []struct {
		name, text string
		to         *time.Duration
	}{
		{"pollActive", j.PollActive, &s.PollActive},
		{"pollInactive", j.PollInactive, &s.PollInactive},
		{"versionEvery", j.VersionEvery, &s.VersionEvery},
		{"undoWindow", j.UndoWindow, &s.UndoWindow},
	} {
		v, err := time.ParseDuration(d.text)
		if err != nil {
			return domain.Settings{}, fmt.Errorf("%s: %w", d.name, err)
		}
		if v <= 0 {
			return domain.Settings{}, fmt.Errorf("%s: %q must be more than zero", d.name, d.text)
		}
		*d.to = v
	}
	return s, nil
}

// LoadSettings reads settings.json. A missing file, or a key missing from
// it, takes domain.DefaultSettings; a file it cannot read is an error.
func (s *Store) LoadSettings() (domain.Settings, error) {
	j := toJSON(domain.DefaultSettings())
	found, err := s.read(settingsName, &j)
	if err != nil || !found {
		return domain.DefaultSettings(), err
	}
	set, err := j.settings()
	if err != nil {
		return domain.DefaultSettings(), fmt.Errorf("%s: %w", s.path(settingsName), err)
	}
	return set, nil
}

// SaveSettings writes settings.json atomically.
func (s *Store) SaveSettings(set domain.Settings) error { return s.write(settingsName, toJSON(set)) }

// LoadState reads state.json; a missing file is the zero State.
func (s *Store) LoadState() (domain.State, error) {
	var st domain.State
	if _, err := s.read(stateName, &st); err != nil {
		return domain.State{}, err
	}
	return st, nil
}

// SaveState writes state.json atomically.
func (s *Store) SaveState(st domain.State) error { return s.write(stateName, st) }

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

// read decodes the named file into v and reports whether it existed.
func (s *Store) read(name string, v any) (bool, error) {
	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", name, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("read %s: %w", s.path(name), err)
	}
	return true, nil
}

func (s *Store) write(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := writeAtomic(s.dir, name, append(data, '\n')); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// writeAtomic writes data to a temp file in dir (CreateTemp makes it 0600:
// state names the accounts), syncs it and renames it over dir/name.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	done = true
	return nil
}

// rename retries briefly: on Windows a reader that has the target open (the
// statusline reads state.json without the lock) makes the rename fail for
// the moment it takes to read.
func rename(from, to string) error {
	var err error
	for range 10 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}
