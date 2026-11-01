package integration

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/cache"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const (
	fullMarker = `{"type":"attachment","attachment":{"hookEvent":"SessionStart","content":"CAVEMAN MODE ACTIVE — level: full\n..."}}`
	liteMarker = `{"type":"attachment","attachment":{"hookEvent":"SessionStart","content":"CAVEMAN MODE ACTIVE — level: lite\n..."}}`
)

func cavemanSetup(t *testing.T, flag string) *env {
	t.Helper()
	e := setup(t)
	installPlugin(t, e, "caveman@caveman", true)
	if flag != "" {
		write(t, filepath.Join(e.dir, ".caveman-active"), flag)
	}
	return e
}

func caveBadge(out *int64) (render.Badge, bool) {
	return caveman{}.Badge(model.Session{Agent: "claude", OutTokens: out})
}

func i64(n int64) *int64 { return &n }

func asst(id string, out int) string {
	return `{"type":"assistant","uuid":"u-` + id + strconv.Itoa(out) + `","message":{"id":"` + id + `","usage":{"output_tokens":` + strconv.Itoa(out) + `}}}`
}

func TestCavemanFull(t *testing.T) {
	e := cavemanSetup(t, "full")
	e.canned[keyCaveTotal] = "84100"
	b, ok := caveBadge(i64(84100))
	// 84100 × 0.65/0.35 = 156,185.7
	if want := "[CAVEMAN] (c: ~156.2k | t: ~156.2k)"; !ok || text(b) != want {
		t.Fatalf("got %q %v want %q", text(b), ok, want)
	}
	for _, s := range b.Lines {
		if s.FG != "38;5;172" && strings.TrimSpace(s.Text) != "" {
			t.Errorf("segment %q has colour %q, want the plugin's 38;5;172", s.Text, s.FG)
		}
	}
}

func TestCavemanNotFull(t *testing.T) {
	for flag, want := range map[string]string{
		"ultra":       "[CAVEMAN:ULTRA]",
		"lite":        "[CAVEMAN:LITE]",
		"wenyan-lite": "[CAVEMAN:WENYAN-LITE]",
		"Lite\n":      "[CAVEMAN:LITE]",
		"off":         "[CAVEMAN:OFF]", // the plugin's own badge for off
	} {
		e := cavemanSetup(t, flag)
		e.canned[keyCaveTotal] = "84100"
		b, ok := caveBadge(i64(84100))
		if !ok || text(b) != want {
			t.Errorf("flag %q: got %q %v want %q", flag, text(b), ok, want)
		}
	}
}

func TestCavemanNoOutput(t *testing.T) {
	e := cavemanSetup(t, "full")
	e.canned[keyCaveTotal] = "0"
	for name, out := range map[string]*int64{"zero": i64(0), "unknown": nil, "negative": i64(-5)} {
		if got := text(first(caveBadge(out))); got != "[CAVEMAN] (c: — | t: —)" {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

func TestCavemanTotalNotYetComputed(t *testing.T) {
	e := cavemanSetup(t, "full")
	cacheGet = func(string, time.Duration) ([]byte, bool) { return nil, false }
	_ = e
	if got := text(first(caveBadge(i64(100)))); got != "[CAVEMAN] (c: ~185 | t: —)" {
		t.Fatalf("got %q", got)
	}
}

func TestCavemanFullIsExactlyFull(t *testing.T) {
	// The old bar compared the raw flag: only "full" (trailing newlines aside) shows numbers.
	for _, flag := range []string{"FULL", " full", "full x"} {
		e := cavemanSetup(t, flag)
		e.canned[keyCaveTotal] = "100"
		if got := text(first(caveBadge(i64(100)))); strings.Contains(got, "c:") {
			t.Errorf("flag %q shows numbers: %q", flag, got)
		}
	}
	e := cavemanSetup(t, "full\n")
	e.canned[keyCaveTotal] = "100"
	if got := text(first(caveBadge(i64(100)))); !strings.Contains(got, "c: ~185") {
		t.Errorf("full+newline hides numbers: %q", got)
	}
}

func TestCavemanHidden(t *testing.T) {
	t.Run("no flag", func(t *testing.T) {
		cavemanSetup(t, "")
		if _, ok := caveBadge(i64(1)); ok {
			t.Fatal("badge without a flag")
		}
	})
	t.Run("empty flag", func(t *testing.T) { // the plugin prints nothing for an empty mode
		cavemanSetup(t, "\n")
		if _, ok := caveBadge(i64(1)); ok {
			t.Fatal("badge for an empty flag")
		}
	})
	t.Run("unknown mode", func(t *testing.T) {
		for _, f := range []string{"bogus", "\x1b]8;;x\x07full", "ful\x1b[31ml"} {
			cavemanSetup(t, f)
			if b, ok := caveBadge(i64(1)); ok {
				t.Errorf("flag %q rendered %q", f, text(b))
			}
		}
	})
	t.Run("plugin disabled", func(t *testing.T) {
		e := cavemanSetup(t, "full")
		installPlugin(t, e, "caveman@caveman", false)
		if _, ok := caveBadge(i64(1)); ok {
			t.Fatal("badge for a disabled plugin")
		}
	})
	t.Run("plugin not installed", func(t *testing.T) {
		e := cavemanSetup(t, "full")
		write(t, filepath.Join(e.dir, "plugins", "installed_plugins.json"), `{"plugins":{}}`)
		if _, ok := caveBadge(i64(1)); ok {
			t.Fatal("badge for a plugin that is not installed")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		e := cavemanSetup(t, "")
		write(t, filepath.Join(e.dir, "secret"), "full")
		if err := os.Symlink(filepath.Join(e.dir, "secret"), filepath.Join(e.dir, ".caveman-active")); err != nil {
			t.Skip("no symlinks:", err)
		}
		if _, ok := caveBadge(i64(1)); ok {
			t.Fatal("followed a symlink")
		}
	})
}

// projectFixture lays out four sessions: A (full, with a subagent), B (lite),
// C (no marker) and D (full, other project).
func projectFixture(t *testing.T, e *env) (a, b, c, d string) {
	t.Helper()
	p1, p2 := filepath.Join(e.dir, "projects", "p1"), filepath.Join(e.dir, "projects", "p2")
	a, b, c, d = filepath.Join(p1, "a.jsonl"), filepath.Join(p1, "b.jsonl"), filepath.Join(p1, "c.jsonl"), filepath.Join(p2, "d.jsonl")
	write(t, a, strings.Join([]string{fullMarker, asst("m1", 10), asst("m1", 120), asst("m2", 30), "garbage"}, "\n")+"\n")
	write(t, filepath.Join(p1, "a", "subagents", "s.jsonl"), asst("m3", 7)+"\n"+asst("m2", 5)+"\n") // m2: max stays 30
	write(t, b, strings.Join([]string{liteMarker, asst("l1", 1000)}, "\n")+"\n")
	write(t, filepath.Join(p1, "b", "subagents", "s.jsonl"), asst("l2", 1000)+"\n")
	write(t, c, asst("n1", 500)+"\n")
	write(t, d, strings.Join([]string{asst("x1", 100), fullMarker}, "\n")+"\n") // marker after the first message
	return
}

func TestCavemanTotalClaude(t *testing.T) {
	e := setup(t)
	projectFixture(t, e)
	// A: 120 + 30 + 7 (main + subagent, max per id); B and C lack the full marker; D: 100
	v, err := computeCaveTotal(t.Context())
	if err != nil || string(v) != "257" {
		t.Fatalf("got %q %v, want 257", v, err)
	}
}

func TestCavemanTotalNeedsTheFullMarker(t *testing.T) {
	e := setup(t)
	p := filepath.Join(e.dir, "projects", "p", "s.jsonl")
	for marker, want := range map[string]string{
		"CAVEMAN MODE ACTIVE — level: lite":  "0",
		"CAVEMAN MODE ACTIVE — level: ultra": "0",
		"level: full":                        "0", // no banner
		"CAVEMAN MODE ACTIVE — level: full":  "50",
	} {
		_ = os.Remove(filepath.Join(e.dir, "in", "cave-files.tsv"))
		write(t, p, marker+"\n"+asst("m", 50)+"\n")
		v, err := computeCaveTotal(t.Context())
		if err != nil || string(v) != want {
			t.Errorf("marker %q: got %q %v want %s", marker, v, err, want)
		}
	}
}

func TestCavemanTotalTakesTheMaxPerMessageID(t *testing.T) {
	e := setup(t)
	p := filepath.Join(e.dir, "projects", "p", "s.jsonl")
	write(t, p, fullMarker+"\n"+asst("m", 5)+"\n"+asst("m", 90)+"\n"+asst("m", 40)+"\n")
	if v, err := computeCaveTotal(t.Context()); err != nil || string(v) != "90" {
		t.Fatalf("got %q %v, want 90", v, err)
	}
}

func TestCavemanTotalIncludesSubagents(t *testing.T) {
	e := setup(t)
	p := filepath.Join(e.dir, "projects", "p", "s.jsonl")
	write(t, p, fullMarker+"\n"+asst("m", 40)+"\n")
	write(t, filepath.Join(e.dir, "projects", "p", "s", "subagents", "a.jsonl"), asst("sub", 2)+"\n")
	write(t, filepath.Join(e.dir, "projects", "p", "s", "subagents", "b.txt"), asst("ignored", 999)+"\n")
	if v, err := computeCaveTotal(t.Context()); err != nil || string(v) != "42" {
		t.Fatalf("got %q %v, want 42", v, err)
	}
}

func TestCavemanTotalReusesTheMtimeIndex(t *testing.T) {
	e := setup(t)
	a, _, _, _ := projectFixture(t, e)
	if v, _ := computeCaveTotal(t.Context()); string(v) != "257" {
		t.Fatalf("first run %q", v)
	}
	idx := filepath.Join(e.dir, "in", "cave-files.tsv")
	raw, err := os.ReadFile(idx)
	if err != nil || !strings.Contains(string(raw), a+"\t") {
		t.Fatalf("index not written: %q %v", raw, err)
	}
	if info, _ := os.Stat(idx); info.Mode().Perm()&0o077 != 0 {
		t.Errorf("index is not owner-only: %v", info.Mode())
	}
	if m, _ := filepath.Glob(filepath.Join(e.dir, "in", "cave-files.tsv.*")); len(m) != 0 {
		t.Errorf("temp files left behind: %v", m)
	}

	// Same mtime: the file is not read again, the index value stands.
	info, _ := os.Stat(a)
	write(t, a, fullMarker+"\n"+asst("m1", 9999)+"\n")
	if err := os.Chtimes(a, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if v, _ := computeCaveTotal(t.Context()); string(v) != "257" {
		t.Fatalf("unchanged mtime re-read the file: %q", v)
	}
	// New mtime: read again.
	later := info.ModTime().Add(time.Minute)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	if v, _ := computeCaveTotal(t.Context()); string(v) != strconv.Itoa(9999+7+5+100) {
		t.Fatalf("changed mtime not re-read: %q", v)
	}
	// A deleted transcript drops out of the total and the index.
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	if v, _ := computeCaveTotal(t.Context()); string(v) != "100" {
		t.Fatalf("deleted transcript still counted: %q", v)
	}
	if raw, _ := os.ReadFile(idx); strings.Contains(string(raw), a) {
		t.Error("deleted transcript still indexed")
	}
}

func TestCavemanTotalIgnoresDamagedIndexLines(t *testing.T) {
	e := setup(t)
	a, _, _, _ := projectFixture(t, e)
	write(t, filepath.Join(e.dir, "in", "cave-files.tsv"), "junk\n"+a+"\tnotanumber\t5\n"+a+"\t1\t-7\n\n")
	if v, err := computeCaveTotal(t.Context()); err != nil || string(v) != "257" {
		t.Fatalf("got %q %v", v, err)
	}
}

func TestCavemanTotalUnreadableSessionIsAnErrorNotAnUndercount(t *testing.T) {
	e := setup(t)
	projectFixture(t, e)
	sub := filepath.Join(e.dir, "projects", "p1", "a", "subagents", "s.jsonl")
	if err := os.Chmod(sub, 0); err != nil {
		t.Skip("cannot remove read permission:", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o644) })
	if f, err := os.Open(sub); err == nil {
		f.Close()
		t.Skip("permissions are not enforced here")
	}
	if v, err := computeCaveTotal(t.Context()); err == nil {
		t.Fatalf("partial sum accepted: %q", v)
	}
}

func TestCavemanTotalWithoutProjects(t *testing.T) {
	setup(t)
	if v, err := computeCaveTotal(t.Context()); err != nil || string(v) != "0" {
		t.Fatalf("got %q %v", v, err)
	}
}

func TestCavemanTotalRefreshesThroughTheCache(t *testing.T) {
	e := setup(t)
	for _, v := range []string{"HOME", "XDG_CACHE_HOME", "LocalAppData"} {
		t.Setenv(v, e.dir)
	}
	inputDir = cache.Dir
	projectFixture(t, e)
	RegisterCaches()
	if err := cache.Refresh(keyCaveTotal); err != nil {
		t.Fatal(err)
	}
	if v, ok := cache.Get(keyCaveTotal, time.Hour); !ok || string(v) != "257" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestAllListsPluginBadgesFirst(t *testing.T) {
	var ids []string
	for _, i := range All() {
		ids = append(ids, i.ID())
	}
	if got := strings.Join(ids, ","); got != "ponytail,caveman,rtk,claude-mem" {
		t.Fatalf("got %s", got)
	}
}

// errAfter is a context that reports cancellation after n calls to Err.
type errAfter struct {
	context.Context
	n int
}

func (c *errAfter) Err() error {
	if c.n--; c.n < 0 {
		return context.DeadlineExceeded
	}
	return nil
}

func TestCavemanTotalKeepsProgressWhenTheRefreshIsCutOff(t *testing.T) {
	// A cold index over thousands of transcripts takes longer than the refresh
	// limit; every attempt must still move forward.
	e := setup(t)
	projectFixture(t, e)
	probe := &errAfter{Context: t.Context(), n: 1 << 30}
	if _, err := computeCaveTotal(probe); err != nil {
		t.Fatal(err)
	}
	calls := 1<<30 - probe.n
	_ = os.Remove(filepath.Join(e.dir, "in", caveIndexName))

	if v, err := computeCaveTotal(&errAfter{Context: t.Context(), n: calls * 2 / 3}); err == nil {
		t.Fatalf("cut-off run reported success: %q", v)
	}
	raw, err := os.ReadFile(filepath.Join(e.dir, "in", caveIndexName))
	if err != nil || len(raw) == 0 {
		t.Fatalf("cut-off run kept no progress: %q %v", raw, err)
	}
	if v, err := computeCaveTotal(t.Context()); err != nil || string(v) != "257" {
		t.Fatalf("resumed run: %q %v", v, err)
	}
}
