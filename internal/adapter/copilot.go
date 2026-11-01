package adapter

import (
	"encoding/json"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
)

// Copilot has no USD cost, effort or cache expiry, so those stay unknown.
// context_window.used_percentage is always null there; the figure is
// current_context_used_percentage against displayed_context_limit.
type copilotInput struct {
	Cwd       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		Limit      json.RawMessage `json:"displayed_context_limit"`
		Used       json.RawMessage `json:"current_context_tokens"`
		UsedPct    json.RawMessage `json:"current_context_used_percentage"`
		CacheRead  json.RawMessage `json:"total_cache_read_tokens"`
		CacheWrite json.RawMessage `json:"total_cache_write_tokens"`
	} `json:"context_window"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

func parseCopilot(stdin []byte) model.Session {
	var in copilotInput
	_ = json.Unmarshal(stdin, &in)

	s := model.Session{
		Agent:          "copilot",
		Cwd:            in.Workspace.CurrentDir,
		Model:          in.Model.DisplayName,
		ContextUsed:    optional[int64](in.ContextWindow.Used),
		ContextSize:    optional[int64](in.ContextWindow.Limit),
		ContextPct:     optional[float64](in.ContextWindow.UsedPct),
		CacheRead:      optional[int64](in.ContextWindow.CacheRead),
		CacheWrite:     optional[int64](in.ContextWindow.CacheWrite),
		SessionID:      in.SessionID,
		TranscriptPath: in.TranscriptPath,
	}
	if s.Cwd == "" {
		s.Cwd = in.Cwd
	}
	return s
}
