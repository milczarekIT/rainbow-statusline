package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

// Replaced in tests.
var (
	lookPath      = exec.LookPath
	rtkDBPath     = findRTKDB
	rtkBinaryName = "rtk"
	runRTKGain    = func(ctx context.Context) ([]byte, error) {
		cmd := exec.CommandContext(ctx, rtkBinaryName, "gain", "-f", "json")
		hideWindow(cmd)
		cmd.WaitDelay = 2 * time.Second
		return cmd.Output()
	}
)

type rtk struct{}

func (rtk) ID() string { return "rtk" }

func (rtk) Detected() bool {
	_, err := lookPath(rtkBinaryName)
	return err == nil
}

func (r rtk) Badge(s model.Session) (render.Badge, bool) {
	if !r.Detected() {
		return offBadge("RTK"), true
	}
	c := format.Dash
	if saved, input, ok := rtkSessionPair(s); ok {
		c = format.Saved(saved, input, 1)
	}
	ts, ti, ok := cachedPair(keyRTKTotal, ageTotal)
	return badge("RTK", "38;5;141", c, savedText(ts, ti, ok, 1)), true
}

// rtkSessionPair asks the cache for this session's saved and input tokens.
// RTK's history has no session id; see computeRTKHook and computeRTKBash.
func rtkSessionPair(s model.Session) (saved, input int64, ok bool) {
	if !validSessionID(s.SessionID) || s.Agent == "copilot" { // no session link on Copilot
		return 0, 0, false
	}
	if s.Agent == "opencode" {
		var runs []model.BashRun
		for _, r := range s.BashRuns {
			if strings.Contains(r.Cmd, "rtk") {
				runs = append(runs, r)
			}
		}
		if len(runs) == 0 {
			return 0, 0, false
		}
		data, err := json.Marshal(runs)
		if err != nil {
			return 0, 0, false
		}
		key := prefixRTKBash + s.SessionID
		putInput(key, data)
		return cachedPair(key, ageSession)
	}
	return cachedPair(prefixRTKHook+s.SessionID, ageSession)
}

func findRTKDB() string {
	home, err := homeDir()
	if err != nil {
		return ""
	}
	for _, d := range rtkDirs(runtime.GOOS, home, os.Getenv) {
		p := filepath.Join(d, "history.db")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// rtkDirs lists where RTK keeps its history. An unset environment variable
// would leave a relative path, which a project folder could fill, so it is skipped.
func rtkDirs(goos, home string, getenv func(string) string) []string {
	switch goos {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "rtk")}
	case "windows":
		var dirs []string
		for _, v := range []string{"LOCALAPPDATA", "APPDATA"} {
			if base := getenv(v); base != "" {
				dirs = append(dirs, filepath.Join(base, "rtk"))
			}
		}
		return append(dirs, filepath.Join(home, ".local", "share", "rtk"))
	}
	return []string{filepath.Join(home, ".local", "share", "rtk")}
}

func openRTK() (*sql.DB, error) {
	p := rtkDBPath()
	if p == "" {
		return nil, errors.New("rtk: history database not found")
	}
	return OpenReadOnly(p)
}

// computeRTKTotal runs `rtk gain -f json` (no shell) and keeps all-time saved and input.
func computeRTKTotal(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := runRTKGain(ctx)
	if err != nil {
		return nil, err
	}
	var r struct {
		Summary struct {
			Saved *json.RawMessage `json:"total_saved"`
			Input *json.RawMessage `json:"total_input"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out, &r); err != nil || r.Summary.Saved == nil || r.Summary.Input == nil {
		return nil, errors.New("rtk: unexpected gain output")
	}
	saved, err1 := strconv.ParseInt(string(*r.Summary.Saved), 10, 64)
	input, err2 := strconv.ParseInt(string(*r.Summary.Input), 10, 64)
	if err1 != nil || err2 != nil {
		return nil, errors.New("rtk: unexpected gain numbers")
	}
	return pairText(saved, input), nil
}

// computeRTKHook matches each `commands` row to this session's hook rewrites
// (hook_decisions.session_id) by text and start time: the row's start must lie
// between 1 s before and 30 s after the rewrite. The id is a bound parameter,
// never part of the SQL text. The match runs in Go, not in one correlated SQL
// query: that query rescans every hook decision for every command and took
// minutes on a real history.
func computeRTKHook(ctx context.Context, id string) ([]byte, error) {
	if !validSessionID(id) {
		return nil, errors.New("rtk: invalid session id")
	}
	db, err := openRTK()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	type rewrite struct {
		at  time.Time
		cmd string
	}
	var rewrites []rewrite
	var first sql.NullString
	if err := db.QueryRowContext(ctx, `select min(timestamp) from hook_decisions where session_id = ?1`, id).Scan(&first); err != nil {
		return nil, err
	}
	if !first.Valid {
		return pairText(0, 0), nil
	}
	hooks, err := db.QueryContext(ctx,
		`select timestamp, rewritten_cmd from hook_decisions where session_id = ?1 and rewritten_cmd is not null`, id)
	if err != nil {
		return nil, err
	}
	defer hooks.Close()
	for hooks.Next() {
		var ts, cmd string
		if err := hooks.Scan(&ts, &cmd); err != nil {
			return nil, err
		}
		if at, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			rewrites = append(rewrites, rewrite{at, cmd})
		}
	}
	if err := hooks.Err(); err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		select timestamp, rtk_cmd, coalesce(exec_time_ms, 0), saved_tokens, input_tokens
		from commands where timestamp >= ?1`, first.String)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var saved, input int64
	for rows.Next() {
		var ts, cmd string
		var execMs, s, i int64
		if err := rows.Scan(&ts, &cmd, &execMs, &s, &i); err != nil {
			return nil, err
		}
		end, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil || cmd == "" {
			continue
		}
		start := end.Add(-time.Duration(execMs) * time.Millisecond)
		for _, h := range rewrites {
			if d := start.Sub(h.at); d >= -time.Second && d <= 30*time.Second && strings.Contains(h.cmd, cmd) {
				saved += s
				input += i
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pairText(saved, input), nil
}

// computeRTKBash is the OpenCode matcher: RTK logs no hook decision there, so a
// row belongs to the session when its rtk_cmd is inside one of the session's
// bash commands and it finished within a second of that command's run window.
func computeRTKBash(ctx context.Context, key string) ([]byte, error) {
	if !validSessionID(strings.TrimPrefix(key, prefixRTKBash)) {
		return nil, errors.New("rtk: invalid session id")
	}
	raw, err := getInput(key)
	if err != nil {
		return nil, err
	}
	var runs []model.BashRun
	if err := json.Unmarshal(raw, &runs); err != nil || len(runs) == 0 {
		return nil, errors.New("rtk: no bash runs")
	}
	since := runs[0].Start
	for _, r := range runs {
		since = min(since, r.Start)
	}
	db, err := openRTK()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`select timestamp, rtk_cmd, saved_tokens, input_tokens from commands where timestamp >= ?1`,
		time.UnixMilli(since-1000).UTC().Format("2006-01-02T15:04:05.000000+00:00"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var saved, input int64
	for rows.Next() {
		var ts, cmd string
		var s, i int64
		if err := rows.Scan(&ts, &cmd, &s, &i); err != nil {
			return nil, err
		}
		at, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil || cmd == "" {
			continue
		}
		end := at.UnixMilli()
		for _, r := range runs {
			if strings.Contains(r.Cmd, cmd) && end >= r.Start-1000 && end <= r.End+1000 {
				saved += s
				input += i
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return pairText(saved, input), nil
}
