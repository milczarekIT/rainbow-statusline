package adapter

import (
	"encoding/json"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

// Values that can be null or malformed stay raw: decoding a wrong type into a
// pointer field would leave a non-nil zero, which reads as a measurement.
type claudeInput struct {
	Cwd       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Effort struct {
		Level string `json:"level"`
	} `json:"effort"`
	ContextWindow struct {
		Size         json.RawMessage `json:"context_window_size"`
		UsedPct      json.RawMessage `json:"used_percentage"`
		CurrentUsage json.RawMessage `json:"current_usage"`
	} `json:"context_window"`
	Cost struct {
		TotalUSD json.RawMessage `json:"total_cost_usd"`
	} `json:"cost"`
	PromptCache    json.RawMessage `json:"prompt_cache"`
	SessionID      string          `json:"session_id"`
	TranscriptPath string          `json:"transcript_path"`
}

type claudeUsage struct {
	Input      json.RawMessage `json:"input_tokens"`
	CacheWrite json.RawMessage `json:"cache_creation_input_tokens"`
	CacheRead  json.RawMessage `json:"cache_read_input_tokens"`
}

type claudeCache struct {
	Warm      json.RawMessage `json:"warm"`
	ExpiresAt json.RawMessage `json:"expires_at"`
	TTL       string          `json:"ttl"`
}

func parseClaude(stdin []byte) model.Session {
	var in claudeInput
	// A type error on one field still fills the others; a syntax error leaves everything unknown.
	_ = json.Unmarshal(stdin, &in)

	s := model.Session{
		Agent:          "claude",
		Cwd:            in.Workspace.CurrentDir,
		Model:          in.Model.DisplayName,
		Effort:         in.Effort.Level,
		ContextSize:    optional[int64](in.ContextWindow.Size),
		ContextPct:     optional[float64](in.ContextWindow.UsedPct),
		CostUSD:        optional[float64](in.Cost.TotalUSD),
		SessionID:      in.SessionID,
		TranscriptPath: in.TranscriptPath,
	}
	if s.Cwd == "" {
		s.Cwd = in.Cwd
	}
	if u := optional[claudeUsage](in.ContextWindow.CurrentUsage); u != nil {
		read, write := orZero(u.CacheRead), orZero(u.CacheWrite)
		s.CacheRead, s.CacheWrite = &read, &write
		if input := orZero(u.Input); input >= 0 && read >= 0 && write >= 0 {
			used := input + read + write
			s.ContextUsed = &used
		}
	}
	if pc := optional[claudeCache](in.PromptCache); pc != nil {
		s.CacheTTL = pc.TTL
		s.CacheWarm = optional[bool](pc.Warm)
		s.CacheExpiresAt = expiry(optional[float64](pc.ExpiresAt))
	}
	return s
}

// maxExpiry is the end of year 9999. Larger values are garbage, or milliseconds.
const maxExpiry = 253402300799

func expiry(sec *float64) *time.Time {
	if sec == nil || *sec <= 0 || *sec > maxExpiry {
		return nil
	}
	at := time.Unix(int64(*sec), 0)
	return &at
}

// optional decodes raw, returning nil when it is absent, null or the wrong type.
func optional[T any](raw json.RawMessage) *T {
	var v *T
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return v
}

func orZero(raw json.RawMessage) int64 {
	if v := optional[int64](raw); v != nil {
		return *v
	}
	return 0
}
