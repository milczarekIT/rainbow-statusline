package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

func draw(t *testing.T, s model.Session) string {
	t.Helper()
	old := render.Now
	render.Now = func() time.Time { return time.Unix(1793566870, 0) } // same clock as the goldens
	t.Cleanup(func() { render.Now = old })
	return render.ANSI(render.Draw(s, nil, config.View{}, 0))
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestClaudeSampleMatchesGolden(t *testing.T) {
	samples, _ := filepath.Glob("../../testdata/claude/*.json")
	if len(samples) == 0 {
		t.Fatal("no samples")
	}
	for _, sample := range samples {
		name := strings.TrimSuffix(filepath.Base(sample), ".json")
		t.Run(name, func(t *testing.T) {
			s := Parse("claude", read(t, sample))
			s.User, s.Host = "user", "host"
			want := strings.ReplaceAll(string(read(t, "../../testdata/golden/claude-"+name+".txt")), "\r\n", "\n")
			if got := draw(t, s); got != want {
				t.Errorf("\n got %q\nwant %q", got, want)
			}
		})
	}
}

func TestEmptyStdin(t *testing.T) {
	if got := draw(t, Parse("claude", nil)); !strings.Contains(got, "ctx \x1b[2m—") {
		t.Errorf("got %q", got)
	}
}

func TestTruncatedJSON(t *testing.T) {
	raw := read(t, "../../testdata/claude/session-after-reply.json")
	if got := draw(t, Parse("claude", raw[:len(raw)/2])); !strings.Contains(got, "ctx \x1b[2m—") {
		t.Errorf("got %q", got)
	}
}

func TestStartSampleNullsStayUnknown(t *testing.T) {
	s := Parse("claude", read(t, "../../testdata/claude/session-start.json"))
	if s.ContextUsed != nil || s.ContextPct != nil || s.CacheRead != nil || s.CacheExpiresAt != nil {
		t.Errorf("null must stay nil: %+v", s)
	}
	if s.ContextSize == nil || *s.ContextSize != 1000000 || s.Model != "Haiku 5.5" || s.Effort != "medium" {
		t.Errorf("got %+v", s)
	}
}

func TestAfterReplyFields(t *testing.T) {
	s := Parse("claude", read(t, "../../testdata/claude/session-after-reply.json"))
	if s.Agent != "claude" || s.SessionID != "redacted" || s.TranscriptPath != "redacted" || s.Cwd != "redacted" {
		t.Errorf("got %+v", s)
	}
	if *s.ContextUsed != 95264 || *s.ContextPct != 10 || *s.CacheRead != 28445 || *s.CacheWrite != 66817 {
		t.Errorf("got %+v", s)
	}
	if s.CacheExpiresAt.Unix() != 1793568670 || s.CacheTTL != "1h" || s.CacheEstimated {
		t.Errorf("got %+v", s)
	}
}

func TestWrongTypesKeepTheRest(t *testing.T) {
	s := Parse("claude", []byte(`{"model":{"display_name":"M"},"cost":{"total_cost_usd":"lots"},"context_window":{"used_percentage":"x","context_window_size":12.5,"current_usage":"y"},"prompt_cache":7}`))
	if s.Model != "M" || s.CostUSD != nil || s.ContextPct != nil || s.ContextSize != nil || s.CacheRead != nil || s.CacheExpiresAt != nil {
		t.Errorf("got %+v", s)
	}
}

func TestNegativeUsageIsUnknown(t *testing.T) {
	s := Parse("claude", []byte(`{"context_window":{"context_window_size":100,"current_usage":{"input_tokens":-5,"cache_read_input_tokens":50}}}`))
	if s.ContextUsed != nil {
		t.Errorf("got %d", *s.ContextUsed)
	}
}

func TestColdCacheIsExpired(t *testing.T) {
	for _, pc := range []string{`{"warm":false,"expires_at":1793568670,"ttl":"1h"}`, `{"warm":true,"ttl":"1h"}`} {
		s := Parse("claude", []byte(`{"context_window":{"context_window_size":100,"current_usage":{"input_tokens":1}},"prompt_cache":`+pc+`}`))
		if got := draw(t, s); !strings.Contains(got, "✖ expired") {
			t.Errorf("%s: got %q", pc, got)
		}
	}
}

func TestCwdPrefersWorkspace(t *testing.T) {
	if s := Parse("claude", []byte(`{"cwd":"/b","workspace":{"current_dir":"/a"}}`)); s.Cwd != "/a" {
		t.Errorf("got %q", s.Cwd)
	}
	if s := Parse("claude", []byte(`{"cwd":"/b"}`)); s.Cwd != "/b" {
		t.Errorf("got %q", s.Cwd)
	}
}

func TestUnknownAgentIsEmpty(t *testing.T) {
	if s := Parse("nope", read(t, "../../testdata/claude/session-after-reply.json")); s != (model.Session{}) {
		t.Errorf("got %+v", s)
	}
}
