package install

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/milczarekIT/rainbow-statusline/plugin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// binMarker is the line in the plugin the copy gets the binary's path in: an
// install folder need not be on the PATH OpenCode was started with.
const binMarker = `const INSTALLED_BIN = ""`

func defaultOpenCodeDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "opencode")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "opencode")
}

func prepareOpenCode(bin string, st *state) ([]action, error) {
	dir := defaultOpenCodeDir()
	file := filepath.Join(dir, "plugins", "rainbow-statusline", "tui.tsx")
	copied, err := preparePluginFile(file, bin, st)
	if err != nil {
		return nil, err
	}
	entry, err := prepareTUI(filepath.Join(dir, "tui.json"), fileURL(goos, file), st)
	if err != nil {
		return nil, err
	}
	return []action{copied, entry}, nil
}

func preparePluginFile(path, bin string, st *state) (action, error) {
	src, err := plugin.OpenCodeFiles.ReadFile("opencode/tui.tsx")
	if err != nil {
		return action{}, fmt.Errorf("%s: %w", path, err)
	}
	if strings.Count(string(src), binMarker) != 1 {
		return action{}, fmt.Errorf("%s: the bundled plugin has no place for the binary path", path)
	}
	next := []byte(strings.Replace(string(src), binMarker, `const INSTALLED_BIN = `+jsonString(bin), 1))
	cur, err := readTarget(path)
	if err != nil {
		return action{}, err
	}
	rec := st.Records[path]
	a := action{Change: Change{Agent: "opencode", Path: path, Summary: "copy the plugin"}, cur: cur, write: true, next: next, ownDir: true}
	if cur.exists && rec == nil {
		a.Summary = "replace the file already here with the plugin (it is backed up)"
	}
	a.record = func(s *state) {
		if rec == nil {
			s.Records[path] = &record{Agent: "opencode", Kind: kindPlugin, Existed: cur.exists, Original: cur.bytes, WroteHash: hash(next)}
			return
		}
		rec.WroteHash = hash(next)
	}
	return a, nil
}

func undoPlugin(path string, rec *record) (action, error) {
	a := action{Change: Change{Agent: rec.Agent, Path: path}, ownDir: true}
	var err error
	if a.cur, err = readTarget(path); err != nil {
		return a, err
	}
	if !a.cur.exists {
		a.note = "missing, left alone"
		return a, nil
	}
	if !restoreOriginal(&a, rec) {
		a.note = "changed since install, left alone"
	}
	return a, nil
}

func prepareTUI(path, entry string, st *state) (action, error) {
	cur, err := readPluginList(path)
	if err != nil {
		return action{}, err
	}
	raw := cur.bytes
	if !cur.exists {
		raw = []byte("{}\n")
	}
	list := gjson.GetBytes(raw, "plugin")
	rec := st.Records[path]
	a := action{Change: Change{Agent: "opencode", Path: path, Summary: "already lists the plugin"}, cur: cur, write: true, next: raw}
	has := listed(list, entry)
	if !has {
		elems := []string{}
		for _, e := range list.Array() {
			elems = append(elems, e.Raw)
		}
		value := rebuildArray(list.Raw, append(elems, jsonString(entry)))
		if a.next, err = setKey(raw, "plugin", value); err != nil {
			return a, fmt.Errorf("%s: %w", path, err)
		}
		a.Summary = "add the plugin to the plugin list"
	}
	a.record = func(s *state) {
		switch {
		case rec == nil && has:
			s.Records[path] = &record{Agent: "opencode", Kind: kindTUI, Existed: cur.exists, Original: cur.bytes, Previous: list.Raw, Entry: entry}
		case rec == nil:
			s.Records[path] = &record{Agent: "opencode", Kind: kindTUI, Existed: cur.exists, Original: cur.bytes, Previous: list.Raw, Entry: entry, WroteHash: hash(a.next)}
		case !has:
			rec.WroteHash = nextHash(rec, cur, a.next)
			rec.Entry = entry
		}
	}
	return a, nil
}

func undoTUI(path string, rec *record) (action, error) {
	a := action{Change: Change{Agent: rec.Agent, Path: path}}
	var err error
	if a.cur, err = readTarget(path); err != nil {
		return a, err
	}
	if !a.cur.exists {
		a.note = "missing, left alone"
		return a, nil
	}
	if restoreOriginal(&a, rec) {
		return a, nil
	}
	if a.cur, err = readPluginList(path); err != nil {
		return a, err
	}
	list := gjson.GetBytes(a.cur.bytes, "plugin")
	if !listed(list, rec.Entry) {
		a.note = "no longer lists our plugin, left alone"
		return a, nil
	}
	elems := []string{}
	for _, e := range list.Array() {
		if !(e.Type == gjson.String && e.String() == rec.Entry) {
			elems = append(elems, e.Raw)
		}
	}
	a.write, a.note = true, "our plugin entry removed"
	if len(elems) == 0 && rec.Previous == "" {
		a.next, err = sjson.DeleteBytes(a.cur.bytes, "plugin")
	} else {
		a.next, err = sjson.SetRawBytes(a.cur.bytes, "plugin", []byte(rebuildArray(list.Raw, elems)))
	}
	if err != nil {
		return a, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func readPluginList(path string) (target, error) {
	t, err := readSettings(path, "plugin")
	if err != nil || !t.exists {
		return t, err
	}
	if v := gjson.GetBytes(t.bytes, "plugin"); v.Exists() && !v.IsArray() {
		return t, fmt.Errorf("%s: \"plugin\" is not a list", path)
	}
	return t, nil
}

func listed(list gjson.Result, entry string) bool {
	for _, e := range list.Array() {
		if e.Type == gjson.String && e.String() == entry {
			return true
		}
	}
	return false
}

var (
	arrayOpen  = regexp.MustCompile(`^\[(\r?\n)([ \t]*)\S`)
	arrayClose = regexp.MustCompile(`\r?\n[ \t]*\]$`)
)

// rebuildArray lays the elements out the way the array was laid out.
func rebuildArray(old string, elems []string) string {
	open, end := arrayOpen.FindStringSubmatch(old), arrayClose.FindString(old)
	if open == nil || end == "" || len(elems) == 0 {
		return "[" + strings.Join(elems, ",") + "]"
	}
	sep := open[1] + open[2]
	return "[" + sep + strings.Join(elems, ","+sep) + end
}

// fileURL builds the file:// URL for a path on goos.
func fileURL(goos, path string) string {
	u := url.URL{Scheme: "file", Path: path}
	if goos == "windows" {
		p := strings.ReplaceAll(path, `\`, "/")
		if rest, ok := strings.CutPrefix(p, "//"); ok {
			host, tail, _ := strings.Cut(rest, "/")
			u.Host, u.Path = host, "/"+tail
		} else {
			u.Path = "/" + p
		}
	}
	return u.String()
}
