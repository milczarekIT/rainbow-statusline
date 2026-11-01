package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/cache"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

var t0 = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

func stamp(d time.Duration) string { return t0.Add(d).Format("2006-01-02T15:04:05.000000+00:00") }

// Rows that must count carry saved 1 / input 10, rows that must not carry 1000 / 10000.
func cmdRow(id int, end time.Duration, cmd string, execMs int, good bool) string {
	saved, input := 1000, 10000
	if good {
		saved, input = 1, 10
	}
	return `insert into commands values (` + strconv.Itoa(id) + `,'` + stamp(end) + `','x','` + cmd + `',` +
		strconv.Itoa(input) + `,0,` + strconv.Itoa(saved) + `,0,` + strconv.Itoa(execMs) + `,'')`
}

func hookRow(id int, session string, at time.Duration, rewritten string) string {
	return `insert into hook_decisions values (` + strconv.Itoa(id) + `,'` + stamp(at) + `','` + session + `','t','raw','rewrite','` + rewritten + `','0.51')`
}

func TestRTKHookMatcherRules(t *testing.T) {
	e := setup(t)
	sqlExec(t, e.rtkDB, rtkSchema,
		hookRow(1, "sess-1", 0, "rtk first"),
		hookRow(2, "sess-1", 100*time.Second, "rtk up"),
		hookRow(3, "sess-1", 200*time.Second, "rtk low"),
		hookRow(4, "sess-1", 300*time.Second, "rtk exec"),
		hookRow(5, "sess-1", 400*time.Second, "rtk text"),
		hookRow(6, "sess-1", 500*time.Second, "rtk empty"),
		hookRow(7, "other", 600*time.Second, "rtk foreign"),
		// counted: plain, exactly +30 s, exactly -1 s, in the window only through exec_time_ms
		cmdRow(1, 2*time.Second, "rtk first", 0, true),
		cmdRow(2, 130*time.Second, "rtk up", 0, true),
		cmdRow(3, 199*time.Second, "rtk low", 0, true),
		cmdRow(4, 340*time.Second, "rtk exec", 20000, true),
		// not counted: 31 s after, 2 s before, other text at the same time, empty text, another session's rewrite
		cmdRow(10, 131*time.Second, "rtk up", 0, false),
		cmdRow(11, 198*time.Second, "rtk low", 0, false),
		cmdRow(12, 401*time.Second, "rtk other", 0, false),
		cmdRow(13, 501*time.Second, "", 0, false),
		cmdRow(14, 602*time.Second, "rtk foreign", 0, false))
	got, err := computeRTKHook(t.Context(), "sess-1")
	if err != nil || string(got) != "4 40" {
		t.Fatalf("got %q %v, want 4 40", got, err)
	}
}

func TestRTKBashMatcherRules(t *testing.T) {
	e := setup(t)
	base := t0.UnixMilli()
	run := model.BashRun{Cmd: "cd x && rtk ls", Start: base + 10_000, End: base + 20_000}
	sqlExec(t, e.rtkDB, rtkSchema,
		cmdRow(1, 15*time.Second, "rtk ls", 0, true),
		cmdRow(2, 9*time.Second, "rtk ls", 0, true),  // exactly Start-1000
		cmdRow(3, 21*time.Second, "rtk ls", 0, true), // exactly End+1000
		cmdRow(4, 8*time.Second+500*time.Millisecond, "rtk ls", 0, false),
		cmdRow(5, 21*time.Second+500*time.Millisecond, "rtk ls", 0, false),
		cmdRow(6, 15*time.Second, "rtk other", 0, false),
		cmdRow(7, 15*time.Second, "", 0, false))
	// an earlier run keeps the query's own lower bound out of the way of the run under test
	data, _ := json.Marshal([]model.BashRun{{Cmd: "rtk early", Start: base, End: base + 500}, run})
	key := prefixRTKBash + "ses_a"
	putInput(key, data)
	got, err := computeRTKBash(t.Context(), key)
	if err != nil || string(got) != "3 30" {
		t.Fatalf("got %q %v, want 3 30", got, err)
	}
}

func TestComputeRTKTotal(t *testing.T) {
	setup(t)
	old := runRTKGain
	defer func() { runRTKGain = old }()
	cases := []struct {
		name, out, want string
		fail            bool
	}{
		{"good", `{"summary":{"total_saved":5000,"total_input":10000}}`, "5000 10000", false},
		{"bad json", `not json`, "", true},
		{"missing input", `{"summary":{"total_saved":5}}`, "", true},
		{"no summary", `{}`, "", true},
		{"float", `{"summary":{"total_saved":1.5,"total_input":10}}`, "", true},
		{"string", `{"summary":{"total_saved":"5","total_input":10}}`, "", true},
	}
	for _, c := range cases {
		runRTKGain = func(context.Context) ([]byte, error) { return []byte(c.out), nil }
		got, err := computeRTKTotal(t.Context())
		if (err != nil) != c.fail || string(got) != c.want {
			t.Errorf("%s: got %q %v", c.name, got, err)
		}
	}
	runRTKGain = func(context.Context) ([]byte, error) { return nil, errors.New("exit 1") }
	if _, err := computeRTKTotal(t.Context()); err == nil {
		t.Error("failed run gave a value")
	}
}

func TestRTKTotalSavedOverInputShowsDash(t *testing.T) {
	e := setup(t)
	runRTKGain = func(context.Context) ([]byte, error) {
		return []byte(`{"summary":{"total_saved":9000,"total_input":1000}}`), nil
	}
	e.rtkDB = filepath.Join(e.dir, "none.db")
	if got := text(first(rtk{}.Badge(claudeSession()))); got != "[RTK] (c: — | t: —)" {
		t.Fatalf("got %q", got)
	}
}

func TestRegisterCachesRefreshesEveryKind(t *testing.T) {
	e := setup(t)
	for _, v := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(v, e.dir)
	}
	inputDir = cache.Dir
	dir, err := cache.Dir()
	if err != nil || !strings.HasPrefix(dir, e.dir) {
		t.Fatalf("cache dir %q %v", dir, err)
	}
	runRTKGain = func(context.Context) ([]byte, error) {
		return []byte(`{"summary":{"total_saved":5,"total_input":10}}`), nil
	}
	seedRTK(t, e, 570, 2000)
	seedMem(t, e)
	tx := writeTranscript(t, e)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("2,000t work | 1,000t saved"))
	}))
	defer srv.Close()
	workerURL = srv.URL
	defer func() { workerURL = workerBase }()

	RegisterCaches()
	bashKey := prefixRTKBash + "ses_a"
	putInput(bashKey, []byte(`[{"Cmd":"rtk ls -la","Start":1793527202000,"End":1793527203000}]`))
	txKey := hashKey(prefixTx, tx)
	putInput(txKey, []byte(tx))
	memKey := memInjectKey("ses_a", "/w")
	putInput(memKey, []byte("/w"))
	for key, want := range map[string]string{
		keyRTKTotal:              "5 10",
		keyMemTotal:              "980 1000",
		prefixRTKHook + "sess-1": "1140 4000",
		bashKey:                  "570 2000",
		txKey:                    "157 176374 195257",
		memKey:                   "1000 2000",
	} {
		if err := cache.Refresh(key); err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		if v, ok := cache.Get(key, time.Hour); !ok || string(v) != want {
			t.Errorf("%s: got %q %v, want %q", key, v, ok, want)
		}
	}
}

func TestInputThatDoesNotMatchTheKeyIsNotRead(t *testing.T) {
	e := setup(t)
	other := writeTranscript(t, e)
	txKey := hashKey(prefixTx, filepath.Join(e.dir, "mine.jsonl"))
	putInput(txKey, []byte(other))
	if v, err := computeTranscript(t.Context(), txKey); err == nil {
		t.Fatalf("read a transcript the key does not name: %q", v)
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	workerURL = srv.URL
	defer func() { workerURL = workerBase }()
	memKey := memInjectKey("ses_a", "/mine")
	putInput(memKey, []byte("/other"))
	if v, err := computeMemInject(t.Context(), memKey); err == nil || called {
		t.Fatalf("fetched for a cwd the key does not name: %q called=%v", v, called)
	}
}

func TestTranscriptErrorsAreNotPartialSums(t *testing.T) {
	e := setup(t)
	good := writeTranscript(t, e)
	key := hashKey(prefixTx, good)
	putInput(key, []byte(good))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if v, err := computeTranscript(ctx, key); err == nil {
		t.Errorf("cancelled scan gave %q", v)
	}

	// a read error mid-file: a directory named like a subagent file cannot be listed as one,
	// so use a subagent entry that is a dangling symlink
	sub := filepath.Join(strings.TrimSuffix(good, ".jsonl"), "subagents")
	if err := os.Symlink(filepath.Join(e.dir, "missing"), filepath.Join(sub, "z.jsonl")); err != nil {
		t.Skip("no symlinks:", err)
	}
	if v, err := computeTranscript(t.Context(), key); err == nil {
		t.Errorf("unreadable subagent gave %q", v)
	}
}

func TestTranscriptDirectoryWithBracketsAndBadTokens(t *testing.T) {
	e := setup(t)
	p := filepath.Join(e.dir, "pro[j]", "s.jsonl")
	write(t, p, `{"type":"assistant","uuid":"a","message":{"id":"m1","usage":{"output_tokens":5}}}`+"\n"+
		`{"type":"assistant","uuid":"b","message":{"id":"m2","usage":{"output_tokens":1e13}}}`+"\n"+
		`{"type":"assistant","uuid":"c","message":{"id":"m3","usage":{"output_tokens":1e999}}}`+"\n")
	write(t, filepath.Join(e.dir, "pro[j]", "s", "subagents", "a.jsonl"),
		`{"type":"assistant","uuid":"d","message":{"id":"m4","usage":{"output_tokens":7}}}`+"\n")
	if out, _, _, ok := TranscriptStats(p); !ok || out != 12 {
		t.Fatalf("out=%d ok=%v, want 12 (huge values ignored, bracket dir listed)", out, ok)
	}
}

func TestMainTranscriptReadErrorIsAnError(t *testing.T) {
	e := setup(t)
	p := filepath.Join(e.dir, "dir.jsonl") // a directory: opens, then fails to read
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	key := hashKey(prefixTx, p)
	putInput(key, []byte(p))
	if v, err := computeTranscript(t.Context(), key); err == nil {
		t.Fatalf("got %q", v)
	}
}

func TestWorkerRedirectIsNotFollowed(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	contacted := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		_, _ = w.Write([]byte("2,000t work | 1,000t saved"))
	}))
	defer target.Close()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer worker.Close()
	workerURL = worker.URL
	defer func() { workerURL = workerBase }()
	s := model.Session{Agent: "opencode", SessionID: "ses_1", Cwd: "/w"}
	if got := text(first(claudeMem{}.Badge(s))); !strings.Contains(got, "(c: — |") || contacted {
		t.Fatalf("got %q contacted=%v", got, contacted)
	}
}

func TestWorkerNon200IsIgnored(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("2,000t work | 1,000t saved"))
	}))
	defer srv.Close()
	workerURL = srv.URL
	defer func() { workerURL = workerBase }()
	s := model.Session{Agent: "opencode", SessionID: "ses_1", Cwd: "/w"}
	if got := text(first(claudeMem{}.Badge(s))); !strings.Contains(got, "(c: — |") {
		t.Fatalf("got %q", got)
	}
}

func TestMemInjectIsPerSessionAndReadOnce(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	var keys []string
	var ages []time.Duration
	inner := cacheGet
	cacheGet = func(key string, age time.Duration) ([]byte, bool) {
		keys, ages = append(keys, key), append(ages, age)
		return inner(key, age)
	}
	a := model.Session{Agent: "opencode", SessionID: "ses_a", Cwd: "/w"}
	b := a
	b.SessionID = "ses_b"
	claudeMem{}.Badge(a)
	claudeMem{}.Badge(b)
	var memKeys []string
	for i, k := range keys {
		if strings.HasPrefix(k, prefixMemInj) {
			memKeys = append(memKeys, k)
			if ages[i] < 24*time.Hour {
				t.Errorf("%s is refetched after %v", k, ages[i])
			}
		}
	}
	if len(memKeys) != 2 || memKeys[0] == memKeys[1] || !strings.Contains(memKeys[0], "ses_a") {
		t.Fatalf("keys %v", memKeys)
	}
	if got := text(first(claudeMem{}.Badge(model.Session{Agent: "opencode", Cwd: "/w", SessionID: "bad id"}))); !strings.Contains(got, "(c: — |") {
		t.Fatalf("unsafe id: %q", got)
	}
}

func TestRTKDirsSkipUnsetWindowsVariables(t *testing.T) {
	env := map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}
	got := rtkDirs("windows", `C:\Users\u`, func(k string) string { return env[k] })
	if len(got) != 2 || !strings.Contains(got[0], "Roaming") {
		t.Fatalf("got %v", got)
	}
	for _, d := range got {
		if strings.HasPrefix(d, "rtk") || d == "rtk" {
			t.Fatalf("relative dir %q", d)
		}
	}
}

func TestInputFilesAreOwnerOnlyAndOldOnesPruned(t *testing.T) {
	setup(t)
	dir, _ := inputDir()
	putInput("rtko-ses_a", []byte("secret command"))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "rtko-ses_a.in"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode %v %v", info, err)
		}
	}
	old := time.Now().Add(-49 * time.Hour)
	for _, n := range []string{"rtko-ses_a.in", "rtko-ses_a.intmp123", "rtks-x", "tx-abc", "memi-ses_a-1", "rtks-x.lock"} {
		write(t, filepath.Join(dir, n), "x")
		_ = os.Chtimes(filepath.Join(dir, n), old, old)
	}
	write(t, filepath.Join(dir, "rtk-total"), "1 2")
	_ = os.Chtimes(filepath.Join(dir, "rtk-total"), old, old)
	write(t, filepath.Join(dir, "rtks-fresh"), "1 2")
	pruneOld()
	left, _ := os.ReadDir(dir)
	var names []string
	for _, f := range left {
		names = append(names, f.Name())
	}
	if strings.Join(names, ",") != "rtk-total,rtks-fresh" {
		t.Fatalf("left %v", names)
	}
}

func TestRTKBashRowAtTheFirstRunsLowerBoundCounts(t *testing.T) {
	e := setup(t)
	base := t0.UnixMilli()
	sqlExec(t, e.rtkDB, rtkSchema, cmdRow(1, 9*time.Second, "rtk ls", 0, true))
	data, _ := json.Marshal([]model.BashRun{{Cmd: "rtk ls", Start: base + 10_000, End: base + 20_000}})
	key := prefixRTKBash + "ses_a"
	putInput(key, data)
	if got, err := computeRTKBash(t.Context(), key); err != nil || string(got) != "1 10" {
		t.Fatalf("got %q %v", got, err)
	}
}
