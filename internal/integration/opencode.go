package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

const (
	keyCaveTotalOC = "cave-total-opencode"
	// caveRecordDir holds one empty file per session that started in Caveman's
	// full mode. Creating a file is atomic on every platform and two OpenCode
	// windows cannot lose each other's record, which rewriting one log could.
	caveRecordDir = "opencode-cave-sessions"
	// prefixCaveSeen marks a session as seen, so only its first sight decides.
	prefixCaveSeen = "caves-"
	caveRecordAge  = 60 * time.Second
)

var (
	// Replaced in tests.
	openCodeDir    = defaultOpenCodeDir
	openCodeDB     = defaultOpenCodeDB
	oldCaveLogPath = func() string {
		h, _ := homeDir()
		return filepath.Join(h, ".cache", "opencode-statusline", "cave-sessions")
	}

	openCodeSessionPattern = regexp.MustCompile(`^ses_[0-9A-Za-z_]+$`)
)

// ValidOpenCodeSession accepts only ids of the form OpenCode gives its sessions.
func ValidOpenCodeSession(id string) bool { return openCodeSessionPattern.MatchString(id) }

// xdgDir is the XDG base directory, or the default under home. A relative value
// is ignored: a project folder could supply the files behind it.
func xdgDir(env string, fallback ...string) string {
	if d := os.Getenv(env); filepath.IsAbs(d) {
		return d
	}
	h, _ := homeDir()
	return filepath.Join(append([]string{h}, fallback...)...)
}

func defaultOpenCodeDir() string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "opencode")
}

func defaultOpenCodeDB() string {
	return filepath.Join(xdgDir("XDG_DATA_HOME", ".local", "share"), "opencode", "opencode.db")
}

// ocPlugin reports whether OpenCode has a tool: listed in opencode.json's
// `plugin` or dropped into plugins/, which it loads by itself.
func ocPlugin(name string) bool {
	if raw, err := os.ReadFile(filepath.Join(openCodeDir(), "opencode.json")); err == nil {
		var cfg struct {
			Plugin json.RawMessage `json:"plugin"`
		}
		if json.Unmarshal(raw, &cfg) == nil && strings.Contains(string(cfg.Plugin), name) {
			return true
		}
	}
	entries, err := os.ReadDir(filepath.Join(openCodeDir(), "plugins"))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), name) {
			return true
		}
	}
	return false
}

// RecordCaveSession remembers a session that started in Caveman's full mode.
// OpenCode does not store the system prompt, and Caveman rewrites its flag when
// a session is created, so a session first seen within a minute of its start
// shows the mode it started in; one opened later is never recorded.
func RecordCaveSession(s model.Session) {
	if s.Agent != "opencode" || s.StartedAt == nil || !ValidOpenCodeSession(s.SessionID) {
		return
	}
	if age := time.Since(*s.StartedAt); age >= caveRecordAge || age <= -caveRecordAge {
		return
	}
	dir, err := inputDir()
	if err != nil || os.MkdirAll(filepath.Join(dir, caveRecordDir), 0o700) != nil {
		return
	}
	seen, err := os.OpenFile(filepath.Join(dir, prefixCaveSeen+s.SessionID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return // seen before
	}
	_ = seen.Close()
	raw, err := readFlagIn(openCodeDir(), ".caveman-active")
	if err != nil || strings.TrimSpace(raw) != "full" {
		return
	}
	if f, err := os.OpenFile(filepath.Join(dir, caveRecordDir, s.SessionID), os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_ = f.Close()
	}
}

// caveSessions lists the recorded sessions: this tool's records plus the log of
// the plugin it replaces, which is read but never written.
func caveSessions() ([]string, error) {
	var ids []string
	raw, err := os.ReadFile(oldCaveLogPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ids = append(ids, strings.Split(string(raw), "\n")...)
	dir, err := inputDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, caveRecordDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		ids = append(ids, e.Name())
	}
	seen := map[string]bool{}
	valid := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if ValidOpenCodeSession(id) && !seen[id] {
			seen[id] = true
			valid = append(valid, id)
		}
	}
	return valid, nil
}

// computeCaveTotalOC sums output and reasoning of the recorded sessions and
// their subagents from opencode.db. The ids go in as one bound parameter.
func computeCaveTotalOC(ctx context.Context) ([]byte, error) {
	ids, err := caveSessions()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []byte("0"), nil
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	db, err := OpenReadOnly(openCodeDB())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var total int64
	err = db.QueryRowContext(ctx, `
		select coalesce(sum(tokens_output + tokens_reasoning), 0) from session
		where id in (select value from json_each(?1)) or parent_id in (select value from json_each(?1))`,
		string(list)).Scan(&total)
	if err != nil {
		return nil, err
	}
	return []byte(strconv.FormatInt(total, 10)), nil
}
