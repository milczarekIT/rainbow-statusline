// Package integration holds the badges of the tools the status line reports on.
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/cache"
	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

// Integration is one tool with a badge. Detected reports whether the tool is
// installed at all; Badge decides what to show for a session.
type Integration interface {
	ID() string
	Detected() bool
	Badge(s model.Session) (render.Badge, bool)
}

// All returns the integrations in badge order.
func All() []Integration { return []Integration{ponytail{}, caveman{}, rtk{}, claudeMem{}} }

// ForAgent drops the badges that have no data on that assistant: Ponytail and
// Caveman read Claude Code and OpenCode flags only.
func ForAgent(agent string) []Integration {
	var out []Integration
	for _, i := range All() {
		if agent == "copilot" && (i.ID() == "ponytail" || i.ID() == "caveman") {
			continue
		}
		out = append(out, i)
	}
	return out
}

const (
	keyRTKTotal   = "rtk-total"
	keyMemTotal   = "mem-total"
	prefixRTKHook = "rtks-" // Claude Code: matched by hook_decisions.session_id
	prefixRTKBash = "rtko-" // OpenCode: matched by the session's bash runs
	prefixTx      = "tx-"   // transcript scan
	prefixMemInj  = "memi-" // claude-mem /api/context/inject
)

const (
	ageTotal   = 60 * time.Second
	ageSession = 30 * time.Second
	ageScan    = 5 * time.Second
	ageInject  = 23 * time.Hour // a long session refetches once a day; the cache hides values after 24 h
)

var (
	// Replaced in tests.
	cacheGet = cache.Get
	inputDir = cache.Dir
	homeDir  = os.UserHomeDir

	sessionIDPattern = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)
	// modePattern is any mode word, so a future upstream mode keeps its badge.
	modePattern = regexp.MustCompile(`^[a-z0-9-]{1,16}$`)
)

// RegisterCaches teaches the cache how to compute every key this package reads.
// Both the drawing process and the refresh process must call it first.
func RegisterCaches() {
	for _, key := range []string{keyRTKTotal, keyMemTotal, keyCaveTotal, keyCaveTotalOC} {
		cache.Register(key, func(ctx context.Context) ([]byte, error) { return compute(ctx, key) })
	}
	for _, p := range []string{prefixRTKHook, prefixRTKBash, prefixTx, prefixMemInj} {
		cache.RegisterPrefix(p, compute)
	}
}

// compute is the refresh side of every key. Anything a key stands for beyond
// its name comes from a validated id or an input file written by the draw side.
func compute(ctx context.Context, key string) ([]byte, error) {
	pruneOnce.Do(pruneOld)
	switch {
	case key == keyRTKTotal:
		return computeRTKTotal(ctx)
	case key == keyMemTotal:
		return computeMemTotal(ctx)
	case key == keyCaveTotal:
		return computeCaveTotal(ctx)
	case key == keyCaveTotalOC:
		return computeCaveTotalOC(ctx)
	case strings.HasPrefix(key, prefixRTKHook):
		return computeRTKHook(ctx, strings.TrimPrefix(key, prefixRTKHook))
	case strings.HasPrefix(key, prefixRTKBash):
		return computeRTKBash(ctx, key)
	case strings.HasPrefix(key, prefixTx):
		return computeTranscript(ctx, key)
	case strings.HasPrefix(key, prefixMemInj):
		return computeMemInject(ctx, key)
	}
	return nil, errors.New("integration: unknown key")
}

var pruneOnce sync.Once

const maxFileAge = 48 * time.Hour

// pruneOld deletes per-session cache values, locks and input files (which may
// hold command text) that nobody has touched for two days.
func pruneOld() {
	dir, err := inputDir()
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !(strings.HasPrefix(n, prefixRTKHook) || strings.HasPrefix(n, prefixRTKBash) ||
			strings.HasPrefix(n, prefixTx) || strings.HasPrefix(n, prefixMemInj) || strings.HasPrefix(n, prefixCaveSeen)) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > maxFileAge {
			_ = os.Remove(filepath.Join(dir, n))
		}
	}
}

func validSessionID(id string) bool { return sessionIDPattern.MatchString(id) }

// hashKey names a cache key after arbitrary text (a path) without putting it in a file name.
func hashKey(prefix, text string) string {
	sum := sha256.Sum256([]byte(text))
	return prefix + hex.EncodeToString(sum[:8])
}

// putInput stores what the refresh of key needs, unless it is already there.
func putInput(key string, data []byte) {
	dir, err := inputDir()
	if err != nil {
		return
	}
	p := filepath.Join(dir, key+".in")
	if old, err := os.ReadFile(p); err == nil && string(old) == string(data) {
		now := time.Now()
		_ = os.Chtimes(p, now, now) // a live session's input must not look old to pruneOld
		return
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, key+".intmp*")
	if err != nil {
		return
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), p) != nil {
		_ = os.Remove(tmp.Name())
	}
}

func getInput(key string) ([]byte, error) {
	dir, err := inputDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, key+".in"))
}

// cachedPair reads "<a> <b>" stored under key.
func cachedPair(key string, maxAge time.Duration) (a, b int64, ok bool) {
	v, hit := cacheGet(key, maxAge)
	if !hit {
		return 0, 0, false
	}
	f := strings.Fields(string(v))
	if len(f) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.ParseInt(f[0], 10, 64)
	b, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return a, b, true
}

func pairText(a, b int64) []byte {
	return []byte(strconv.FormatInt(a, 10) + " " + strconv.FormatInt(b, 10))
}

// savedText renders "<tokens> · <pct>" for a cached pair, or a dash.
func savedText(saved, whole int64, ok bool, digits int) string {
	if !ok {
		return format.Dash
	}
	return format.Saved(saved, whole, digits)
}

func badge(name, colour string, c, t string) render.Badge {
	return render.Badge{Name: name, Lines: []render.Segment{
		{Text: "[" + name + "]", FG: colour},
		{Text: " (c: " + c + " | t: " + t + ")"},
	}}
}

func offBadge(name string) render.Badge {
	return render.Badge{Name: name, Lines: []render.Segment{{Text: "[" + name + " off]", Dim: true}}}
}
