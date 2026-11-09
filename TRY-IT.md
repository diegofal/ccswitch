# Try it (WP-6: the first run with real accounts)

Everything below runs on your real `~/.claude`. Until this list is done, the app has only been run against
copies of your login. Binaries: `bin\ccswitch.exe` (CLI) and `bin\ccswitch-app.exe` (tray), built by
`task build`. Data lives in `%APPDATA%\ccswitch` (vault `accounts.json` sealed with DPAPI, `state.json`,
`settings.json`, `ccswitch.log`).

Before you start: `claude auth status --json` should show the account you expect as logged in.

This is the maintainer's checklist for the facts PLAN §6 could not settle from the desk; a new user wants
the [README](README.md) instead.

**Done on 2026-11-09 on the real login:** steps 1, 2, 3, 5 and 6 (`check` captured the work account; three
more accounts added (V1); A→B→A kept both tokens working (V2); the tray —
[docs/tray-menu.png](docs/tray-menu.png); "Update now" installed 2.1.319 (V6)). **Left:** step 4 (V3, does an
open session follow a switch), V4 (an inactive account's token refresh, first due about 12 h after it was
added) and V5 (a Mac).

## 1. See the current account (nothing is written to Claude Code)

    bin\ccswitch.exe check

Expected: one row with your email and plan, each active limit as a bar with its percentage and reset time, the
auto-switch line (off), and `Claude Code 2.1.300 · 2.1.319 available`. The vault now holds this account.

## 2. Add your second account (V1)

    bin\ccswitch.exe add

A browser login opens for `claude auth login`, running in a scratch folder so your current login is untouched.
Log in with the other account. Expected afterwards: "Added <email> (<plan>)", `ccswitch status` shows both
accounts with their own numbers, and `claude auth status --json` still shows the first account.

If `add` fails after the login, keep the terminal output: it tells whether Claude Code wrote `.claude.json`
inside the scratch folder (PLAN V1).

## 3. Switch and come back (V2)

    bin\ccswitch.exe use <prefix of the second email>
    claude auth status --json        → the second email
    bin\ccswitch.exe use <prefix of the first email>
    claude auth status --json        → the first email again

Then in a new terminal, start `claude` and send one short message on each account. Both must work. If the first
account fails after the round trip, Claude Code revoked its token on login: tell me, that changes `add`.

## 4. Does a running session follow the switch? (V3)

Keep an interactive `claude` open on account A. From another terminal `ccswitch use B`, then send a message in
the open session. Run `ccswitch check`: whichever account's percentage moved is the one that served the message.
Tell me which; it decides the wording of the switch notification.

## 5. Tray

    bin\ccswitch-app.exe

Right-click (or left-click) the icon. Expected: both accounts with bars, "Use this account" on the inactive
one, the Auto-switch checkbox, Add account…, the Claude Code version row with "Update now", Start at login,
Refresh now, Open log, Quit. Switching from the menu must change `claude auth status`. The icon turns amber at
80 % and red at 95 % of the active account's highest limit; grey means the last read is old; red with a dot
means the read failed.

## 6. Update Claude Code from the tray (V6)

Click "Update now". Expected: a notification "Claude Code 2.1.319 installed — new sessions use it" and
`claude --version` says 2.1.319. Sessions already open keep 2.1.300.

## 7. Auto-switch

Tick "Auto-switch at 95 %" (or `ccswitch auto on`). Nothing happens until an account passes 95 %. The work
seat can be kept out of the pool: `ccswitch pool <email> off`. Each automatic switch sends a notification and
offers Undo in the menu for 10 minutes.

## 8. Status line in Claude Code (optional)

In `~/.claude\settings.json`:

    { "statusLine": { "type": "command", "command": "D:\\pocs\\ccswitch\\bin\\ccswitch.exe statusline" } }

It prints `<account> · weekly 9% · Claude Code 2.1.300 (2.1.319 available)` from the saved state, no network.

## If something goes wrong

- `ccswitch status` shows "Last error: …" and the log is `%APPDATA%\ccswitch\ccswitch.log` (no tokens in it).
- When an account's usage stops being readable, the icon turns red with a dot and one notification "Usage
  unreadable: <email>: usage could not be read (<reason>)" is sent; it is not repeated while the failure lasts.
- A failed switch restores the previous login by itself. If Claude Code ever says you are logged out, run
  `claude auth login` and then `ccswitch check`: the new login is captured and the vault updated.
- Nothing here deletes anything in `~/.claude`; the only members ccswitch ever writes are `claudeAiOauth`,
  `organizationUuid` (in `.credentials.json`) and `oauthAccount` (in `.claude.json`).
