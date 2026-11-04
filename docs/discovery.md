# Discovery: real data per assistant

Findings from running the real assistants on macOS (2026-11-01), plus what is still open for Windows.
Samples live in `testdata/` (scrubbed, see `testdata/README.md`). Collection scripts: `scripts/collect-samples.sh`, `scripts/collect-samples.ps1`.

Versions seen: Claude Code 2.1.316, Copilot CLI 1.0.105, OpenCode 1.19.5, RTK 0.51.0.

## How samples were captured without touching real settings

| Assistant | Override used | Notes |
|---|---|---|
| Claude Code | `claude --settings <file>` with a `statusLine` command | The file wins over `~/.claude/settings.json`. Needs a folder already trusted (the trust dialog blocks otherwise). Interactive only: `-p` never draws a status line. |
| Copilot CLI | `COPILOT_HOME=<temp dir>` holding a copy of `config.json` (login) and a `settings.json` with `statusLine` | Login survived the copy (token is not in `config.json`). Interactive only; `-p` draws no status line. A folder-trust prompt appears on first run in a new home. |
| OpenCode | `XDG_DATA_HOME`, `XDG_CONFIG_HOME`, `XDG_CACHE_HOME`, `XDG_STATE_HOME` set to a temp dir, then `opencode run --pure -m opencode/big-pickle "..."` and `opencode export <id>` | OpenCode has no status line command; the sample is the session export. `--pure` disables plugins, so the TUI probe runs without it. |

Real settings were not written: `~/.claude/settings.json` and `~/.copilot/config.json` have the same checksum before and after.

## Claude Code

- Input: JSON on stdin, no flag needed. `refreshInterval` is honoured (about one run per second in a live session).
- Samples: `testdata/claude/session-start.json` (first call: `used_percentage`, `current_usage` are `null`, cost 0), `testdata/claude/session-after-reply.json`.
- Fields seen: `session_id`, `transcript_path`, `cwd`, `workspace.{current_dir,project_dir,git_worktree,repo.{host,owner,name}}`, `model.{id,display_name}`, `effort.level`, `thinking.enabled`, `version`, `output_style.name`, `cost.{total_cost_usd,total_duration_ms,total_api_duration_ms,total_lines_added,total_lines_removed}`, `context_window.{context_window_size,total_input_tokens,total_output_tokens,used_percentage,remaining_percentage,current_usage.{input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens}}`, `exceeds_200k_tokens`, `fast_mode`, `rate_limits.{five_hour,seven_day}.{used_percentage,resets_at}` (subscription accounts), `prompt_cache.{warm,ttl,expires_at,requests,misses,hit_ratio,cache_write_tokens,...}` (cache timer: `expires_at` is epoch seconds).
- Percentages and token counts can be `null` at session start; the adapter must treat `null` as missing, not zero.

## Copilot CLI

- Input: JSON on stdin. **No experimental flag needed in 1.0.105**: a `statusLine` block (`type: "command"`, `command`, `padding`, `refreshInterval`) in `<COPILOT_HOME>/settings.json` (default `~/.copilot/settings.json`) is enough. `copilot help environment` has no `STATUS_LINE` variable and `copilot --help` no status flag. The May 2026 `STATUS_LINE` note is obsolete for this version. Still to confirm on Windows.
- `~/.copilot/config.json` is managed by Copilot ("This file is managed automatically"); user settings go to `settings.json`, which does not exist on this Mac until set.
- Command is run with `/bin/sh` on Unix and `cmd.exe` on Windows (from `copilot help config`).
- Samples: `testdata/copilot/session-start.json`, `testdata/copilot/session-after-reply.json`.
- Fields seen: `cwd`, `session_id`, `session_name` (null at start), `transcript_path` (a directory, not a file), `model.{id,display_name,auto_tier,pending_auto_tier}`, `workspace.current_dir`, `username` (null), `remote.connected`, `version`, `cost.{total_api_duration_ms,total_duration_ms,total_lines_added,total_lines_removed,total_premium_requests}`, `context_window.{total_input_tokens,total_output_tokens,total_cache_read_tokens,total_cache_write_tokens,total_reasoning_tokens,total_tokens,last_call_input_tokens,last_call_output_tokens,current_context_tokens,displayed_context_limit,current_context_used_percentage}`, `ai_used.{total_nano_aiu,formatted}`, `allow_all_enabled`.
- Differences from Claude Code the adapter must handle:
  - `context_window.used_percentage`, `remaining_percentage`, `remaining_tokens` and `context_window_size` are `null` even after a reply. The usable context figure is `current_context_used_percentage` (20 after the trivial prompt) against `displayed_context_limit` (200000).
  - No cost in USD: `cost.total_premium_requests` and `ai_used` (nano AIU, `formatted` string) instead.
  - No effort, no cache expiry/TTL, no rate limits. Cache fields are only token counts (`total_cache_read_tokens`, `total_cache_write_tokens`).
  - `model.display_name` changes while running: `Copilot Fusion` before the first reply, `Copilot Fusion → mai-code-1.1-flash` after (auto routing); `model.id` stays `auto`.
- Plugins in Copilot: `~/.copilot/installed-plugins/` only holds `orca.json` here. No Ponytail, Caveman, RTK or claude-mem data for Copilot was found on this Mac, so those fields have no source there (`—`, badges hidden). Not yet checked on Windows.

## OpenCode

- No status line command. The TUI plugin (`~/.config/opencode/plugins/statusline/tui.tsx`) reads the session from the TUI API; nothing arrives on stdin.
- Sample: `testdata/opencode/export-session.json` from `opencode export <session id>`: `{ info, messages[] }`. `info` has `id`, `directory`, `title`, `agent`, `model.{id,providerID,variant}`, `cost`, `tokens.{input,output,reasoning,cache.{read,write}}`, `version`, `time`. Assistant messages carry `modelID`, `providerID`, `cost`, `tokens.{total,input,output,reasoning,cache.{read,write}}`, `time.{created,completed}`, `finish`; parts have `type` (`step-start`, `text`, `step-finish`, `tool`).
- `tokens.total` on a message equals input + output + reasoning + cache read (100417 = 98623 + 2 + 0 + 1792). The session-level `tokens` has no `total`.
- Database: `$XDG_DATA_HOME/opencode/opencode.db` (default `~/.local/share/opencode/opencode.db`). Table `session` columns: `id, project_id, workspace_id, parent_id, slug, directory, path, title, version, share_url, summary_*, metadata, cost, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write, revert, permission, agent, model, time_created, time_updated, time_compacting, time_archived`.
- Config: `$XDG_CONFIG_HOME/opencode/` (`opencode.json`, `tui.json`, `plugins/`). Plugin flag files `.caveman-active` and `.ponytail-active` live there.
- A TUI plugin can run an executable on macOS: `scripts/probe/exec-probe.tsx` ran `echo` every 2 s (24-52 ms per run) and drew the result in the `app_bottom` slot. `opencode --pure` skips plugins, so it must not be used for this test. Windows: not yet checked.

## Data locations of the integrations (macOS)

| Integration | Location | Used for |
|---|---|---|
| RTK | `~/Library/Application Support/rtk/history.db` (SQLite: `commands`, `hook_decisions`); `rtk gain -f json` gives `summary.{total_saved,total_input,...}` | `t:` and `c:` |
| claude-mem | `~/.claude-mem/claude-mem.db` (`observations`, `sdk_sessions`, `session_summaries`, ...); `~/.claude-mem/settings.json`; worker on `127.0.0.1:37777` (`/api/health` answered 200, `/api/context/inject`) | `t:` and `c:` |
| Caveman | Claude Code: flag `~/.claude/.caveman-active` (content `full`); transcripts contain `CAVEMAN MODE ACTIVE — level: full`. OpenCode: `~/.config/opencode/.caveman-active`, `~/.cache/opencode-statusline/cave-sessions` | badge, `c:`, `t:` |
| Ponytail | Claude Code: plugin script `hooks/ponytail-statusline.sh` found via `~/.claude/plugins/installed_plugins.json`; OpenCode: `~/.config/opencode/.ponytail-active` (absent here, default `full`) | badge |
| Claude Code plugin registry | `~/.claude/settings.json` (`enabledPlugins`), `~/.claude/plugins/installed_plugins.json` | which plugins are installed |

Windows equivalents are guesses until checked on Windows: RTK probably `%LOCALAPPDATA%\rtk\history.db` (the script also checks `%APPDATA%\rtk` and `~\.local\share\rtk`), claude-mem `%USERPROFILE%\.claude-mem`, OpenCode `%USERPROFILE%\.config\opencode` and `%USERPROFILE%\.local\share\opencode` (xdg-basedir style), Copilot `%USERPROFILE%\.copilot`.

## Status of the spec's open questions

| Question | Status |
|---|---|
| Copilot: exact stdin JSON fields | Answered above (1.0.105, macOS). |
| Copilot: still needs an experimental flag? | No in 1.0.105 on macOS; confirm on Windows. |
| Copilot: plugin data and Ponytail/Caveman/RTK/claude-mem there | No source found on macOS; not yet checked on Windows. |
| Windows: database and flag locations of the four plugins | Open. |
| Windows: Claude Code and Copilot run the status line command (shell, quoting) | Open, stages 2 and 4. |
| OpenCode plugin runs an executable on Windows without a flashing window | Open, stage 3. |
| Blockers from "Co blokuje upublicznienie" | None so far: Claude Code and Copilot both draw a status line on macOS. Windows can still produce one. |
