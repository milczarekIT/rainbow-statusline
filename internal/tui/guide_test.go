package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/install"
)

func newGuideTest(t *testing.T, plugins map[string]bool, agents []install.Agent) *guide {
	t.Helper()
	sandbox(t)
	g := newGuide(config.Config{}, plugins, agents)
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	return g
}

func pressG(g *guide, names ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, n := range names {
		switch n {
		case "enter":
			_, cmd = g.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		case "esc":
			_, cmd = g.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		default:
			_, cmd = g.Update(key(n))
		}
	}
	return cmd
}

func body(g *guide) string { return plain(g.body()) }

func TestGuideStartsWithDetectedSelected(t *testing.T) {
	g := newGuideTest(t, map[string]bool{"ponytail": true, "caveman": false, "rtk": true, "claude-mem": true}, testAgents())
	b := body(g)
	for _, want := range []string{"Step 1 of 4: Where should the bar run?", "[x] Claude Code", "detected", "[x] OpenCode", "[ ] Copilot CLI", "not detected"} {
		if !strings.Contains(b, want) {
			t.Errorf("step 1 missing %q:\n%s", want, b)
		}
	}
	pressG(g, "enter")
	b = body(g)
	for _, want := range []string{"Step 2 of 4: What should the bar show?", "[x] RTK", "[x] Caveman", "not detected, appears after install", "estimate"} {
		if !strings.Contains(b, want) {
			t.Errorf("step 2 missing %q:\n%s", want, b)
		}
	}
	if strings.Contains(plain(g.preview()), "[CAVEMAN]") {
		t.Errorf("undetected plugin starts hidden:\n%s", plain(g.preview()))
	}
}

func TestGuideMentionsExistingStatusLine(t *testing.T) {
	agents := []install.Agent{{ID: "claude", Name: "Claude Code", Found: true, CurrentStatusLine: "my-own-bar"}}
	g := newGuideTest(t, allPlugins(), agents)
	if b := body(g); !strings.Contains(b, "already has a status line") {
		t.Errorf("step 1 should warn:\n%s", b)
	}
}

func TestGuideUndetectedAssistantCannotBeSelected(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	pressG(g, "down", "space") // copilot, not found
	if strings.Contains(body(g), "[x] Copilot CLI") {
		t.Error("an undetected assistant must stay unselected")
	}
}

func TestGuideEscGoesBackKeepingChoices(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	pressG(g, "space")         // claude off
	pressG(g, "enter", "down") // step 2, cursor on caveman
	pressG(g, "space")         // caveman off
	pressG(g, "enter", "down", "space", "enter")
	if !strings.Contains(body(g), "Step 4 of 4") {
		t.Fatalf("expected step 4:\n%s", body(g))
	}
	pressG(g, "esc", "esc")
	if !strings.Contains(body(g), "Step 2 of 4") || !strings.Contains(body(g), "[ ] Caveman") {
		t.Errorf("step 2 should keep the toggle:\n%s", body(g))
	}
	pressG(g, "esc")
	if b := body(g); !strings.Contains(b, "Step 1 of 4") || !strings.Contains(b, "[ ] Claude Code") {
		t.Errorf("step 1 should keep the choice:\n%s", b)
	}
	pressG(g, "esc")
	if !strings.Contains(body(g), "Step 1 of 4") {
		t.Error("esc on step 1 stays")
	}
}

func TestGuideQuitOnAnyStepWritesNothing(t *testing.T) {
	for steps := 0; steps < 4; steps++ {
		home := sandbox(t)
		mkdir(t, filepath.Join(home, ".claude"))
		g := newGuide(config.Config{}, allPlugins(), testAgents())
		g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
		for i := 0; i < steps; i++ {
			pressG(g, "enter")
		}
		if !isQuit(pressG(g, "q")) {
			t.Errorf("q on step %d should quit", steps+1)
		}
		if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
			t.Errorf("step %d: config written", steps+1)
		}
		if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
			t.Errorf("step %d: settings written", steps+1)
		}
	}
}

func TestGuideDensityStep(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	pressG(g, "enter", "enter")
	b := body(g)
	for _, want := range []string{"Step 3 of 4: How much to show?", "Full", "three lines", "Compact", "no user or directory", "Minimal", "one line: model, context, cost", "rainbow-statusline"} {
		if !strings.Contains(b, want) {
			t.Errorf("step 3 missing %q:\n%s", want, b)
		}
	}
	pressG(g, "down", "space")
	if g.view.Density != "compact" {
		t.Errorf("density: %q", g.view.Density)
	}
	pressG(g, "down", "space")
	if got := len(strings.Split(strings.TrimSpace(plain(g.preview())), "\n")); got != 1 {
		t.Errorf("minimal preview should be one line, got %d:\n%s", got, plain(g.preview()))
	}
}

func TestGuideSummaryListsEveryPathAndTheReplacedStatusLine(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settings))
	mkdir(t, filepath.Join(home, ".config", "opencode"))
	if err := os.WriteFile(settings, []byte(`{"statusLine":{"type":"command","command":"my-own-bar --fancy"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	agents := []install.Agent{
		{ID: "claude", Name: "Claude Code", Found: true, CurrentStatusLine: "my-own-bar --fancy"},
		{ID: "opencode", Name: "OpenCode", Found: true},
	}
	g := newGuide(config.Config{}, allPlugins(), agents)
	g.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	pressG(g, "enter", "enter", "enter")
	b := body(g)
	changes, err := install.Plan([]string{"claude", "opencode"})
	if err != nil || len(changes) < 2 {
		t.Fatalf("plan: %v %v", changes, err)
	}
	for _, c := range changes {
		if !strings.Contains(b, c.Path) {
			t.Errorf("summary misses %s:\n%s", c.Path, b)
		}
	}
	for _, want := range []string{"Step 4 of 4: Review what will change.", "my-own-bar --fancy", "replaced", "backup", "uninstall", "Install", "Back", "Quit without changes"} {
		if !strings.Contains(b, want) {
			t.Errorf("summary missing %q:\n%s", want, b)
		}
	}
	if got, _ := os.ReadFile(settings); !strings.Contains(string(got), "my-own-bar") {
		t.Error("reviewing must not write")
	}
}

func TestGuideInstallAppliesAndSaves(t *testing.T) {
	home := sandbox(t)
	mkdir(t, filepath.Join(home, ".claude"))
	agents := []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}}
	g := newGuide(config.Config{}, allPlugins(), agents)
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "down", "space", "enter")
	if isQuit(pressG(g, "enter")) {
		t.Fatal("install should show the final message first")
	}
	if !strings.Contains(body(g), "Done. Restart your assistant to see the bar.") {
		t.Errorf("final message:\n%s", body(g))
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(raw), "--agent claude") {
		t.Errorf("settings: %v %s", err, raw)
	}
	cfg, found := config.Load()
	if !found || cfg.Density != "compact" || strings.Join(cfg.Agents, ",") != "claude" {
		t.Errorf("config: %v %+v", found, cfg)
	}
	if !isQuit(pressG(g, "enter")) {
		t.Error("a key after Done leaves the guide")
	}
}

func TestGuideBackOptionAndQuitOption(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	pressG(g, "enter", "enter", "enter", "down", "enter")
	if !strings.Contains(body(g), "Step 3 of 4") {
		t.Errorf("Back option:\n%s", body(g))
	}
	pressG(g, "enter", "down", "down", "enter")
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Error("quit option wrote the config")
	}
}

func TestGuidePlanErrorStaysOnSummary(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settings))
	if err := os.WriteFile(settings, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	g := newGuide(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "enter")
	b := body(g)
	if !strings.Contains(b, "Step 4 of 4") || !strings.Contains(strings.ToLower(b), "error") {
		t.Errorf("summary should show the error:\n%s", b)
	}
	pressG(g, "enter")
	if !strings.Contains(body(g), "Step 4 of 4") {
		t.Error("a failing install stays on step 4")
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Error("config written despite the error")
	}
}

func TestGuideApplyErrorStaysAndSavesNothing(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settings))
	g := newGuide(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "enter")
	if err := os.WriteFile(settings, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	pressG(g, "enter")
	b := body(g)
	if !strings.Contains(b, "Step 4 of 4") || !strings.Contains(strings.ToLower(b), "error") {
		t.Errorf("apply error should show on step 4:\n%s", b)
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Error("config must not be saved when Apply fails")
	}
}

func TestGuideWithNoAssistantStillSavesBarSettings(t *testing.T) {
	none := []install.Agent{{ID: "claude", Name: "Claude Code"}, {ID: "opencode", Name: "OpenCode"}}
	g := newGuideTest(t, allPlugins(), none)
	if b := body(g); !strings.Contains(b, "No supported assistant was found") {
		t.Errorf("step 1 should say so:\n%s", b)
	}
	pressG(g, "enter", "enter", "enter")
	if b := body(g); !strings.Contains(b, "only the bar settings") {
		t.Errorf("summary:\n%s", b)
	}
	pressG(g, "enter")
	if !strings.Contains(body(g), "Done.") {
		t.Errorf("final message:\n%s", body(g))
	}
	if _, found := config.Load(); !found {
		t.Error("bar settings should be saved")
	}
}

func TestGuideTinyTerminal(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	g.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	if !strings.Contains(body(g), "Terminal too small") {
		t.Errorf("body:\n%s", body(g))
	}
}

func TestGuideUndetectedPluginIsNotSavedAsHidden(t *testing.T) {
	home := sandbox(t)
	mkdir(t, filepath.Join(home, ".claude"))
	plugins := map[string]bool{"ponytail": true, "caveman": false, "rtk": true, "claude-mem": true}
	g := newGuide(config.Config{}, plugins, []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "enter", "enter")
	cfg, found := config.Load()
	if !found {
		t.Fatal("config not saved")
	}
	for _, h := range cfg.Hidden {
		if h == "caveman" {
			t.Errorf("undetected plugin saved as hidden: %v", cfg.Hidden)
		}
	}
}

func TestGuideExplicitlyHiddenUndetectedPluginIsSaved(t *testing.T) {
	sandbox(t)
	plugins := map[string]bool{"ponytail": true, "caveman": false, "rtk": true, "claude-mem": true}
	g := newGuide(config.Config{}, plugins, nil)
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "down", "space")
	if !strings.Contains(body(g), "[ ] Caveman") {
		t.Errorf("toggled off:\n%s", body(g))
	}
	pressG(g, "enter", "enter", "enter")
	cfg, _ := config.Load()
	if strings.Join(cfg.Hidden, ",") != "caveman" {
		t.Errorf("hidden: %v", cfg.Hidden)
	}
}

func TestGuideApplyFailureRefreshesThePlan(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settings))
	g := newGuide(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	pressG(g, "enter", "enter", "enter")
	if err := os.WriteFile(settings, []byte(`{"statusLine":{"type":"command","command":"my-own-bar"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pressG(g, "enter")
	b := body(g)
	if !strings.Contains(strings.ToLower(b), "error") || !strings.Contains(b, "my-own-bar") || !strings.Contains(b, "replaced") {
		t.Fatalf("refreshed plan should name the status line:\n%s", b)
	}
	if _, err := os.Stat(configFile(t)); !os.IsNotExist(err) {
		t.Fatal("config saved after a failed install")
	}
	pressG(g, "enter")
	if !strings.Contains(body(g), "Done.") {
		t.Errorf("retry after review should install:\n%s", body(g))
	}
}

func TestGuideRetriesOnlyTheSaveAfterSaveFailure(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settings))
	cfgFile := configFile(t)
	mkdir(t, filepath.Join(cfgFile, "in-the-way")) // a folder where the file belongs
	g := newGuide(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "enter", "enter")
	if !strings.Contains(body(g), "Step 4 of 4") || !strings.Contains(body(g), "not saved") {
		t.Fatalf("save failure should stay on step 4:\n%s", body(g))
	}
	if _, err := os.Stat(settings); err != nil {
		t.Fatalf("assistant should be set up: %v", err)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(cfgFile); err != nil {
		t.Fatal(err)
	}
	pressG(g, "enter")
	if !strings.Contains(body(g), "Done.") {
		t.Errorf("retry:\n%s", body(g))
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Error("the retry must not apply again")
	}
	if _, found := config.Load(); !found {
		t.Error("config not saved on retry")
	}
}

func TestGuideReplansAfterGoingBack(t *testing.T) {
	home := sandbox(t)
	mkdir(t, filepath.Join(home, ".claude"))
	mkdir(t, filepath.Join(home, ".config", "opencode"))
	agents := []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}, {ID: "opencode", Name: "OpenCode", Found: true}}
	g := newGuide(config.Config{}, allPlugins(), agents)
	g.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	pressG(g, "enter", "enter", "enter")
	changes, _ := install.Plan([]string{"opencode"})
	if len(changes) == 0 || !strings.Contains(body(g), changes[0].Path) {
		t.Fatalf("opencode should be listed:\n%s", body(g))
	}
	pressG(g, "esc", "esc", "esc", "down", "space", "enter", "enter", "enter")
	if strings.Contains(body(g), changes[0].Path) {
		t.Errorf("deselected assistant still listed:\n%s", body(g))
	}
}

func TestGuideSummaryNamesSettingsFileAndUnchangedAssistants(t *testing.T) {
	sandbox(t)
	g := newGuide(config.Config{}, allPlugins(), []install.Agent{{ID: "claude", Name: "Claude Code", Found: true}})
	g.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	g.step, g.planned = 3, true // a plan with nothing to do for the selected assistant
	b := body(g)
	if !strings.Contains(b, "Claude Code: already set up, nothing to change") {
		t.Errorf("unchanged assistant:\n%s", b)
	}
	if !strings.Contains(b, "Bar settings: "+configFile(t)) {
		t.Errorf("settings path:\n%s", b)
	}
}

func TestGuideEnterOnDensitySelectsHighlighted(t *testing.T) {
	g := newGuideTest(t, allPlugins(), testAgents())
	pressG(g, "enter", "enter", "down", "enter")
	if g.view.Density != "compact" || !strings.Contains(body(g), "Step 4 of 4") {
		t.Errorf("density %q:\n%s", g.view.Density, body(g))
	}
}

func TestGuideDoneQuitsOnAnyKey(t *testing.T) {
	sandbox(t)
	g := newGuide(config.Config{}, allPlugins(), nil)
	g.Update(tea.WindowSizeMsg{Width: screenWidth, Height: screenHeight})
	pressG(g, "enter", "enter", "enter", "enter")
	if !strings.Contains(body(g), "Done.") {
		t.Fatalf("not done:\n%s", body(g))
	}
	if !isQuit(pressG(g, "x")) {
		t.Error("any key leaves")
	}
}
