// Package tui is the configuration screen.
package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

func ptr[T any](v T) *T { return &v }

// DemoSession is the made-up session the preview draws. It never reads an
// assistant's data; the numbers are the spec's example.
func DemoSession() model.Session {
	expires := render.Now().Add(2*time.Minute + 32*time.Second)
	return model.Session{
		User: "user", Host: "host", Cwd: "project",
		Model: "Opus 5.5", Effort: "medium",
		ContextUsed: ptr[int64](118100), ContextSize: ptr[int64](200000), ContextPct: ptr(59.0),
		OutTokens: ptr[int64](84100), CostUSD: ptr(8.67),
		CacheRead: ptr[int64](8200), CacheWrite: ptr[int64](0),
		CacheWarm: ptr(true), CacheExpiresAt: &expires, CacheTTL: "1h",
	}
}

func pair(name, colour, c, t string) render.Badge {
	return render.Badge{Name: name, Lines: []render.Segment{
		{Text: "[" + name + "]", FG: colour},
		{Text: " (c: " + c + " | t: " + t + ")"},
	}}
}

// demoBadges are the four badges with the spec's example figures.
func demoBadges() []render.Badge {
	return []render.Badge{
		{Name: "PONYTAIL", Lines: []render.Segment{{Text: "[PONYTAIL]", FG: "38;5;108"}}},
		pair("CAVEMAN", "38;5;172", "~156.1k", "~185.8M"),
		pair("RTK", "38;5;141", "1.1k · 28.5%", "38.3M · 55.0%"),
		pair("CLAUDE-MEM", "38;5;75", "403.2k · 87%", "67.5M · 92%"),
	}
}

// Preview draws the status line for the view with demo badges and the given
// session, the same way the assistants get it.
func Preview(v config.View, s model.Session) string {
	return render.ANSI(previewLines(v, s, 0))
}

// previewLines draws the whole demo bar untrimmed and, when width is positive,
// wraps lines that are wider at segment boundaries. Only the preview wraps:
// the real bar shortens itself instead.
func previewLines(v config.View, s model.Session, width int) []render.Line {
	lines := render.Draw(s, demoBadges(), v, 0)
	if width <= 0 {
		return lines
	}
	var out []render.Line
	for _, l := range lines {
		out = append(out, wrap(l, width)...)
	}
	return out
}

func wrap(l render.Line, width int) []render.Line {
	var out []render.Line
	var cur render.Line
	used := 0
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
		}
		cur, used = nil, 0
	}
	for _, seg := range l {
		if used == 0 {
			seg.Text = strings.TrimLeft(seg.Text, " ")
		}
		w := lipgloss.Width(seg.Text)
		if used+w > width && used > 0 {
			flush()
			seg.Text = strings.TrimLeft(seg.Text, " ")
			w = lipgloss.Width(seg.Text)
		}
		for w > width { // one segment wider than the screen: cut it
			cut := []rune(seg.Text)
			n := len(cut)
			for n > 0 && lipgloss.Width(string(cut[:n])) > width {
				n--
			}
			head := seg
			head.Text = string(cut[:n])
			cur = append(cur, head)
			flush()
			seg.Text = string(cut[n:])
			w = lipgloss.Width(seg.Text)
		}
		if seg.Text == "" {
			continue
		}
		cur = append(cur, seg)
		used += w
	}
	flush()
	return out
}
