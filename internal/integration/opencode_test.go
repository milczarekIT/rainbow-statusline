package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

const sessionSchema = `create table session (id text primary key, parent_id text, tokens_output integer, tokens_reasoning integer)`

func ocInstall(t *testing.T, e *env, names ...string) {
	t.Helper()
	for _, n := range names {
		write(t, filepath.Join(e.ocDir, "plugins", n+".ts"), "")
	}
}

func ocSession() model.Session {
	return model.Session{Agent: "opencode", SessionID: "ses_1", Cwd: "/w"}
}

func TestOCPluginInstalledWhenListedOrInPluginsDir(t *testing.T) {
	e := setup(t)
	if ocPlugin("rtk") {
		t.Fatal("nothing installed")
	}
	write(t, filepath.Join(e.ocDir, "opencode.json"), `{"plugin":["file:///x/caveman/plugin.js",["ponytail-pkg",{}]],"note":"rtk"}`)
	if !ocPlugin("caveman") || !ocPlugin("ponytail") {
		t.Fatal("listed plugin not seen")
	}
	if ocPlugin("rtk") {
		t.Fatal("a name outside the plugin list counted")
	}
	write(t, filepath.Join(e.ocDir, "plugins", "rtk.ts"), "")
	if !ocPlugin("rtk") {
		t.Fatal("plugins dir not seen")
	}
	write(t, filepath.Join(e.ocDir, "opencode.json"), `{not json`)
	if ocPlugin("caveman") {
		t.Fatal("broken config counted")
	}
}

func TestOpenCodeDirHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.FromSlash("/x/cfg"))
	if got := defaultOpenCodeDir(); got != filepath.FromSlash("/x/cfg/opencode") {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", filepath.FromSlash("/x/data"))
	if got := defaultOpenCodeDB(); got != filepath.FromSlash("/x/data/opencode/opencode.db") {
		t.Fatalf("got %q", got)
	}
	// A relative value could point into a project folder.
	home, _ := homeDir()
	t.Setenv("XDG_CONFIG_HOME", "rel")
	if got := defaultOpenCodeDir(); got != filepath.Join(home, ".config", "opencode") {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_DATA_HOME", "")
	if got := defaultOpenCodeDB(); got != filepath.Join(home, ".local", "share", "opencode", "opencode.db") {
		t.Fatalf("got %q", got)
	}
}

func TestOpenCodePonytail(t *testing.T) {
	e := setup(t)
	// Enabled for Claude Code only: not installed on OpenCode.
	installPlugin(t, e, "ponytail@ponytail", true)
	write(t, filepath.Join(e.dir, ".ponytail-active"), "full")
	if _, ok := (ponytail{}).Badge(ocSession()); ok {
		t.Fatal("not installed on OpenCode: must hide")
	}
	ocInstall(t, e, "ponytail")
	flagPath := filepath.Join(e.ocDir, ".ponytail-active")
	os.Remove(filepath.Join(e.dir, ".ponytail-active"))
	if b, ok := (ponytail{}).Badge(ocSession()); !ok || text(b) != "[PONYTAIL]" {
		t.Fatalf("no flag means full: %q %v", text(b), ok)
	}
	for flag, want := range map[string]string{"full\n": "[PONYTAIL]", "lite\n": "[PONYTAIL:LITE]", "off": "", "<x>": ""} {
		write(t, flagPath, flag)
		b, ok := (ponytail{}).Badge(ocSession())
		if ok != (want != "") || (ok && text(b) != want) {
			t.Errorf("flag %q: %q %v", flag, text(b), ok)
		}
	}
	// Claude Code still needs its own flag.
	os.Remove(flagPath)
	if _, ok := (ponytail{}).Badge(model.Session{Agent: "claude"}); ok {
		t.Fatal("claude without a flag must hide")
	}
}

func TestOpenCodePonytailRefusesSymlinkFlag(t *testing.T) {
	e := setup(t)
	ocInstall(t, e, "ponytail")
	target := filepath.Join(e.dir, "secret")
	write(t, target, "lite")
	if err := os.Symlink(target, filepath.Join(e.ocDir, ".ponytail-active")); err != nil {
		t.Skip(err)
	}
	if b, ok := (ponytail{}).Badge(ocSession()); ok {
		t.Fatalf("symlink flag: %q", text(b))
	}
}

func TestOpenCodeCaveman(t *testing.T) {
	e := setup(t)
	e.canned[keyCaveTotalOC] = "84100"
	e.canned[keyCaveTotal] = "1"
	s := ocSession()
	s.OutTokens = i64(350)
	if _, ok := (caveman{}).Badge(s); ok {
		t.Fatal("not installed")
	}
	ocInstall(t, e, "caveman")
	if _, ok := (caveman{}).Badge(s); ok {
		t.Fatal("no flag: hidden, as in the plugin")
	}
	flagPath := filepath.Join(e.ocDir, ".caveman-active")
	for flag, want := range map[string]string{
		"full":    "[CAVEMAN] (c: ~650 | t: ~156.2k)",
		"full \n": "[CAVEMAN] (c: ~650 | t: ~156.2k)",
		"ultra":   "[CAVEMAN:ULTRA]",
		"off":     "",
	} {
		write(t, flagPath, flag)
		b, ok := (caveman{}).Badge(s)
		if ok != (want != "") || (ok && text(b) != want) {
			t.Errorf("flag %q: %q %v", flag, text(b), ok)
		}
	}
	write(t, flagPath, "full")
	s.OutTokens = nil
	if b, _ := (caveman{}).Badge(s); !strings.Contains(text(b), "(c: — |") {
		t.Errorf("no output: %q", text(b))
	}
}

func TestOpenCodeRTKAndMemOffFollowInstallNotMachine(t *testing.T) {
	e := setup(t)
	seedMem(t, e) // the data directory exists and the rtk binary is on PATH
	if b, _ := (rtk{}).Badge(ocSession()); text(b) != "[RTK off]" {
		t.Errorf("rtk: %q", text(b))
	}
	if b, _ := (claudeMem{}).Badge(ocSession()); text(b) != "[CLAUDE-MEM off]" {
		t.Errorf("mem: %q", text(b))
	}
	e.canned[keyRTKTotal] = "5 10"
	e.canned[keyMemTotal] = "3 4"
	ocInstall(t, e, "rtk", "claude-mem")
	if b, _ := (rtk{}).Badge(ocSession()); !strings.HasPrefix(text(b), "[RTK] (c: — | t: 5 · 50.0%") {
		t.Errorf("rtk on: %q", text(b))
	}
	if b, _ := (claudeMem{}).Badge(ocSession()); !strings.HasPrefix(text(b), "[CLAUDE-MEM] (c: — | t: 3 · 75%") {
		t.Errorf("mem on: %q", text(b))
	}
	// Claude Code keeps its own rule.
	if b, _ := (rtk{}).Badge(model.Session{Agent: "claude"}); text(b) == "[RTK off]" {
		t.Error("claude rtk follows the binary")
	}
}

// openWALWriter leaves rows only in the WAL: the connection stays open and nothing is checkpointed.
func openWALWriter(t *testing.T, path string, stmts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sqlOpen(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, s := range append([]string{"pragma journal_mode=wal", "pragma wal_autocheckpoint=0", sessionSchema}, stmts...) {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if info, err := os.Stat(path + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("no wal: %v", err)
	}
}

func caveTotal(t *testing.T) string {
	t.Helper()
	v, err := compute(context.Background(), keyCaveTotalOC)
	if err != nil {
		t.Fatal(err)
	}
	return string(v)
}

func TestOpenCodeCaveTotalReadsLiveWALWithBothLogs(t *testing.T) {
	e := setup(t)
	openWALWriter(t, e.ocDB,
		`insert into session values ('ses_old', null, 100, 10)`,
		`insert into session values ('ses_new', null, 1000, 0)`,
		`insert into session values ('ses_sub', 'ses_new', 5, 5)`,
		`insert into session values ('ses_other', null, 70000, 0)`,
		`insert into session values ('bad-id', null, 90000, 0)`,
	)
	raw := "ses_old\nses_old\n\nbad-id\nses_x' or 1=1 --\n"
	write(t, e.oldCaveLog, raw)
	dir, _ := inputDir()
	write(t, filepath.Join(dir, caveRecordDir, "ses_new"), "")
	write(t, filepath.Join(dir, caveRecordDir, "not a session"), "")

	if got := caveTotal(t); got != "1120" { // 110 + 1000 + 10
		t.Fatalf("got %q", got)
	}
	if after, _ := os.ReadFile(e.oldCaveLog); string(after) != raw {
		t.Fatal("old log changed")
	}
}

func TestOpenCodeCaveTotalWithoutSessionsNeedsNoDB(t *testing.T) {
	setup(t)
	if got := caveTotal(t); got != "0" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenCodeCaveTotalMissingDBIsAnError(t *testing.T) {
	e := setup(t)
	write(t, e.oldCaveLog, "ses_a\n")
	if _, err := compute(context.Background(), keyCaveTotalOC); err == nil {
		t.Fatal("no db must be no value")
	}
}

func started(age time.Duration) *time.Time {
	at := time.Now().Add(-age)
	return &at
}

func TestRecordCaveSession(t *testing.T) {
	e := setup(t)
	raw := "ses_kept\n"
	write(t, e.oldCaveLog, raw)
	flag := filepath.Join(e.ocDir, ".caveman-active")
	dir, _ := inputDir()
	recorded := func(id string) bool {
		_, err := os.Stat(filepath.Join(dir, caveRecordDir, id))
		return err == nil
	}
	rec := func(id string, age time.Duration) {
		RecordCaveSession(model.Session{Agent: "opencode", SessionID: id, StartedAt: started(age)})
	}

	write(t, flag, "full\n")
	rec("ses_young", 10*time.Second)
	if !recorded("ses_young") {
		t.Error("young full session not recorded")
	}
	rec("ses_young", 20*time.Second) // repeat is harmless

	rec("ses_old", 61*time.Second)
	if recorded("ses_old") {
		t.Error("session first seen after 60 s recorded")
	}
	RecordCaveSession(model.Session{Agent: "opencode", SessionID: "ses_nomsg"})
	if recorded("ses_nomsg") {
		t.Error("session with no message recorded")
	}
	RecordCaveSession(model.Session{Agent: "claude", SessionID: "ses_claude", StartedAt: started(0)})
	if recorded("ses_claude") {
		t.Error("other assistant recorded")
	}
	for _, bad := range []string{"../ses_x", "ses_a b", "abc", "ses_"} {
		rec(bad, 0)
		if entries, _ := os.ReadDir(filepath.Join(dir, caveRecordDir)); len(entries) != 1 {
			t.Errorf("%q wrote %v", bad, entries)
		}
	}

	// The mode at first sight decides: switching to full later must not count it.
	write(t, flag, "lite")
	rec("ses_lite", 5*time.Second)
	write(t, flag, "full")
	rec("ses_lite", 6*time.Second)
	if recorded("ses_lite") {
		t.Error("session that started in lite recorded")
	}
	if after, _ := os.ReadFile(e.oldCaveLog); string(after) != raw {
		t.Fatal("old log changed")
	}
}

func TestPruneRemovesSeenMarkersButKeepsRecords(t *testing.T) {
	setup(t)
	dir, _ := inputDir()
	marker := filepath.Join(dir, prefixCaveSeen+"ses_a")
	record := filepath.Join(dir, caveRecordDir, "ses_a")
	write(t, marker, "")
	write(t, record, "")
	old := time.Now().Add(-3 * 24 * time.Hour)
	_ = os.Chtimes(marker, old, old)
	_ = os.Chtimes(record, old, old)
	pruneOld()
	if _, err := os.Stat(marker); err == nil {
		t.Error("old marker kept")
	}
	if _, err := os.Stat(record); err != nil {
		t.Error("record removed")
	}
}
