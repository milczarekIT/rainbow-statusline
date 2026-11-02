package adapter

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

func drawn(s model.Session) string {
	var out string
	for _, l := range render.Draw(s, nil, config.View{}, 0) {
		for _, seg := range l {
			out += seg.Text
		}
		out += "\n"
	}
	return out
}

func TestOpenCodeNoSessionIsAllUnknown(t *testing.T) {
	for _, in := range []string{``, `{`, `{"cwd":"/w"}`, `{"cwd":"/w","messages":null,"bash":null}`, `{"cwd":"/w","messages":[]}`} {
		s := Parse("opencode", []byte(in))
		if s.Agent != "opencode" {
			t.Errorf("%q: agent %q", in, s.Agent)
		}
		if s.ContextUsed != nil || s.OutTokens != nil || s.CostUSD != nil || s.CacheRead != nil ||
			s.CacheWarm != nil || s.CacheExpiresAt != nil || s.SessionID != "" {
			t.Errorf("%q: home screen must have no numbers: %+v", in, s)
		}
		want := "@:" + s.Cwd + "\n— │ ctx — │ session ~$— ‖ cache —\n"
		if got := drawn(s); got != want {
			t.Errorf("%q:\ngot  %q\nwant %q", in, got, want)
		}
	}
}

// Numbers are those of testdata/opencode/export-session.json plus a second
// message, worked out by hand with the formulas of the plugin being replaced.
const fullSession = `{
  "sessionID": "ses_abc123", "cwd": "/w", "model": "opencode/big-pickle", "variant": "high", "contextLimit": 200000,
  "started": 1793565479000,
  "messages": [
    {"tokens": {"input": 10, "output": 200, "reasoning": 50, "cache": {"read": 5, "write": 7}}, "cost": 0.5, "completed": 1793565479500},
    {"tokens": {"input": 98623, "output": 2, "reasoning": 0, "cache": {"read": 1792, "write": 3000}}, "cost": 0.25, "completed": 1793565480000}
  ],
  "bash": [{"cmd": "rtk git status", "start": 1793565479100, "end": 1793565479200}, {"cmd": 5}]
}`

func TestOpenCodeFullSessionMatchesThePlugin(t *testing.T) {
	render.Now = func() time.Time { return time.UnixMilli(1793565480000 + 60_000) }
	defer func() { render.Now = time.Now }()

	s := Parse("opencode", []byte(fullSession))
	if s.Model != "big-pickle" || s.Effort != "high" || s.Cwd != "/w" || s.SessionID != "ses_abc123" {
		t.Errorf("identity: %+v", s)
	}
	// ctx = last message's input + cache read + cache write
	if s.ContextUsed == nil || *s.ContextUsed != 103415 || s.ContextSize == nil || *s.ContextSize != 200000 {
		t.Errorf("ctx: %v %v", s.ContextUsed, s.ContextSize)
	}
	// Σout = output + reasoning over all messages
	if s.OutTokens == nil || *s.OutTokens != 252 {
		t.Errorf("out: %v", s.OutTokens)
	}
	if s.CostUSD == nil || *s.CostUSD != 0.75 {
		t.Errorf("cost: %v", s.CostUSD)
	}
	if s.CacheRead == nil || *s.CacheRead != 1792 || s.CacheWrite == nil || *s.CacheWrite != 3000 {
		t.Errorf("cache: %v %v", s.CacheRead, s.CacheWrite)
	}
	if s.StartedAt == nil || s.StartedAt.UnixMilli() != 1793565479000 {
		t.Errorf("started: %v", s.StartedAt)
	}
	if len(s.BashRuns) != 1 || s.BashRuns[0] != (model.BashRun{Cmd: "rtk git status", Start: 1793565479100, End: 1793565479200}) {
		t.Errorf("bash: %+v", s.BashRuns)
	}
	want := "@:/w\nbig-pickle (high) │ ctx ▰▰▰▰▱▱▱▱ 103.4k / 200k (51.7%) · Σout 252 │ session ~$0.75 ‖ ⚡ read 1.8k · write 3k · ⏱ ~4:00\n"
	if got := drawn(s); got != want {
		t.Errorf("\ngot  %q\nwant %q", got, want)
	}
}

func TestOpenCodeExpiredAndRunning(t *testing.T) {
	render.Now = func() time.Time { return time.UnixMilli(1793565480000 + 301_000) }
	defer func() { render.Now = time.Now }()
	s := Parse("opencode", []byte(fullSession))
	if s.CacheExpiresAt == nil || !s.CacheEstimated {
		t.Fatalf("expiry: %+v", s)
	}
	if !strings.HasSuffix(drawn(s), "✖ expired\n") {
		t.Errorf("expired: %s", drawn(s))
	}
	// A reply still streaming has no completion time, so the timer is unknown.
	s = Parse("opencode", []byte(`{"sessionID":"ses_a","messages":[{"tokens":{"input":1,"output":0,"reasoning":0,"cache":{"read":2,"write":3}},"cost":0}]}`))
	if got := drawn(s); !strings.HasSuffix(got, "read 2 · write 3 · ⏱ —\n") {
		t.Errorf("running: %s", got)
	}
}

func TestOpenCodeBadValuesAreUnknownNotClamped(t *testing.T) {
	cases := map[string]string{
		"negative output": `{"sessionID":"ses_a","contextLimit":100,"messages":[{"tokens":{"input":1,"output":-5,"reasoning":0,"cache":{"read":0,"write":0}},"cost":1,"completed":1}]}`,
		"text output":     `{"sessionID":"ses_a","contextLimit":100,"messages":[{"tokens":{"input":1,"output":"x","reasoning":0,"cache":{"read":0,"write":0}},"cost":1,"completed":1}]}`,
		"no tokens":       `{"sessionID":"ses_a","contextLimit":100,"messages":[{"cost":1,"completed":1}]}`,
	}
	for name, in := range cases {
		if s := Parse("opencode", []byte(in)); s.OutTokens != nil {
			t.Errorf("%s: out %v", name, *s.OutTokens)
		}
	}
	s := Parse("opencode", []byte(`{"sessionID":"ses_a","messages":[{"tokens":{"input":1,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":-1,"completed":1}]}`))
	if s.CostUSD != nil {
		t.Errorf("negative cost: %v", *s.CostUSD)
	}
	s = Parse("opencode", []byte(`{"sessionID":"ses_a","contextLimit":"big","messages":[{"tokens":{"input":-9,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}]}`))
	if s.ContextUsed != nil || s.ContextSize != nil {
		t.Errorf("ctx: %v %v", s.ContextUsed, s.ContextSize)
	}
	// As in the plugin: nothing counted yet is no reading, not 0%.
	s = Parse("opencode", []byte(`{"sessionID":"ses_a","contextLimit":100,"messages":[{"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}},"cost":0}]}`))
	if s.ContextUsed != nil || s.OutTokens != nil {
		t.Errorf("zero: %v %v", s.ContextUsed, s.OutTokens)
	}
}

func TestOpenCodeSessionIDIsValidated(t *testing.T) {
	for _, id := range []string{"", "ses_", "abc", "ses_a b", "ses_a'--", "../ses_a", "ses_a/b", "ses_a-b", "ses_a\n"} {
		in, _ := jsonString(id)
		if s := Parse("opencode", []byte(`{"sessionID":`+in+`}`)); s.SessionID != "" {
			t.Errorf("%q kept", id)
		}
	}
	if s := Parse("opencode", []byte(`{"sessionID":"ses_Az09_x"}`)); s.SessionID != "ses_Az09_x" {
		t.Errorf("valid id dropped: %q", s.SessionID)
	}
}

func TestOpenCodeModelWithSlashes(t *testing.T) {
	if s := Parse("opencode", []byte(`{"model":"openrouter/anthropic/claude"}`)); s.Model != "anthropic/claude" {
		t.Errorf("got %q", s.Model)
	}
	if s := Parse("opencode", []byte(`{"model":"bare"}`)); s.Model != "bare" {
		t.Errorf("got %q", s.Model)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func jsonString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func TestOpenCodeOutputAboveTheCeilingIsUnknown(t *testing.T) {
	huge := int64(math.MaxInt64)
	msg := func(out int64) openCodeMessage {
		var m openCodeMessage
		m.Tokens.Output = json.RawMessage(strconv.FormatInt(out, 10))
		m.Tokens.Reasoning = json.RawMessage("0")
		return m
	}
	if got := sumOut([]openCodeMessage{msg(huge), msg(huge)}); got != nil {
		t.Errorf("overflowing values: %d", *got)
	}
	if got := sumOut([]openCodeMessage{msg(format.MaxTokens), msg(format.MaxTokens)}); got != nil {
		t.Errorf("sum above the ceiling: %d", *got)
	}
	if got := sumOut([]openCodeMessage{msg(5), msg(6)}); got == nil || *got != 11 {
		t.Errorf("ordinary sum: %v", got)
	}
}
