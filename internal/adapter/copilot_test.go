package adapter

import (
	"strings"
	"testing"
)

func TestCopilotAfterReplyFields(t *testing.T) {
	s := Parse("copilot", read(t, "../../testdata/copilot/session-after-reply.json"))
	if s.Agent != "copilot" || s.Model != "Copilot Fusion → mai-code-1.1-flash" {
		t.Errorf("agent/model: %q %q", s.Agent, s.Model)
	}
	if s.ContextUsed == nil || *s.ContextUsed != 40708 {
		t.Errorf("used: %v", s.ContextUsed)
	}
	if s.ContextSize == nil || *s.ContextSize != 200000 {
		t.Errorf("size: %v", s.ContextSize)
	}
	if s.ContextPct == nil || *s.ContextPct != 20 {
		t.Errorf("pct: %v", s.ContextPct)
	}
	if s.CacheRead == nil || *s.CacheRead != 1280 || s.CacheWrite == nil || *s.CacheWrite != 0 {
		t.Errorf("cache: %v %v", s.CacheRead, s.CacheWrite)
	}
	if s.CostUSD != nil || s.Effort != "" || s.CacheWarm != nil || s.CacheExpiresAt != nil || s.CacheTTL != "" {
		t.Errorf("fields Copilot does not have must stay unknown: %+v", s)
	}
	if s.SessionID != "redacted" || s.Cwd != "redacted" {
		t.Errorf("ids: %q %q", s.SessionID, s.Cwd)
	}
}

func TestCopilotRendersContextAndDashes(t *testing.T) {
	for name, want := range map[string]string{
		"session-after-reply": "\x1b[2mctx\x1b[0m \x1b[32m▰▰▱▱▱▱▱▱\x1b[0m \x1b[1;32m40.7k\x1b[0m\x1b[2m / 200k \x1b[0m\x1b[32m(20.0%)",
		"session-start":       "\x1b[2mctx\x1b[0m \x1b[32m▱▱▱▱▱▱▱▱\x1b[0m \x1b[1;32m0\x1b[0m\x1b[2m / 200k \x1b[0m\x1b[32m(0.0%)",
	} {
		s := Parse("copilot", read(t, "../../testdata/copilot/"+name+".json"))
		got := draw(t, s)
		t.Logf("%s: %q", name, got)
		if !strings.Contains(got, want) {
			t.Errorf("%s: missing %q in %q", name, want, got)
		}
		if !strings.Contains(got, "cache —") || !strings.Contains(got, "$—") {
			t.Errorf("%s: cost and cache must render a dash: %q", name, got)
		}
	}
}

func TestCopilotUsedPercentageIsNotRead(t *testing.T) {
	raw := `{"context_window":{"used_percentage":95,"current_context_tokens":1000,"displayed_context_limit":200000,"current_context_used_percentage":null}}`
	s := Parse("copilot", []byte(raw))
	if s.ContextPct != nil {
		t.Errorf("pct must come from current_context_used_percentage only: %v", *s.ContextPct)
	}
}

func TestCopilotDisagreeingPercentageIsNotTrusted(t *testing.T) {
	raw := `{"context_window":{"current_context_tokens":40000,"displayed_context_limit":200000,"current_context_used_percentage":90}}`
	got := draw(t, Parse("copilot", []byte(raw)))
	if !strings.Contains(got, "(20.0%)") {
		t.Errorf("counted tokens must win: %q", got)
	}
}

func TestCopilotBadInput(t *testing.T) {
	for _, in := range []string{"", "{", `{"context_window":{"current_context_tokens":"x","displayed_context_limit":null}}`, `{"context_window":{"current_context_tokens":-5,"displayed_context_limit":200000}}`} {
		if got := draw(t, Parse("copilot", []byte(in))); !strings.Contains(got, "ctx \x1b[2m—") {
			t.Errorf("%q: got %q", in, got)
		}
	}
}
