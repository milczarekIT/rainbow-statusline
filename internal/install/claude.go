package install

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func defaultClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); filepath.IsAbs(d) {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".claude")
}

func prepareClaude(bin string, st *state) ([]action, error) {
	return prepareStatusLine("claude", filepath.Join(defaultClaudeDir(), "settings.json"), bin, st)
}

// prepareStatusLine plans the settings edit Claude Code and Copilot share.
func prepareStatusLine(id, path, bin string, st *state) ([]action, error) {
	cmd, err := shellCommand(goos, bin, "--agent", id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cur, err := readSettings(path, "statusLine")
	if err != nil {
		return nil, err
	}
	raw := cur.bytes
	if !cur.exists {
		raw = []byte("{}\n")
	}
	prev := gjson.GetBytes(raw, "statusLine")
	prevCmd := prev.Get("command").String()
	rec := st.Records[path]
	same := prevCmd == cmd
	ours := rec != nil && prevCmd == rec.Entry

	a := action{Change: Change{Agent: id, Path: path, Summary: "already set: " + cmd}, cur: cur, write: true, next: raw}
	if !same {
		entry := `{"type":"command","command":` + jsonString(cmd) + `,"padding":0,"refreshInterval":1}`
		if a.next, err = setKey(raw, "statusLine", entry); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		a.Summary = "set statusLine to " + cmd
		if !ours {
			a.Replaces = statusLineText(prev)
		}
	}
	a.record = func(s *state) {
		switch {
		case rec == nil && same: // state was lost: our entry is there, the original is not known
			s.Records[path] = &record{Agent: id, Kind: kindStatusLine, Existed: cur.exists, Original: cur.bytes, Entry: cmd}
		case rec == nil:
			s.Records[path] = &record{Agent: id, Kind: kindStatusLine, Existed: cur.exists, Original: cur.bytes, Previous: prev.Raw, Entry: cmd, WroteHash: hash(a.next)}
		case !same:
			rec.WroteHash = nextHash(rec, cur, a.next)
			if !ours {
				rec.Previous = prev.Raw
			}
			rec.Entry = cmd
		}
	}
	return []action{a}, nil
}

// nextHash keeps byte-identical restore only while the file was still exactly
// what we wrote. Otherwise the user's edits are in it and the original would lose them.
func nextHash(rec *record, cur target, next []byte) string {
	if cur.exists && hash(cur.bytes) == rec.WroteHash {
		return hash(next)
	}
	return ""
}

func undoStatusLine(path string, rec *record) (action, error) {
	a := action{Change: Change{Agent: rec.Agent, Path: path}}
	var err error
	if a.cur, err = readTarget(path); err != nil {
		return a, err
	}
	if !a.cur.exists {
		a.note = "missing, left alone"
		return a, nil
	}
	if restoreOriginal(&a, rec) {
		return a, nil
	}
	if a.cur, err = readSettings(path, "statusLine"); err != nil {
		return a, err
	}
	if gjson.GetBytes(a.cur.bytes, "statusLine.command").String() != rec.Entry {
		a.note = "no longer has our status line, left alone"
		return a, nil
	}
	a.write = true
	if rec.Previous != "" {
		a.next, err = sjson.SetRawBytes(a.cur.bytes, "statusLine", []byte(rec.Previous))
		a.note = "our status line removed, the previous one is back"
	} else {
		a.next, err = sjson.DeleteBytes(a.cur.bytes, "statusLine")
		a.note = "our status line removed"
	}
	if err != nil {
		return a, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

// restoreOriginal handles a file that is still exactly what we wrote.
func restoreOriginal(a *action, rec *record) bool {
	if rec.WroteHash == "" || hash(a.cur.bytes) != rec.WroteHash {
		return false
	}
	if rec.Existed {
		a.write, a.next, a.note = true, rec.Original, "original restored"
	} else {
		a.remove, a.note = true, "removed, we had created it"
	}
	return true
}

func statusLineText(v gjson.Result) string {
	if !v.Exists() || v.Type == gjson.Null {
		return ""
	}
	if c := v.Get("command").String(); c != "" {
		return c
	}
	return v.Raw
}

var plainArg = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellCommand builds the command line for the shell the assistant runs it
// with: sh on Unix, cmd.exe on Windows.
func shellCommand(goos, path string, args ...string) (string, error) {
	parts := make([]string, 0, len(args)+1)
	if goos == "windows" {
		// cmd.exe expands %VAR% even inside quotes, and on a /c line there is no escape.
		for _, s := range append([]string{path}, args...) {
			if strings.ContainsAny(s, "%\"\r\n") {
				return "", fmt.Errorf("cmd.exe cannot run %q safely: move it to a folder without %%, quotes or line breaks", s)
			}
		}
		parts = append(parts, `"`+path+`"`)
		for _, s := range args {
			if !plainArg.MatchString(s) {
				s = `"` + s + `"`
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, " "), nil
	}
	for _, s := range append([]string{path}, args...) {
		if !plainArg.MatchString(s) {
			s = "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " "), nil
}
