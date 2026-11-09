# ccswitch

Tray app + CLI (Go 1.26, Wails v3.0.0, pure Go, no cgo) that holds several Claude Code logins, shows their
limits, warns, switches, and updates Claude Code. Read [PLAN.md](PLAN.md) before changing anything: §2 lists
the facts the design rests on, §3 the decisions, §6 what is still unverified.

## Dev setup on this PC

The system Go is 1.25.6; put the 1.26 toolchain first on PATH before any `go` or `task` command:

    export PATH="/c/Users/User/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.10.windows-amd64/bin:$PATH"

`task ci` = fmt check, vet, golangci-lint, tests with coverage gates (domain + core ≥ 85 %), the architecture
test, and the build. Run the full Go suite before every commit.

## Hard rules

1. **Never touch the real `~/.claude` or the real vault from tests or test copies.** Every binary honours
   `CCSWITCH_DATA_DIR`, `CCSWITCH_CLAUDE_DIR`, `CCSWITCH_CLAUDE_BIN`; tests set them to temp folders; the
   arch test fails any `os.UserHomeDir` outside `internal/adapters/paths`. A real-login end-to-end run works
   on a *copy* of `~/.claude` and only with the maintainer watching.
2. **Secrets never leave the machine or reach a log.** Access and refresh tokens are not logged, not printed
   by the CLI, not in test fixtures. Fixtures use `usage-max20x.json` (no secrets) and made-up tokens.
3. **Only `claudeAiOauth`, `organizationUuid` and `oauthAccount` are ever written to Claude Code's files.**
   Every other key is preserved byte for byte (test enforced). Writes are atomic (temp + rename, mode kept).
4. **ccswitch never refreshes the active account's token** (PLAN D2).
5. **A failed read is shown, never hidden.** No stale number displayed as current, no empty catch.
6. Red first: write the failing test, then the code. Front ends (tray, CLI, statusline) hold no logic.

## Code rules (same as Fieldwork)

- Reuse before writing; standard library first; a new module needs a reason in the commit message.
- No comments that restate the code; comment only the *why*. No placeholders, no TODO without an issue.
- Errors propagate with context (`fmt.Errorf("…: %w", err)`); never log-and-continue.
- No speculative abstraction: ports exist because they have a fake in `internal/testkit` and a real adapter.
- Delete dead code in the same change. Kebab-case files, Go names in code, user-facing text in English.
- Child processes go through `internal/adapters/execx` (hidden console window on Windows: the tray has no
  console and a flashing `claude` window steals focus).
- Stop a test copy by its exact PID (`powershell -NoProfile -Command "Stop-Process -Id <pid> -Force"`); never
  by exe path, both copies report the same path.

## Pitfalls already known

- `.credentials.json` also holds MCP OAuth logins (Linear, Slack, Vercel). Overwriting the file logs them out.
- `.claude.json` is big and rewritten by Claude Code on most sessions. Read-modify-write it, fast, atomically.
- Refresh tokens rotate. Snapshot the live login before switching or the previous account's stored refresh
  token may be dead.
- The usage endpoint is undocumented: `limits[]` is the part we rely on; everything else is optional.
- Defender ML sometimes quarantines tiny stripped Go test binaries: build test helpers unstripped.
