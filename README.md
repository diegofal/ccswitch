# ccswitch

Several Claude Code logins, one tray icon: see how much of each account's limits is used, get warned before
a limit, switch accounts by hand or automatically, and see / update the installed Claude Code version.

Go + Wails v3, one core, three front ends: tray (Windows tray · macOS menu bar · Linux AppIndicator),
`ccswitch` CLI, and a Claude Code status line. Plan and decisions in [PLAN.md](PLAN.md).

## Decided

| Topic | Decision |
|---|---|
| Scope | Claude Code subscription logins only. No claude.ai web/desktop, no API keys. |
| Switching | Swap the token in the one `~/.claude` (only `claudeAiOauth`, `organizationUuid`, `oauthAccount`); MCP logins, history and settings stay. |
| Active token | Never refreshed by ccswitch; Claude Code owns it. Snapshot before every switch. |
| Adding an account | `claude auth login` in a scratch `CLAUDE_CONFIG_DIR`, harvested into the vault. |
| Vault | Windows DPAPI file · macOS Keychain · Linux 0600 file. |
| Usage | Claude Code's own usage endpoint (`limits[]`, generic). Unreadable = red, never a stale number as current. |
| Alerts | 80 % and 95 % per limit, once per reset window; "limit reached"; "available again". |
| Auto-switch | Off by default. At ≥ 95 % move to the account with the most room (< 95 %, fresh, in pool). Undo 10 min. |
| Version | `claude --version` vs npm registry; "Update now" runs `claude update`. |
| Front ends | Hold no logic. Tray menu + notifications (no window in v1), CLI, `statusline`, `watch`. |
| Concurrency | No daemon, no IPC: a file lock per operation; tray watches the login file. |
| Terms | Whether switching plans to avoid limits is allowed under Anthropic's rules is the maintainer's check; a Team/Enterprise seat is out of the pool by default. |

## Status line

In `~/.claude/settings.json`:

```json
{ "statusLine": { "type": "command", "command": "ccswitch statusline" } }
```

## Dev

See [CLAUDE.md](CLAUDE.md). `task ci` runs what CI runs.
