# ccswitch — plan

Written 2026-11-09 by Fable, from facts checked on the maintainer's PC. Opus builds. Status table at the end.

## 1. What it is

A small always-on app that holds several Claude Code logins, shows how much of each one's limits is used,
warns before a limit is hit, switches the active login (by hand or automatically), and shows / updates the
installed Claude Code version. One Go core, three front ends: tray (Windows tray, macOS menu bar, Linux
AppIndicator), CLI, Claude Code status line.

Out of scope: claude.ai web/desktop logins (cookies), API-key accounts, anything that reads conversations.

## 2. Facts the design rests on (checked 2026-11-09, Claude Code 2.1.300, Windows)

F1. The active login lives in two files:
  - `~/.claude/.credentials.json` → key `claudeAiOauth` {accessToken, refreshToken, expiresAt,
    refreshTokenExpiresAt, scopes, subscriptionType, subscriptionTypeSource, rateLimitTier} and key
    `organizationUuid`. **The same file holds `mcpOAuth` (Linear, Slack, Vercel logins).** A switch must
    replace only `claudeAiOauth` + `organizationUuid` and keep every other key byte-for-byte.
  - `~/.claude.json` → key `oauthAccount` {accountUuid, emailAddress, organizationUuid, organizationName,
    planDisplayName, …}. Claude Code rewrites this file often; patch it read-modify-write, atomically.
  - macOS keeps the credentials in the Keychain (item `Claude Code-credentials`, same JSON) instead of the
    file. Not checked here (no Mac): see V5.
F2. Access token lives ~12 h (`expiresAt`), refresh token ~28 days (`refreshTokenExpiresAt`). An account
    nobody refreshes for 28 days is lost and must be added again.
F3. Usage: `GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer <accessToken>` and
    `anthropic-beta: oauth-2025-04-20` → 200 JSON. The generic part is `limits[]`:
    `{kind, group ("weekly"|…), percent, severity ("normal"|…), resets_at, scope{model{display_name}}, is_active}`.
    Also `five_hour` / `seven_day` objects (nullable), `extra_usage`, `spend`, `seven_day_breakdown`.
    Recorded response (no secrets): `internal/adapters/usageapi/testdata/usage-max20x.json`.
    **Undocumented endpoint.** It can change without notice → one adapter, and a failed or unparseable read
    is a visible red state, never a stale number shown as current.
F4. Token refresh: `POST https://console.anthropic.com/v1/oauth/token`
    `{grant_type:"refresh_token", refresh_token, client_id:"9d1c250a-e61b-44d9-88ed-5944d1962f5e"}` →
    `{access_token, refresh_token, expires_in}`. Known from Claude Code's own flow, **not exercised here**
    (doing so would rotate the maintainer's live token): V4.
F5. `claude auth login [--email x]`, `claude auth logout`, `claude auth status --json` (fields: loggedIn,
    authMethod, email, orgId, orgName, subscriptionType, configDirectory) exist. `CLAUDE_CONFIG_DIR` moves the
    config folder, so a login in a scratch folder never touches the real one (V1 confirms both files land there).
F6. Version: `claude --version` → `2.1.300 (Claude Code)`. Latest: npm registry
    `https://registry.npmjs.org/@anthropic-ai/claude-code/latest` → `version` (also the native bucket's
    `…/claude-code-releases/latest`; both said 2.1.319). `claude update` installs it (native install, versions
    under `~/.local/share/claude/versions/`). Running sessions keep the old binary; new ones get the new.
F7. the maintainer's tray stack is Go 1.26 + Wails v3.0.0, pure Go, no cgo (Fieldwork). Reused here.

## 3. Decisions

D1. **Token swap in the one `~/.claude`**, not one config folder per account. Reason: history, `--continue`,
    settings, plugins and MCP logins stay shared; after a switch `claude --continue` resumes the same chat.
D2. **The switcher never refreshes the active account's token.** Claude Code owns it. Before any switch the
    live tokens are read back into the vault (they may have been refreshed). Inactive accounts are refreshed
    by the switcher only (nobody else uses those tokens), when expired and at the latest 7 days before the
    refresh token dies.
D3. **Add account = `CLAUDE_CONFIG_DIR=<scratch> claude auth login` in the user's terminal**, then harvest
    the two files from the scratch folder into the vault and delete the scratch folder. The real login is
    untouched. The tray's "Add account…" opens a terminal running `ccswitch add`.
D4. **Vault** (inactive accounts' tokens + profile): one file `accounts.json` in the app data folder.
    Windows: DPAPI-encrypted (user scope, `golang.org/x/sys/windows` CryptProtectData, pure Go).
    macOS: Keychain generic password via the `security` CLI (no cgo). Linux: plain file 0600, which is
    exactly how Claude Code itself stores the live token there. Credential Manager was rejected: 2560-byte
    blob limit per entry.
D5. **Claude Code's JSON is opaque.** The vault stores `claudeAiOauth`, `organizationUuid` and
    `oauthAccount` as raw JSON; the code reads only accessToken, refreshToken, expiresAt,
    refreshTokenExpiresAt, emailAddress, organizationName, planDisplayName. New fields pass through.
D6. **Limits are generic.** Every `limits[]` row with `is_active` is shown, labelled by `group` and scope
    display name; nothing is hard-coded to "5 hour" or "weekly". Binding percent of an account = max percent
    over its active rows. A read older than 3 × the inactive poll interval is *stale*: shown grey "as of
    HH:MM", never a switch target. Before deciding a switch, core re-reads every candidate's usage, so the
    decision never rests on a 15-minute-old number. A failed read is *unreadable*: red row, red tray badge.
D7. **Alerts** at 80 % and 95 % per limit row, plus "limit reached" (percent ≥ 100; the endpoint's severity
    says "warning" from ~78 % so it is displayed, never read as reached), each fired once per (account,
    kind+scope, resets_at to the minute, threshold). Also "X is available again" when a row that
    was ≥ 95 % drops below 50 % (reset).
D8. **Auto-switch** off by default, toggle in the tray and `ccswitch auto on|off`. Threshold 95 % (setting).
    Rule, a pure function: if the active account's binding percent ≥ threshold, switch to the account in the
    pool with the lowest binding percent, if that is < threshold and its usage is fresh; otherwise do nothing
    and say so. Flapping is impossible by construction (the target is under the threshold by definition).
    Each account has `in_pool` (default true) so a work seat can be excluded. Every automatic switch posts
    a notification "Switched to X · Y resets at 18:00" and the tray offers Undo (switch back) for 10 min.
D9. **Switch = read live → snapshot → write target → verify.** Verify means a 200 from the usage endpoint
    with the token now in the live file, and `claude auth status --json` reporting the target email. If
    verify fails, the previous login is restored and the error shown.
D10. **Version / update**: installed = `claude --version` (cached, re-read after update and every 6 h);
    latest = npm registry (every 6 h). Tray row "Claude Code 2.1.300 · 2.1.319 available — Update now" runs
    `claude update` (hidden console, output to the log), re-reads the version, notifies the result. The
    notification says new sessions use the new version.
D11. **Front ends hold no logic.** Tray menu + notifications (no window in v1), CLI, `ccswitch statusline`
    (prints one line from the state cache, no network, for Claude Code's statusLine setting), and
    `ccswitch watch` (the same poll loop headless, for Linux without a tray / SSH).
D12. **One writer at a time, no daemon, no IPC.** Tray and CLI both work on the files directly; every
    operation takes an OS file lock (`ccswitch.lock`, like Fieldwork). The tray notices CLI switches because
    it watches the live login file's mtime every 10 s.
D13. **Logging in to the same email twice** updates that account; the key is `emailAddress`.
D14. **Terms**: switching between subscriptions to avoid limits may fall under Anthropic's usage rules; that
    check is the maintainer's, and a Team/Enterprise seat goes out of the pool (D8) unless he decides otherwise.

Alternatives rejected: Tauri/Electron (new stack, bigger binaries); `CLAUDE_CONFIG_DIR` per account (splits
history and MCP logins); a details window in v1 (menu rows with bars carry the same facts; window is M2).

## 4. Architecture

```
cmd/ccswitch/          CLI: status | use <acct> | add | remove | auto on|off | pool <acct> on|off
                            | update | watch | statusline
cmd/ccswitch-app/      Tray (Wails v3 systray + notifications service + autostart)
internal/domain/       Account, Usage, Limit, PatchCredentials, PatchConfig, Binding, Decide, Alerts,
                       CompareVersion. Pure: no IO, no time.Now (clock passed in).
internal/ports/        LoginStore, Vault, UsageAPI, Claude, Releases, Notifier, Clock, Lock
internal/adapters/     loginfile (win/linux) · loginkeychain (mac) · vault/{dpapi,keychain,file}
                       · usageapi (http) · npmreg · claudecli (exec, hidden console) · notify · lock · execx
internal/core/         Switcher: Capture, Snapshot, Switch, Undo, Poll, Alert, AutoSwitch, CheckVersion,
                       Update, State cache (state.json, atomic write)
internal/testkit/      fakes for every port, fixture loaders
internal/arch_test.go  front ends import only core + domain; domain imports nothing internal
```

App data: `%APPDATA%/ccswitch` · `~/Library/Application Support/ccswitch` · `~/.local/share/ccswitch`:
`accounts.json` (vault), `settings.json` (threshold, auto, poll intervals), `state.json` (last usage per
account, versions, alerts fired, undo), `ccswitch.log`, `ccswitch.lock`. Env `CCSWITCH_DATA_DIR` overrides
(test copies, like Fieldwork's `FIELDWORK_DATA_DIR`); `CCSWITCH_CLAUDE_DIR` overrides `~/.claude` and
`CCSWITCH_CLAUDE_BIN` the binary, so end-to-end tests never touch the real login.

### Core flows

Poll (every 2 min active, 15 min inactive, settings):
1. Read live login. Email unknown → Capture as a new account (D13), mark active. Email known → Snapshot its
   tokens into the vault if they changed.
2. For each account: token expired and account inactive → Refresh (F4) and save. Fetch usage. Write result or
   error with `ReadAt` into state.
3. `Alerts(prev, cur)` → notify each. `Decide(state, settings, now)` → if a switch, run Switch, notify.
4. Every 6 h: version check. Write state.json atomically. Tray re-renders from state.

Switch(target):
1. Lock. Read live login; Snapshot (D2). 2. Target expired → Refresh. 3. `PatchCredentials` +
`PatchConfig` → write both files atomically (temp + rename, keep mode 0600). 4. Verify (D9); on failure
restore and return the error. 5. State: active = target, undo = previous for 10 min. Unlock.

Add: scratch folder under the data dir → run `claude auth login` with `CLAUDE_CONFIG_DIR=scratch`, stdio
inherited → read scratch files → Capture into vault (not active) → delete scratch → print the account.

Update: `claude update` through execx (no console window), log output → `claude --version` → state.

### Tray menu (rendered from state.json)

```
● ana@example.com · Max 20x                   active
    weekly   ▓░░░░░░░░░   1 %   resets Sat 14 Nov 18:00
  personal@…  · Pro
    session  ▓▓▓▓▓▓▓▓░░  78 %   resets 13:00
    weekly   ▓▓▓▓░░░░░░  41 %   resets Thu
    Use this account
────────
☐ Auto-switch at 95 %
Add account…
────────
Claude Code 2.1.300 · 2.1.319 available
    Update now
────────
Refresh now · Open log · Quit
```

Icon: drawn at runtime (Go `image`): the active account's binding percent as a ring; green < 80, amber ≥ 80,
red ≥ 95 or unreadable (with "!"). Tooltip = active email + binding percent. Undo appears as a menu row
while valid.

Status line: `ccswitch statusline` → `ana@example · wk 1% · Claude Code 2.1.300 (2.1.319 available)`.
Reads state.json only. Snippet for `~/.claude/settings.json` in README.

## 5. TDD plan

Red first in every work package; `task ci` (fmt, vet, golangci-lint, tests with coverage gates: ≥ 90 % on domain, ≥ 85 % on
domain and core, arch test, build) must pass before merge. Tests never touch `~/.claude` or the real vault
(the env overrides above; the arch test fails any `os.UserHomeDir` call outside `internal/adapters/paths`).

Domain tests (table-driven):
- `PatchCredentials`: real-shape fixture with `mcpOAuth` and an unknown key → only `claudeAiOauth` and
  `organizationUuid` change, everything else byte-identical, key order kept.
- `PatchConfig`: large `.claude.json`-shaped fixture → only `oauthAccount` changes.
- `ParseUsage` on `usage-max20x.json` → 2 limit rows, one active; a second fixture with a session row at
  78 % and severity "warning"; malformed body → error, not zero values.
- `Binding`, `Decide` (active under threshold → nil; over, best target fresh → that target; over, every
  target over or stale or out of pool → nil with reason), `Alerts` (crossings, once per window, reset
  message), `CompareVersion` ("2.1.300" < "2.1.319"; equal; malformed → error).

Adapter tests: `loginfile` round-trip in a temp dir keeping mode 0600 and unknown keys; `usageapi` and
`npmreg` against `httptest` (200, 401, 500, bad JSON, timeout); `vault/file` and `vault/dpapi` round-trip
(dpapi test runs on Windows only); `claudecli` against a fake `claude` binary (Go test binary pattern, like
Fieldwork's fake-claude.exe) for `--version`, `update`, `auth status --json`; `lock` blocks a second holder.

Core tests with testkit fakes, one scenario each: capture of a new login; snapshot of refreshed tokens before
a switch; switch success; switch with expired target → refresh called once; verify fails → previous login
restored and error surfaced; unreadable usage → state red, no auto-switch; auto-switch fires exactly once
and posts the notification; undo; update success and failure; stale accounts excluded.

End-to-end (scripted, on a test copy with `CCSWITCH_DATA_DIR` + `CCSWITCH_CLAUDE_DIR` pointing at a copy of
`~/.claude` holding the maintainer's login): `ccswitch status` reads real usage; `ccswitch add` with his second
account (he does the browser step); `ccswitch use <second>`; `claude auth status --json` in that config dir
shows the second email and its usage reads 200; `use <first>` again and the first still works (V2). Then the
same from the tray on the test copy. Only after this does the real `~/.claude` get used.

## 6. Verifications the build must perform (facts not yet established)

- V1 `claude auth login` in a scratch `CLAUDE_CONFIG_DIR` writes `.credentials.json` and `.claude.json`
  there (if `.claude.json` stays at `~/`, harvest `oauthAccount` from `auth status --json` instead).
- V2 After A→B→A the A tokens still work (Claude Code did not revoke them on login). If they don't, the
  switcher must refresh A immediately after capture-from-scratch, before anything else can revoke.
- V3 Does a running `claude` session pick up swapped tokens? Test: long interactive session, switch, send a
  message, check which account's usage moved. Outcome decides the notification text ("new sessions and
  `claude --continue`" vs nothing).
- V4 Refresh endpoint (F4) works for an inactive account and returns a new refresh token.
- V5 macOS: Keychain item name/shape and `security` adapter; notifications from an unsigned Wails app.
  Ships untested until a Mac is available; the tray is still built for darwin in CI.
- V6 `claude update` run from a process without a console (tray) installs and `--version` changes.

## 7. Work packages and order

| WP | Scope | Depends on |
|---|---|---|
| 1 | domain + ports + testkit fakes (the contract) | — |
| 2 | adapters: loginfile, usageapi, npmreg, vault/file, vault/dpapi, claudecli, lock, execx, paths | 1 |
| 3 | core Switcher + state cache | 1 (fakes), merges after 2 |
| 4 | CLI (all commands incl. watch, statusline) | 2, 3 |
| 5 | tray app (Wails v3): menu, icon, notifications, autostart, Add/Update actions | 2, 3 |
| 6 | V1–V4, V6 with the maintainer on a test copy; fix what they reveal; TRY-IT.md | 4, 5 |
| 7 | macOS adapters (loginkeychain, vault/keychain) + CI matrix ubuntu/windows/macos + release zips | 4 |

Lanes: WP-1 alone first. Then WP-2 ∥ WP-3. Then WP-4 ∥ WP-5 ∥ WP-7. WP-6 last, with the maintainer.

## 8. Status

2026-11-09 08:30 — plan written; facts F1–F7 checked; nothing built.
2026-11-09 10:30 — WP-1..5 and WP-7 built by six Opus lanes and merged (main aaca1b5): `task ci` green on
Windows (domain 98 %, core 97 %, cli 97 %, tray model 99 %, adapters 87–100 %), bin/ccswitch.exe and
bin/ccswitch-app.exe built. Run only against copies of the login so far: CLI `check` and the tray's first poll
both read the real weekly usage and versions. WP-6 (TRY-IT.md: V1–V4, V6 with the maintainer's second account) is next;
V5 needs a Mac. Not pushed; no GitHub repo yet.
2026-11-09 12:30 — WP-6 part done on the real login: `check` captured the Example Co seat (vault sealed, no
token in state/log, `.credentials.json` untouched); the tray ran on it (docs/tray-menu.png); "Update now"
installed Claude Code 2.1.319 (V6). A read that starts failing now sends one notification "Usage unreadable".
Pushed to github.com/diegofal/ccswitch (private); first CI run: macOS and Windows green, ubuntu needed GTK 4 /
WebKitGTK 6.0 packages (Wails v3 default) — fixed. Left: V1–V4 with the maintainer's second account (TRY-IT.md steps
2–4), V5 on a Mac.
2026-11-09 13:30 — the maintainer added home@example.net, dev@example.org and seat@example.com
(V1 passed; V2 passed: A→B→A within 15 s, both tokens still read usage). The Example account showed
"unreadable: limits[0]: no resets_at": the endpoint returns the session row with `resets_at: null` (0 %, not
active) while no 5-hour window is open. Fixed: a null reset is a limit without a window (shown "—", alert key
"none", never pruned); fixture usage-session-null-reset.json; scripts/usageraw.go prints an account's raw
response (no secret) for the next shape surprise. Left: V3, V5.
