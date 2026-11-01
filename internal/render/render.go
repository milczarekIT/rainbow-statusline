// Package render turns a session and its badges into styled lines.
package render

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

// Now is the clock behind the cache timer; tests replace it.
var Now = time.Now

// Segment is a run of text in one style. FG is an SGR colour code such as "32" or "38;5;208".
type Segment struct {
	Text string `json:"text"`
	FG   string `json:"fg"`
	Bold bool   `json:"bold"`
	Dim  bool   `json:"dim"`
}

// Line is one row of the status line.
type Line []Segment

// Badge is a plugin's contribution to the badge row.
type Badge struct {
	Name  string
	Lines []Segment
}

const (
	red    = "31"
	yellow = "33"
	green  = "32"
)

var effortColours = map[string]Segment{
	"low":    {FG: "38;5;110"},
	"medium": {FG: "38;5;114"},
	"high":   {FG: "38;5;220"},
	"xhigh":  {FG: "38;5;208"},
	"max":    {FG: "38;5;196", Bold: true},
}

// Draw lays out the status line. A width of 0 or less means no limit.
func Draw(s model.Session, badges []Badge, cfg config.View, width int) []Line {
	var lines []Line
	if cfg.Density != "compact" && cfg.Density != "minimal" {
		lines = append(lines, Line{
			{Text: s.User + "@" + s.Host, FG: green},
			{Text: ":"},
			{Text: s.Cwd, FG: "34"},
		})
	}
	if cfg.Density != "minimal" {
		if row := badgeRow(badges, cfg); len(row) > 0 {
			lines = append(lines, row)
		}
	}
	stats := statsLine(s, 0, cfg)
	if width > 0 {
		for stage := 1; stage <= maxStage && lineCells(stats) > width; stage++ {
			stats = statsLine(s, stage, cfg)
		}
		for i, l := range lines {
			lines[i] = fit(l, width)
		}
		stats = fit(stats, width)
	}
	if len(stats) > 0 {
		lines = append(lines, stats)
	}
	for i, l := range lines {
		lines[i] = sanitize(l)
	}
	return lines
}

// badgeRow joins the badges the view shows, those named in cfg.Order first.
func badgeRow(badges []Badge, cfg config.View) Line {
	var shown []Badge
	used := make([]bool, len(badges))
	take := func(i int) {
		used[i] = true
		if cfg.Shows(strings.ToLower(badges[i].Name)) {
			shown = append(shown, badges[i])
		}
	}
	for _, id := range cfg.Order {
		for i, b := range badges {
			if !used[i] && strings.EqualFold(b.Name, id) {
				take(i)
			}
		}
	}
	for i := range badges {
		if !used[i] {
			take(i)
		}
	}
	var row Line
	for i, b := range shown {
		if i > 0 {
			row = append(row, Segment{Text: " "})
		}
		row = append(row, b.Lines...)
	}
	return row
}

// sanitize is the only gate between outside text and the terminal: a control
// rune in a path or a model name could otherwise drive it. Both outputs read
// Draw's lines. '?' is as wide as what it replaces, so fitting stays correct.
func sanitize(l Line) Line {
	out := make(Line, len(l))
	for i, s := range l {
		s.Text = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return '?'
			}
			return r
		}, s.Text)
		out[i] = s
	}
	return out
}

// maxStage is the last step of width trimming: 1 drops the cache details,
// 2 also Σout, 3 also the "/ size (pct)" part.
const maxStage = 3

func statsLine(s model.Session, stage int, cfg config.View) Line {
	minimal := cfg.Density == "minimal"
	show := func(id string) bool {
		if minimal && id != config.Model && id != config.Context && id != config.Cost {
			return false
		}
		return cfg.Shows(id)
	}
	sep := Segment{Text: " │ ", Dim: true}
	var l Line
	add := func(connector Segment, part Line) {
		if len(l) > 0 {
			l = append(l, connector)
		}
		l = append(l, part...)
	}
	if show(config.Model) {
		add(sep, modelSegments(s))
	}
	if show(config.Context) {
		add(sep, contextSegments(s, stage >= 3))
	}
	if show(config.Output) && s.OutTokens != nil && stage < 2 {
		connector := sep
		if show(config.Context) {
			connector = Segment{Text: " · ", Dim: true}
		}
		add(connector, Line{{Text: "Σout ", Dim: true}, {Text: k(*s.OutTokens), FG: "38;5;114", Bold: true}})
	}
	if show(config.Cost) {
		add(sep, Line{{Text: "session ~", Dim: true}, {Text: cost(s.CostUSD), FG: "38;5;220"}})
	}
	if show(config.Cache) {
		add(Segment{Text: " ‖ ", FG: "38;5;240"}, cacheSegments(s, stage >= 1))
	}
	return l
}

func modelSegments(s model.Session) Line {
	name := s.Model
	if name == "" {
		name = "?"
	}
	l := Line{{Text: name, FG: "38;5;208", Bold: true}}
	if s.Effort == "" {
		return l
	}
	eff, ok := effortColours[s.Effort]
	if !ok {
		eff.Dim = true
	}
	eff.Text = s.Effort
	return append(l, Segment{Text: " "}, Segment{Text: "(", Dim: true}, eff, Segment{Text: ")", Dim: true})
}

func contextSegments(s model.Session, short bool) Line {
	pct, ok := contextPct(s)
	if !ok {
		return Line{{Text: "ctx "}, {Text: format.Dash, Dim: true}}
	}
	col := green
	if pct >= 80 {
		col = red
	} else if pct >= 50 {
		col = yellow
	}
	l := Line{
		{Text: "ctx", Dim: true},
		{Text: " "},
		{Text: bar(pct, 8), FG: col},
		{Text: " "},
		{Text: k(*s.ContextUsed), FG: col, Bold: true},
	}
	if short {
		return l
	}
	return append(l,
		Segment{Text: " / " + k(*s.ContextSize) + " ", Dim: true},
		Segment{Text: "(" + fixed(pct, 1) + "%)", FG: col})
}

// maxPctDisagreement is how many points the assistant's own percentage may
// differ from used/size before the counted tokens win.
const maxPctDisagreement = 2

func contextPct(s model.Session) (float64, bool) {
	if s.ContextUsed == nil || s.ContextSize == nil {
		return 0, false
	}
	used, size := float64(*s.ContextUsed), float64(*s.ContextSize)
	if _, ok := format.Pct(used, size, 1); !ok {
		return 0, false
	}
	pct := used * 100 / size
	if s.ContextPct != nil && math.Abs(*s.ContextPct-pct) <= maxPctDisagreement {
		pct = *s.ContextPct
	}
	_, ok := format.Pct(pct, 100, 1)
	return pct, ok
}

func bar(pct float64, w int) string {
	n := int(math.Round(pct / 100 * float64(w)))
	return strings.Repeat("▰", n) + strings.Repeat("▱", w-n)
}

func cost(c *float64) string {
	if c == nil || *c < 0 || math.IsNaN(*c) || math.IsInf(*c, 0) {
		return "$" + format.Dash
	}
	return "$" + fixed(*c, 2)
}

func cacheSegments(s model.Session, short bool) Line {
	if s.CacheWarm == nil && s.CacheExpiresAt == nil {
		return Line{{Text: "cache " + format.Dash, Dim: true}}
	}
	if short {
		return Line{timer(s)}
	}
	dot := Segment{Text: " · ", Dim: true}
	l := Line{
		{Text: "⚡", FG: "38;5;39"},
		{Text: " "},
		{Text: "read ", Dim: true},
		{Text: optionalK(s.CacheRead), FG: "38;5;45"},
		dot,
		{Text: "write ", Dim: true},
		{Text: optionalK(s.CacheWrite), FG: "38;5;213"},
		dot,
		timer(s),
	}
	if s.CacheEstimated { // the timer's "~" already says the life is assumed
		return l
	}
	ttl := s.CacheTTL
	if ttl == "" {
		ttl = "?"
	}
	return append(l, Segment{Text: " "}, Segment{Text: ttl, Dim: true})
}

// maxCacheLeft is far beyond any cache TTL; a longer countdown is not believable.
const maxCacheLeft = 24 * 60 * 60

func timer(s model.Session) Segment {
	expired := Segment{Text: "✖ expired", FG: red, Bold: true}
	if s.CacheWarm != nil && !*s.CacheWarm {
		return expired
	}
	unknown := Segment{Text: "⏱ " + format.Dash, Dim: true}
	if s.CacheExpiresAt == nil {
		return unknown
	}
	left := s.CacheExpiresAt.Unix() - Now().Unix()
	if left > maxCacheLeft {
		return unknown
	}
	if left <= 0 {
		return expired
	}
	life := int64(300)
	if s.CacheTTL == "1h" {
		life = 3600
	}
	text := "⏱ "
	if s.CacheEstimated {
		text += "~"
	}
	text += clock(left)
	switch {
	case left <= 60:
		return Segment{Text: text, FG: red, Bold: true}
	case float64(left) <= float64(life)*0.4:
		return Segment{Text: text, FG: yellow}
	}
	return Segment{Text: text, FG: green}
}

func clock(sec int64) string {
	m := sec / 60
	if m >= 60 {
		return fmt.Sprintf("%d:%02d:%02d", m/60, m%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", m, sec%60)
}

func optionalK(n *int64) string {
	if n == nil {
		return format.Dash
	}
	return k(*n)
}

// k is the script's compact count for this line: 1k and 1000k rather than
// format.Tokens' 1.0k and 1.0M.
func k(n int64) string {
	switch {
	case n < 0:
		return format.Dash
	case n < 1000:
		return fmt.Sprint(n)
	}
	v := math.Round(float64(n)/100) / 10
	if v == math.Floor(v) {
		return fmt.Sprintf("%.0fk", v)
	}
	return fmt.Sprintf("%.1fk", v)
}

// fixed rounds half away from zero, as the script does, before formatting.
func fixed(v float64, digits int) string {
	scale := math.Pow(10, float64(digits))
	return fmt.Sprintf("%.*f", digits, math.Round(v*scale)/scale)
}

func lineCells(l Line) int {
	n := 0
	for _, s := range l {
		n += cells(s.Text)
	}
	return n
}

// fit cuts a line to width cells, ending it with an ellipsis when it had to cut.
func fit(l Line, width int) Line {
	if lineCells(l) <= width {
		return l
	}
	room := width - 1
	var out Line
	for _, s := range l {
		var kept strings.Builder
		for _, r := range s.Text {
			w := runeCells(r)
			if w > room {
				room = 0
				break
			}
			room -= w
			kept.WriteRune(r)
		}
		if kept.Len() > 0 {
			s.Text = kept.String()
			out = append(out, s)
		}
		if room == 0 {
			break
		}
	}
	return append(out, Segment{Text: "…", Dim: true})
}

func cells(s string) int {
	n := 0
	for _, r := range s {
		n += runeCells(r)
	}
	return n
}

// runeCells is the terminal width of a rune: 2 for East Asian wide characters and emoji.
func runeCells(r rune) int {
	switch {
	case r == '⚡',
		r >= 0x1100 && r <= 0x115F,
		r >= 0x2E80 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3,
		r >= 0xF900 && r <= 0xFAFF,
		r >= 0xFE30 && r <= 0xFE6F,
		r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1FAFF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}
