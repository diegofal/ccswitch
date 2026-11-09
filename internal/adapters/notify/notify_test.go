package notify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/diegofal/ccswitch/internal/testkit"
)

// recorder stands in for running a program and for the toast API.
type recorder struct {
	calls [][]string
	err   error
}

func (r *recorder) run(_ context.Context, name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.err
}

func (r *recorder) toast(title, body string) error {
	r.calls = append(r.calls, []string{"toast", title, body})
	return r.err
}

func desktop(goos string, r *recorder) *Desktop {
	return &Desktop{goos: goos, run: r.run, toast: r.toast}
}

func TestNotifyPerOS(t *testing.T) {
	title, body := `Switched to "b"`, "a's 5h resets at 14:00; $(rm -rf /)"
	cases := map[string][]string{
		"windows": {"toast", title, body},
		"darwin": {"osascript", "-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body},
		"linux": {"notify-send", "--app-name=ccswitch", "--", title, body},
	}
	for goos, want := range cases {
		r := &recorder{}
		if err := desktop(goos, r).Notify(title, body); err != nil {
			t.Errorf("%s: %v", goos, err)
		}
		if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], want) {
			t.Errorf("%s: ran %q, want %q", goos, r.calls, want)
		}
	}
}

func TestNotifyFailuresAreErrors(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		r := &recorder{err: errors.New("no desktop")}
		if err := desktop(goos, r).Notify("t", "b"); err == nil || !strings.Contains(err.Error(), "no desktop") {
			t.Errorf("%s: got %v, want the failure", goos, err)
		}
	}
	if err := desktop("plan9", &recorder{}).Notify("t", "b"); err == nil {
		t.Error("unknown OS: want an error")
	}
}

func TestNewPicksThisOS(t *testing.T) {
	if d := New(); d.goos == "" || d.run == nil || d.toast == nil {
		t.Errorf("New() = %+v", d)
	}
}

func TestLogWritesTheNotification(t *testing.T) {
	l := &testkit.FakeLogger{}
	if err := NewLog(l).Notify("Limit at 80 %", "a@x.com · 5h"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"notification: Limit at 80 % · a@x.com · 5h"}; !reflect.DeepEqual(l.Lines, want) {
		t.Errorf("logged %q, want %q", l.Lines, want)
	}
}
