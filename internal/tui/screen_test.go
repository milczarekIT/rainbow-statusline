package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/install"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const (
	screenWidth  = 80
	screenHeight = 30
)

var fixedNow = time.Unix(1793566870, 0)

// sandbox points every root at a temp dir: no test may reach the owner's files.
func sandbox(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": "", "COPILOT_HOME": "",
		"CLAUDE_CONFIG_DIR": "", "PATH": "", "APPDATA": filepath.Join(home, "appdata"),
	} {
		t.Setenv(k, v)
	}
	oldDir, oldNow := config.ConfigDir, render.Now
	config.ConfigDir = func() (string, error) { return filepath.Join(home, "cfg"), nil }
	render.Now = func() time.Time { return fixedNow }
	t.Cleanup(func() { config.ConfigDir, render.Now = oldDir, oldNow })
	return home
}

func testAgents() []install.Agent {
	return []install.Agent{
		{ID: "claude", Name: "Claude Code", Found: true},
		{ID: "copilot", Name: "Copilot CLI", Found: false},
		{ID: "opencode", Name: "OpenCode", Found: true},
	}
}

func allPlugins() map[string]bool {
	return map[string]bool{"ponytail": true, "caveman": true, "rtk": true, "claude-mem": true}
}

func newTest(t *testing.T, cfg config.Config) *screen {
	t.Helper()
	sandbox(t)
	m := newScreen(cfg, allPlugins(), testAgents())
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	return m
}

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func key(name string) tea.KeyPressMsg {
	switch name {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "shift+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	case "shift+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(name[0]), Text: name}
}

func press(m *screen, names ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, n := range names {
		_, cmd = m.Update(key(n))
	}
	return cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func configFile(t *testing.T) string {
	t.Helper()
	dir, _ := config.ConfigDir()
	return filepath.Join(dir, "rainbow-statusline", "config.json")
}

func TestSpaceHidesItemAndPreviewChanges(t *testing.T) {
	m := newTest(t, config.Config{})
	if !strings.Contains(plain(m.body()), "[RTK]") {
		t.Fatal("RTK badge should show at first")
	}
	press(m, "down", "down", "space") // ponytail, caveman, rtk
	if strings.Contains(plain(m.preview()), "[RTK]") {
		t.Errorf("RTK should be hidden:\n%s", plain(m.preview()))
	}
	if !strings.Contains(plain(m.preview()), "[CAVEMAN]") {
		t.Error("other badges stay")
	}
	press(m, "space")
	if !strings.Contains(plain(m.preview()), "[RTK]") {
		t.Error("second space shows RTK again")
	}
}

func TestArrowsChangeVariant(t *testing.T) {
	m := newTest(t, config.Config{})
	press(m, "down", "down", "down") // claude-mem
	if !strings.Contains(plain(m.preview()), "(c: 403.2k") || !strings.Contains(plain(m.preview()), "t: 67.5M") {
		t.Fatalf("default shows both:\n%s", plain(m.preview()))
	}
	press(m, "right")
	p := plain(m.preview())
	if !strings.Contains(p, "(c: 403.2k") || strings.Contains(p, "t: 67.5M") {
		t.Errorf("variant c should drop the total:\n%s", p)
	}
	press(m, "right")
	p = plain(m.preview())
	if strings.Contains(p, "(c: 403.2k") || !strings.Contains(p, "(t: 67.5M") {
		t.Errorf("variant t should drop the session figure:\n%s", p)
	}
	press(m, "left", "left")
	if !strings.Contains(plain(m.preview()), "t: 67.5M") || m.view.Variants["claude-mem"] != "" {
		t.Errorf("back to the default, which is not stored: %v", m.view.Variants)
	}
}

func TestContextAndCacheVariants(t *testing.T) {
	m := newTest(t, config.Config{})
	for i := 0; i < 5; i++ {
		press(m, "down")
	}
	if m.rows[m.cursor] != config.Context {
		t.Fatalf("cursor on %s", m.rows[m.cursor])
	}
	press(m, "right")
	if strings.ContainsAny(plain(m.preview()), "▰▱") {
		t.Errorf("pct variant has no bar:\n%s", plain(m.preview()))
	}
	for i := 0; i < 3; i++ {
		press(m, "down")
	}
	if m.rows[m.cursor] != config.Cache {
		t.Fatalf("cursor on %s", m.rows[m.cursor])
	}
	press(m, "right")
	if strings.Contains(plain(m.preview()), "read") {
		t.Errorf("timer variant has no read/write:\n%s", plain(m.preview()))
	}
}

func TestShiftArrowsReorderBadges(t *testing.T) {
	m := newTest(t, config.Config{})
	press(m, "down", "down", "shift+up") // rtk above caveman
	p := plain(m.preview())
	if strings.Index(p, "[RTK]") > strings.Index(p, "[CAVEMAN]") {
		t.Errorf("RTK should precede CAVEMAN:\n%s", p)
	}
	if m.cursor != 1 || m.rows[1] != config.RTK {
		t.Errorf("cursor follows the item, got row %d (%s)", m.cursor, m.rows[m.cursor])
	}
	press(m, "shift+down")
	p = plain(m.preview())
	if strings.Index(p, "[RTK]") < strings.Index(p, "[CAVEMAN]") {
		t.Errorf("back in the original order:\n%s", p)
	}
}

func TestSessionItemsDoNotMove(t *testing.T) {
	m := newTest(t, config.Config{})
	for i := 0; i < 4; i++ {
		press(m, "down")
	}
	before := append([]string(nil), m.rows...)
	press(m, "shift+up")
	if strings.Join(before, ",") != strings.Join(m.rows, ",") {
		t.Errorf("rows moved: %v", m.rows)
	}
	if !strings.Contains(plain(m.body()), "cannot be moved") {
		t.Errorf("footer should say so:\n%s", plain(m.body()))
	}
}

func TestNotDetectedPlugin(t *testing.T) {
	sandbox(t)
	m := newScreen(config.Config{}, map[string]bool{"ponytail": true, "caveman": true, "rtk": false, "claude-mem": true}, testAgents())
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	var rtkLine string
	for _, l := range strings.Split(plain(m.body()), "\n") {
		if strings.Contains(l, "RTK") && strings.Contains(l, "[x]") {
			rtkLine = l
		}
	}
	if !strings.Contains(rtkLine, "not detected") {
		t.Errorf("RTK row: %q", rtkLine)
	}
	if strings.Count(plain(m.body()), "not detected") != 1 {
		t.Errorf("only RTK is undetected:\n%s", plain(m.body()))
	}
}

func TestQuitWritesNothing(t *testing.T) {
	home := sandbox(t)
	m := newScreen(config.Config{}, allPlugins(), testAgents())
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	press(m, "down", "space")
	if !isQuit(press(m, "q")) {
		t.Error("q should quit")
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Errorf("config written (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("claude dir touched (err=%v)", err)
	}
	m2 := newScreen(config.Config{}, allPlugins(), testAgents())
	if !isQuit(press(m2, "ctrl+c")) {
		t.Error("ctrl+c should quit")
	}
}

func TestSaveWritesConfigAndInstalls(t *testing.T) {
	home := sandbox(t)
	mkdir(t, filepath.Join(home, ".claude"))
	m := newScreen(config.Config{}, allPlugins(), testAgents())
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	press(m, "down", "space", "right") // hide caveman... then variant is ignored for hidden? still stored
	press(m, "down", "shift+up")       // rtk above caveman
	press(m, "tab", "down", "down", "space")
	// agents: claude [x], copilot [ ] (not found), opencode [x] -> unselect opencode
	if cmd := press(m, "tab", "s"); cmd != nil && isQuit(cmd) {
		t.Fatal("s must not quit")
	}
	if !strings.Contains(m.footer, "Restart") || !strings.Contains(m.footer, "Claude Code") {
		t.Errorf("footer should ask for a restart: %q", m.footer)
	}
	if strings.Contains(m.footer, "OpenCode") && !strings.Contains(m.footer, "stays") {
		t.Errorf("OpenCode was not selected: %q", m.footer)
	}
	cfg, found := config.Load()
	if !found {
		t.Fatal("config not written")
	}
	if len(cfg.Hidden) != 1 || cfg.Hidden[0] != "caveman" {
		t.Errorf("hidden: %v", cfg.Hidden)
	}
	if len(cfg.Order) < 2 || cfg.Order[0] != "ponytail" || cfg.Order[1] != "rtk" {
		t.Errorf("order: %v", cfg.Order)
	}
	if strings.Join(cfg.Agents, ",") != "claude" {
		t.Errorf("agents: %v", cfg.Agents)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(raw), "statusLine") {
		t.Errorf("claude settings not written: %v %s", err, raw)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); err == nil {
		t.Error("OpenCode was not selected")
	}
}

func TestSaveAsksBeforeReplacingStatusLine(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{"statusLine":{"type":"command","command":"my-own-bar --fancy"}}`
	if err := os.WriteFile(settings, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}}
	m := newScreen(config.Config{}, allPlugins(), agents)
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})

	press(m, "tab", "s")
	if !strings.Contains(m.footer, "my-own-bar --fancy") || !strings.Contains(m.footer, "y/n") {
		t.Fatalf("confirmation should name the status line: %q", m.footer)
	}
	if got, _ := os.ReadFile(settings); string(got) != original {
		t.Fatal("nothing may be written before y")
	}

	press(m, "n")
	if got, _ := os.ReadFile(settings); string(got) != original {
		t.Error("n must not write")
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Error("n must not write the config")
	}
	if strings.Contains(m.footer, "y/n") {
		t.Errorf("confirmation should be gone: %q", m.footer)
	}

	press(m, "s", "y")
	got, _ := os.ReadFile(settings)
	if strings.Contains(string(got), "my-own-bar") || !strings.Contains(string(got), "--agent claude") {
		t.Errorf("y should replace: %s", got)
	}
	if !strings.Contains(m.footer, "Restart") {
		t.Errorf("footer: %q", m.footer)
	}
}

func TestPlanErrorShowsInFooter(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{ this is not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newScreen(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	cmd := press(m, "tab", "s")
	if isQuit(cmd) {
		t.Error("an error must not exit")
	}
	if !strings.Contains(strings.ToLower(m.footer), "error") {
		t.Errorf("footer should show the error: %q", m.footer)
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Error("config must not be written when the install fails")
	}
}

func TestUnselectedInstalledAssistantIsNotUninstalled(t *testing.T) {
	mkdir(t, filepath.Join(sandbox(t), ".config", "opencode"))
	agents := []install.Agent{
		{ID: "claude", Name: "Claude Code", Found: true, Installed: true},
		{ID: "opencode", Name: "OpenCode", Found: true},
	}
	m := newScreen(config.Config{Agents: []string{"opencode"}}, allPlugins(), agents)
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	if !strings.Contains(plain(m.body()), "installed") {
		t.Errorf("installed assistant should be marked:\n%s", plain(m.body()))
	}
	press(m, "s")
	if !strings.Contains(m.footer, "Claude Code stays installed") || !strings.Contains(m.footer, "uninstall") {
		t.Errorf("footer: %q", m.footer)
	}
}

func TestPreviewFitsWidth(t *testing.T) {
	m := newTest(t, config.Config{})
	for _, w := range []int{64, 70, 80, 100} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: screenHeight})
		for i, l := range strings.Split(plain(m.body()), "\n") {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("width %d line %d is %d cells: %q", w, i, got, l)
			}
		}
	}
}

func TestTinyTerminalShowsMessage(t *testing.T) {
	m := newTest(t, config.Config{})
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	body := plain(m.body())
	if !strings.Contains(body, "too small") {
		t.Errorf("got %q", body)
	}
	for _, l := range strings.Split(body, "\n") {
		if lipgloss.Width(l) > 30 {
			t.Errorf("message overflows: %q", l)
		}
	}
	if !isQuit(press(m, "q")) {
		t.Error("q still quits when small")
	}
}

func TestPreviewUsesDemoData(t *testing.T) {
	sandbox(t)
	p := plain(Preview(config.View{}, DemoSession()))
	for _, want := range []string{"user@host", "Opus 5.5", "118.1k", "(59.0%)", "Σout 84.1k", "~$8.67", "2:32", "[PONYTAIL]", "~156.1k", "28.5%", "87%"} {
		if !strings.Contains(p, want) {
			t.Errorf("preview lacks %q:\n%s", want, p)
		}
	}
}

func TestPreviewShowsEveryBadgeAtNarrowWidths(t *testing.T) {
	for _, w := range []int{80, 64} {
		m := newTest(t, config.Config{})
		m.Update(tea.WindowSizeMsg{Width: w, Height: screenHeight})
		before := plain(m.body())
		if !strings.Contains(before, "[CLAUDE-MEM]") || !strings.Contains(before, "t: 67.5M") {
			t.Fatalf("width %d: claude-mem missing from the preview:\n%s", w, before)
		}
		press(m, "down", "down", "down", "space")
		after := plain(m.body())
		if strings.Contains(after, "[CLAUDE-MEM]") || before == after {
			t.Errorf("width %d: hiding claude-mem changed nothing visible:\n%s", w, after)
		}
		for i, l := range strings.Split(after, "\n") {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("width %d line %d is %d cells: %q", w, i, got, l)
			}
		}
		if !strings.Contains(before, "shortens") {
			t.Errorf("width %d: note about the real bar missing", w)
		}
	}
}

func TestUndetectedAssistantCannotBeSelected(t *testing.T) {
	home := sandbox(t)
	mkdir(t, filepath.Join(home, ".claude"))
	m := newScreen(config.Config{Agents: []string{"claude", "copilot"}}, allPlugins(), testAgents())
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	if got := strings.Join(m.selectedIDs(), ","); got != "claude" {
		t.Fatalf("copilot is not found and must be dropped: %q", got)
	}
	press(m, "tab", "down", "space") // copilot
	if got := strings.Join(m.selectedIDs(), ","); got != "claude" {
		t.Errorf("space must not select a missing assistant: %q", got)
	}
	if !strings.Contains(m.footer, "not found") {
		t.Errorf("footer: %q", m.footer)
	}
	press(m, "s")
	if strings.Contains(m.footer, "Error") || !strings.Contains(m.footer, "Saved") {
		t.Errorf("save should go through: %q", m.footer)
	}
}

func TestBarSettingsSaveWithNoAssistant(t *testing.T) {
	sandbox(t)
	m := newScreen(config.Config{}, allPlugins(), []install.Agent{{ID: "copilot", Name: "Copilot CLI"}})
	m.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	press(m, "space", "s")
	cfg, found := config.Load()
	if !found || len(cfg.Hidden) != 1 {
		t.Errorf("bar settings should save: %v %v (%q)", found, cfg.Hidden, m.footer)
	}
}

func TestDemoBadgeStatsTakeTheBadgeColour(t *testing.T) {
	for _, b := range demoBadges() {
		for _, s := range b.Lines {
			if s.FG != b.Lines[0].FG {
				t.Errorf("%s: segment %q has colour %q, want %q", b.Name, s.Text, s.FG, b.Lines[0].FG)
			}
		}
	}
}
