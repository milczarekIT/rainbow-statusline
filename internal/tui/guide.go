package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/install"
	"github.com/milczarekIT/rainbow-statusline/internal/integration"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const guideMinHeight = 22

var densities = []struct{ id, name, note string }{
	{"full", "Full", "up to three lines"},
	{"compact", "Compact", "up to two lines, no user or directory"},
	{"minimal", "Minimal", "one line: model, context, cost"},
}

var summaryOptions = []string{"Install", "Back", "Quit without changes"}

// guide is the first-run walk-through: assistants, items, density, review.
type guide struct {
	view    config.View
	plugins map[string]bool
	agents  []agentRow
	step    int // 0..3
	cursor  int
	done    bool
	changes []install.Change
	planned bool // changes is current
	applied bool // the files are written; only the config is left
	err     string
	width   int
	height  int
}

func newGuide(cfg config.Config, plugins map[string]bool, agents []install.Agent) *guide {
	g := &guide{view: cfg.View, plugins: plugins, width: 80, height: 24}
	g.view.Hidden = append([]string(nil), cfg.Hidden...)
	if g.view.Density == "" {
		g.view.Density = "full"
	}
	for _, a := range agents {
		g.agents = append(g.agents, agentRow{Agent: a, selected: a.Found})
	}
	return g
}

func (g *guide) Init() tea.Cmd { return nil }

func (g *guide) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.width, g.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if g.done {
			return g, tea.Quit
		}
		return g, g.key(actionFor(msg))
	}
	return g, nil
}

func (g *guide) key(a action) tea.Cmd {
	if a == actQuit {
		return tea.Quit
	}
	switch a {
	case actUp:
		g.cursor = clamp(g.cursor-1, g.listLen())
	case actDown:
		g.cursor = clamp(g.cursor+1, g.listLen())
	case actToggle:
		g.toggle()
	case actNo:
		g.back()
	case actNext:
		return g.next()
	}
	return nil
}

func (g *guide) listLen() int {
	switch g.step {
	case 0:
		return len(g.agents)
	case 1:
		return len(config.Items())
	case 2:
		return len(densities)
	}
	return len(summaryOptions)
}

func (g *guide) toggle() {
	g.err = ""
	switch g.step {
	case 0:
		if len(g.agents) > 0 {
			g.err = g.agents[g.cursor].toggle()
		}
	case 1:
		toggleItem(&g.view, config.Items()[g.cursor])
	case 2:
		g.view.Density = densities[g.cursor].id
	}
}

func (g *guide) goTo(step int) {
	g.step, g.err = step, ""
	g.cursor = 0
	if step == 2 {
		for i, d := range densities {
			if d.id == g.view.Density {
				g.cursor = i
			}
		}
	}
	if step == 3 {
		g.plan()
	}
}

func (g *guide) back() {
	if g.step > 0 {
		g.goTo(g.step - 1)
	}
}

func (g *guide) next() tea.Cmd {
	if g.step < 3 {
		if g.step == 2 {
			g.view.Density = densities[g.cursor].id
		}
		g.goTo(g.step + 1)
		return nil
	}
	switch g.cursor {
	case 0:
		g.install()
	case 1:
		g.back()
	default:
		return tea.Quit
	}
	return nil
}

func (g *guide) selectedIDs() []string {
	ids := []string{}
	for _, a := range g.agents {
		if a.selected {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func (g *guide) agentName(id string) string {
	for _, a := range g.agents {
		if a.ID == id {
			return a.Name
		}
	}
	return id
}

// plan reads what installing would change; nothing is written.
func (g *guide) plan() {
	g.changes, g.planned, g.applied, g.err = nil, false, false, ""
	ids := g.selectedIDs()
	if len(ids) > 0 {
		changes, err := install.Plan(ids)
		if err != nil {
			g.err = "Error: " + err.Error()
			return
		}
		g.changes = changes
	}
	g.planned = true
}

// install applies the reviewed changes, then saves the settings: only a full
// success marks the guide done, so an aborted run starts the guide again.
func (g *guide) install() {
	if !g.planned {
		if g.plan(); !g.planned {
			return
		}
	}
	if !g.applied && len(g.changes) > 0 {
		if err := install.Apply(g.changes); err != nil {
			msg := "Error: " + err.Error()
			if g.plan(); g.err == "" {
				g.err = msg + " Review the list again and choose Install to retry."
			}
			return
		}
	}
	g.applied = true
	if err := config.Save(config.Config{View: g.view, Agents: g.selectedIDs()}); err != nil {
		g.err = "Error: the assistants are set up, but the settings were not saved: " + err.Error()
		return
	}
	g.err, g.done = "", true
}

func (g *guide) View() tea.View {
	v := tea.NewView(g.body())
	v.AltScreen = true
	return v
}

// previewView hides the plugins that are not installed yet; the saved view
// keeps them shown so their badges appear once they are installed.
func (g *guide) previewView() config.View {
	v := g.view
	v.Hidden = append([]string(nil), g.view.Hidden...)
	for id, detected := range g.plugins {
		if !detected {
			v.Hidden = append(v.Hidden, id)
		}
	}
	return v
}

func (g *guide) preview() string {
	return render.ANSI(previewLines(g.previewView(), DemoSession(), g.width-2))
}

func (g *guide) body() string {
	if g.width < minWidth || g.height < guideMinHeight {
		msg := fmt.Sprintf("Terminal too small: need at least %dx%d. q quits.", minWidth, guideMinHeight)
		return lipgloss.NewStyle().Width(max(g.width, 1)).Render(msg)
	}
	bold, dim := lipgloss.NewStyle().Bold(true), lipgloss.NewStyle().Faint(true)
	wrap := lipgloss.NewStyle().Width(g.width)
	var b strings.Builder
	line := func(s string) { b.WriteString(wrap.Render(s) + "\n") }

	if g.done {
		if len(g.selectedIDs()) > 0 {
			line(bold.Render("Done. Restart your assistant to see the bar."))
		} else {
			line(bold.Render("Done. The bar settings are saved; no assistant was changed."))
		}
		line("")
		line(dim.Render("Press any key to leave."))
		return b.String()
	}
	titles := []string{"Where should the bar run?", "What should the bar show?", "How much to show?", "Review what will change."}
	line(bold.Render(fmt.Sprintf("Step %d of 4: %s", g.step+1, titles[g.step])))
	line("")
	switch g.step {
	case 0:
		g.agentsStep(line)
	case 1:
		g.itemsStep(line, dim)
	case 2:
		g.densityStep(line, dim)
	case 3:
		g.summaryStep(line)
	}
	if g.err != "" {
		line("")
		line(g.err)
	}
	line("")
	hints := []string{"space select · enter next · esc back · q quit with no changes",
		"space toggle · enter next · esc back · q quit with no changes",
		"space select · enter next · esc back · q quit with no changes",
		"↑↓ choose · enter confirm · esc back · q quit with no changes"}
	line(dim.Render(hints[g.step]))
	return strings.TrimRight(b.String(), "\n")
}

func (g *guide) agentsStep(line func(string)) {
	found := false
	for i, a := range g.agents {
		row := mark(i == g.cursor, a.selected) + fmt.Sprintf("%-14s", a.Name)
		switch {
		case !a.Found:
			row += "not detected"
		case a.Installed:
			row += "detected, already set up"
		case a.CurrentStatusLine != "":
			row += "detected, already has a status line"
		default:
			row += "detected"
		}
		found = found || a.Found
		line(row)
	}
	if !found {
		line("")
		line("No supported assistant was found. Install Claude Code, OpenCode or Copilot CLI, then run rainbow-statusline again. You can still save the bar settings now.")
	}
}

func (g *guide) itemsStep(line func(string), dim lipgloss.Style) {
	for i, id := range config.Items() {
		row := mark(i == g.cursor, g.view.Shows(id)) + fmt.Sprintf("%-14s", itemNames[id])
		if id == config.Caveman {
			row = strings.TrimRight(row, " ") + " (estimate)"
		}
		if detected, isPlugin := g.plugins[id]; isPlugin && !detected {
			row += "  not detected, appears after install"
		}
		line(row)
	}
	line("")
	for _, l := range strings.Split(g.preview(), "\n") {
		line("  " + l)
	}
	line(dim.Render("Preview with demo data. Caveman's saved tokens are an estimate."))
}

func (g *guide) densityStep(line func(string), dim lipgloss.Style) {
	for i, d := range densities {
		line(mark(i == g.cursor, d.id == g.view.Density) + fmt.Sprintf("%-10s", d.name) + d.note)
	}
	line("")
	for _, l := range strings.Split(g.preview(), "\n") {
		line("  " + l)
	}
	line("")
	line(dim.Render("Everything can be changed later: run rainbow-statusline."))
}

func (g *guide) summaryStep(line func(string)) {
	if len(g.selectedIDs()) == 0 {
		line("No assistant is selected: only the bar settings will be saved. No assistant files change.")
	}
	for _, c := range g.changes {
		line(fmt.Sprintf("  %s: %s", c.Path, c.Summary))
	}
	if g.planned {
		for _, id := range g.selectedIDs() {
			touched := false
			for _, c := range g.changes {
				touched = touched || c.Agent == id
			}
			if !touched {
				line(fmt.Sprintf("  %s: already set up, nothing to change", g.agentName(id)))
			}
		}
	}
	if p, err := config.Path(); err == nil {
		line("  Bar settings: " + p)
	}
	for _, c := range g.changes {
		if c.Replaces != "" {
			line("")
			line(fmt.Sprintf("%s's current status line (%s) will be replaced. A backup is kept, and `rainbow-statusline uninstall` restores it.", g.agentName(c.Agent), render.Sanitize(c.Replaces)))
		}
	}
	line("")
	for i, o := range summaryOptions {
		c := "  "
		if i == g.cursor {
			c = "> "
		}
		line(c + o)
	}
}

// RunGuide walks a first-time user through setup. It writes nothing unless the
// user chooses Install on the last step.
func RunGuide() error {
	plugins := map[string]bool{}
	for _, i := range integration.All() {
		plugins[i.ID()] = i.Detected()
	}
	cfg, _ := config.Load()
	_, err := tea.NewProgram(newGuide(cfg, plugins, install.Detect())).Run()
	return err
}
