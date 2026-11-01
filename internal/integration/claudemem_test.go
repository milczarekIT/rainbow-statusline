package integration

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

const memSchema = `create table observations (id integer primary key, title text, subtitle text, narrative text, facts text, discovery_tokens integer)`

func seedMem(t *testing.T, e *env) {
	t.Helper()
	sqlExec(t, filepath.Join(e.memDir, "claude-mem.db"), memSchema,
		// read cost (77+0+0+2+3)/4 = 20 tokens
		`insert into observations values (1, '`+strings.Repeat("x", 77)+`', null, null, null, 1000)`,
		`insert into observations values (2, 'ignored', null, null, null, 0)`)
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"claude-mem@thedotmack":true}}`)
}

func writeTranscript(t *testing.T, e *env) string {
	t.Helper()
	p := filepath.Join(e.dir, "proj", "s1.jsonl")
	write(t, p, strings.Join([]string{
		`{"type":"assistant","uuid":"u1","message":{"id":"m1","usage":{"output_tokens":10}}}`,
		`{"type":"assistant","uuid":"u2","message":{"id":"m1","usage":{"output_tokens":120}}}`,
		`not json`,
		`{"type":"assistant","uuid":"u3","message":{"id":"m2","usage":{"output_tokens":30}}}`,
		`{"type":"attachment","attachment":{"hookEvent":"SessionStart","content":"x \u001b[1mStats: 1 | 195,257t work | 176,374t saved | 90%\u001b[0m"}}`,
		`{"type":"attachment","attachment":{"hookEvent":"Other","content":"5t work | 5t saved"}}`,
	}, "\n")+"\n")
	write(t, filepath.Join(e.dir, "proj", "s1", "subagents", "a.jsonl"),
		`{"type":"assistant","uuid":"u9","message":{"id":"m3","usage":{"output_tokens":7}}}`+"\n")
	return p
}

func TestMemTotal(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	s := model.Session{Agent: "claude", SessionID: "s1", TranscriptPath: writeTranscript(t, e)}
	b, ok := claudeMem{}.Badge(s)
	// total: 1000 - 20 = 980 saved of 1000 worked; session: 176,374 of 195,257
	if want := "[CLAUDE-MEM] (c: 176.4k · 90% | t: 980 · 98%)"; !ok || text(b) != want {
		t.Fatalf("got %q want %q", text(b), want)
	}
	if strings.Contains(text(b), "~") {
		t.Fatal("claude-mem t: carries ~")
	}
}

func TestMemOff(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"claude-mem@thedotmack":false}}`)
	if got := text(first(claudeMem{}.Badge(model.Session{Agent: "claude"}))); got != "[CLAUDE-MEM off]" {
		t.Fatalf("plugin disabled: %q", got)
	}
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"claude-mem@thedotmack":true}}`)
	e.memDir = filepath.Join(e.dir, "absent")
	if got := text(first(claudeMem{}.Badge(model.Session{Agent: "claude"}))); got != "[CLAUDE-MEM off]" {
		t.Fatalf("no data dir: %q", got)
	}
}

func TestMemMissingAndOtherSchema(t *testing.T) {
	e := setup(t)
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"claude-mem@thedotmack":true}}`)
	write(t, filepath.Join(e.memDir, "keep"), "")
	s := model.Session{Agent: "claude"}
	if got := text(first(claudeMem{}.Badge(s))); got != "[CLAUDE-MEM] (c: — | t: —)" {
		t.Fatalf("missing db: %q", got)
	}
	sqlExec(t, filepath.Join(e.memDir, "claude-mem.db"), `create table observations (id integer primary key, x text)`)
	if got := text(first(claudeMem{}.Badge(s))); got != "[CLAUDE-MEM] (c: — | t: —)" {
		t.Fatalf("other schema: %q", got)
	}
}

func TestTranscriptStats(t *testing.T) {
	e := setup(t)
	out, saved, work, ok := TranscriptStats(writeTranscript(t, e))
	// max per message id (m1 -> 120) + m2 + subagent m3
	if !ok || out != 157 || saved != 176374 || work != 195257 {
		t.Fatalf("got %d %d %d %v", out, saved, work, ok)
	}
	if _, _, _, ok := TranscriptStats(filepath.Join(e.dir, "gone.jsonl")); ok {
		t.Fatal("missing transcript reported ok")
	}
	if _, _, _, ok := TranscriptStats("/etc/passwd"); ok {
		t.Fatal("non-jsonl path accepted")
	}
}

func TestMemInjectFromWorker(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte("Stats: 10 | 2,000t work | 1,000t saved | 50%"))
	}))
	defer srv.Close()
	old := workerURL
	workerURL = srv.URL
	defer func() { workerURL = old }()
	ocInstall(t, e, "claude-mem")
	s := model.Session{Agent: "opencode", SessionID: "ses_1", Cwd: "/a b/c"}
	if got := text(first(claudeMem{}.Badge(s))); got != "[CLAUDE-MEM] (c: 1.0k · 50% | t: 980 · 98%)" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(gotQuery, "platformSource=opencode") || !strings.Contains(gotQuery, "cwd=%2Fa+b%2Fc") {
		t.Fatalf("query %q", gotQuery)
	}
}

func TestMemCopilotHasTotalOnly(t *testing.T) {
	e := setup(t)
	seedMem(t, e)
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"claude-mem@thedotmack":false}}`)
	writeTranscript(t, e)
	s := model.Session{Agent: "copilot", SessionID: "s1", TranscriptPath: filepath.Join(e.dir, "proj")}
	if got := text(first(claudeMem{}.Badge(s))); got != "[CLAUDE-MEM] (c: — | t: 980 · 98%)" {
		t.Fatalf("got %q", got)
	}
	e.memDir = filepath.Join(e.dir, "absent")
	if got := text(first(claudeMem{}.Badge(s))); got != "[CLAUDE-MEM off]" {
		t.Fatalf("no data dir: %q", got)
	}
}
