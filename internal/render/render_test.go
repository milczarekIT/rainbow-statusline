package render

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

func ptr[T any](v T) *T { return &v }

var fixedNow = time.Unix(1793566870, 0)

func freeze(t *testing.T) {
	t.Helper()
	old := Now
	Now = func() time.Time { return fixedNow }
	t.Cleanup(func() { Now = old })
}

func fullSession() model.Session {
	return model.Session{
		User: "user", Host: "host", Cwd: "/a/very/long/working/directory/that/does/not/fit/in/forty/cells",
		Model: "Haiku 5.5", Effort: "medium",
		ContextUsed: ptr[int64](95264), ContextSize: ptr[int64](1000000), OutTokens: ptr[int64](1234),
		CacheRead: ptr[int64](28445), CacheWrite: ptr[int64](66817), CostUSD: ptr(0.01),
		CacheExpiresAt: ptr(fixedNow.Add(30 * time.Minute)), CacheTTL: "1h",
	}
}

func text(l Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

func last(lines []Line) string { return text(lines[len(lines)-1]) }

func TestContextOver100(t *testing.T) {
	s := model.Session{ContextUsed: ptr[int64](3000000), ContextSize: ptr[int64](200000)}
	got := last(Draw(s, nil, config.View{}, 0))
	if !strings.Contains(got, "ctx —") {
		t.Errorf("want ctx dash, got %q", got)
	}
	if strings.ContainsAny(got, "▰▱") {
		t.Errorf("no bar expected, got %q", got)
	}
}

func TestReportedContextPct(t *testing.T) {
	cases := []struct {
		name       string
		used, size int64
		reported   float64
		want       string
	}{
		{"close keeps reported", 95264, 1000000, 10, "(10.0%)"},
		{"exactly 2 points keeps reported", 10, 100, 12, "(12.0%)"},
		{"disagreement uses used/size", 1000, 200000, 100, "(0.5%)"},
		{"out of range reported uses used/size", 1, 10, 140, "(10.0%)"},
	}
	for _, c := range cases {
		s := model.Session{ContextUsed: ptr(c.used), ContextSize: ptr(c.size), ContextPct: ptr(c.reported)}
		if got := last(Draw(s, nil, config.View{}, 0)); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in %q", c.name, c.want, got)
		}
	}
}

func TestReportedPctOver100WithinToleranceIsDash(t *testing.T) {
	s := model.Session{ContextUsed: ptr[int64](100), ContextSize: ptr[int64](100), ContextPct: ptr(101.0)}
	if got := last(Draw(s, nil, config.View{}, 0)); !strings.Contains(got, "ctx —") {
		t.Errorf("got %q", got)
	}
}

func TestNegativeValuesRenderDash(t *testing.T) {
	s := model.Session{
		ContextUsed: ptr[int64](-1), ContextSize: ptr[int64](10), CostUSD: ptr(-1.0),
		CacheRead: ptr[int64](-5), CacheWrite: ptr[int64](-5), CacheExpiresAt: ptr(time.Unix(0, 0)),
	}
	got := last(Draw(s, nil, config.View{}, 0))
	for _, want := range []string{"ctx —", "$—", "read —", "write —"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in %q", want, got)
		}
	}
}

func TestNarrowWidth(t *testing.T) {
	freeze(t)
	for _, width := range []int{40, 1} {
		lines := Draw(fullSession(), []Badge{{Name: "RTK", Lines: []Segment{{Text: strings.Repeat("x", 80)}}}}, config.View{}, width)
		if len(lines) != 3 {
			t.Fatalf("want 3 lines, got %d", len(lines))
		}
		for _, l := range lines {
			if c := cells(text(l)); c > width {
				t.Errorf("width %d: line has %d cells: %q", width, c, text(l))
			}
		}
		for _, raw := range strings.Split(ANSI(lines), "\n") {
			if strings.Contains(raw, "\x1b[") && !strings.HasSuffix(raw, "\x1b[0m") {
				t.Errorf("unbalanced ANSI: %q", raw)
			}
			if opened, reset := len(regexp.MustCompile(`\x1b\[[0-9;]*[1-9][0-9;]*m`).FindAllString(raw, -1)), strings.Count(raw, "\x1b[0m"); opened != reset {
				t.Errorf("%d opened, %d reset: %q", opened, reset, raw)
			}
		}
	}
}

func TestTrimOrder(t *testing.T) {
	freeze(t)
	full := last(Draw(fullSession(), nil, config.View{}, 0))
	if !strings.Contains(full, "read 28.4k · write 66.8k · ⏱ 30:00 1h") || !strings.Contains(full, "Σout 1.2k") {
		t.Fatalf("unexpected full line %q", full)
	}
	cases := []struct {
		stage      int
		drop, keep []string
	}{
		{stage: 1, drop: []string{"read", "write", "1h"}, keep: []string{"Σout", "/ 1000k (9.5%)", "⏱ 30:00"}},
		{stage: 2, drop: []string{"read", "Σout"}, keep: []string{"/ 1000k (9.5%)", "⏱ 30:00"}},
		{stage: 3, drop: []string{"read", "Σout", "/ 1000k"}, keep: []string{"95.3k", "⏱ 30:00"}},
	}
	for _, c := range cases {
		got := text(statsLine(fullSession(), c.stage))
		for _, d := range c.drop {
			if strings.Contains(got, d) {
				t.Errorf("%q must be dropped from %q", d, got)
			}
		}
		for _, k := range c.keep {
			if !strings.Contains(got, k) {
				t.Errorf("%q must stay in %q", k, got)
			}
		}
		if got2 := last(Draw(fullSession(), nil, config.View{}, cells(got))); got2 != got {
			t.Errorf("width %d: got %q, want %q", cells(got), got2, got)
		}
	}
}

func TestCacheTimer(t *testing.T) {
	freeze(t)
	cases := []struct {
		left time.Duration
		ttl  string
		want Segment
	}{
		{30 * time.Minute, "1h", Segment{Text: "⏱ 30:00", FG: "32"}},
		{90*time.Minute + 5*time.Second, "1h", Segment{Text: "⏱ 1:30:05", FG: "32"}},
		{24 * time.Minute, "1h", Segment{Text: "⏱ 24:00", FG: "33"}},
		{2 * time.Minute, "5m", Segment{Text: "⏱ 2:00", FG: "33"}},
		{3 * time.Minute, "5m", Segment{Text: "⏱ 3:00", FG: "32"}},
		{60 * time.Second, "5m", Segment{Text: "⏱ 1:00", FG: "31", Bold: true}},
		{0, "5m", Segment{Text: "✖ expired", FG: "31", Bold: true}},
	}
	for _, c := range cases {
		s := fullSession()
		s.CacheExpiresAt, s.CacheTTL = ptr(fixedNow.Add(c.left)), c.ttl
		if got := timer(s); got != c.want {
			t.Errorf("left %v ttl %s: got %+v, want %+v", c.left, c.ttl, got, c.want)
		}
	}
}

func TestEstimatedCacheTimerHasTilde(t *testing.T) {
	freeze(t)
	s := fullSession()
	s.CacheEstimated = true
	if got := timer(s).Text; got != "⏱ ~30:00" {
		t.Errorf("got %q", got)
	}
}

func TestContextColourThresholds(t *testing.T) {
	cases := []struct {
		used int64
		fg   string
		bar  string
	}{
		{49, "32", "▰▰▰▰▱▱▱▱"},
		{50, "33", "▰▰▰▰▱▱▱▱"},
		{79, "33", "▰▰▰▰▰▰▱▱"},
		{80, "31", "▰▰▰▰▰▰▱▱"},
		{100, "31", "▰▰▰▰▰▰▰▰"},
	}
	for _, c := range cases {
		segs := contextSegments(model.Session{ContextUsed: ptr(c.used), ContextSize: ptr[int64](100)}, false)
		if segs[2].Text != c.bar || segs[2].FG != c.fg {
			t.Errorf("used %d: got %+v", c.used, segs[2])
		}
	}
}

func TestTokenFormatMatchesScript(t *testing.T) {
	cases := map[int64]string{0: "0", 999: "999", 1000: "1k", 1100: "1.1k", 95264: "95.3k", 1000000: "1000k", -1: "—"}
	for in, want := range cases {
		if got := k(in); got != want {
			t.Errorf("k(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestBadgesFormMiddleLine(t *testing.T) {
	badges := []Badge{{Name: "A", Lines: []Segment{{Text: "[A]"}}}, {Name: "B", Lines: []Segment{{Text: "[B]"}}}}
	lines := Draw(model.Session{}, badges, config.View{}, 0)
	if len(lines) != 3 || text(lines[1]) != "[A] [B]" {
		t.Errorf("got %d lines, middle %q", len(lines), text(lines[1]))
	}
}

func TestANSI(t *testing.T) {
	got := ANSI([]Line{
		{{Text: "a"}, {Text: "b", FG: "32"}, {Text: "c", Dim: true}, {Text: "d", FG: "31", Bold: true}},
		{{Text: "e"}},
	})
	want := "a\x1b[32mb\x1b[0m\x1b[2mc\x1b[0m\x1b[1;31md\x1b[0m\ne"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJSON(t *testing.T) {
	got := JSON([]Line{{{Text: "a", FG: "32", Bold: true, Dim: true}}})
	var parsed [][]map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	seg := parsed[0][0]
	if seg["text"] != "a" || seg["fg"] != "32" || seg["bold"] != true || seg["dim"] != true {
		t.Errorf("got %s", got)
	}
	if string(JSON(nil)) != "[]" {
		t.Errorf("nil lines: %s", JSON(nil))
	}
}

func TestControlCharactersAreReplaced(t *testing.T) {
	freeze(t)
	payloads := []string{"a\x1b[31mb", "a\x1b]0;title\x07b", "a\nb", "a\rb", "a\x7fb", "a\u009bb"}
	for _, p := range payloads {
		s := fullSession()
		s.Cwd, s.Model, s.Effort, s.CacheTTL = p, p, p, p
		s.User, s.Host = p, p
		badges := []Badge{{Name: "B", Lines: []Segment{{Text: p}}}}
		for _, width := range []int{0, 40} {
			lines := Draw(s, badges, config.View{}, width)
			for _, l := range lines {
				for _, seg := range l {
					if strings.ContainsFunc(seg.Text, unicode.IsControl) {
						t.Errorf("%q width %d: control rune in segment %q", p, width, seg.Text)
					}
				}
			}
			out := ANSI(lines)
			if width == 0 && !strings.Contains(out, "\x1b[34m"+strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return '?'
				}
				return r
			}, p)+"\x1b[0m") {
				t.Errorf("%q: own colour codes must stay around the replaced text: %q", p, out)
			}
			if n := strings.Count(out, "\n"); n != len(lines)-1 {
				t.Errorf("%q: %d newlines for %d lines", p, n, len(lines))
			}
			stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")
			if strings.ContainsFunc(strings.ReplaceAll(stripped, "\n", ""), unicode.IsControl) {
				t.Errorf("%q: injected control rune in ANSI: %q", p, out)
			}
			var parsed [][]Segment
			if err := json.Unmarshal(JSON(lines), &parsed); err != nil {
				t.Fatal(err)
			}
			for _, l := range parsed {
				for _, seg := range l {
					if strings.ContainsFunc(seg.Text, unicode.IsControl) {
						t.Errorf("%q: control rune in JSON %q", p, seg.Text)
					}
				}
			}
		}
	}
}

func TestDrawDoesNotChangeCallersBadges(t *testing.T) {
	badges := []Badge{{Lines: []Segment{{Text: "a\x1bb"}}}}
	Draw(model.Session{}, badges, config.View{}, 0)
	if badges[0].Lines[0].Text != "a\x1bb" {
		t.Errorf("got %q", badges[0].Lines[0].Text)
	}
}

func TestCacheState(t *testing.T) {
	freeze(t)
	future := ptr(fixedNow.Add(30 * time.Minute))
	cases := []struct {
		name    string
		warm    *bool
		expires *time.Time
		want    string
	}{
		{"unknown warm keeps timer", nil, future, "⏱ 30:00"},
		{"warm keeps timer", ptr(true), future, "⏱ 30:00"},
		{"cold is expired", ptr(false), future, "✖ expired"},
		{"cold without expiry is expired", ptr(false), nil, "✖ expired"},
		{"warm without expiry is unknown", ptr(true), nil, "⏱ —"},
		{"past expiry is expired", nil, ptr(fixedNow.Add(-time.Minute)), "✖ expired"},
		{"unbelievable expiry is unknown", ptr(true), ptr(fixedNow.Add(48 * time.Hour)), "⏱ —"},
	}
	for _, c := range cases {
		s := model.Session{CacheWarm: c.warm, CacheExpiresAt: c.expires, CacheTTL: "1h"}
		got := last(Draw(s, nil, config.View{}, 0))
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: want %q in %q", c.name, c.want, got)
		}
		if !strings.Contains(got, "read —") || !strings.Contains(got, "write —") {
			t.Errorf("%s: read and write must be dashes: %q", c.name, got)
		}
	}
	if got := last(Draw(model.Session{CacheRead: ptr[int64](5)}, nil, config.View{}, 0)); !strings.Contains(got, "cache —") {
		t.Errorf("no cache data: got %q", got)
	}
}
