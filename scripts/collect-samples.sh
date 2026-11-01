#!/usr/bin/env bash
# Collects real status line input from Claude Code, Copilot CLI and OpenCode, and reports where
# RTK, claude-mem, Caveman and Ponytail keep their data.
#
# Nothing in the assistants' real settings or data is written: Claude Code gets a per-run
# `--settings` file, Copilot a throw-away COPILOT_HOME (config.json copied for the login),
# OpenCode throw-away XDG dirs. Raw captures contain private data; they stay in $OUT/raw and
# are scrubbed into $OUT/scrubbed with scripts/scrub-sample.go (needs Go). Never commit raw/.
#
# usage: scripts/collect-samples.sh [info] [claude] [copilot] [opencode]   (default: all)
set -u
cd "$(dirname "$0")/.."
OUT=${OUT:-$PWD/collected/$(date +%Y%m%d-%H%M%S)}
RAW=$OUT/raw
mkdir -p "$RAW"
stages=("$@"); [ ${#stages[@]} -eq 0 ] && stages=(info claude copilot opencode)

exists() { if [ -e "$1" ]; then echo "  found    $1"; else echo "  missing  $1"; fi; }
ver() {
  command -v "$1" >/dev/null 2>&1 || { echo "  $1: not installed"; return; }
  if [ "$1" = go ]; then echo "  go: $(go version)"; else echo "  $1: $("$1" --version 2>&1 | head -1)"; fi
}

info() {
  echo "== versions"; ver claude; ver copilot; ver opencode; ver rtk; ver go
  echo "== Copilot: statusLine needs a flag?"
  if command -v copilot >/dev/null 2>&1; then
    copilot help config 2>&1 | grep -n -i 'statusLine' | head -3
    copilot help environment 2>&1 | grep -n -i 'STATUS_LINE' || echo "  no STATUS_LINE variable in 'copilot help environment'"
    copilot --help 2>&1 | grep -n -i 'status' || echo "  no status-related flag in 'copilot --help'"
  fi
  echo "== data locations"
  local uh=${USERPROFILE:-$HOME}
  exists "$HOME/.claude/settings.json"; exists "$HOME/.claude/.caveman-active"; exists "$HOME/.claude/plugins/installed_plugins.json"
  exists "$HOME/.copilot/settings.json"; exists "$HOME/.copilot/config.json"; exists "$HOME/.copilot/installed-plugins"
  exists "$HOME/.config/opencode/opencode.json"; exists "$HOME/.config/opencode/tui.json"; exists "$HOME/.local/share/opencode/opencode.db"
  exists "$HOME/.claude-mem/claude-mem.db"; exists "$HOME/.claude-mem/settings.json"
  exists "$HOME/Library/Application Support/rtk/history.db"; exists "$HOME/.local/share/rtk/history.db"
  [ -n "${LOCALAPPDATA:-}" ] && { exists "$LOCALAPPDATA/rtk/history.db"; exists "$APPDATA/rtk/history.db"; } || true
  exists "$HOME/.config/opencode/.caveman-active"; exists "$HOME/.config/opencode/.ponytail-active"
  exists "$HOME/.cache/opencode-statusline/cave-sessions"
  if command -v rtk >/dev/null 2>&1; then echo "== rtk"; rtk gain -f json 2>&1 | head -c 400; echo; fi
  if command -v curl >/dev/null 2>&1; then
    echo "== claude-mem worker"; curl -s -m 2 -o /dev/null -w '  127.0.0.1:37777 -> HTTP %{http_code}\n' http://127.0.0.1:37777/api/health || echo "  127.0.0.1:37777 not reachable"
  fi
}

# One-line capture command: copies stdin of every status line run into $RAW/<name>-<n>.json.
make_capture() { # make_capture <name>
  local f=$RAW/capture-$1.sh
  printf '#!/bin/sh\nf="%s/%s-$(date +%%s)-$$.json"\ncat > "$f"\nprintf captured\n' "$RAW" "$1" > "$f"
  chmod +x "$f"; echo "$f"
}

claude_stage() {
  command -v claude >/dev/null 2>&1 || { echo "claude: not installed, skipping"; return; }
  local cap; cap=$(make_capture claude)
  printf '{"statusLine":{"type":"command","command":"%s","refreshInterval":1}}\n' "$cap" > "$RAW/settings-claude.json"
  echo "== Claude Code: type one short message (e.g. 'say ok'), wait for the answer, then /exit."
  echo "   If asked 'Do you trust this folder?' answer as you normally would."
  claude --settings "$RAW/settings-claude.json" --model haiku
}

copilot_stage() {
  command -v copilot >/dev/null 2>&1 || { echo "copilot: not installed, skipping"; return; }
  local cap home=$RAW/copilot-home; cap=$(make_capture copilot)
  mkdir -p "$home"
  [ -f "$HOME/.copilot/config.json" ] && cp "$HOME/.copilot/config.json" "$home/config.json"
  printf '{"statusLine":{"type":"command","command":"%s","refreshInterval":1}}\n' "$cap" > "$home/settings.json"
  echo "== Copilot CLI (throw-away COPILOT_HOME): if it says you are not signed in, run /login inside it."
  echo "   Type one short message (e.g. 'say ok'), wait for the answer, then /exit."
  COPILOT_HOME=$home copilot --model auto
}

opencode_stage() {
  command -v opencode >/dev/null 2>&1 || { echo "opencode: not installed, skipping"; return; }
  local x=$RAW/opencode-xdg; mkdir -p "$x/data" "$x/config" "$x/cache" "$x/state"
  export XDG_DATA_HOME=$x/data XDG_CONFIG_HOME=$x/config XDG_CACHE_HOME=$x/cache XDG_STATE_HOME=$x/state
  echo "== OpenCode (throw-away XDG dirs, free model, one trivial prompt)"
  (cd "$x" && opencode run --pure -m opencode/big-pickle "reply with the single word ok") || return
  local id; id=$(cd "$x" && opencode session list 2>/dev/null | awk '/^ses_/{print $1; exit}')
  [ -n "$id" ] && (cd "$x" && opencode export "$id" > "$RAW/opencode-export.json")
  if command -v sqlite3 >/dev/null 2>&1; then sqlite3 "$x/data/opencode/opencode.db" '.schema session' > "$RAW/opencode-session-schema.sql"; fi
}

# first and last capture of a run (before and after the first reply), or the only file
pick() { ls -tr "$RAW"/$1-*.json 2>/dev/null | sed -n '1p;$p' | sort -u; }

scrub() {
  command -v go >/dev/null 2>&1 || { echo "Go is not installed: scrubbing skipped. Raw files are private; do not share them."; return; }
  mkdir -p "$OUT/scrubbed"
  for f in $(pick claude) $(pick copilot) "$RAW/opencode-export.json"; do
    [ -f "$f" ] || continue
    go run scripts/scrub-sample.go "$f" > "$OUT/scrubbed/$(basename "$f")" || echo "scrub failed: $f"
  done
  echo "== scrubbed samples in $OUT/scrubbed (leak check, expect nothing below):"
  grep -rniE "$(id -un)|$(hostname -s)|@[a-z0-9-]+\.[a-z]|/Users/|/home/|C:\\\\Users" "$OUT/scrubbed" | head -5
}

for s in "${stages[@]}"; do
  case $s in
    info) info | tee "$OUT/info.txt" ;;
    claude) claude_stage ;;
    copilot) copilot_stage ;;
    opencode) opencode_stage ;;
    *) echo "unknown stage: $s" ;;
  esac
done
scrub
echo "== done: $OUT"
