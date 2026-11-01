package integration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/render"
	_ "modernc.org/sqlite"
)

// env points every seam of the package at a temp dir. Cached values are
// computed on the spot, the way a finished refresh would have stored them;
// keys in canned return a fixed value instead.
type env struct {
	dir                     string
	rtkDB                   string
	memDir                  string
	ocDir, ocDB, oldCaveLog string
	canned                  map[string]string
}

func setup(t *testing.T) *env {
	t.Helper()
	e := &env{dir: t.TempDir(), canned: map[string]string{}}
	e.rtkDB = filepath.Join(e.dir, "history.db")
	e.memDir = filepath.Join(e.dir, "claude-mem")
	oldGet, oldIn, oldLook, oldDB := cacheGet, inputDir, lookPath, rtkDBPath
	oldMem, oldSet, oldClaude := claudeMemDir, claudeSettings, claudeDir
	oldOC, oldOCDB, oldLog := openCodeDir, openCodeDB, oldCaveLogPath
	e.ocDir = filepath.Join(e.dir, "opencode")
	e.ocDB = filepath.Join(e.dir, "data", "opencode.db")
	e.oldCaveLog = filepath.Join(e.dir, "old-cache", "cave-sessions")
	openCodeDir = func() string { return e.ocDir }
	openCodeDB = func() string { return e.ocDB }
	oldCaveLogPath = func() string { return e.oldCaveLog }
	cacheGet = func(key string, _ time.Duration) ([]byte, bool) {
		if v, ok := e.canned[key]; ok {
			return []byte(v), true
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v, err := compute(ctx, key)
		return v, err == nil
	}
	inputDir = func() (string, error) { return filepath.Join(e.dir, "in"), nil }
	lookPath = func(string) (string, error) { return "/bin/rtk", nil }
	rtkDBPath = func() string { return e.rtkDB }
	claudeMemDir = func() string { return e.memDir }
	claudeSettings = func() string { return filepath.Join(e.dir, "settings.json") }
	claudeDir = func() string { return e.dir }
	t.Cleanup(func() {
		cacheGet, inputDir, lookPath, rtkDBPath = oldGet, oldIn, oldLook, oldDB
		claudeMemDir, claudeSettings, claudeDir = oldMem, oldSet, oldClaude
		openCodeDir, openCodeDB, oldCaveLogPath = oldOC, oldOCDB, oldLog
	})
	return e
}

func sqlExec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func text(b render.Badge) string {
	var sb strings.Builder
	for _, s := range b.Lines {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sqlOpen(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

func TestCopilotHidesPonytailAndCaveman(t *testing.T) {
	var ids []string
	for _, i := range ForAgent("copilot") {
		ids = append(ids, i.ID())
	}
	if got := strings.Join(ids, ","); got != "rtk,claude-mem" {
		t.Fatalf("copilot: %q", got)
	}
	if len(ForAgent("claude")) != 4 {
		t.Fatal("claude keeps all four")
	}
}
