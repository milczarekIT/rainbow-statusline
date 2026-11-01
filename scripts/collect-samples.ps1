# Collects real status line input from Claude Code, Copilot CLI and OpenCode, and reports where
# RTK, claude-mem, Caveman and Ponytail keep their data. Windows PowerShell 5.1 and PowerShell 7.
#
# Nothing in the assistants' real settings or data is written: Claude Code gets a per-run
# `--settings` file, Copilot a throw-away COPILOT_HOME (config.json copied for the login),
# OpenCode throw-away XDG dirs. Raw captures contain private data; they stay in $Out\raw and
# are scrubbed into $Out\scrubbed with scripts\scrub-sample.go (needs Go). Never commit raw\.
#
# usage: pwsh -File scripts\collect-samples.ps1 [-Stages info,claude,copilot,opencode] [-Out <dir>]
param(
  [string[]]$Stages = @('info', 'claude', 'copilot', 'opencode'),
  [string]$Out = ''
)
$ErrorActionPreference = 'Continue'
Set-Location (Split-Path -Parent $PSScriptRoot)
if (-not $Out) { $Out = Join-Path (Get-Location) ('collected\' + (Get-Date -Format 'yyyyMMdd-HHmmss')) }
$Raw = Join-Path $Out 'raw'
New-Item -ItemType Directory -Force -Path $Raw | Out-Null
$Utf8 = New-Object System.Text.UTF8Encoding($false)

function Have($name) { [bool](Get-Command $name -ErrorAction SilentlyContinue) }
function Exists($p) { if (Test-Path -LiteralPath $p) { "  found    $p" } else { "  missing  $p" } }
function Ver($name) {
  if (-not (Have $name)) { "  ${name}: not installed"; return }
  $line = if ($name -eq 'go') { & go version 2>&1 } else { & $name --version 2>&1 }
  "  ${name}: $(($line | Select-Object -First 1))"
}

function Info {
  '== versions'; Ver claude; Ver copilot; Ver opencode; Ver rtk; Ver go
  '== shell'; "  PowerShell $($PSVersionTable.PSVersion), Windows $([Environment]::OSVersion.Version)"
  '== Copilot: statusLine needs a flag?'
  if (Have copilot) {
    (& copilot help config 2>&1) | Select-String -Pattern 'statusLine' | Select-Object -First 3 | ForEach-Object { $_.Line }
    $ev = (& copilot help environment 2>&1) | Select-String -Pattern 'STATUS_LINE'
    if ($ev) { $ev | ForEach-Object { $_.Line } } else { "  no STATUS_LINE variable in 'copilot help environment'" }
    $fl = (& copilot --help 2>&1) | Select-String -Pattern 'status'
    if ($fl) { $fl | ForEach-Object { $_.Line } } else { "  no status-related flag in 'copilot --help'" }
  }
  '== data locations'
  $h = $HOME
  Exists "$h\.claude\settings.json"; Exists "$h\.claude\.caveman-active"; Exists "$h\.claude\plugins\installed_plugins.json"
  Exists "$h\.copilot\settings.json"; Exists "$h\.copilot\config.json"; Exists "$h\.copilot\installed-plugins"
  Exists "$h\.config\opencode\opencode.json"; Exists "$h\.config\opencode\tui.json"; Exists "$h\.local\share\opencode\opencode.db"
  Exists "$env:APPDATA\opencode"; Exists "$env:LOCALAPPDATA\opencode"
  Exists "$h\.claude-mem\claude-mem.db"; Exists "$h\.claude-mem\settings.json"
  Exists "$env:LOCALAPPDATA\rtk\history.db"; Exists "$env:APPDATA\rtk\history.db"; Exists "$h\.local\share\rtk\history.db"
  Exists "$h\.config\opencode\.caveman-active"; Exists "$h\.config\opencode\.ponytail-active"
  Exists "$h\.cache\opencode-statusline\cave-sessions"
  if (Have rtk) { '== rtk'; $j = (& rtk gain -f json 2>&1 | Out-String); $j.Substring(0, [Math]::Min(400, $j.Length)) }
  '== claude-mem worker'
  try { $r = Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 http://127.0.0.1:37777/api/health; "  127.0.0.1:37777 -> HTTP $($r.StatusCode)" }
  catch { '  127.0.0.1:37777 not reachable' }
}

# Capture command: copies stdin of every status line run into $Raw\<name>-<n>.json.
function MakeCapture($name) {
  $f = Join-Path $Raw "capture-$name.ps1"
  $body = @"
`$t = [Console]::In.ReadToEnd()
`$p = Join-Path '$Raw' ('$name-' + [DateTimeOffset]::Now.ToUnixTimeMilliseconds() + '-' + `$PID + '.json')
[IO.File]::WriteAllText(`$p, `$t, (New-Object System.Text.UTF8Encoding(`$false)))
[Console]::Out.Write('captured')
"@
  [IO.File]::WriteAllText($f, $body, $Utf8)
  "powershell -NoProfile -ExecutionPolicy Bypass -File `"$f`""
}

function StatusLineJson($cmd) {
  @{ statusLine = @{ type = 'command'; command = $cmd; refreshInterval = 1 } } | ConvertTo-Json -Depth 4
}

function ClaudeStage {
  if (-not (Have claude)) { 'claude: not installed, skipping'; return }
  $cmd = MakeCapture 'claude'
  $s = Join-Path $Raw 'settings-claude.json'
  [IO.File]::WriteAllText($s, (StatusLineJson $cmd), $Utf8)
  "== Claude Code: type one short message (e.g. 'say ok'), wait for the answer, then /exit."
  '   If asked "Do you trust this folder?" answer as you normally would.'
  & claude --settings $s --model haiku
}

function CopilotStage {
  if (-not (Have copilot)) { 'copilot: not installed, skipping'; return }
  $cmd = MakeCapture 'copilot'
  $home2 = Join-Path $Raw 'copilot-home'
  New-Item -ItemType Directory -Force -Path $home2 | Out-Null
  if (Test-Path "$HOME\.copilot\config.json") { Copy-Item "$HOME\.copilot\config.json" "$home2\config.json" }
  [IO.File]::WriteAllText((Join-Path $home2 'settings.json'), (StatusLineJson $cmd), $Utf8)
  '== Copilot CLI (throw-away COPILOT_HOME): if it says you are not signed in, run /login inside it.'
  "   Type one short message (e.g. 'say ok'), wait for the answer, then /exit."
  $env:COPILOT_HOME = $home2
  try { & copilot --model auto } finally { Remove-Item Env:\COPILOT_HOME -ErrorAction SilentlyContinue }
}

function OpenCodeStage {
  if (-not (Have opencode)) { 'opencode: not installed, skipping'; return }
  $x = Join-Path $Raw 'opencode-xdg'
  foreach ($d in 'data', 'config', 'cache', 'state') { New-Item -ItemType Directory -Force -Path (Join-Path $x $d) | Out-Null }
  $env:XDG_DATA_HOME = "$x\data"; $env:XDG_CONFIG_HOME = "$x\config"; $env:XDG_CACHE_HOME = "$x\cache"; $env:XDG_STATE_HOME = "$x\state"
  '== OpenCode (throw-away XDG dirs, free model, one trivial prompt)'
  Push-Location $x
  try {
    & opencode run --pure -m opencode/big-pickle 'reply with the single word ok'
    $id = (& opencode session list 2>$null | Select-String -Pattern '^ses_\S+' | Select-Object -First 1)
    if ($id) {
      $sid = ($id.Matches[0].Value)
      $p = Join-Path $Raw 'opencode-export.json'
      [IO.File]::WriteAllText($p, ((& opencode export $sid) -join "`n"), $Utf8)
      if (-not (Test-Path -LiteralPath $p) -or (Get-Item -LiteralPath $p).Length -eq 0) {
        "opencode export produced no output for session $sid; the OpenCode sample is missing"
      }
    }
    if (Have sqlite3) { & sqlite3 "$x\data\opencode\opencode.db" '.schema session' | Out-File -Encoding utf8 (Join-Path $Raw 'opencode-session-schema.sql') }
  } finally {
    Pop-Location
    foreach ($v in 'XDG_DATA_HOME', 'XDG_CONFIG_HOME', 'XDG_CACHE_HOME', 'XDG_STATE_HOME') { Remove-Item "Env:\$v" -ErrorAction SilentlyContinue }
  }
}

# first and last capture of a run (before and after the first reply)
function Pick($prefix) {
  $f = @(Get-ChildItem -Path $Raw -Filter "$prefix-*.json" -ErrorAction SilentlyContinue | Where-Object { $_.Name -notlike 'settings-*' } | Sort-Object LastWriteTime)
  if ($f.Count -gt 0) { $f[0]; if ($f.Count -gt 1) { $f[-1] } }
}

function Scrub {
  if (-not (Have go)) { 'Go is not installed: scrubbing skipped. Raw files are private; do not share them.'; return }
  $dst = Join-Path $Out 'scrubbed'
  New-Item -ItemType Directory -Force -Path $dst | Out-Null
  $files = @(Pick 'claude') + @(Pick 'copilot')
  $ex = Join-Path $Raw 'opencode-export.json'
  if (Test-Path $ex) { $files += Get-Item $ex }
  foreach ($f in $files) {
    & go run scripts/scrub-sample.go $f.FullName (Join-Path $dst $f.Name)
    if ($LASTEXITCODE -ne 0) { "scrub failed: $($f.Name)" }
  }
  "== scrubbed samples in $dst (leak check, expect nothing below):"
  Get-ChildItem $dst -Filter *.json | Select-String -Pattern ([regex]::Escape($env:USERNAME)), ([regex]::Escape($env:COMPUTERNAME)), '@[A-Za-z0-9-]+\.[a-z]', 'C:\\\\Users', '/Users/', '/home/' | Select-Object -First 5
}

foreach ($s in $Stages) {
  switch ($s) {
    'info' { Info | Tee-Object -FilePath (Join-Path $Out 'info.txt') }
    'claude' { ClaudeStage }
    'copilot' { CopilotStage }
    'opencode' { OpenCodeStage }
    default { "unknown stage: $s" }
  }
}
Scrub
"== done: $Out"
