# Windows and Copilot steps for the owner

Goal: answer the open questions in `docs/discovery.md`, collect real samples on Windows and with the Copilot account, then run the product itself there. About an hour. Nothing here changes your real assistant settings: each run uses a throw-away config (`claude --settings`, a temporary `COPILOT_HOME`, temporary `XDG_*` dirs, a temporary profile). Stage 5 Part B is the one exception outside those settings: it changes your user PATH for a moment and restores it. Each stage says what to send back; the last stage collects it.

All commands are PowerShell (Windows PowerShell 5.1 or PowerShell 7) unless stated. Use Windows Terminal.

## Stage 0: prerequisites, repo and Go

Working directory: any folder for code, e.g. `C:\dev`.

On the Mac first, so the branch exists on GitHub (the agent never pushes):

```
git -C ~/dev/rainbow-statusline push -u origin feat/v1
```

On Windows, install what the stages need (Git for the clone and for Git Bash, GitHub CLI for the private repo, Go for the scrubber and the builds, Node.js only to install the assistants; `winget` ships with current Windows 10/11):

```
winget install --id Git.Git -e
winget install --id GitHub.cli -e
winget install --id GoLang.Go -e
winget install --id OpenJS.NodeJS.LTS -e
```

Open a new terminal so `git`, `gh`, `go` and `npm` are on PATH, then install the assistants:

```
npm install -g @anthropic-ai/claude-code @github/copilot opencode-ai
claude --version
copilot --version
opencode --version
```

Expected: three version numbers. If PowerShell says running scripts is disabled, run `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned` and repeat; `Set-ExecutionPolicy -Scope CurrentUser Undefined` undoes it when you are done. Stage 2 needs a Claude account (`claude`, then `/login`) and Stage 4 a Copilot account (`copilot`, then `/login`). Without one of them, skip that stage and the matching assistant in Stage 5, and say so. OpenCode needs no account.

Then sign in to GitHub (choose GitHub.com, HTTPS, and log in with the browser):

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
If you do not want to install Go, skip it: Stages 1 to 4 still collect samples, but the scrub step prints "Go is not installed" and the raw files stay private (do not send them; tell the agent and it will arrange another way). Stages 4b and 5 build the binary, so they need Go.

Send back: only a failure here, with the command and its output.

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

Send back: what the status line showed. The samples go with the last stage.

## Stage 3: OpenCode on Windows

Working directory: `C:\dev\rainbow-statusline`. Part B and Part C move to a temporary folder and come back.

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

Send back: the answers to the three questions. The samples go with the last stage.

## Stage 4b: paths the installer must survive

Working directory: `C:\dev\rainbow-statusline`. This stage runs no installer, so your real settings are not touched: it only runs the binary by hand and starts Claude Code with a one-run settings file. Quoting is only tested as text on the Mac, so this stage is what proves it. `--%` below keeps PowerShell from rewriting the quotes; the text after it reaches the shell exactly as written.

Build into a folder the later steps share:

```
New-Item -ItemType Directory -Force C:\rs-test | Out-Null
go build -o C:\rs-test\rainbow-statusline.exe .\cmd\rainbow-statusline
```

Step 1, ordinary window: which shell does Claude Code use for the status line?

```
$f = "C:\rs-test\shell-settings.json"
[IO.File]::WriteAllText($f, '{"statusLine":{"type":"command","command":"echo %COMSPEC% $BASH_VERSION"}}')
claude --settings $f
```

Read the status line, then type `/exit`. Expected: a `cmd.exe` path means cmd.exe, a version number means Git Bash.

Step 2a, **elevated window** (right-click Windows Terminal, Run as administrator; only these commands need it), because `Program Files (x86)` and SMB shares are admin-only:

```
New-Item -ItemType Directory -Force 'C:\Program Files (x86)\rs-test' | Out-Null
Copy-Item C:\rs-test\rainbow-statusline.exe 'C:\Program Files (x86)\rs-test\'
New-SmbShare -Name rstest -Path C:\rs-test -ReadAccess $env:USERNAME
```

Expected: the share is listed with `Name rstest`. A real install refuses a `$` path, so `C$` is not used. Leave this window open for the cleanup.

Step 2b, ordinary window, in `C:\dev\rainbow-statusline`. Each runs the binary quoted the way the installer writes it, under both shells:

```
Get-Content testdata\claude\session-after-reply.json | cmd --% /d /s /c ""C:\Program Files (x86)\rs-test\rainbow-statusline.exe" --agent claude"
Get-Content testdata\claude\session-after-reply.json | cmd --% /c ""C:\Program Files (x86)\rs-test\rainbow-statusline.exe" --agent claude"
Get-Content testdata\claude\session-after-reply.json | & "$env:ProgramFiles\Git\bin\bash.exe" --% -c "\"C:\Program Files (x86)\rs-test\rainbow-statusline.exe\" --agent claude"
Get-Content testdata\claude\session-after-reply.json | cmd --% /d /s /c ""\\localhost\rstest\rainbow-statusline.exe" --agent claude"
Get-Content testdata\claude\session-after-reply.json | & "$env:ProgramFiles\Git\bin\bash.exe" --% -c "\"\\localhost\rstest\rainbow-statusline.exe\" --agent claude"
```

Expected: each prints a status line. Report any that does not; the UNC form under bash is the most likely to fail, because bash collapses `\\`.

Step 3, ordinary window: the plugin in a folder with a space and non-ASCII letters, in live OpenCode. Do not send a prompt.

```
$d = 'C:\rs-test\Żółć T'
New-Item -ItemType Directory -Force $d, C:\rs-test\oc\opencode | Out-Null
Copy-Item plugin\opencode\tui.tsx $d
Set-Content -Encoding ascii C:\rs-test\oc\opencode\tui.json '{"plugin":["file:///C:/rs-test/%C5%BB%C3%B3%C5%82%C4%87%20T/tui.tsx"]}'
$env:XDG_CONFIG_HOME = 'C:\rs-test\oc'; $env:RAINBOW_STATUSLINE_BIN = 'C:\rs-test\rainbow-statusline.exe'
opencode
```

Expected: the bar appears at the bottom. If it does not, quit, run this and start `opencode` again, then report which form loads:

```
[IO.File]::WriteAllText('C:\rs-test\oc\opencode\tui.json', '{"plugin":["file:///C:/rs-test/Żółć T/tui.tsx"]}')
```

Quit with `/exit`, then clean up:

```
Remove-Item Env:XDG_CONFIG_HOME, Env:RAINBOW_STATUSLINE_BIN
```

In the elevated window:

```
Remove-SmbShare -Name rstest -Force
Remove-Item -Recurse -Force 'C:\Program Files (x86)\rs-test', C:\rs-test
```

Send back: the shell Claude Code uses, which of the five commands failed and their output, and which plugin URL form loaded.

## Stage 5: the product itself

Stages 1 to 4b prove the pieces. This one installs and runs the real program. Part A uses a throw-away profile, so your settings stay untouched. Part B changes your user PATH for a moment and restores it.

### Part A: guide, real assistants, uninstall

Working directory: `C:\dev\rainbow-statusline`. Open a **new PowerShell window** for this part, because the variables below redirect everything to a temporary profile; closing the window ends that. No elevation.

```
$t = "$env:TEMP\rs-product"
$realCopilot = "$env:USERPROFILE\.copilot\config.json"
New-Item -ItemType Directory -Force "$t\bin", "$t\home\.claude", "$t\copilot", "$t\xdg\config\opencode", "$t\xdg\data", "$t\xdg\state", "$t\xdg\cache", "$t\appdata", "$t\localappdata" | Out-Null
go build -o "$t\bin\rainbow-statusline.exe" .\cmd\rainbow-statusline
if (Test-Path $realCopilot) { Copy-Item $realCopilot "$t\copilot\config.json" }
[IO.File]::WriteAllText("$t\home\.claude\settings.json", '{"theme":"dark","statusLine":{"type":"command","command":"echo old-claude-bar"}}')
[IO.File]::WriteAllText("$t\copilot\settings.json", '{"statusLine":{"type":"command","command":"echo old-copilot-bar"}}')
[IO.File]::WriteAllText("$t\xdg\config\opencode\tui.json", '{"theme":"system"}')
$settings = "$t\home\.claude\settings.json", "$t\copilot\settings.json", "$t\xdg\config\opencode\tui.json"
$before = Get-FileHash $settings
$env:USERPROFILE = "$t\home"; $env:HOME = "$t\home"; $env:APPDATA = "$t\appdata"; $env:LOCALAPPDATA = "$t\localappdata"
$env:COPILOT_HOME = "$t\copilot"; $env:XDG_CONFIG_HOME = "$t\xdg\config"; $env:XDG_DATA_HOME = "$t\xdg\data"; $env:XDG_STATE_HOME = "$t\xdg\state"; $env:XDG_CACHE_HOME = "$t\xdg\cache"
Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
& "$t\bin\rainbow-statusline.exe" --version
& "$t\bin\rainbow-statusline.exe" --help
& "$t\bin\rainbow-statusline.exe"
```

Expected: `dev`, then the usage text, then the guide. On step 1 all three assistants are detected. Leave them ticked and go through the steps. On the last step **every path must be under `rs-product`, and each assistant's old bar must be named as replaced**; if a real path appears, choose Quit and tell the agent. Otherwise choose Install. Expected: `Done. Restart your assistant to see the bar.`

Check what the install wrote and that the screen opens now:

```
Get-Content $settings
& "$t\bin\rainbow-statusline.exe"
```

Expected: each `settings.json` has a `rainbow-statusline` status line command, `theme` is still there, and `tui.json` lists the plugin. The second command opens the configuration screen with all three assistants ticked; press `q`.

Draw in each real assistant, from the repo folder. They do not see your normal logins here: in Claude Code run `/login`; Copilot uses the copied `config.json`, or `/login` if it asks; OpenCode needs no account. Type `say ok`, wait for the answer, note the bar, then `/exit`.

```
claude
copilot
opencode
```

Expected: the same layout in all three, with `user@host:dir` and a `ctx` figure after the reply. Claude Code shows a cost and a cache timer; Copilot shows `$—` and `cache —`. Note any console window that flashes while OpenCode runs.

Uninstall and compare:

```
& "$t\bin\rainbow-statusline.exe" uninstall
$after = Get-FileHash $settings
Compare-Object $before $after -Property Hash, Path
Get-Content $settings
Get-ChildItem -Recurse "$t\appdata\rainbow-statusline" | Select-Object FullName
```

Expected: `Compare-Object` prints nothing. An assistant may rewrite its own settings while it runs; then the hash differs, and `Get-Content` must still show the old bar back and no `rainbow-statusline`. The last command lists the kept configuration and the backups. Close this window, then in a new one:

```
Remove-Item -Recurse -Force "$env:TEMP\rs-product"
```

Testing on your real profile instead is your decision: skip the block that sets the variables and the seeded files, and run the guide only after saying yes to that. The uninstall restores the settings, and the backups stay in `%APPDATA%\rainbow-statusline\state\backups`.

### Part B: install.ps1 on a one-entry user PATH

Working directory: `C:\dev\rainbow-statusline`. Open a new PowerShell window and keep it open until the restore has run. No elevation: the user PATH lives under `HKCU`. Open no other program in between, since it would start with the test PATH.

Save your PATH, in the registry's raw form, and set a single entry that holds a `%VAR%`:

```
$a = "$env:TEMP\rs-script"
New-Item -ItemType Directory -Force $a | Out-Null
$key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
$raw = [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames
$savedPath = $key.GetValue('Path', $null, $raw)
if ($null -ne $savedPath) { $savedKind = $key.GetValueKind('Path'); Set-Content -Encoding utf8 "$env:USERPROFILE\rs-path-backup.txt" $savedPath }
$key.SetValue('Path', '%USERPROFILE%\rs-one-entry', [Microsoft.Win32.RegistryValueKind]::ExpandString)
```

Build a local release and run the script against it:

```
go build -o "$a\rainbow-statusline.exe" .\cmd\rainbow-statusline
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$zip = "rainbow-statusline_windows_$arch.zip"
Compress-Archive -Force -LiteralPath "$a\rainbow-statusline.exe" -DestinationPath "$a\$zip"
"$((Get-FileHash -Algorithm SHA256 "$a\$zip").Hash.ToLower())  $zip" | Set-Content -Encoding ascii "$a\checksums.txt"
$env:RAINBOW_STATUSLINE_BASE_URL = $a; $env:RAINBOW_STATUSLINE_DIR = "$a\installed"
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
$key.GetValue('Path', $null, $raw)
$key.GetValueKind('Path')
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
$key.GetValue('Path', $null, $raw)
& "$a\installed\rainbow-statusline.exe" --version
```

Expected: the script prints `Installed` and `Added ... to your user PATH`. The value is exactly `%USERPROFILE%\rs-one-entry;<TEMP path>\rs-script\installed`, with `%USERPROFILE%` still unexpanded and separated by `;`. The kind is `ExpandString`. The second run does not print `Added` and the value is unchanged. The last command prints `dev`.

Restore, whatever the result was:

```
if ($null -ne $savedPath) { $key.SetValue('Path', $savedPath, $savedKind) } else { $key.DeleteValue('Path') }
$key.GetValue('Path', $null, $raw)
[Environment]::SetEnvironmentVariable('RAINBOW_STATUSLINE_PATH_CHANGED', $null, 'User')
$key.Close()
Remove-Item Env:RAINBOW_STATUSLINE_BASE_URL, Env:RAINBOW_STATUSLINE_DIR
Remove-Item -Recurse -Force $a
Remove-Item "$env:USERPROFILE\rs-path-backup.txt" -ErrorAction SilentlyContinue
```

Expected: the printed value is your original PATH. If the window was closed before this ran, your original is in `%USERPROFILE%\rs-path-backup.txt`; set it back with the `SetValue` line above, using `ExpandString`.

Send back: for Part A, the `Compare-Object` output, what each assistant drew and any flash; for Part B, the PATH value and kind after each run, without the rest of your PATH. Scrub your user name from what you paste.

## Stage 6: send back

Working directory: any. Expected: the list of scrubbed files and the text of `info.txt`.

Send the contents of these (all scrubbed; do not send `raw\`):

```
Get-ChildItem C:\dev\rs-collected\scrubbed
Get-Content C:\dev\rs-collected\info.txt
```

- `C:\dev\rs-collected\scrubbed\*.json`
- `C:\dev\rs-collected\info.txt`
- the answers: Stage 1 missing paths, Stage 2 what the status line showed, Stage 3 flash yes or no and the `probe.log` lines, Stage 4 questions 1 to 3, Stage 4b the shell Claude Code uses, which commands failed, and which plugin URL form loaded, Stage 5 the comparison, the bars and the PATH results.

Before sending, confirm no personal data is left (expected: no output):

```
Get-ChildItem C:\dev\rs-collected\scrubbed -Filter *.json | Select-String -Pattern $env:USERNAME, $env:COMPUTERNAME, '@', 'C:\\\\Users'
```

Cleanup when done: `Remove-Item -Recurse C:\dev\rs-collected, $env:TEMP\oc-probe`.

The agent records the answers in `docs/discovery.md`. Anything in the spec's "Co blokuje upublicznienie" blocker list (Claude Code failing on a system, or no status line at all in Copilot CLI) stops the plan and comes back to you.
