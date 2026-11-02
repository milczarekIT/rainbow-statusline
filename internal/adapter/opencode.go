package adapter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/integration"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

// openCodeCacheLife is in ms. OpenCode reports no cache expiry; as in the plugin
// this replaces, the timer assumes Anthropic's default 5 minutes from the last
// response, so it is an estimate.
const openCodeCacheLife = 300_000

// openCodeInput is what plugin/opencode/tui.tsx collects from the TUI API.
type openCodeInput struct {
	SessionID    string            `json:"sessionID"`
	Cwd          string            `json:"cwd"`
	Model        string            `json:"model"`
	Variant      string            `json:"variant"`
	ContextLimit json.RawMessage   `json:"contextLimit"`
	Started      json.RawMessage   `json:"started"`
	Messages     []openCodeMessage `json:"messages"`
	Bash         []json.RawMessage `json:"bash"`
}

type openCodeMessage struct {
	Tokens struct {
		Input     json.RawMessage `json:"input"`
		Output    json.RawMessage `json:"output"`
		Reasoning json.RawMessage `json:"reasoning"`
		Cache     struct {
			Read  json.RawMessage `json:"read"`
			Write json.RawMessage `json:"write"`
		} `json:"cache"`
	} `json:"tokens"`
	Cost      json.RawMessage `json:"cost"`
	Completed json.RawMessage `json:"completed"`
}

func parseOpenCode(stdin []byte) model.Session {
	var in openCodeInput
	_ = json.Unmarshal(stdin, &in)

	s := model.Session{
		Agent:       "opencode",
		Cwd:         in.Cwd,
		Model:       openCodeModel(in.Model),
		Effort:      in.Variant,
		ContextSize: optional[int64](in.ContextLimit),
		BashRuns:    openCodeBash(in.Bash),
	}
	if integration.ValidOpenCodeSession(in.SessionID) {
		s.SessionID = in.SessionID
	}
	if ms := positive(in.Started); ms != nil {
		at := time.UnixMilli(*ms)
		s.StartedAt = &at
	}
	if s.SessionID != "" {
		s.CostUSD = sumCost(in.Messages)
	}
	if len(in.Messages) == 0 {
		return s
	}
	s.OutTokens = sumOut(in.Messages)

	last := in.Messages[len(in.Messages)-1]
	s.CacheRead = token(last.Tokens.Cache.Read)
	s.CacheWrite = token(last.Tokens.Cache.Write)
	input := token(last.Tokens.Input)
	// Nothing counted yet is no reading, not 0%.
	if input != nil && s.CacheRead != nil && s.CacheWrite != nil {
		if used := *input + *s.CacheRead + *s.CacheWrite; used > 0 {
			s.ContextUsed = &used
		}
	}
	s.CacheEstimated = true
	if done := positive(last.Completed); done != nil {
		at := time.UnixMilli(*done + openCodeCacheLife)
		s.CacheExpiresAt = &at
	} else {
		warm := true // the reply is still running
		s.CacheWarm = &warm
	}
	return s
}

// openCodeModel drops the provider from "provider/model".
func openCodeModel(m string) string {
	if m == "" {
		return format.Dash
	}
	if _, rest, ok := strings.Cut(m, "/"); ok && rest != "" {
		return rest
	}
	return m
}

func openCodeBash(raw []json.RawMessage) []model.BashRun {
	var runs []model.BashRun
	for _, r := range raw {
		var run struct {
			Cmd        string `json:"cmd"`
			Start, End int64
		}
		if json.Unmarshal(r, &run) == nil && run.Cmd != "" && run.Start > 0 && run.End >= run.Start {
			runs = append(runs, model.BashRun{Cmd: run.Cmd, Start: run.Start, End: run.End})
		}
	}
	return runs
}

// token is a count that is present, not negative and within the ceiling.
func token(raw json.RawMessage) *int64 {
	if v := optional[int64](raw); v != nil && plausible(*v) {
		return v
	}
	return nil
}

func positive(raw json.RawMessage) *int64 {
	if v := optional[int64](raw); v != nil && *v > 0 {
		return v
	}
	return nil
}

// sumOut is output + reasoning over the messages; one bad value makes the sum
// unknown, and no output yet shows nothing.
func sumOut(msgs []openCodeMessage) *int64 {
	var total int64
	for _, m := range msgs {
		o, r := token(m.Tokens.Output), token(m.Tokens.Reasoning)
		if o == nil || r == nil {
			return nil
		}
		if total += *o + *r; total > format.MaxTokens {
			return nil
		}
	}
	if total == 0 {
		return nil
	}
	return &total
}

func sumCost(msgs []openCodeMessage) *float64 {
	var total float64
	for _, m := range msgs {
		c := optional[float64](m.Cost)
		if c == nil || *c < 0 {
			return nil
		}
		total += *c
	}
	return &total
}
