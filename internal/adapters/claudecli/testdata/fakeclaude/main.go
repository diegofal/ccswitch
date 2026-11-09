// fakeclaude stands in for the claude binary in claudecli's tests. It
// answers --version, update, auth status --json and auth login with canned
// output; FAKE_CLAUDE_MODE picks a failure.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	mode := os.Getenv("FAKE_CLAUDE_MODE")
	switch strings.Join(os.Args[1:], " ") {
	case "--version":
		switch mode {
		case "empty":
		default:
			fmt.Print("\x1b[1m2.1.300 (Claude Code)\x1b[0m\nsecond line\n")
		}
	case "update":
		if mode == "fail" {
			fmt.Println("Error: EACCES: permission denied")
			os.Exit(1)
		}
		fmt.Println("Current version: 2.1.300")
		fmt.Println("Successfully updated to 2.1.319")
	case "auth status --json":
		switch mode {
		case "badjson":
			fmt.Println("not json")
		case "noflag":
			fmt.Println(`{"email":"a@x.com"}`)
		default:
			out, _ := json.Marshal(map[string]any{
				"loggedIn": true, "authMethod": "claude.ai", "email": "a@x.com", "orgName": "Org",
				"subscriptionType": "max", "configDirectory": os.Getenv("CLAUDE_CONFIG_DIR"),
			})
			fmt.Println(string(out))
		}
	case "auth login":
		if mode == "fail" {
			os.Exit(2)
		}
		if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "login-ran"), nil, 0o600)
		}
	default:
		fmt.Println("unknown command:", os.Args[1:])
		os.Exit(9)
	}
}
