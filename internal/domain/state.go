package domain

import (
	"strings"
	"time"
)

// Settings are the user's choices (settings.json).
type Settings struct {
	Auto             bool
	ThresholdPercent float64       // default 95
	AlertThresholds  []float64     // default 80, 95
	PollActive       time.Duration // default 2m
	PollInactive     time.Duration // default 15m
	VersionEvery     time.Duration // default 6h
	UndoWindow       time.Duration // default 10m
}

// DefaultSettings are the settings when the user has chosen nothing.
func DefaultSettings() Settings {
	return Settings{
		ThresholdPercent: 95,
		AlertThresholds:  []float64{80, 95},
		PollActive:       2 * time.Minute,
		PollInactive:     15 * time.Minute,
		VersionEvery:     6 * time.Hour,
		UndoWindow:       10 * time.Minute,
	}
}

// AccountState is what the front ends show for one account.
type AccountState struct {
	Email, OrgName, Plan string
	InPool               bool
	Usage                Usage
}

// Undo is the switch back offered after an automatic switch.
type Undo struct {
	From, To string // From = account to switch back to
	Until    time.Time
}

// State is the last known picture (state.json), rendered by every front end.
type State struct {
	Active        string
	Accounts      []AccountState
	Fired         map[string]bool // alert keys sent
	Undo          *Undo
	Installed     string // claude --version
	Latest        string
	VersionReadAt time.Time
	UpdatedAt     time.Time
	LastError     string // last operation error shown in tray/CLI, "" if none
}

// Prune forgets fired alerts whose window has reset: their keys can never
// come back, since a new window has a new reset time. Keys it cannot read
// are kept.
func (s *State) Prune(now time.Time) {
	for key := range s.Fired {
		parts := strings.Split(key, "|")
		if len(parts) < 4 {
			continue
		}
		resets, err := time.Parse(time.RFC3339, parts[len(parts)-2])
		if err == nil && !resets.After(now) {
			delete(s.Fired, key)
		}
	}
}
