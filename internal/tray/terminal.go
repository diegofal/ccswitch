package tray

import "strings"

// AddCommand is the program and arguments that open a terminal window
// running `<cli> add` (PLAN D3): the login needs a browser step and a
// terminal the user can see, which the tray has not.
func AddCommand(goos, cli string) (name string, args []string) {
	switch goos {
	case "windows":
		return "cmd", []string{"/c", "start", "", cli, "add"}
	case "darwin":
		script := shellQuote(cli) + " add"
		return "osascript", []string{
			"-e", `tell application "Terminal" to do script ` + appleString(script),
			"-e", `tell application "Terminal" to activate`,
		}
	}
	return "x-terminal-emulator", []string{"-e", cli, "add"}
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// appleString is s as an AppleScript string literal.
func appleString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
