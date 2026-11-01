# Rainbow Statusline

One status line for AI coding assistants. It shows where you are, which plugins are active, the model, how full the context is, what the session costs and how long the prompt cache lives, in the same layout in Claude Code, Copilot CLI and OpenCode.

```
user@host:project
[PONYTAIL] [CAVEMAN] (c: ~156.1k | t: ~185.8M) [RTK] (c: 1.1k · 28.5% | t: 38.3M · 55.0%) [CLAUDE-MEM] (c: 403.2k · 87% | t: 67.5M · 92%)
Opus 5.5 (medium) │ ctx ▰▰▰▱▱▱▱▱ 118.1k / 200.0k (59.0%) · Σout 84.1k │ session ~$8.67 ‖ ⚡ read 8.2k · write 0 · ⏱ 2:32 1h
```

It is a single program with no runtime dependencies (no jq, sqlite3, bash, perl or Node). It sends nothing anywhere: the only network call is to the local claude-mem worker on `127.0.0.1`.

## Install

macOS and Windows are supported. Linux is not promised.

| System | Command |
|---|---|
| macOS, Homebrew | `brew install --cask milczarekIT/tap/rainbow-statusline` |
| macOS, script | `curl -fsSL https://raw.githubusercontent.com/milczarekIT/rainbow-statusline/main/install.sh \| sh` |
| Windows, Scoop | `scoop bucket add milczarekIT https://github.com/milczarekIT/scoop-bucket` then `scoop install rainbow-statusline` |
| Windows, script | `irm https://raw.githubusercontent.com/milczarekIT/rainbow-statusline/main/install.ps1 \| iex` |
| Either | download the archive for your system from the GitHub Releases page and put `rainbow-statusline` on your PATH |

These commands download from GitHub without signing in, so they work only while the repository is public. For a private copy, sign in with the GitHub CLI and download the release yourself:

```
gh release download --repo milczarekIT/rainbow-statusline --pattern 'rainbow-statusline_*' --pattern checksums.txt
```

The scripts check the download against `checksums.txt` before installing and refuse on a mismatch. They install for the current user only (`~/.local/bin` on macOS, `%LOCALAPPDATA%\Programs\rainbow-statusline` on Windows), never use sudo, and tell you what to do about PATH: on macOS they print the line to add to your shell profile, on Windows they add the folder to your user PATH and say so.

## First run

Run the program with no arguments in a terminal:

```
rainbow-statusline
```

The first time, a short guide lists the assistants it finds, lets you pick which items to show and how dense the line is, and shows a live preview. Nothing in your assistants' settings changes until you confirm, and every file is backed up first. If an assistant already has a different status line, the guide shows it and asks before replacing it. Afterwards the same command opens a single configuration screen with a list of items and a live preview; `s` saves.

## What works where

| Item | Claude Code | OpenCode | Copilot CLI |
|---|---|---|---|
| Model | yes | yes | yes |
| Effort | yes | variant, when set | — |
| Context (tokens, bar, %) | yes | yes | yes |
| Σout (output tokens) | yes | yes | — |
| Session cost | yes | yes | — (no USD cost) |
| Cache read / write | yes | yes | yes (totals) |
| Cache timer | yes | estimated (`⏱ ~`) | — |
| Ponytail, Caveman | yes | yes | not verified |
| RTK, claude-mem session `c:` | yes | yes | — |
| RTK, claude-mem total `t:` | yes | yes | from the machine-wide data, when installed |
| macOS | verified | verified | verified |
| Windows | to be verified | to be verified | to be verified |

A dash (`—`) means the assistant does not provide the value or it could not be read. A value that is not a measurement carries a `~`. A percentage outside 0–100, a negative count or an unreadable value is shown as `—`, never clamped. Windows support is built and unit-tested, but the real assistants have not yet been run there, so treat it as unverified until this table says otherwise.

## Where every field comes from

| Field | Formula and source |
|---|---|
| `user@host:dir` | Operating system user and host name; the assistant's working directory. |
| Model, effort | The assistant's own model name and effort level (OpenCode: its variant). |
| `ctx` | Tokens in use over the context size. Used tokens are the latest call's input plus cache read plus cache write. The percentage is the assistant's own figure when it is within 2 points of used/size, otherwise used/size. Green below 50%, yellow below 80%, red from 80%. |
| `Σout` | Output tokens summed over the session: the largest output count seen per message in the Claude Code transcript (subagents included), or the sum over messages in OpenCode. |
| `session ~$` | The assistant's own running cost in USD (Claude Code `cost.total_cost_usd`; OpenCode: sum of message costs). It is an estimate, hence the `~`. Copilot reports no USD cost. |
| `⚡ read / write` | Cache read and write tokens of the latest call (Claude Code, OpenCode) or the session totals (Copilot). |
| `⏱` | Time left on the prompt cache, from the assistant's own expiry (Claude Code). OpenCode gives no expiry, so it is the last reply plus five minutes and is marked `~`. Yellow in the last 40% of the cache life, red in the last minute, `✖ expired` after. The suffix `5m` or `1h` is the cache life. |
| `[PONYTAIL]`, `[CAVEMAN]` | The plugin's own mode flag file. The badge is hidden when the plugin is missing or off. |
| RTK `c:` | Tokens saved and percentage for this session: rows of RTK's `history.db` matched to the session through RTK's hook decisions (Claude Code) or the session's bash commands (OpenCode). |
| RTK `t:` | All-time saved and input tokens from `rtk gain -f json`, and their ratio. |
| claude-mem `c:` | The `work | saved` line claude-mem injected into this session: read from the transcript (Claude Code) or from the local worker at `127.0.0.1:37777` (OpenCode). |
| claude-mem `t:` | claude-mem's own formula over its database: saved = sum of `discovery_tokens` minus the tokens needed to read the observations (about one token per four characters); work = sum of `discovery_tokens`; the percentage is saved over work. It is a potential, not a measurement, and is shown without `~`. |
| Caveman `c:` / `t:` | Only in `full` mode: output tokens × 0.65 / 0.35 for the session and for all sessions. This is an estimate from Caveman's benchmark ratio, so both numbers carry `~`. |

A missing RTK or claude-mem shows `[RTK off]` or `[CLAUDE-MEM off]`. Ponytail and Caveman simply disappear.

### Cold start

The `t:` totals are computed in the background and cached for a minute, so drawing never waits on them. The very first draws after install show `—` for them, and the Caveman `t:` total, which reads all past transcripts, can take a few minutes to appear on a large history.

## Uninstall

Run this first, while the program is still installed:

```
rainbow-statusline uninstall
```

It removes only the entries it added to your assistants' settings and restores the status line you had before. Then remove the package:

```
brew uninstall --cask rainbow-statusline     # macOS, Homebrew
scoop uninstall rainbow-statusline           # Windows, Scoop
```

If you used a script, delete the file it printed (`~/.local/bin/rainbow-statusline`, or the install folder on Windows, and its entry in your user PATH). Removing the package first leaves your assistants pointing at a program that no longer exists; run `uninstall` before removing it.
