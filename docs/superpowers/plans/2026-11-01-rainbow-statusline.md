# Rainbow Statusline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One Go binary that draws the Rainbow status line in Claude Code, OpenCode and Copilot CLI on macOS and Windows, with a first-run guide and a configuration TUI.

**Architecture:** Assistant adapters turn each assistant's input into one `model.Session`. Integrations (one file per plugin) add badges from local data. The renderer turns both into segments, printed as ANSI or JSON. Slow numbers come from a file cache refreshed by a detached copy of the binary, so drawing never waits.

**Tech Stack:** Go 1.27, Bubble Tea + Lip Gloss (TUI), `modernc.org/sqlite` (pure Go SQLite), goreleaser, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-11-01-rainbow-statusline-design.md`

## Global Constraints

- Platforms: macOS and Windows. Linux is not promised, but no code may block it.
- Assistants: Claude Code, OpenCode, Copilot CLI.
- A percentage outside 0–100, negative tokens, NaN or an unparsable value renders as `—`. Never clamp silently.
- A value that is not a measurement has `~`. Exception: claude-mem `t:` stays without `~`.
- Caveman saved = `Σout × 0.65/0.35`, only in `full` mode. claude-mem `t:` = claude-mem's own formula. Both unchanged from today.
- Missing plugin: Ponytail and Caveman badges hide; RTK and claude-mem show `[X off]`.
- Drawing never crashes, never prints errors to the assistant's screen, always exits 0.
- No telemetry and no network calls, except reading the local claude-mem worker on `127.0.0.1`.
- Assistant settings change only after explicit confirmation and after a backup.
- No runtime dependency on `jq`, `sqlite3`, `bash`, `perl` or Node.
- UI and README in English. License MIT, added when the repo goes public.
- Source of truth for colours, thresholds and queries: `~/.claude/statusline-command.sh`, `~/.claude/statusline-caveman-total.pl`, `~/.config/opencode/plugins/statusline/tui.tsx`.
- Commit messages follow Conventional Commits, without `Co-Authored-By`. Work on branches, never on `main`; the owner merges.

## Review Focus

1. Empty stdin, truncated JSON or a session with no messages yet: draws what it can with `—`, exits 0 (the home-screen crash in OpenCode was this). Test in Task 3.
2. A settings file with comments, invalid JSON or read-only: the installer refuses, names the file and changes nothing. Test in Task 10.
3. A narrow terminal: lines shorten by dropping details, without broken ANSI or wrapped garbage. Test in Task 3.
4. A SQLite file that is missing, locked by a running `rtk` or has a different schema: `—` within the time limit, no hang. Test in Task 5.
5. A Windows user name or path with spaces or Polish letters: the command written to settings runs through `cmd.exe`. Test in Task 10.

---

### Task 1: Scaffold and safe formatting

**Files:**
- Create: `go.mod`, `cmd/rainbow-statusline/main.go`, `internal/format/format.go`, `.github/workflows/ci.yml`
- Test: `internal/format/format_test.go`

**Interfaces:**
- Produces: `format.Tokens(n int64) string`, `format.Pct(part, whole float64, digits int) (string, bool)`, `format.Saved(saved, whole int64, digits int) string`, `format.Dash = "—"`

- [ ] **Step 1: Install Go**: run `brew install go`. Expected: `go version` prints 1.27.x.
- [ ] **Step 2: Write failing tests**

```go
func TestTokens(t *testing.T)  // 999→"999", 1100→"1.1k", 38300000→"38.3M"
func TestSavedNormal(t *testing.T) // Saved(1100, 3860, 1) == "1.1k · 28.5%"
func TestSavedImpossible(t *testing.T) // Saved(15000, 1000, 1), Saved(-1, 100, 1), Saved(5, 0, 1) all == "—"
func TestPctRejectsNaN(t *testing.T)  // Pct(NaN, 100, 1) and Pct(1, Inf, 1) return ok == false
```

- [ ] **Step 3: Run**: `go test ./internal/format/`. Expected: FAIL, undefined functions.
- [ ] **Step 4: Implement `internal/format/format.go`** and a `main.go` that exits 0. `Tokens` matches the awk `h()` in the existing script.
- [ ] **Step 5: Add CI**: `go vet` and `go test ./...` on `macos-latest` and `windows-latest`.
- [ ] **Step 6: Run**: `go test ./...`. Expected: PASS.
- [ ] **Step 7: Commit**: `feat: add safe number formatting`

### Task 2: Discovery scripts for real data

Answers the spec's open questions and collects the samples every later test uses. Must be ready before the owner's Windows and Copilot access on 2026-11-02.

**Files:**
- Create: `scripts/collect-samples.sh`, `scripts/collect-samples.ps1`, `scripts/scrub-sample.go`, `docs/discovery.md`, `testdata/README.md`
- Test: `scripts/scrub-sample_test.go`

**Interfaces:**
- Produces: `testdata/<assistant>/*.json` samples, `docs/discovery.md` with paths and flags per system

- [ ] **Step 1: Write failing scrub test**: a sample with a user name, an e-mail, a home path and message text comes out with none of them, while numeric fields and keys stay identical.
- [ ] **Step 2: Run**: `go test ./scripts/`. Expected: FAIL.
- [ ] **Step 3: Implement the scrubber**: replace all strings except a key allowlist (model names, ids of plugins, status enums).
- [ ] **Step 4: Run**: `go test ./scripts/`. Expected: PASS.
- [ ] **Step 5: Write both collection scripts**: each prints the assistant versions, the locations of RTK, claude-mem, Caveman and Ponytail data, whether `statusLine` needs a Copilot flag, and installs a temporary status line command that saves stdin to a file. Each restores the settings afterwards.
- [ ] **Step 6: Run on the Mac** for Claude Code, OpenCode and Copilot if signed in. Expected: scrubbed samples in `testdata/`, the settings files unchanged afterwards (`git diff --no-index` against a backup is empty).
- [ ] **Step 7: Give the owner exact steps for Windows**: working directory and commands, per stage. Record the answers in `docs/discovery.md`. Any blocker from the spec's "Co blokuje upublicznienie" table stops the plan and goes to the owner.
- [ ] **Step 8: Commit**: `chore: add data discovery scripts and samples`

### Task 3: Session model, renderer and Claude Code adapter

**Files:**
- Create: `internal/model/session.go`, `internal/render/render.go`, `internal/render/ansi.go`, `internal/render/json.go`, `internal/adapter/claude.go`, `internal/adapter/adapter.go`
- Modify: `cmd/rainbow-statusline/main.go`
- Test: `internal/render/render_test.go`, `internal/adapter/claude_test.go`, `testdata/golden/`

**Interfaces:**
- Consumes: `format.*`
- Produces:
  - `model.Session{User, Host, Cwd, Model, Effort string; ContextUsed, ContextSize, OutTokens, CacheRead, CacheWrite *int64; CostUSD *float64; CacheExpiresAt *time.Time; CacheTTL string; CacheEstimated bool; SessionID, TranscriptPath string; Agent string}` (nil = unknown, renders `—`)
  - `adapter.Parse(agent string, stdin []byte) model.Session`, never fails
  - `render.Segment{Text, FG string; Bold, Dim bool}`, `render.Line []Segment`, `render.Badge{Name string; Lines []Segment}`
  - `render.Draw(s model.Session, badges []Badge, cfg config.View, width int) []Line`
  - `render.ANSI(lines []Line) string`, `render.JSON(lines []Line) []byte`

Task 9 defines `config.View`; until then define it in `internal/config/view.go` with `Density` and `Hidden []string`.

- [ ] **Step 1: Write failing tests**

```go
func TestClaudeSampleMatchesGolden(t *testing.T)  // testdata/claude/*.json → testdata/golden/claude-*.txt, ANSI included
func TestEmptyStdin(t *testing.T)        // Parse("claude", nil), Draw → no panic, ctx shows "—"
func TestTruncatedJSON(t *testing.T)     // half of a sample → same
func TestContextOver100(t *testing.T)    // used 3M of 200k → ctx value "—", no bar overflow
func TestNarrowWidth(t *testing.T)       // width 40 → every line at most 40 visible cells, ANSI balanced
```

Produce the golden files by running today's `~/.claude/statusline-command.sh` on the same samples, with plugin badges removed.

- [ ] **Step 2: Run**: `go test ./internal/...`. Expected: FAIL.
- [ ] **Step 3: Implement** the model, the Claude parser and the renderer. Line 3 colours, bar and thresholds are ported from the `jq` block in the script. Width trimming drops, in order: cache details, Σout, the `/ size (pct)` part.
- [ ] **Step 4: Wire `main.go`**: stdin not a terminal → `--agent` flag (default `claude`) → parse, draw, print, exit 0. A top-level `recover` prints nothing and exits 0.
- [ ] **Step 5: Run**: `go test ./...`. Expected: PASS.
- [ ] **Step 6: Commit**: `feat: draw session line for claude code`

### Task 4: Background cache

**Files:**
- Create: `internal/cache/cache.go`
- Modify: `cmd/rainbow-statusline/main.go`
- Test: `internal/cache/cache_test.go`

**Interfaces:**
- Produces: `cache.Get(key string, maxAge time.Duration) (value []byte, ok bool)`: returns the last value, and when older than `maxAge` starts one detached `rainbow-statusline refresh <key>`. `cache.Register(key string, fn func(ctx context.Context) ([]byte, error))`, `cache.Refresh(key string) error`. Files live in `os.UserCacheDir()/rainbow-statusline/`. A refresh has a 20 s limit.

- [ ] **Step 1: Write failing tests**

```go
func TestGetNeverBlocks(t *testing.T)     // a slow fn (3 s) → Get returns in under 50 ms
func TestAtomicWrite(t *testing.T)        // a failed refresh keeps the previous value
func TestOneRefreshAtATime(t *testing.T)  // 20 concurrent Get → one lock taken
```

- [ ] **Step 2: Run**: `go test ./internal/cache/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Detach through `os/exec` with per-OS attributes (`Setsid` on Unix; `CREATE_NO_WINDOW | DETACHED_PROCESS` on Windows, in `_windows.go`). Lock by creating a file; a lock older than 30 s is stale.
- [ ] **Step 4: Add the `refresh <key>` subcommand** to `main.go`.
- [ ] **Step 5: Run**: `go test ./internal/cache/`. Expected: PASS.
- [ ] **Step 6: Commit**: `feat: add background cache`

### Task 5: RTK and claude-mem integrations

**Files:**
- Create: `internal/integration/integration.go`, `internal/integration/rtk.go`, `internal/integration/claudemem.go`, `internal/integration/sqlite.go`
- Test: `internal/integration/rtk_test.go`, `internal/integration/claudemem_test.go`

**Interfaces:**
- Consumes: `model.Session`, `cache.Get`, `format.Saved`, `render.Badge`
- Produces: `integration.Integration` interface `{ ID() string; Detected() bool; Badge(s model.Session) (render.Badge, bool) }`, `integration.All() []Integration`, `integration.OpenReadOnly(path string) (*sql.DB, error)`

- [ ] **Step 1: Write failing tests** on small SQLite fixtures built in the test:

```go
func TestRTKSessionMatch(t *testing.T)    // fixture rows → c: "1.1k · 28.5%"
func TestRTKNotInstalled(t *testing.T)    // → "[RTK off]"
func TestRTKSavedOverInput(t *testing.T)  // saved > input → "—", never over 100%
func TestMemTotal(t *testing.T)           // formula from the script → "67.5M · 92%", no "~"
func TestMissingDB(t *testing.T)          // → "—"
func TestLockedDB(t *testing.T)           // exclusive lock held → "—" within 1 s
func TestOtherSchema(t *testing.T)        // table without the columns → "—"
```

- [ ] **Step 2: Run**: `go test ./internal/integration/`. Expected: FAIL.
- [ ] **Step 3: Implement** with the queries from today's code. RTK `c:` has two matchers: `hook_decisions` (Claude Code) and the session's bash commands (OpenCode, Task 8). claude-mem `c:` reads the `Nt work | Nt saved` line from the transcript or from `/api/context/inject`. Open SQLite read-only with a 500 ms busy timeout. Paths come from `docs/discovery.md`.
- [ ] **Step 4: Run**: `go test ./internal/integration/`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat: add rtk and claude-mem badges`

### Task 6: Ponytail and Caveman integrations

**Files:**
- Create: `internal/integration/ponytail.go`, `internal/integration/caveman.go`
- Test: `internal/integration/ponytail_test.go`, `internal/integration/caveman_test.go`

**Interfaces:**
- Consumes: `Integration`, `cache.Get`, `format.Tokens`

- [ ] **Step 1: Write failing tests**

```go
func TestPonytailBadge(t *testing.T)    // flag "full" → "[PONYTAIL]", "lite" → "[PONYTAIL:LITE]", "off" or not installed → hidden
func TestCavemanFull(t *testing.T)      // Σout 84100, full → "(c: ~156.2k | t: …)"
func TestCavemanNotFull(t *testing.T)   // "ultra" → badge without numbers
func TestCavemanNoOutput(t *testing.T)  // Σout 0 → "(c: — | …)"
func TestCavemanTotalClaude(t *testing.T) // transcripts fixture: only those with the "level: full" line are summed; max output_tokens per message id
```

- [ ] **Step 2: Run**: `go test ./internal/integration/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Claude Code detection reads `enabledPlugins` and `installed_plugins.json`. The Caveman total is a port of `statusline-caveman-total.pl`, with the same per-file mtime index, as a cache refresh function.
- [ ] **Step 4: Run**: `go test ./internal/integration/`. Expected: PASS.
- [ ] **Step 5: Parity check on the Mac**: `echo "$SAMPLE" | rainbow-statusline` next to `bash ~/.claude/statusline-command.sh`. Expected: the same values in all fields.
- [ ] **Step 6: Commit**: `feat: add ponytail and caveman badges`

### Task 7: Copilot CLI adapter

**Files:**
- Create: `internal/adapter/copilot.go`
- Test: `internal/adapter/copilot_test.go`

**Interfaces:**
- Consumes: `testdata/copilot/*.json` from Task 2, `adapter.Parse`

- [ ] **Step 1: Write failing tests**: the sample maps to the `model.Session` fields it really contains; fields absent from the sample stay nil and render `—`.
- [ ] **Step 2: Run**: `go test ./internal/adapter/`. Expected: FAIL.
- [ ] **Step 3: Implement** the mapping from the sample, not from any write-up of the format.
- [ ] **Step 4: Run**: `go test ./internal/adapter/`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat: add copilot cli adapter`

### Task 8: OpenCode plugin and adapter

**Files:**
- Create: `plugin/opencode/tui.tsx`, `internal/adapter/opencode.go`, `plugin/embed.go`
- Test: `internal/adapter/opencode_test.go`

**Interfaces:**
- Consumes: `render.JSON`, RTK bash matcher from Task 5
- Produces: `rainbow-statusline --agent opencode --format json`, reading `{sessionID, cwd, model, variant, messages: [{tokens, cost, completed}], bash: [{cmd, start, end}], contextLimit}`. `plugin.OpenCodeFiles embed.FS`.

- [ ] **Step 1: Write failing tests**: input with no session → valid JSON with `—`, exit 0 (the home screen); a full session → the same numbers as today's plugin; a fixture with a recorded Caveman session → `t:` from the fixture `opencode.db`, read with WAL.
- [ ] **Step 2: Run**: `go test ./internal/adapter/`. Expected: FAIL.
- [ ] **Step 3: Implement the adapter**, moving Caveman session recording here (first seen under 60 s old with flag `full`).
- [ ] **Step 4: Write `tui.tsx`**: collects data, runs the binary at most once per second with a 2 s limit, and draws the last result through `<span>`. No calculations. Failure keeps the last result. On Windows `execFile` uses `windowsHide: true`.
- [ ] **Step 5: Verify in real OpenCode on the Mac**: pty capture with `pyte` as in the existing setup. Expected: home screen without a crash, and a session with the same values as the current plugin.
- [ ] **Step 6: Commit**: `feat: add opencode plugin`

### Task 9: Configuration

**Files:**
- Create: `internal/config/config.go`
- Modify: `internal/config/view.go`, `internal/render/render.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load() (Config, bool)` (false = no file, triggers the guide), `config.Save(Config) error`, `Config{View; Agents []string}`, `View{Density string /* full|compact|minimal */; Order, Hidden []string; Variants map[string]string}`. File: `os.UserConfigDir()/rainbow-statusline/config.json`.

- [ ] **Step 1: Write failing tests**: round trip; a broken file → defaults and drawing still works; density `compact` drops line 1; `minimal` draws one line with model, context, cost; `Hidden` removes a badge; unknown keys survive a save.
- [ ] **Step 2: Run**: `go test ./internal/config/ ./internal/render/`. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run**: `go test ./...`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat: add configuration file`

### Task 10: Install and uninstall

**Files:**
- Create: `internal/install/install.go`, `internal/install/claude.go`, `internal/install/copilot.go`, `internal/install/opencode.go`, `internal/install/detect.go`
- Modify: `cmd/rainbow-statusline/main.go`
- Test: `internal/install/install_test.go`

**Interfaces:**
- Consumes: `plugin.OpenCodeFiles`
- Produces: `install.Detect() []Agent` where `Agent{ID, Name string; Found bool; SettingsPath, CurrentStatusLine string}`; `install.Plan(ids []string) ([]Change, error)` where `Change{Agent, Path, Summary string; Replaces string}`; `install.Apply([]Change) error`; `install.Uninstall() error`.

- [ ] **Step 1: Write failing tests** in a temporary home:

```go
func TestInstallKeepsOtherKeys(t *testing.T)     // only statusLine changes, key order kept
func TestBackupBeforeWrite(t *testing.T)
func TestUninstallByteIdentical(t *testing.T)    // unchanged file → same bytes as before install
func TestUninstallKeepsLaterEdits(t *testing.T)  // user added a key → it stays, our entry goes, previous statusLine returns
func TestReplacesReported(t *testing.T)          // existing statusLine appears in Change.Replaces
func TestInvalidSettingsRefused(t *testing.T)    // comments, broken JSON, read-only → error naming the file, no change
func TestCommandWithSpaces(t *testing.T)         // path "C:\Users\Łukasz K\bin\rainbow-statusline.exe" → command quoted for cmd.exe, Unix path for sh
```

- [ ] **Step 2: Run**: `go test ./internal/install/`. Expected: FAIL.
- [ ] **Step 3: Implement.** Edit settings with `github.com/tidwall/sjson` to keep formatting. Save original bytes and previous value in the state directory. Copilot also gets the flag if Task 2 found it necessary. OpenCode gets the plugin file and a `tui.json` entry.
- [ ] **Step 4: Run**: `go test ./internal/install/`. Expected: PASS.
- [ ] **Step 5: Add the `uninstall` subcommand to `main.go`**
- [ ] **Step 6: Run**: `go test ./...`. Expected: PASS.
- [ ] **Step 7: Commit**: `feat: add install and uninstall`

### Task 11: Configuration screen (layout A)

**Files:**
- Create: `internal/tui/screen.go`, `internal/tui/preview.go`, `internal/tui/keys.go`
- Modify: `cmd/rainbow-statusline/main.go`
- Test: `internal/tui/screen_test.go`

**Interfaces:**
- Consumes: `config.*`, `install.*`, `render.*`, `integration.All`
- Produces: `tui.RunScreen(cfg config.Config) error`, `tui.Preview(v config.View, s model.Session) string`, `tui.DemoSession() model.Session`

- [ ] **Step 1: Write failing tests** by sending key messages to the model: `space` hides an item and the preview changes; `←→` changes a variant; `shift+↑` reorders; a plugin not detected shows `not detected`; `q` writes nothing; `s` saves and calls install for selected assistants; preview on an 80-column screen stays within 80 cells.
- [ ] **Step 2: Run**: `go test ./internal/tui/`. Expected: FAIL.
- [ ] **Step 3: Implement** per the spec's "TUI (układ A)" section. The preview uses `render.Draw`, so it cannot differ from the real bar.
- [ ] **Step 4: Wire `main.go`**: stdin is a terminal and a config exists → `RunScreen`.
- [ ] **Step 5: Run**: `go test ./internal/tui/`. Expected: PASS.
- [ ] **Step 6: Commit**: `feat: add configuration screen`

### Task 12: First-run guide

**Files:**
- Create: `internal/tui/guide.go`
- Modify: `cmd/rainbow-statusline/main.go`
- Test: `internal/tui/guide_test.go`

**Interfaces:**
- Consumes: `tui.Preview`, `install.Detect`, `install.Plan`, `install.Apply`, `config.Save`
- Produces: `tui.RunGuide() error`

- [ ] **Step 1: Write failing tests**: detected assistants and plugins start selected; `esc` goes back without losing choices; `q` on any step writes no file; step 4 lists every path from `install.Plan` and names a replaced status line; `Install` applies and saves; a second run opens the screen, not the guide.
- [ ] **Step 2: Run**: `go test ./internal/tui/`. Expected: FAIL.
- [ ] **Step 3: Implement** the four steps from the approved mockup: assistants, what to show, density (Full, Compact, Minimal), summary and install. The final message tells the user to restart the assistant.
- [ ] **Step 4: Wire `main.go`**: stdin is a terminal and no config → `RunGuide`.
- [ ] **Step 5: Run**: `go test ./...`. Expected: PASS.
- [ ] **Step 6: Commit**: `feat: add first-run guide`

### Task 13: Distribution and README

**Files:**
- Create: `.goreleaser.yaml`, `install.sh`, `install.ps1`, `.github/workflows/release.yml`, `README.md` (replace the generated one), `LICENSE` (only at publication)

- [ ] **Step 1: Configure goreleaser**: darwin amd64/arm64, windows amd64/arm64, checksums, Homebrew formula and Scoop manifest.
- [ ] **Step 2: Write `install.sh` and `install.ps1`**: download the release for the platform, verify the checksum, install to a user directory, add it to PATH if needed, and print the next command.
- [ ] **Step 3: Test a pre-release while the repo is private**: `gh release download` on Mac and Windows. Anonymous `curl`, `irm`, `brew` and `scoop` work only once the repo is public, so they are verified in Task 14.
- [ ] **Step 4: Write the README**: what it does, install, what works where (table from `docs/discovery.md`), the formula and source of every field, Caveman and claude-mem described as estimates, uninstall (`rainbow-statusline uninstall` before removing the package).
- [ ] **Step 5: Commit**: `chore: add release tooling and readme`

### Task 14: Acceptance and switch-over

- [ ] **Step 1: Mac parity**: run the new and old bars on the same live sessions in Claude Code and OpenCode. Expected: equal values; the only allowed difference is `—` for an impossible value.
- [ ] **Step 2: Replace the owner's old bar** after the owner's consent, keeping the old files as backups.
- [ ] **Step 3: Windows run with the owner**: stages with commands, per the owner's rule "Krok N → commands → zgoda?". Guide, three assistants, uninstall. Expected: a working bar with no manual file edits, or a limitation allowed by the spec's table and listed in the README.
- [ ] **Step 4: Ask the owner for consent to go public.** Only on a yes: add `LICENSE`, create the tap and bucket repositories, change visibility with `gh repo edit --visibility public --accept-visibility-change-consequences`.
- [ ] **Step 5: Verify anonymous install** from a clean machine: `curl`, `irm`, `brew`, `scoop`.
