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

const (
	minWidth, minHeight = 64, 26
	leftWidth           = 44
	badgeCount          = 4 // the first rows; only these can be reordered
)

// variants are the choices an item offers; the first one is today's look.
var variants = map[string][]string{
	config.Caveman:   {"c+t", "c", "t"},
	config.RTK:       {"c+t", "c", "t"},
	config.ClaudeMem: {"c+t", "c", "t"},
	config.Context:   {"bar+pct", "pct"},
	config.Cache:     {"full", "timer"},
}

var itemNames = map[string]string{
	config.Ponytail: "Ponytail", config.Caveman: "Caveman", config.RTK: "RTK", config.ClaudeMem: "claude-mem",
	config.Model: "Model", config.Context: "Context", config.Output: "Output tokens", config.Cost: "Cost", config.Cache: "Cache",
}

type agentRow struct {
	install.Agent
	selected bool
}

// toggle flips the selection and returns a note when it cannot.
func (a *agentRow) toggle() string {
	if !a.Found {
		return a.Name + " was not found on this computer, so it cannot be selected."
	}
	a.selected = !a.selected
	return ""
}

type screen struct {
	view     config.View
	rows     []string // item ids in display order
	plugins  map[string]bool
	agents   []agentRow
	cursor   int
	agentCur int
	onAgents bool
	footer   string
	pending  []install.Change // a plan waiting for y or n
	width    int
	height   int
}

func newScreen(cfg config.Config, plugins map[string]bool, agents []install.Agent) *screen {
	v := cfg.View
	v.Order = append([]string(nil), v.Order...)
	v.Hidden = append([]string(nil), v.Hidden...)
	v.Variants = map[string]string{}
	for k, val := range cfg.Variants {
		v.Variants[k] = val
	}
	m := &screen{view: v, plugins: plugins, width: 80, height: 24}
	m.rows = orderedRows(v.Order)
	chosen := map[string]bool{}
	for _, id := range cfg.Agents {
		chosen[id] = true
	}
	for _, a := range agents {
		sel := chosen[a.ID]
		if cfg.Agents == nil {
			sel = a.Found
		}
		sel = sel && a.Found
		m.agents = append(m.agents, agentRow{Agent: a, selected: sel})
	}
	return m
}

// orderedRows puts the badges named in order first, then the other badges, then the session items.
func orderedRows(order []string) []string {
	items := config.Items()
	badges := items[:badgeCount]
	var rows []string
	seen := map[string]bool{}
	for _, id := range order {
		for _, b := range badges {
			if id == b && !seen[id] {
				seen[id] = true
				rows = append(rows, id)
			}
		}
	}
	for _, b := range badges {
		if !seen[b] {
			rows = append(rows, b)
		}
	}
	return append(rows, items[badgeCount:]...)
}

func (m *screen) Init() tea.Cmd { return nil }

func (m *screen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m, m.key(actionFor(msg))
	}
	return m, nil
}

func (m *screen) key(a action) tea.Cmd {
	if m.pending != nil {
		switch a {
		case actYes:
			changes := m.pending
			m.pending = nil
			m.apply(changes)
		case actNo:
			m.pending = nil
			m.footer = "Nothing was changed."
		case actQuit:
			return tea.Quit
		}
		return nil
	}
	m.footer = ""
	switch a {
	case actQuit:
		return tea.Quit
	case actSwitch:
		m.onAgents = !m.onAgents
	case actUp:
		m.step(-1)
	case actDown:
		m.step(1)
	case actToggle:
		m.toggle()
	case actLeft:
		m.cycle(-1)
	case actRight:
		m.cycle(1)
	case actMoveUp:
		m.move(-1)
	case actMoveDown:
		m.move(1)
	case actSave:
		m.save()
	}
	return nil
}

func (m *screen) step(d int) {
	if m.onAgents {
		m.agentCur = clamp(m.agentCur+d, len(m.agents))
		return
	}
	m.cursor = clamp(m.cursor+d, len(m.rows))
}

func clamp(i, n int) int {
	if i < 0 {
		return 0
	}
	if n > 0 && i >= n {
		return n - 1
	}
	return i
}

func (m *screen) toggle() {
	if m.onAgents {
		if len(m.agents) > 0 {
			m.footer = m.agents[m.agentCur].toggle()
		}
		return
	}
	toggleItem(&m.view, m.rows[m.cursor])
}

// toggleItem hides a shown item and shows a hidden one.
func toggleItem(v *config.View, id string) {
	if v.Shows(id) {
		v.Hidden = append(v.Hidden, id)
		return
	}
	var kept []string
	for _, h := range v.Hidden {
		if h != id {
			kept = append(kept, h)
		}
	}
	v.Hidden = kept
}

func (m *screen) variant(id string) string {
	options := variants[id]
	if len(options) == 0 {
		return ""
	}
	for _, o := range options {
		if o == m.view.Variants[id] {
			return o
		}
	}
	return options[0]
}

func (m *screen) cycle(d int) {
	if m.onAgents {
		return
	}
	id := m.rows[m.cursor]
	options := variants[id]
	if len(options) == 0 {
		m.footer = itemNames[id] + " has no variants."
		return
	}
	next := 0
	for i, o := range options {
		if o == m.variant(id) {
			next = (i + d + len(options)) % len(options)
		}
	}
	if next == 0 {
		delete(m.view.Variants, id)
		return
	}
	m.view.Variants[id] = options[next]
}

func (m *screen) move(d int) {
	if m.onAgents {
		return
	}
	to := m.cursor + d
	if m.cursor >= badgeCount {
		m.footer = itemNames[m.rows[m.cursor]] + " cannot be moved; only the four badges can."
		return
	}
	if to < 0 || to >= badgeCount {
		return
	}
	m.rows[m.cursor], m.rows[to] = m.rows[to], m.rows[m.cursor]
	m.cursor = to
	m.view.Order = append([]string(nil), m.rows[:badgeCount]...)
}

func (m *screen) selectedIDs() []string {
	ids := []string{}
	for _, a := range m.agents {
		if a.selected {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func (m *screen) agentName(id string) string {
	for _, a := range m.agents {
		if a.ID == id {
			return a.Name
		}
	}
	return id
}

func (m *screen) save() {
	ids := m.selectedIDs()
	var changes []install.Change
	if len(ids) > 0 {
		var err error
		if changes, err = install.Plan(ids); err != nil {
			m.footer = "Error: " + err.Error()
			return
		}
	}
	var replaced []string
	for _, c := range changes {
		if c.Replaces != "" {
			replaced = append(replaced, fmt.Sprintf("%s's current status line (%s)", m.agentName(c.Agent), render.Sanitize(c.Replaces)))
		}
	}
	if len(replaced) > 0 {
		m.pending = changes
		m.footer = "Replace " + strings.Join(replaced, " and ") + "? y/n"
		return
	}
	m.apply(changes)
}

func (m *screen) apply(changes []install.Change) {
	if len(changes) > 0 {
		if err := install.Apply(changes); err != nil {
			m.footer = "Error: " + err.Error()
			return
		}
	}
	ids := m.selectedIDs()
	if err := config.Save(config.Config{View: m.view, Agents: ids}); err != nil {
		m.footer = "Error: installed, but the settings were not saved: " + err.Error()
		return
	}
	var names []string
	for _, id := range ids {
		names = append(names, m.agentName(id))
	}
	msg := "Saved. No assistant selected, so nothing was installed."
	if len(names) > 0 {
		msg = "Saved. Restart " + strings.Join(names, " and ") + " to see the new status line."
	}
	for _, a := range m.agents {
		if !a.selected && a.Installed {
			msg += " " + a.Name + " stays installed; run `rainbow-statusline uninstall` to remove it."
		}
	}
	m.footer = msg
}

func (m *screen) View() tea.View {
	v := tea.NewView(m.body())
	v.AltScreen = true
	return v
}

// preview is the live bar for the current choices, as ANSI.
func (m *screen) preview() string {
	return render.ANSI(previewLines(m.view, DemoSession(), m.width-2))
}

func (m *screen) body() string {
	if m.width < minWidth || m.height < minHeight {
		msg := fmt.Sprintf("Terminal too small: need at least %dx%d. q quits.", minWidth, minHeight)
		return lipgloss.NewStyle().Width(max(m.width, 1)).Render(msg)
	}
	bold, dim := lipgloss.NewStyle().Bold(true), lipgloss.NewStyle().Faint(true)
	var left []string
	left = append(left, m.header("Bar items", !m.onAgents))
	for i, id := range m.rows {
		left = append(left, m.itemRow(i, id, bold, dim))
	}
	right := []string{m.header("Assistants", m.onAgents)}
	for i, a := range m.agents {
		right = append(right, m.agentRow(i, a, bold, dim))
	}
	rightWidth := m.width - leftWidth - 2
	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).MaxWidth(leftWidth).Render(strings.Join(left, "\n")),
		"  ",
		lipgloss.NewStyle().MaxWidth(rightWidth).Render(strings.Join(right, "\n")))

	var b strings.Builder
	b.WriteString(bold.Render("Rainbow Statusline") + "\n")
	b.WriteString(dim.Render(lipgloss.NewStyle().Width(m.width).Render("Preview with demo data. The real bar shortens itself in narrow terminals.")) + "\n")
	for _, l := range strings.Split(m.preview(), "\n") {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString("\n" + cols + "\n\n")
	wrap := lipgloss.NewStyle().Width(m.width)
	if m.footer != "" {
		b.WriteString(wrap.Render(m.footer) + "\n")
	}
	b.WriteString(dim.Render(wrap.Render("space toggle · ←→ variant · ⇧↑↓ order · tab switch · s save and install · q quit")))
	return b.String()
}

func (m *screen) header(title string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().Bold(true).Underline(true).Render(title)
	}
	return lipgloss.NewStyle().Faint(true).Render(title)
}

func mark(cursor, selected bool) string {
	c, box := "  ", "[ ] "
	if cursor {
		c = "> "
	}
	if selected {
		box = "[x] "
	}
	return c + box
}

func (m *screen) itemRow(i int, id string, bold, dim lipgloss.Style) string {
	line := mark(i == m.cursor && !m.onAgents, m.view.Shows(id)) + fmt.Sprintf("%-14s", itemNames[id])
	if v := m.variant(id); v != "" {
		line += fmt.Sprintf("%-10s", "‹"+v+"›")
	} else {
		line += strings.Repeat(" ", 10)
	}
	if detected, isPlugin := m.plugins[id]; isPlugin && !detected {
		line += "not detected"
	}
	switch {
	case i == m.cursor && !m.onAgents:
		return bold.Render(line)
	case !m.view.Shows(id):
		return dim.Render(line)
	}
	return line
}

func (m *screen) agentRow(i int, a agentRow, bold, dim lipgloss.Style) string {
	line := mark(i == m.agentCur && m.onAgents, a.selected) + a.Name
	switch {
	case a.Installed:
		line += "  installed"
	case !a.Found:
		line += "  not found"
	}
	if i == m.agentCur && m.onAgents {
		return bold.Render(line)
	}
	return line
}

// RunScreen opens the configuration screen and returns when the user quits.
func RunScreen(cfg config.Config) error {
	plugins := map[string]bool{}
	for _, i := range integration.All() {
		plugins[i.ID()] = i.Detected()
	}
	_, err := tea.NewProgram(newScreen(cfg, plugins, install.Detect())).Run()
	return err
}
