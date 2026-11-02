package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const (
	claudeMemPlugin = "claude-mem@thedotmack"
	workerBase      = "http://127.0.0.1:37777"
)

var (
	// claudeMemDir and claudeSettings are replaced in tests.
	claudeMemDir   = func() string { h, _ := homeDir(); return filepath.Join(h, ".claude-mem") }
	claudeSettings = defaultClaudeSettings
	workerURL      = workerBase

	ansiPattern  = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	statsPattern = regexp.MustCompile(`([0-9,]+)t work \| ([0-9,]+)t saved`)
)

func defaultClaudeSettings() string { return filepath.Join(claudeDir(), "settings.json") }

type claudeMem struct{}

func (claudeMem) ID() string { return "claude-mem" }

// Detected means claude-mem has a data directory on this machine.
func (claudeMem) Detected() bool {
	info, err := os.Stat(claudeMemDir())
	return err == nil && info.IsDir()
}

func claudePluginEnabled() bool {
	raw, err := os.ReadFile(claudeSettings())
	if err != nil {
		return false
	}
	var cfg struct {
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	return json.Unmarshal(raw, &cfg) == nil && cfg.Enabled[claudeMemPlugin]
}

func (m claudeMem) Badge(s model.Session) (render.Badge, bool) {
	installed := m.Detected() && (s.Agent != "claude" || claudePluginEnabled())
	if s.Agent == "opencode" {
		installed = ocPlugin("claude-mem")
	}
	if !installed {
		return offBadge("CLAUDE-MEM"), true
	}
	saved, work, ok := memSessionPair(s)
	ts, tw, tok := cachedPair(keyMemTotal, ageTotal)
	c := memText(saved, work, ok)
	return badge("CLAUDE-MEM", "38;5;75", c, totalOrDash(c, memText(ts, tw, tok), saved, ts)), true
}

// memText is claude-mem's own display: nothing saved or nothing worked is a dash.
func memText(saved, work int64, ok bool) string {
	if !ok || saved <= 0 || work <= 0 {
		return format.Dash
	}
	return format.Saved(saved, work, 0)
}

// memSessionPair is what claude-mem injected into this session: from the
// transcript for Claude Code, from the worker's inject endpoint for OpenCode.
func memSessionPair(s model.Session) (saved, work int64, ok bool) {
	if s.Agent == "copilot" { // no session link on Copilot
		return 0, 0, false
	}
	if s.Agent == "opencode" {
		if s.Cwd == "" || !validSessionID(s.SessionID) {
			return 0, 0, false
		}
		// Like the OpenCode plugin, read once per session; a session that runs
		// longer than ageInject fetches again, before the cache hides the value.
		key := memInjectKey(s.SessionID, s.Cwd)
		putInput(key, []byte(s.Cwd))
		return cachedPair(key, ageInject)
	}
	_, saved, work, ok = TranscriptStats(s.TranscriptPath)
	return saved, work, ok
}

// TranscriptStats scans a Claude Code transcript and its subagents (cached, 5 s):
// out is the sum over messages of the largest output_tokens seen per message id,
// memSaved and memWork come from claude-mem's "Nt work | Nt saved" line in the
// SessionStart attachments. ok is false until the first scan has finished.
func TranscriptStats(path string) (out, memSaved, memWork int64, ok bool) {
	if !strings.HasSuffix(path, ".jsonl") {
		return 0, 0, 0, false
	}
	key := hashKey(prefixTx, path)
	putInput(key, []byte(path))
	v, hit := cacheGet(key, ageScan)
	if !hit {
		return 0, 0, 0, false
	}
	f := strings.Fields(string(v))
	if len(f) != 3 {
		return 0, 0, 0, false
	}
	var n [3]int64
	for i := range n {
		x, err := strconv.ParseInt(f[i], 10, 64)
		if err != nil || x < 0 {
			return 0, 0, 0, false
		}
		n[i] = x
	}
	return n[0], n[1], n[2], true
}

// memInjectKey names the inject result of one session in one folder.
func memInjectKey(sessionID, cwd string) string {
	return hashKey(prefixMemInj+sessionID+"-", cwd)
}

func computeTranscript(ctx context.Context, key string) ([]byte, error) {
	raw, err := getInput(key)
	if err != nil {
		return nil, err
	}
	path := string(raw)
	if hashKey(prefixTx, path) != key || !strings.HasSuffix(path, ".jsonl") {
		return nil, errors.New("transcript: input does not match key")
	}
	files := []string{path}
	subDir := filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries { // sorted by name; a literal listing, not a glob
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(subDir, e.Name()))
		}
	}

	byID := map[string]int64{}
	var saved, work int64
	for _, f := range files {
		// A file we cannot fully read would undercount, so it is an error.
		if err := scanTranscript(ctx, f, byID, &saved, &work); err != nil {
			return nil, err
		}
	}
	var out int64
	for _, n := range byID {
		out += n
	}
	return []byte(strconv.FormatInt(out, 10) + " " + strconv.FormatInt(saved, 10) + " " + strconv.FormatInt(work, 10)), nil
}

type transcriptLine struct {
	Type    string `json:"type"`
	UUID    string `json:"uuid"`
	Message struct {
		ID    string `json:"id"`
		Usage *struct {
			Output *float64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Attachment struct {
		HookEvent string          `json:"hookEvent"`
		Content   json.RawMessage `json:"content"`
		Stdout    json.RawMessage `json:"stdout"`
	} `json:"attachment"`
}

func scanTranscript(ctx context.Context, path string, byID map[string]int64, saved, work *int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			applyLine(line, byID, saved, work)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func applyLine(line []byte, byID map[string]int64, saved, work *int64) {
	var e transcriptLine
	if json.Unmarshal(line, &e) != nil {
		return
	}
	switch {
	case e.Type == "assistant" && e.Message.Usage != nil:
		id := e.Message.ID
		if id == "" {
			id = e.UUID
		}
		if id == "" {
			return
		}
		var n int64
		if o := e.Message.Usage.Output; o != nil {
			if math.IsNaN(*o) || math.IsInf(*o, 0) || *o > format.MaxTokens {
				return
			}
			if *o > 0 {
				n = int64(*o)
			}
		}
		byID[id] = max(byID[id], n)
	case e.Type == "attachment" && e.Attachment.HookEvent == "SessionStart":
		for _, raw := range []json.RawMessage{e.Attachment.Content, e.Attachment.Stdout} {
			var text string
			if json.Unmarshal(raw, &text) != nil {
				continue
			}
			if s, w, ok := parseStats(text); ok {
				*saved += s
				*work += w
				return
			}
		}
	}
}

// parseStats reads claude-mem's "<work>t work | <saved>t saved" line.
func parseStats(text string) (saved, work int64, ok bool) {
	m := statsPattern.FindStringSubmatch(ansiPattern.ReplaceAllString(text, ""))
	if m == nil {
		return 0, 0, false
	}
	w, err1 := strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64)
	s, err2 := strconv.ParseInt(strings.ReplaceAll(m[2], ",", ""), 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return s, w, true
}

// computeMemTotal is claude-mem's formula over the whole database: tokens the
// observations took to work out minus tokens to read them, and the work itself.
func computeMemTotal(ctx context.Context) ([]byte, error) {
	db, err := OpenReadOnly(filepath.Join(claudeMemDir(), "claude-mem.db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	saved, work, err := pair(ctx, db, `
		select sum(discovery_tokens) - sum((length(coalesce(title,'')) + length(coalesce(subtitle,'')) + length(coalesce(narrative,''))
		  + length(coalesce(facts,'[]')) + 3) / 4), sum(discovery_tokens) from observations where discovery_tokens > 0`)
	if err != nil {
		return nil, err
	}
	return pairText(saved, work), nil
}

// computeMemInject asks the local claude-mem worker (127.0.0.1 only) what it
// injects for a folder and reads the same stats line as the transcript carries.
func computeMemInject(ctx context.Context, key string) ([]byte, error) {
	raw, err := getInput(key)
	if err != nil {
		return nil, err
	}
	cwd := string(raw)
	rest := strings.TrimPrefix(key, prefixMemInj)
	i := strings.LastIndex(rest, "-")
	if i < 0 || !validSessionID(rest[:i]) || memInjectKey(rest[:i], cwd) != key {
		return nil, errors.New("claude-mem: input does not match key")
	}
	q := url.Values{"cwd": {cwd}}
	if !includeAllSources() {
		q.Set("platformSource", "opencode")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, workerURL+"/api/context/inject?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	// A redirect could send the request off 127.0.0.1, so none is followed.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("claude-mem: worker answered " + resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	saved, work, ok := parseStats(string(body))
	if !ok {
		return nil, errors.New("claude-mem: no stats line")
	}
	return pairText(saved, work), nil
}

func includeAllSources() bool {
	raw, err := os.ReadFile(filepath.Join(claudeMemDir(), "settings.json"))
	if err != nil {
		return false
	}
	var cfg map[string]any
	return json.Unmarshal(raw, &cfg) == nil && cfg["CLAUDE_MEM_SESSION_START_INCLUDE_ALL_SOURCES"] == "true"
}
