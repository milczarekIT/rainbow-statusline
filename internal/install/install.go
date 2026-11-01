// Package install adds Rainbow Statusline to the assistants' settings and takes
// it out again. Nothing here runs while drawing.
package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Change is one file an install would write.
type Change struct {
	Agent, Path, Summary string
	Replaces             string // a status line we did not write and would replace
}

// Report receives what Uninstall did, one line per file.
var Report io.Writer = io.Discard

// Replaced in tests.
var (
	executable = stableExecutable
	goos       = runtime.GOOS

	beforeStateSave = func() {}
)

const (
	kindStatusLine = "statusline"
	kindTUI        = "tui"
	kindPlugin     = "plugin"
)

// record is what one install remembers about one file.
type record struct {
	Agent, Kind string
	Existed     bool
	Original    []byte   // bytes before our first install; a reinstall keeps them
	Previous    []byte   // raw JSON of the value we replaced; empty when there was none
	Entry       string   // our command, or our tui.json URL
	WroteHash   string   // hash of the bytes we wrote; empty once the user's edits are mixed in
	Dirs        []string // folders we created for this file, deepest first
}

type state struct {
	Records map[string]*record
}

// target is a file as found on disk.
type target struct {
	path   string
	exists bool
	mode   fs.FileMode
	bytes  []byte
}

// action is one prepared step. Preparing touches nothing.
type action struct {
	Change
	cur    target
	write  bool
	next   []byte
	remove bool
	dirs   []string // folders that go with the file, once empty
	record func(*state)
	note   string
}

// Plan says what installing would change. It never writes.
func Plan(ids []string) ([]Change, error) {
	st, err := loadState()
	if err != nil {
		return nil, err
	}
	var out []Change
	for _, id := range ids {
		actions, err := prepare(id, st)
		if err != nil {
			return nil, err
		}
		for _, a := range actions {
			out = append(out, a.Change)
		}
	}
	return out, nil
}

// Apply makes the planned changes, all of them or none. It works from the files
// as they are now and refuses if a status line the user did not agree to
// replace has appeared since Plan.
func Apply(changes []Change) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	prepared := map[string][]action{}
	var actions []action
	for _, c := range changes {
		if _, ok := prepared[c.Agent]; !ok {
			if prepared[c.Agent], err = prepare(c.Agent, st); err != nil {
				return err
			}
		}
		found := false
		for _, a := range prepared[c.Agent] {
			if a.Path != c.Path {
				continue
			}
			if a.Replaces != c.Replaces {
				return fmt.Errorf("%s: its status line changed after the plan was shown", c.Path)
			}
			actions = append(actions, a)
			found = true
		}
		if !found {
			return fmt.Errorf("%s: not part of the %s install", c.Path, c.Agent)
		}
	}
	return commit(actions, st, false)
}

// Uninstall undoes every recorded install. It restores the original bytes where
// the file is still what we wrote, and otherwise takes out only our entry. If
// any file cannot be handled, nothing changes.
func Uninstall() error {
	st, err := loadState()
	if err != nil {
		return err
	}
	if len(st.Records) == 0 {
		return uninstallWithoutRecords(st)
	}
	paths := make([]string, 0, len(st.Records))
	for p := range st.Records {
		paths = append(paths, p)
	}
	// Reverse order takes tui.json out before the plugin file it points to.
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	var actions []action
	for _, p := range paths {
		a, err := undo(p, st.Records[p])
		if err != nil {
			return err
		}
		a.record = func(s *state) { delete(s.Records, p) }
		actions = append(actions, a)
	}
	if err := commit(actions, st, true); err != nil {
		return err
	}
	report(actions)
	return nil
}

// uninstallWithoutRecords handles a lost state: it looks for our entries where
// an install would have put them and takes out only those.
func uninstallWithoutRecords(st *state) error {
	actions, err := findLost()
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		fmt.Fprintln(Report, "nothing to uninstall")
		return nil
	}
	if err := commit(actions, st, true); err != nil {
		return err
	}
	dir, err := stateDir()
	if err != nil {
		return err
	}
	for i := range actions {
		if actions[i].write || actions[i].remove {
			actions[i].note += "; no original was available, earlier copies may be in " + filepath.Join(dir, "backups")
		}
	}
	report(actions)
	return nil
}

func findLost() ([]action, error) {
	var out []action
	add := func(path string, rec *record) error {
		a, err := undo(path, rec)
		if err != nil {
			return err
		}
		a.record = func(*state) {}
		out = append(out, a)
		return nil
	}
	for _, d := range definitions() {
		path := filepath.Join(d.dir(), d.settings)
		raw, err := os.ReadFile(path)
		// A file that never mentions us is none of our business, whatever its format.
		mentioned := err == nil && bytes.Contains(raw, []byte("rainbow-statusline"))
		if d.id == "opencode" {
			file := openCodePluginPath()
			if mentioned {
				if url := fileURL(goos, file); bytes.Contains(raw, []byte(url)) {
					if err := add(path, &record{Agent: d.id, Kind: kindTUI, Existed: true, Entry: url}); err != nil {
						return nil, err
					}
				}
			}
			if cur, err := os.ReadFile(file); err == nil {
				rec := &record{Agent: d.id, Kind: kindPlugin, Dirs: []string{filepath.Dir(file)}}
				if isOurPlugin(cur) {
					rec.WroteHash = hash(cur)
				}
				if err := add(file, rec); err != nil {
					return nil, err
				}
			}
			continue
		}
		if !mentioned {
			continue
		}
		if !json.Valid(raw) {
			_, err := readSettings(path, "statusLine")
			return nil, err
		}
		if cmd := gjson.GetBytes(raw, "statusLine.command").String(); isOurCommand(cmd, d.id) {
			if err := add(path, &record{Agent: d.id, Kind: kindStatusLine, Existed: true, Entry: cmd}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func report(actions []action) {
	for _, a := range actions {
		fmt.Fprintf(Report, "%s: %s\n", a.Path, a.note)
	}
}

func prepare(id string, st *state) ([]action, error) {
	for _, d := range definitions() {
		if d.id != id {
			continue
		}
		if !d.found() {
			return nil, fmt.Errorf("%s was not detected on this computer", d.name)
		}
		bin, err := executable()
		if err != nil {
			return nil, fmt.Errorf("cannot find this program's path: %w", err)
		}
		return d.prepare(bin, st)
	}
	return nil, fmt.Errorf("unknown assistant %q", id)
}

func undo(path string, rec *record) (a action, err error) {
	switch rec.Kind {
	case kindStatusLine:
		a, err = undoStatusLine(path, rec)
	case kindTUI:
		a, err = undoTUI(path, rec)
	case kindPlugin:
		a, err = undoPlugin(path, rec)
	default:
		err = fmt.Errorf("%s: unknown record kind %q", path, rec.Kind)
	}
	a.dirs = rec.Dirs
	return a, err
}

// missingDirs lists the folders writing path would have to create, deepest first.
func missingDirs(path string) []string {
	var out []string
	for d := filepath.Dir(path); ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); !os.IsNotExist(err) || d == filepath.Dir(d) {
			return out
		}
		out = append(out, d)
	}
}

// removeDirs drops the folders that are empty; os.Remove refuses the others.
func removeDirs(dirs []string) {
	for _, d := range dirs {
		_ = os.Remove(d)
	}
}

// commit backs up and writes each file. A failure restores the files already
// changed and the state; backups stay.
//
// Install saves the state before the files and uninstall after them, so a kill
// at any point leaves records a rerun of uninstall can finish from: a record
// whose entry is not in the file is reported and dropped.
func commit(actions []action, st *state, stateLast bool) error {
	var writes []action
	for _, a := range actions {
		a.record(st)
		if a.remove || (a.write && !(a.cur.exists && bytes.Equal(a.cur.bytes, a.next))) {
			writes = append(writes, a)
		}
	}
	for _, a := range writes {
		if a.cur.exists {
			if err := backup(a); err != nil {
				return fmt.Errorf("%s: backup failed: %w", a.Path, err)
			}
		}
	}
	restoreState := func() error { return nil }
	var err error
	if !stateLast {
		if restoreState, err = saveState(st); err != nil {
			return err
		}
	}
	for i, a := range writes {
		if err := perform(a); err != nil {
			err = fmt.Errorf("%s: %w", a.Path, err)
			if !a.cur.exists {
				removeDirs(a.dirs)
			}
			return errors.Join(err, restoreAll(writes[:i]), restoreState())
		}
	}
	if stateLast {
		beforeStateSave()
		if _, err := saveState(st); err != nil {
			return errors.Join(err, restoreAll(writes))
		}
	}
	return nil
}

func restoreAll(done []action) error {
	var err error
	for _, a := range done {
		err = errors.Join(err, restore(a))
	}
	return err
}

func perform(a action) error {
	if a.remove {
		if err := os.Remove(a.Path); err != nil {
			return err
		}
		removeDirs(a.dirs)
		return nil
	}
	mode := fs.FileMode(0o644)
	if a.cur.exists {
		mode = a.cur.mode
	}
	return atomicWrite(a.Path, a.next, mode)
}

func restore(a action) error {
	var err error
	if a.cur.exists {
		err = atomicWrite(a.Path, a.cur.bytes, a.cur.mode)
	} else {
		err = os.Remove(a.Path)
		removeDirs(a.dirs)
	}
	if err != nil {
		return fmt.Errorf("%s: could not restore it, see the backups: %w", a.Path, err)
	}
	return nil
}

// atomicWrite replaces the file through a temp file. A symlink stays a symlink:
// its target gets the new bytes.
func atomicWrite(path string, data []byte, mode fs.FileMode) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".rainbow-statusline-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode.Perm())
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func stateDir() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rainbow-statusline", "state"), nil
}

func loadState() (*state, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "state.json")
	st := &state{Records: map[string]*record{}}
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err == nil {
		err = json.Unmarshal(raw, st)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	if st.Records == nil {
		st.Records = map[string]*record{}
	}
	for path, rec := range st.Records {
		if rec == nil {
			return nil, fmt.Errorf("%s: corrupt, no record for %s", p, path)
		}
	}
	return st, nil
}

// saveState writes the state. The returned function puts the old one back.
func saveState(st *state) (func() error, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "state.json")
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return nil, err
	}
	old, err := os.ReadFile(p)
	existed := err == nil
	if existed && bytes.Equal(old, data) {
		return func() error { return nil }, nil
	}
	if err := atomicWrite(p, data, 0o600); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return func() error {
		if existed {
			return atomicWrite(p, old, 0o600)
		}
		return os.Remove(p)
	}, nil
}

// backup stores the file under a new name and never replaces an earlier one.
func backup(a action) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	base := time.Now().Format("20060102T150405.000") + "-" + a.Agent + "-" + filepath.Base(a.Path)
	for n := 0; ; n++ {
		name := base
		if n > 0 {
			name += "." + strconv.Itoa(n)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.Write(a.cur.bytes)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// readTarget reads a file we may change, refusing one we could not write back.
func readTarget(path string) (target, error) {
	t := target{path: path}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return t, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return t, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return t, fmt.Errorf("%s: not a regular file", path)
	}
	if info.Mode().Perm()&0o200 == 0 {
		return t, fmt.Errorf("%s: read-only", path)
	}
	t.bytes, err = os.ReadFile(path)
	if err != nil {
		return t, fmt.Errorf("%s: %w", path, err)
	}
	t.exists, t.mode = true, info.Mode()
	return t, nil
}

// readSettings reads a settings file as strict JSON. Comments, trailing commas
// and a repeated key are refused: the assistant and we could read them differently.
func readSettings(path, key string) (target, error) {
	t, err := readTarget(path)
	if err != nil || !t.exists {
		return t, err
	}
	if bytes.HasPrefix(t.bytes, []byte("\xef\xbb\xbf")) {
		return t, fmt.Errorf("%s: starts with a byte order mark (BOM); save it as UTF-8 without BOM and try again", path)
	}
	if !json.Valid(t.bytes) {
		return t, fmt.Errorf("%s: not valid JSON (comments are not supported); fix it and try again", path)
	}
	root := gjson.ParseBytes(t.bytes)
	if !root.IsObject() {
		return t, fmt.Errorf("%s: expected a JSON object", path)
	}
	seen := 0
	root.ForEach(func(k, _ gjson.Result) bool {
		if k.String() == key {
			seen++
		}
		return true
	})
	if seen > 1 {
		return t, fmt.Errorf("%s: %q appears %d times", path, key, seen)
	}
	return t, nil
}

var firstIndent = regexp.MustCompile(`\n([ \t]*)"`)

// setKey sets a top-level key. An existing key keeps its place, a new one goes
// last in the file's own layout. All other bytes stay as they are.
func setKey(raw []byte, key, value string) ([]byte, error) {
	if gjson.GetBytes(raw, key).Exists() {
		return sjson.SetRawBytes(raw, key, []byte(value))
	}
	s := string(raw)
	end := strings.LastIndex(s, "}")
	head, tail := strings.TrimRight(s[:end], " \t\r\n"), s[end:]
	nl := "\n"
	if strings.Contains(s, "\r\n") {
		nl = "\r\n"
	}
	field := jsonString(key) + ": " + value
	indent := "  "
	if m := firstIndent.FindStringSubmatch(head); m != nil {
		indent = m[1]
	}
	switch {
	case head == "{":
		return []byte("{" + nl + indent + field + nl + tail), nil
	case !strings.Contains(head, "\n"):
		return []byte(head + "," + jsonString(key) + ":" + value + tail), nil
	}
	return []byte(head + "," + nl + indent + field + nl + tail), nil
}

func jsonString(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a string cannot fail
	return strings.TrimSpace(b.String())
}
