package tray

import (
	"reflect"
	"testing"
)

func TestAddCommand(t *testing.T) {
	cases := []struct {
		goos, cli string
		name      string
		args      []string
	}{
		// start's first quoted argument is the window title; "" keeps the
		// program path from being read as one.
		{"windows", `C:\Program Files\ccswitch\ccswitch.exe`, "cmd", []string{"/c", "start", "", `C:\Program Files\ccswitch\ccswitch.exe`, "add"}},
		{"darwin", "/Applications/cc switch/ccswitch", "osascript", []string{
			"-e", `tell application "Terminal" to do script "'/Applications/cc switch/ccswitch' add"`,
			"-e", `tell application "Terminal" to activate`}},
		{"darwin", `/tmp/it's "odd"\x/ccswitch`, "osascript", []string{
			"-e", `tell application "Terminal" to do script "'/tmp/it'\\''s \"odd\"\\x/ccswitch' add"`,
			"-e", `tell application "Terminal" to activate`}},
		{"linux", "/opt/cc switch/ccswitch", "x-terminal-emulator", []string{"-e", "/opt/cc switch/ccswitch", "add"}},
		{"freebsd", "/usr/local/bin/ccswitch", "x-terminal-emulator", []string{"-e", "/usr/local/bin/ccswitch", "add"}},
	}
	for _, c := range cases {
		name, args := AddCommand(c.goos, c.cli)
		if name != c.name || !reflect.DeepEqual(args, c.args) {
			t.Errorf("AddCommand(%s, %q) =\n  %s %q\nwant\n  %s %q", c.goos, c.cli, name, args, c.name, c.args)
		}
	}
}
