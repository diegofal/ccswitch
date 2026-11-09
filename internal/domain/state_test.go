package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/diegofal/ccswitch/internal/domain"
)

func TestDefaultSettings(t *testing.T) {
	want := domain.Settings{
		Auto:             false,
		ThresholdPercent: 95,
		AlertThresholds:  []float64{80, 95},
		PollActive:       2 * time.Minute,
		PollInactive:     15 * time.Minute,
		VersionEvery:     6 * time.Hour,
		UndoWindow:       10 * time.Minute,
	}
	if got := domain.DefaultSettings(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultSettings = %+v, want %+v", got, want)
	}
	a := domain.DefaultSettings()
	a.AlertThresholds[0] = 1
	if domain.DefaultSettings().AlertThresholds[0] != 80 {
		t.Error("DefaultSettings shares its AlertThresholds slice")
	}
}

func TestStatePrune(t *testing.T) {
	s := domain.State{Fired: map[string]bool{
		"a@x|session_all|2026-11-09T11:59:00Z|80":         true, // reset passed
		"a@x|session_all|2026-11-09T12:00:00Z|95":         true, // resets now: passed
		"a@x|weekly_all|2026-11-13T18:00:00Z|80":          true,
		"b|c@x|weekly_scoped:Opus|2026-11-08T00:00:00Z|0": true, // email with a bar still parses from the end
		"not a key": true,
	}}
	s.Prune(t0)
	want := map[string]bool{"a@x|weekly_all|2026-11-13T18:00:00Z|80": true, "not a key": true}
	if !reflect.DeepEqual(s.Fired, want) {
		t.Errorf("Fired after Prune = %v, want %v", s.Fired, want)
	}
	var empty domain.State
	empty.Prune(t0) // nil map: nothing to do, no panic
}
