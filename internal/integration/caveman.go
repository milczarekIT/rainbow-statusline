package integration

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/milczarekIT/rainbow-statusline/internal/format"
	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const (
	cavemanPlugin = "caveman@caveman"
	cavemanColour = "38;5;172"
	keyCaveTotal  = "cave-total"
	caveIndexName = "cave-files.tsv"
)

// cavemanFullBanner is what Caveman injects at the start of a session in full mode.
var cavemanFullBanner = []byte("CAVEMAN MODE ACTIVE — level: full")

// cavemanModes are the modes the plugin's own status line accepts.
var cavemanModes = map[string]bool{
	"off": true, "lite": true, "full": true, "ultra": true, "wenyan-lite": true, "wenyan": true,
	"wenyan-full": true, "wenyan-ultra": true, "commit": true, "review": true, "compress": true,
}

type caveman struct{}

func (caveman) ID() string { return "caveman" }

func (caveman) Detected() bool { return pluginActive(cavemanPlugin) }

// Badge is the plugin's own ([CAVEMAN], [CAVEMAN:MODE]); in full mode it adds
// "(c: ~saved | t: ~saved)", estimated as output tokens × 0.65/0.35 (Caveman's
// benchmark ratio). Both numbers are estimates, so both carry "~".
func (c caveman) Badge(s model.Session) (render.Badge, bool) {
	if !c.Detected() {
		return render.Badge{}, false
	}
	raw, ok := readFlag(".caveman-active")
	if !ok {
		return render.Badge{}, false
	}
	mode := cavemanMode(raw)
	if !cavemanModes[mode] {
		return render.Badge{}, false
	}
	label := "[CAVEMAN]"
	if mode != "full" {
		label = "[CAVEMAN:" + strings.ToUpper(mode) + "]"
	}
	segs := []render.Segment{{Text: label, FG: cavemanColour}}
	if strings.TrimRight(raw, "\n") == "full" {
		var session int64
		if s.OutTokens != nil {
			session = *s.OutTokens
		}
		segs = append(segs, render.Segment{Text: " "},
			render.Segment{Text: "(c: " + cavemanSaved(session) + " | t: " + cavemanSaved(cachedCaveTotal()) + ")", FG: cavemanColour})
	}
	return render.Badge{Name: "CAVEMAN", Lines: segs}, true
}

// cavemanMode is the plugin's cleaning of the flag: no CR/LF, lower case,
// only [a-z0-9-].
func cavemanMode(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cavemanSaved turns output tokens into the estimated tokens saved, or a dash
// when there is no output.
func cavemanSaved(out int64) string {
	if out <= 0 {
		return format.Dash
	}
	return "~" + format.Tokens(int64(float64(out)*0.65/0.35))
}

func cachedCaveTotal() int64 {
	v, ok := cacheGet(keyCaveTotal, ageTotal)
	if !ok {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(v)), 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// computeCaveTotal sums the output tokens of every session that started in
// Caveman's full mode (main transcript and subagents). Each transcript's sum is
// kept in an index keyed by its mtime, so only changed files are read again.
func computeCaveTotal(ctx context.Context) ([]byte, error) {
	idxPath, err := caveIndexPath()
	if err != nil {
		return nil, err
	}
	old := readCaveIndex(idxPath)
	files, err := filepath.Glob(filepath.Join(globEscape(filepath.Join(claudeDir(), "projects")), "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	total, fresh, err := sumCaveFiles(ctx, files, old)
	// Even a cut-off run keeps what it learned, so the next one resumes; a cold
	// index over thousands of transcripts takes longer than one refresh may.
	if werr := writeCaveIndex(idxPath, fresh); err == nil {
		err = werr
	}
	if err != nil {
		return nil, err
	}
	return []byte(strconv.FormatInt(total, 10)), nil
}

// sumCaveFiles returns the total and the index to store. When it stops early,
// files it did not reach keep their old entries.
func sumCaveFiles(ctx context.Context, files []string, old map[string]caveEntry) (int64, map[string]caveEntry, error) {
	var total int64
	fresh := map[string]caveEntry{}
	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return 0, withUnvisited(fresh, files[i:], old), err
		}
		info, err := os.Stat(f)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		mtime := info.ModTime().UnixNano()
		e, hit := old[f]
		if !hit || e.mtime != mtime {
			sum, err := caveSessionSum(ctx, f)
			if err != nil {
				// An unreadable file would undercount the total.
				return 0, withUnvisited(fresh, files[i:], old), err
			}
			e = caveEntry{mtime, sum}
		}
		fresh[f] = e
		total += e.sum
	}
	return total, fresh, nil
}

func withUnvisited(fresh map[string]caveEntry, rest []string, old map[string]caveEntry) map[string]caveEntry {
	for _, f := range rest {
		if e, ok := old[f]; ok {
			fresh[f] = e
		}
	}
	return fresh
}

type caveEntry struct{ mtime, sum int64 }

func caveIndexPath() (string, error) {
	dir, err := inputDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, caveIndexName), nil
}

// globEscape makes a literal folder safe inside a filepath.Glob pattern.
func globEscape(p string) string {
	if filepath.Separator == '\\' { // `\` is the separator, not an escape, on Windows
		return p
	}
	return strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(p)
}

func readCaveIndex(path string) map[string]caveEntry {
	idx := map[string]caveEntry{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return idx
	}
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		m, err1 := strconv.ParseInt(f[1], 10, 64)
		s, err2 := strconv.ParseInt(f[2], 10, 64)
		if err1 != nil || err2 != nil || s < 0 {
			continue
		}
		idx[f[0]] = caveEntry{m, s}
	}
	return idx
}

// writeCaveIndex replaces the index atomically; a path with a tab or newline
// cannot be stored in it and is simply read again next time.
func writeCaveIndex(path string, idx map[string]caveEntry) error {
	var b strings.Builder
	for p, e := range idx {
		if strings.ContainsAny(p, "\t\n") {
			continue
		}
		b.WriteString(p + "\t" + strconv.FormatInt(e.mtime, 10) + "\t" + strconv.FormatInt(e.sum, 10) + "\n")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, caveIndexName+".tmp*")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(b.String())
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(tmp.Name(), 0o600) != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr, errors.New("caveman: index not written"))
	}
	return nil
}

// caveSessionSum is the output of one session if it started in full mode, else 0.
func caveSessionSum(ctx context.Context, path string) (int64, error) {
	full, err := startedInFull(ctx, path)
	if err != nil || !full {
		return 0, err
	}
	files := []string{path}
	subDir := filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents")
	entries, err := os.ReadDir(subDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(subDir, e.Name()))
		}
	}
	byID := map[string]int64{}
	var saved, work int64 // claude-mem's numbers, not used here
	for _, f := range files {
		if err := scanTranscript(ctx, f, byID, &saved, &work); err != nil {
			return 0, err
		}
	}
	var out int64
	for _, n := range byID {
		out += n
	}
	return out, nil
}

// startedInFull looks for Caveman's full-mode banner in the main transcript.
func startedInFull(ctx context.Context, path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		line, err := r.ReadBytes('\n')
		if bytes.Contains(line, cavemanFullBanner) {
			return true, nil
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}
