# Windows and Copilot steps for the owner

Goal: answer the open questions in `docs/discovery.md` and collect real samples on Windows and with the Copilot account. About 30 minutes. Nothing here changes your real assistant settings: each run uses a throw-away config (`claude --settings`, a temporary `COPILOT_HOME`, temporary `XDG_*` dirs). Each stage prints what it did; send back what the last stage lists.

All commands are PowerShell (Windows PowerShell 5.1 or PowerShell 7) unless stated. Use Windows Terminal.

## Stage 0: prerequisites, repo and Go

Working directory: any folder for code, e.g. `C:\dev`.

On the Mac first, so the branch exists on GitHub (the agent never pushes):

```
git -C ~/dev/rainbow-statusline push -u origin feat/v1
```

On Windows, install what the stages need (Git for the clone, GitHub CLI for the private repo, Go for the scrubber; `winget` ships with current Windows 10/11):

```
winget install --id Git.Git -e
winget install --id GitHub.cli -e
winget install --id GoLang.Go -e
```

Open a new terminal so `git`, `gh` and `go` are on PATH, then sign in to GitHub (choose GitHub.com, HTTPS, and log in with the browser):

```
gh auth login
gh auth status
```

Expected: `Logged in to github.com account milczarekIT`. Then:

```
cd C:\dev
gh repo clone milczarekIT/rainbow-statusline
cd rainbow-statusline
git switch feat/v1
go version
go test ./scripts/
```

Expected: `go version go1.2x ... windows/amd64` and `ok  github.com/milczarekIT/rainbow-statusline/scripts`.
If you do not want to install Go, skip it: the stages still collect samples, but the scrub step prints "Go is not installed" and the raw files stay private (do not send them; tell the agent and it will arrange another way).

## Stage 1: versions and data locations

Working directory: `C:\dev\rainbow-statusline`

```
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\collect-samples.ps1 -Stages info -Out C:\dev\rs-collected
```

Expected: a list of versions (`claude`, `copilot`, `opencode`, `rtk`, `go`), the Copilot statusLine lines, one `found` or `missing` line per data location (RTK database, claude-mem database, Caveman and Ponytail flags, OpenCode config and database), `rtk gain` output if RTK is installed, and `127.0.0.1:37777 -> HTTP 200` if claude-mem is running. The same text is saved in `C:\dev\rs-collected\info.txt`.

If a tool you use is `missing` at the expected place, find the real place and write it down, e.g.:

```
Get-ChildItem -Recurse -Filter history.db $env:LOCALAPPDATA, $env:APPDATA, $HOME -ErrorAction SilentlyContinue | Select-Object -First 5 FullName
Get-ChildItem -Recurse -Filter claude-mem.db $HOME -ErrorAction SilentlyContinue | Select-Object -First 5 FullName
```

`info.txt` contains your full user paths: send it to the agent only, never paste it into the repo.

Send back: `C:\dev\rs-collected\info.txt` plus the output of those two commands if anything was missing.

## Stage 2: Claude Code on Windows

Working directory: a folder you already trust in Claude Code (so the trust dialog does not appear), e.g. the repo.

```
cd C:\dev\rainbow-statusline
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\collect-samples.ps1 -Stages claude -Out C:\dev\rs-collected
```

What happens: Claude Code starts with a temporary settings file whose status line command saves its stdin. Type `say ok`, wait for the answer, type `/exit`.

Expected: while it runs the status line shows the word `captured`. If it shows an error or nothing, note exactly what is shown (that tells which shell Claude Code uses on Windows). After exit the script prints `scrubbed samples in ...` and the leak check prints nothing.

Check: `Get-ChildItem C:\dev\rs-collected\raw\claude-*.json | Measure-Object` shows at least a few files.

## Stage 3: OpenCode on Windows

Part A, session export (one free-model prompt, no account needed):

```
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\collect-samples.ps1 -Stages opencode -Out C:\dev\rs-collected
```

Expected: OpenCode answers `ok`, then `opencode-export.json` appears in `C:\dev\rs-collected\raw` and a scrubbed copy in `C:\dev\rs-collected\scrubbed`.

Part B, can a TUI plugin run an executable on Windows without a flashing window (this is a question of looking at the screen). Do not send any prompt in this part.

```
$p = "$env:TEMP\oc-probe"
New-Item -ItemType Directory -Force "$p\data", "$p\config\opencode\plugins", "$p\cache", "$p\state" | Out-Null
Copy-Item scripts\probe\exec-probe.tsx "$p\config\opencode\plugins\"
$url = ([Uri]"$p\config\opencode\plugins\exec-probe.tsx").AbsoluteUri
Set-Content -Encoding ascii "$p\config\opencode\tui.json" ('{"plugin":["' + $url + '"]}')
$env:XDG_DATA_HOME = "$p\data"; $env:XDG_CONFIG_HOME = "$p\config"; $env:XDG_CACHE_HOME = "$p\cache"; $env:XDG_STATE_HOME = "$p\state"
$env:PROBE_LOG = "$p\probe.log"
Set-Location $p
opencode
```

Watch for about 20 seconds without typing anything. Expected: at the bottom a line `probe #N: probe-ok` whose N grows every 2 seconds. Note: does a console window flash or steal focus every 2 seconds (yes or no)? Quit with `Ctrl+C` or `/exit`, then:

```
Get-Content "$env:TEMP\oc-probe\probe.log" | Select-Object -First 5
Remove-Item Env:XDG_DATA_HOME, Env:XDG_CONFIG_HOME, Env:XDG_CACHE_HOME, Env:XDG_STATE_HOME, Env:PROBE_LOG
Set-Location C:\dev\rainbow-statusline
```

Expected log lines: `run=1 ms=... err=none out=probe-ok`. An `err=` other than `none` is the answer to the question.

Part C, a project folder must not supply the binary. Run this with `rainbow-statusline` not on `PATH`, and do not send any prompt.

```
$p = "$env:TEMP\oc-project-check"
New-Item -ItemType Directory -Force "$p\project", "$p\config\opencode", "$p\state" | Out-Null
go build -o "$p\project\rainbow-statusline.exe" .\cmd\rainbow-statusline
$url = ([Uri](Resolve-Path plugin\opencode\tui.tsx).Path).AbsoluteUri
Set-Content -Encoding ascii "$p\config\opencode\tui.json" ('{"plugin":["' + $url + '"]}')
$env:XDG_CONFIG_HOME = "$p\config"; $env:XDG_STATE_HOME = "$p\state"
Remove-Item Env:RAINBOW_STATUSLINE_BIN -ErrorAction SilentlyContinue
Set-Location "$p\project"
opencode
```

Watch for about 20 seconds. Expected: no status bar at the bottom, because the only binary is the project's. A bar with `[RTK` in it means the project's `.exe` was started, which is a security bug. Quit with `Ctrl+C` or `/exit`, then:

```
Remove-Item Env:XDG_CONFIG_HOME, Env:XDG_STATE_HOME
Set-Location C:\dev\rainbow-statusline
Remove-Item -Recurse -Force "$env:TEMP\oc-project-check"
```

Send back: flash yes or no, the log lines, whether the `probe #N` line appeared at all, and whether Part C drew a bar.

## Stage 4: Copilot CLI (Windows and the Copilot account)

Working directory: `C:\dev\rainbow-statusline`. Run this on Windows. If the Copilot account is also usable on the Mac, the same stage works there with `scripts/collect-samples.sh copilot`.

```
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\collect-samples.ps1 -Stages copilot -Out C:\dev\rs-collected
```

What happens: Copilot starts with a temporary `COPILOT_HOME` that holds a copy of your `config.json` and a `settings.json` with the capture status line. If it says you are not signed in, run `/login` inside it. Answer the folder trust prompt with the first option (this session only). Type `say ok`, wait for the answer, type `/exit`.

Expected: the status line shows `captured` below the input (no experimental flag was needed on the Mac). Then the scrub output and an empty leak check.

Questions to answer from this stage:
1. Did the status line show up without any flag or environment variable? If not, what did you have to set?
2. Does `copilot help config` on Windows still describe `statusLine` and `cmd.exe`? (`copilot help config | Select-String statusLine`)
3. If you use Ponytail, Caveman, RTK or claude-mem with Copilot: where does each keep data on Windows? (Same two `Get-ChildItem` searches as stage 1, plus `Get-ChildItem $HOME\.copilot -Recurse -Depth 2`.)

## Stage 4b: paths the installer must survive

Working directory: `C:\dev\rainbow-statusline`. Needs a build of the binary (`go build -o $env:TEMP\rs.exe .\cmd\rainbow-statusline`). Run in a temporary profile: the installer must not touch your real settings during this check. Quoting is only tested as text on the Mac, so this stage is what proves it.

1. Which shell does Claude Code use for the status line? Point a test `statusLine` command (via `claude --settings <file>`) at `echo %COMSPEC% $BASH_VERSION` and report what it draws: a `cmd.exe` path means cmd.exe, a version number means Git Bash.
2. Copy `rs.exe` to `C:\Program Files (x86)\rs-test\rainbow-statusline.exe` and to a UNC path such as `\\localhost\C$\rs-test\rainbow-statusline.exe`. Note: a real install refuses a `$` path, so for the UNC case use a share without `$`, e.g. `New-SmbShare -Name rstest -Path C:\rs-test` and `\\localhost\rstest\rainbow-statusline.exe`. Run each, quoted the way the installer writes it, under both shells, with sample input on stdin:

   ```
   Get-Content testdata\claude\session-after-reply.json | cmd /d /s /c '"C:\Program Files (x86)\rs-test\rainbow-statusline.exe" --agent claude'
   Get-Content testdata\claude\session-after-reply.json | cmd /c '"C:\Program Files (x86)\rs-test\rainbow-statusline.exe" --agent claude'
   Get-Content testdata\claude\session-after-reply.json | bash -c '"C:\Program Files (x86)\rs-test\rainbow-statusline.exe" --agent claude'
   Get-Content testdata\claude\session-after-reply.json | cmd /d /s /c '"\\localhost\rstest\rainbow-statusline.exe" --agent claude'
   Get-Content testdata\claude\session-after-reply.json | bash -c '"\\localhost\rstest\rainbow-statusline.exe" --agent claude'
   ```

   Expected: each prints a status line. Report any that does not; the UNC form under bash is the most likely to fail, because bash collapses `\\`.
3. In live OpenCode with a temporary `XDG_CONFIG_HOME`, copy the plugin to `C:\rs-test\Łukasz K\tui.tsx` and list it in `tui.json` as `file:///C:/rs-test/%C5%81ukasz%20K/tui.tsx`. Expected: the bar appears. If it does not, try the unencoded form and report which one loads. Repeat on the Mac with a path holding a space and `Ł`.

## Stage 5: send back

Send the contents of these (all scrubbed; do not send `raw\`):

```
Get-ChildItem C:\dev\rs-collected\scrubbed
Get-Content C:\dev\rs-collected\info.txt
```

- `C:\dev\rs-collected\scrubbed\*.json`
- `C:\dev\rs-collected\info.txt`
- the answers: Stage 1 missing paths, Stage 2 what the status line showed, Stage 3 flash yes or no and the `probe.log` lines, Stage 4 questions 1 to 3, Stage 4b the shell Claude Code uses, which commands failed, and which plugin URL form loaded.

Before sending, confirm no personal data is left (expected: no output):

```
Get-ChildItem C:\dev\rs-collected\scrubbed -Filter *.json | Select-String -Pattern $env:USERNAME, $env:COMPUTERNAME, '@', 'C:\\\\Users'
```

Cleanup when done: `Remove-Item -Recurse C:\dev\rs-collected, $env:TEMP\oc-probe`.

The agent records the answers in `docs/discovery.md`. Anything in the spec's "Co blokuje upublicznienie" blocker list (Claude Code failing on a system, or no status line at all in Copilot CLI) stops the plan and comes back to you.
