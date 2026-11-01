// Package tui is the configuration screen.
package tui

import (
	"time"

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

func previewLines(v config.View, s model.Session, width int) []render.Line {
	return render.Draw(s, demoBadges(), v, width)
}
