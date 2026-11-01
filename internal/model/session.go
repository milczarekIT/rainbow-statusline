// Package model holds what the status line knows about a session.
package model

import "time"

// Session is one assistant session. A nil pointer means unknown and renders as a dash.
type Session struct {
	User, Host, Cwd, Model, Effort                             string
	ContextUsed, ContextSize, OutTokens, CacheRead, CacheWrite *int64
	// ContextPct is the assistant's own figure; when nil it is derived from used and size.
	ContextPct *float64
	CostUSD    *float64
	// CacheWarm: nil unknown, false cold, true warm.
	CacheWarm                 *bool
	CacheExpiresAt            *time.Time
	CacheTTL                  string
	CacheEstimated            bool
	SessionID, TranscriptPath string
	Agent                     string
	// BashRuns are the session's completed shell commands, for assistants whose
	// history carries no session id on RTK's side (OpenCode).
	BashRuns []BashRun
}

// BashRun is one shell command of a session; Start and End are ms since epoch.
type BashRun struct {
	Cmd        string
	Start, End int64
}
