package integration

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

const rtkSchema = `
create table commands (id integer primary key, timestamp text not null, original_cmd text not null, rtk_cmd text not null,
  input_tokens integer not null, output_tokens integer not null, saved_tokens integer not null, savings_pct real not null,
  exec_time_ms integer default 0, project_path text default '');
create table hook_decisions (id integer primary key, timestamp text not null, session_id text not null,
  tool_use_id text not null, raw_cmd text not null, decision text not null, rewritten_cmd text, rtk_version text not null);`

func seedRTK(t *testing.T, e *env, savedA, inputA int) {
	t.Helper()
	sqlExec(t, e.rtkDB, rtkSchema,
		`insert into hook_decisions values (1,'2026-11-01T10:00:00.000000+00:00','sess-1','t1','ls','rewrite','rtk ls -la','0.51')`,
		`insert into hook_decisions values (2,'2026-11-01T10:05:00.000000+00:00','sess-1','t2','git status','rewrite','rtk git status','0.51')`,
		`insert into hook_decisions values (3,'2026-11-01T10:00:00.000000+00:00','other','t3','ls','rewrite','rtk ls -la','0.51')`,
		`insert into commands values (1,'2026-11-01T10:00:02.000000+00:00','ls','rtk ls -la',`+strconv.Itoa(inputA)+`,0,`+strconv.Itoa(savedA)+`,0,100,'')`,
		`insert into commands values (2,'2026-11-01T10:05:03.000000+00:00','git status','rtk git status',`+strconv.Itoa(inputA)+`,0,`+strconv.Itoa(savedA)+`,0,100,'')`,
		// before the session's first rewrite, and not rewritten by any hook of this session
		`insert into commands values (3,'2026-11-01T09:00:00.000000+00:00','ls','rtk ls -la',999999,0,999999,0,100,'')`,
		`insert into commands values (4,'2026-11-01T10:06:00.000000+00:00','cat','rtk cat x',999999,0,999999,0,100,'')`)
}

func claudeSession() model.Session { return model.Session{Agent: "claude", SessionID: "sess-1"} }

func TestRTKSessionMatch(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	e.canned[keyRTKTotal] = "5000 10000"
	b, ok := rtk{}.Badge(claudeSession())
	if !ok || text(b) != "[RTK] (c: 1.1k · 28.5% | t: 5.0k · 50.0%)" {
		t.Fatalf("got %q %v", text(b), ok)
	}
}

func TestRTKNotInstalled(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	b, ok := rtk{}.Badge(claudeSession())
	if !ok || text(b) != "[RTK off]" || !b.Lines[0].Dim {
		t.Fatalf("got %q %v", text(b), ok)
	}
}

func TestRTKSavedOverInput(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 3000, 2000)
	e.canned[keyRTKTotal] = "9000 1000"
	b, _ := rtk{}.Badge(claudeSession())
	if got := text(b); got != "[RTK] (c: — | t: —)" {
		t.Fatalf("got %q", got)
	}
}

func TestMissingDB(t *testing.T) {
	e := setup(t)
	e.canned[keyRTKTotal] = "1 2"
	rtkDBPath = func() string { return "" }
	if got := text(first(rtk{}.Badge(claudeSession()))); got != "[RTK] (c: — | t: 1 · 50.0%)" {
		t.Fatalf("got %q", got)
	}
	rtkDBPath = func() string { return e.dir + "/nope.db" }
	if got := text(first(rtk{}.Badge(claudeSession()))); !strings.Contains(got, "(c: — |") {
		t.Fatalf("got %q", got)
	}
}

func TestLockedDB(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	locker, err := sqlOpen(e.rtkDB)
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	if _, err := locker.Exec("begin exclusive"); err != nil {
		t.Fatal(err)
	}
	e.canned[keyRTKTotal] = "5000 10000"
	start := time.Now()
	got := text(first(rtk{}.Badge(claudeSession())))
	if took := time.Since(start); took > time.Second {
		t.Fatalf("took %v", took)
	}
	if got != "[RTK] (c: — | t: 5.0k · 50.0%)" {
		t.Fatalf("got %q", got)
	}
}

func TestOtherSchema(t *testing.T) {
	e := setup(t)
	sqlExec(t, e.rtkDB, `create table commands (id integer primary key, something text)`,
		`create table hook_decisions (id integer primary key)`)
	e.canned[keyRTKTotal] = "5000 10000"
	if got := text(first(rtk{}.Badge(claudeSession()))); got != "[RTK] (c: — | t: 5.0k · 50.0%)" {
		t.Fatalf("got %q", got)
	}
}

func TestRTKUnsafeSessionIDRunsNoQuery(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	e.canned[keyRTKTotal] = "5000 10000"
	s := claudeSession()
	s.SessionID = `sess-1' or '1'='1`
	if got := text(first(rtk{}.Badge(s))); !strings.Contains(got, "(c: — |") {
		t.Fatalf("got %q", got)
	}
	if _, err := compute(t.Context(), prefixRTKHook+s.SessionID); err == nil {
		t.Fatal("compute accepted an unsafe id")
	}
}

func TestRTKOpenCodeBashMatch(t *testing.T) {
	e := setup(t)
	sqlExec(t, e.rtkDB, rtkSchema,
		`insert into commands values (1,'2026-11-01T10:00:02.500000+00:00','ls','rtk ls -la',2000,0,570,0,100,'')`,
		`insert into commands values (2,'2026-11-01T10:00:03.000000+00:00','ls','rtk git log',2000,0,570,0,100,'')`,
		`insert into commands values (3,'2026-11-01T10:30:00.000000+00:00','ls','rtk ls -la',2000,0,570,0,100,'')`)
	base := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC).UnixMilli()
	ocInstall(t, e, "rtk")
	s := model.Session{Agent: "opencode", SessionID: "ses_abc123", BashRuns: []model.BashRun{
		{Cmd: "cd x && rtk ls -la", Start: base + 1000, End: base + 3000},
		{Cmd: "echo hi", Start: base, End: base + 100},
	}}
	e.canned[keyRTKTotal] = "5000 10000"
	if got := text(first(rtk{}.Badge(s))); got != "[RTK] (c: 570 · 28.5% | t: 5.0k · 50.0%)" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenReadOnlyWalFileWithoutSidecars(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/w.db"
	sqlExec(t, p, `pragma journal_mode=wal`, `create table t (a integer)`, `insert into t values (7)`)
	db, err := OpenReadOnly(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("select a from t").Scan(&n); err != nil || n != 7 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := db.Exec("insert into t values (8)"); err == nil {
		t.Fatal("handle is writable")
	}
}

func first[T any](v T, _ bool) T { return v }

func TestRTKCopilotHasTotalOnly(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	e.canned[keyRTKTotal] = "5000 10000"
	b, _ := rtk{}.Badge(model.Session{Agent: "copilot", SessionID: "sess-1"})
	if got := text(b); got != "[RTK] (c: — | t: 5.0k · 50.0%)" {
		t.Fatalf("got %q", got)
	}
}

func TestRTKSessionAboveTheTotalHidesTheTotal(t *testing.T) {
	e := setup(t)
	seedRTK(t, e, 570, 2000)
	e.canned[keyRTKTotal] = "1000 10000"
	if got, want := text(first(rtk{}.Badge(claudeSession()))), "[RTK] (c: 1.1k · 28.5% | t: —)"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
