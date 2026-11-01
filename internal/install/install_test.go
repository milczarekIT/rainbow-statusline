package install

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
)

const (
	fakeBin = "/opt/rs/rainbow-statusline"
	ourCmd  = fakeBin + " --agent claude"
)

// sandbox points every root at a temp dir. No test may reach the owner's files.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home, "XDG_CONFIG_HOME": "", "COPILOT_HOME": "",
		"CLAUDE_CONFIG_DIR": "", "PATH": "", "APPDATA": filepath.Join(home, "appdata"),
	} {
		t.Setenv(k, v)
	}
	oldCfg, oldExe, oldReport := config.ConfigDir, executable, Report
	config.ConfigDir = func() (string, error) { return filepath.Join(home, "cfg"), nil }
	executable = func() (string, error) { return fakeBin, nil }
	Report = &bytes.Buffer{}
	t.Cleanup(func() { config.ConfigDir, executable, Report = oldCfg, oldExe, oldReport })
	return home
}

func put(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s should not exist (err=%v)", path, err)
	}
}

func install(t *testing.T, ids ...string) []Change {
	t.Helper()
	changes, err := Plan(ids)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(changes); err != nil {
		t.Fatal(err)
	}
	return changes
}

func uninstall(t *testing.T) string {
	t.Helper()
	buf := Report.(*bytes.Buffer)
	buf.Reset()
	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// tree maps every file under root to its bytes and mode.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			out[p] = "dir"
			return nil
		}
		b, _ := os.ReadFile(p)
		out[p] = info.Mode().String() + string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(t *testing.T, want, got map[string]string) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s changed:\n got %q\nwant %q", k, got[k], v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s appeared", k)
		}
	}
}

func backups(t *testing.T, home string) []string {
	t.Helper()
	dir := filepath.Join(home, "cfg", "rainbow-statusline", "state", "backups")
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, get(t, filepath.Join(dir, e.Name())))
	}
	return out
}

const statusEntry = `"statusLine": {"type":"command","command":"` + ourCmd + `","padding":0,"refreshInterval":1}`

func TestInstallKeepsOtherKeys(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\n  \"zeta\": 1,\n  \"alpha\": {\"x\": [1,  2]},\n  \"mid\": \"ą\"\n}\n")
	install(t, "claude")
	want := "{\n  \"zeta\": 1,\n  \"alpha\": {\"x\": [1,  2]},\n  \"mid\": \"ą\",\n  " + statusEntry + "\n}\n"
	if got := get(t, p); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	// An existing statusLine is replaced where it stands.
	home = sandbox(t)
	p = filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\n  \"statusLine\": {\"command\": \"old\"},\n  \"z\": 1\n}\n")
	install(t, "claude")
	want = "{\n  " + statusEntry + ",\n  \"z\": 1\n}\n"
	if got := get(t, p); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestInstallIntoCRLFAndEmptyObject(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\r\n    \"a\": 1\r\n}\r\n")
	install(t, "claude")
	want := "{\r\n    \"a\": 1,\r\n    " + statusEntry + "\r\n}\r\n"
	if got := get(t, p); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	home = sandbox(t)
	p = filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{}")
	install(t, "claude")
	var v map[string]any
	if err := json.Unmarshal([]byte(get(t, p)), &v); err != nil {
		t.Fatal(err)
	}
	if _, ok := v["statusLine"]; !ok {
		t.Errorf("no statusLine in %s", get(t, p))
	}
}

func TestBackupBeforeWrite(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	orig := "{\"a\":1}\n"
	put(t, p, orig)
	install(t, "claude")
	got := backups(t, home)
	if len(got) != 1 || got[0] != orig {
		t.Fatalf("backups = %q", got)
	}

	// Later backups pile up; none is overwritten or deleted, even at once.
	put(t, p, "{\"a\":2}\n")
	install(t, "claude")
	uninstall(t)
	all := strings.Join(backups(t, home), "|")
	if n := len(backups(t, home)); n < 3 {
		t.Errorf("want 3 or more backups, got %d: %s", n, all)
	}
	for _, want := range []string{orig, "{\"a\":2}\n"} {
		if !strings.Contains(all, want) {
			t.Errorf("backup of %q lost: %s", want, all)
		}
	}
}

func TestUninstallByteIdentical(t *testing.T) {
	for name, orig := range map[string]string{
		"no status line": "{\n  \"b\": 1,\n\n    \"a\" :   2\n}\n\n",
		"status line":    "{ \"statusLine\" : { \"command\":\"old\" } ,\n\"z\":1}",
		"crlf":           "{\r\n  \"a\": 1\r\n}\r\n",
		// Taking out only our key could not bring back this whitespace.
		"space before brace": "{ \"a\": 1  }\n",
		"blank lines at end": "{\n  \"a\": 1\n\n\n}\n",
		"spaced empty":       "{   }",
	} {
		t.Run(name, func(t *testing.T) {
			home := sandbox(t)
			p := filepath.Join(home, ".claude", "settings.json")
			put(t, p, orig)
			if err := os.Chmod(p, 0o600); err != nil {
				t.Fatal(err)
			}
			install(t, "claude")
			if get(t, p) == orig {
				t.Fatal("install changed nothing")
			}
			uninstall(t)
			if got := get(t, p); got != orig {
				t.Errorf("got %q want %q", got, orig)
			}
			if info, _ := os.Stat(p); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
				t.Errorf("mode %v", info.Mode())
			}
		})
	}
}

func TestUninstallKeepsLaterEdits(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\n  \"statusLine\": {\"command\":  \"old\"},\n  \"a\": 1\n}\n")
	install(t, "claude")
	put(t, p, strings.Replace(get(t, p), "\"a\": 1", "\"a\": 1,\n  \"mine\": true", 1))
	uninstall(t)
	want := "{\n  \"statusLine\": {\"command\":  \"old\"},\n  \"a\": 1,\n  \"mine\": true\n}\n"
	if got := get(t, p); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	// Without a previous statusLine our key simply goes.
	home = sandbox(t)
	p = filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\n  \"a\": 1\n}\n")
	install(t, "claude")
	put(t, p, strings.Replace(get(t, p), "\"a\": 1", "\"a\": 1,\n  \"mine\": true", 1))
	uninstall(t)
	want = "{\n  \"a\": 1,\n  \"mine\": true\n}\n"
	if got := get(t, p); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestUninstallLeavesFileWithoutOurEntry(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\"a\":1}")
	install(t, "claude")
	mine := `{"statusLine":{"command":"mine"},"a":1}`
	put(t, p, mine)
	out := uninstall(t)
	if got := get(t, p); got != mine {
		t.Errorf("file touched: %q", got)
	}
	if !strings.Contains(out, p) || !strings.Contains(out, "left alone") {
		t.Errorf("report = %q", out)
	}
}

func TestSecondUninstallIsNoOp(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\"a\":1}")
	install(t, "claude")
	uninstall(t)
	put(t, p, `{"a":1,"later":true}`)
	before := tree(t, home)
	out := uninstall(t)
	sameTree(t, before, tree(t, home))
	if !strings.Contains(out, "nothing to uninstall") {
		t.Errorf("report = %q", out)
	}
}

func TestReinstallIsIdempotent(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	orig := `{"statusLine":{"command":"old"}}`
	put(t, p, orig)
	if c := install(t, "claude"); c[0].Replaces != "old" {
		t.Errorf("first Replaces = %q", c[0].Replaces)
	}
	before := tree(t, home)
	c := install(t, "claude")
	if c[0].Replaces != "" {
		t.Errorf("our own entry reported as replaced: %q", c[0].Replaces)
	}
	sameTree(t, before, tree(t, home))
	uninstall(t)
	if got := get(t, p); got != orig {
		t.Errorf("original lost: %q", got)
	}
}

func TestReinstallWithNewBinaryKeepsFirstOriginal(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	orig := `{"statusLine":{"command":"old"}}`
	put(t, p, orig)
	install(t, "claude")
	executable = func() (string, error) { return "/new/rainbow-statusline", nil }
	if c := install(t, "claude"); c[0].Replaces != "" {
		t.Errorf("Replaces = %q", c[0].Replaces)
	}
	if !strings.Contains(get(t, p), "/new/rainbow-statusline --agent claude") {
		t.Errorf("not updated: %s", get(t, p))
	}
	uninstall(t)
	if got := get(t, p); got != orig {
		t.Errorf("got %q want %q", got, orig)
	}
}

// A reinstall that rewrites a file the user edited must not make those edits
// look like ours: uninstall may no longer restore the old bytes.
func TestReinstallOverUserEditsKeepsThem(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, `{"statusLine":{"command":"old"},"a":1}`)
	install(t, "claude")
	put(t, p, strings.Replace(get(t, p), `"a":1`, `"a":1,"mine":true`, 1))
	executable = func() (string, error) { return "/new/rainbow-statusline", nil }
	install(t, "claude")
	uninstall(t)
	want := `{"statusLine":{"command":"old"},"a":1,"mine":true}`
	if got := get(t, p); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestReinstallOverForeignLineReturnsToIt(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, `{"statusLine":{"command":"first"}}`)
	install(t, "claude")
	put(t, p, `{"statusLine":{"command":"second"},"b":2}`)
	c := install(t, "claude")
	if c[0].Replaces != "second" {
		t.Errorf("Replaces = %q", c[0].Replaces)
	}
	uninstall(t)
	if got, want := get(t, p), `{"statusLine":{"command":"second"},"b":2}`; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestReplacesReported(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, `{"statusLine":{"type":"command","command":"~/mine.sh"}}`)
	c, err := Plan([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 1 || c[0].Replaces != "~/mine.sh" || c[0].Path != p || c[0].Agent != "claude" || c[0].Summary == "" {
		t.Errorf("%+v", c)
	}
	put(t, p, `{"a":1}`)
	if c, _ = Plan([]string{"claude"}); c[0].Replaces != "" {
		t.Errorf("no statusLine, Replaces = %q", c[0].Replaces)
	}
}

func TestPlanNeverWrites(t *testing.T) {
	home := sandbox(t)
	put(t, filepath.Join(home, ".claude", "settings.json"), `{"a":1}`)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := tree(t, home)
	if _, err := Plan([]string{"claude", "copilot", "opencode"}); err != nil {
		t.Fatal(err)
	}
	sameTree(t, before, tree(t, home))
}

func TestInvalidSettingsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"comment":   "// note\n{\"a\":1}",
		"broken":    `{"a":`,
		"trailing":  `{"a":1,}`,
		"empty":     "",
		"array":     `[1]`,
		"duplicate": `{"statusLine":{"command":"a"},"statusLine":{"command":"b"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			home := sandbox(t)
			p := filepath.Join(home, ".claude", "settings.json")
			put(t, p, body)
			before := tree(t, home)
			_, err := Plan([]string{"claude"})
			if err == nil || !strings.Contains(err.Error(), p) {
				t.Errorf("Plan err = %v", err)
			}
			err = Apply([]Change{{Agent: "claude", Path: p}})
			if err == nil || !strings.Contains(err.Error(), p) {
				t.Errorf("Apply err = %v", err)
			}
			sameTree(t, before, tree(t, home))
		})
	}
	t.Run("read-only", func(t *testing.T) {
		home := sandbox(t)
		p := filepath.Join(home, ".claude", "settings.json")
		put(t, p, `{"a":1}`)
		if err := os.Chmod(p, 0o444); err != nil {
			t.Fatal(err)
		}
		before := tree(t, home)
		if _, err := Plan([]string{"claude"}); err == nil || !strings.Contains(err.Error(), p) {
			t.Errorf("err = %v", err)
		}
		sameTree(t, before, tree(t, home))
	})
}

func TestUninstallRefusesBrokenFileAndChangesNothing(t *testing.T) {
	home := sandbox(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	copilot := filepath.Join(home, ".copilot", "settings.json")
	put(t, claude, `{"a":1}`)
	put(t, copilot, `{"b":1}`)
	install(t, "claude", "copilot")
	put(t, copilot, "// mine\n{}")
	before := tree(t, home)
	err := Uninstall()
	if err == nil || !strings.Contains(err.Error(), copilot) {
		t.Fatalf("err = %v", err)
	}
	sameTree(t, before, tree(t, home))
}

func TestApplyRestoresEarlierFilesOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions differ on Windows")
	}
	home := sandbox(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	copilot := filepath.Join(home, ".copilot", "settings.json")
	put(t, claude, `{"a":1}`)
	put(t, copilot, `{"b":1}`)
	changes, err := Plan([]string{"claude", "copilot"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(copilot)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err = Apply(changes)
	if err == nil || !strings.Contains(err.Error(), copilot) {
		t.Fatalf("err = %v", err)
	}
	if got := get(t, claude); got != `{"a":1}` {
		t.Errorf("claude not restored: %q", got)
	}
	if got := get(t, copilot); got != `{"b":1}` {
		t.Errorf("copilot changed: %q", got)
	}
	// The failed install left no record to undo.
	_ = os.Chmod(dir, 0o755)
	if out := uninstall(t); !strings.Contains(out, "nothing to uninstall") {
		t.Errorf("report = %q", out)
	}
	if got := get(t, claude); got != `{"a":1}` {
		t.Errorf("claude touched: %q", got)
	}
}

func TestApplyRefusesLineChangedAfterPlan(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, `{"statusLine":{"command":"seen"}}`)
	c, err := Plan([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	put(t, p, `{"statusLine":{"command":"unseen"}}`)
	if err := Apply(c); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err = %v", err)
	}
	if got := get(t, p); !strings.Contains(got, "unseen") {
		t.Errorf("overwritten: %s", got)
	}
}

func TestSymlinkedSettingsStayLinked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	home := sandbox(t)
	real := filepath.Join(home, "dotfiles", "settings.json")
	p := filepath.Join(home, ".claude", "settings.json")
	orig := `{"a":1}`
	put(t, real, orig)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, p); err != nil {
		t.Fatal(err)
	}
	install(t, "claude")
	if info, _ := os.Lstat(p); info.Mode()&os.ModeSymlink == 0 {
		t.Error("link replaced by a file")
	}
	if !strings.Contains(get(t, real), "statusLine") {
		t.Error("target not written")
	}
	uninstall(t)
	if info, _ := os.Lstat(p); info.Mode()&os.ModeSymlink == 0 {
		t.Error("link replaced by a file on uninstall")
	}
	if got := get(t, real); got != orig {
		t.Errorf("got %q", got)
	}
}

func TestMissingSettingsCreatedOnlyWhenDetected(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	if _, err := Plan([]string{"claude"}); err == nil {
		t.Error("planned for an assistant that is not there")
	}
	if err := Apply([]Change{{Agent: "claude", Path: p}}); err == nil {
		t.Error("applied for an assistant that is not there")
	}
	gone(t, filepath.Join(home, ".claude"))

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	install(t, "claude")
	if !strings.Contains(get(t, p), ourCmd) {
		t.Errorf("got %s", get(t, p))
	}
	uninstall(t)
	gone(t, p)

	// A later edit keeps the file we created.
	install(t, "claude")
	put(t, p, strings.Replace(get(t, p), "{", `{"mine":true,`, 1))
	uninstall(t)
	if got := get(t, p); !strings.Contains(got, "mine") || strings.Contains(got, "statusLine") {
		t.Errorf("got %s", got)
	}
}

func TestDetectViaPathAndUnknownAgent(t *testing.T) {
	home := sandbox(t)
	if _, err := Plan([]string{"nope"}); err == nil {
		t.Error("unknown agent accepted")
	}
	bin := filepath.Join(home, "bin")
	put(t, filepath.Join(bin, "copilot"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(bin, "copilot"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	put(t, filepath.Join(home, ".claude", "settings.json"), `{"statusLine":{"command":"x"}}`)
	got := map[string]Agent{}
	for _, a := range Detect() {
		got[a.ID] = a
	}
	if len(got) != 3 || got["opencode"].Found || !got["claude"].Found {
		t.Errorf("%+v", got)
	}
	if runtime.GOOS != "windows" && !got["copilot"].Found {
		t.Errorf("copilot on PATH not found: %+v", got["copilot"])
	}
	if got["claude"].CurrentStatusLine != "x" || got["claude"].Name != "Claude Code" ||
		got["claude"].SettingsPath != filepath.Join(home, ".claude", "settings.json") {
		t.Errorf("%+v", got["claude"])
	}
}

func TestClaudeConfigDirHonoured(t *testing.T) {
	home := sandbox(t)
	dir := filepath.Join(home, "elsewhere")
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	put(t, filepath.Join(dir, "settings.json"), `{}`)
	c := install(t, "claude")
	if c[0].Path != filepath.Join(dir, "settings.json") {
		t.Errorf("path %s", c[0].Path)
	}
}

func TestCopilotUsesSettingsJSONOnly(t *testing.T) {
	home := sandbox(t)
	dir := filepath.Join(home, "copilot-home")
	t.Setenv("COPILOT_HOME", dir)
	// config.json really carries comments and is managed by Copilot.
	cfg := "// This file is managed automatically.\n{\"firstLaunchAt\":\"x\"}\n"
	put(t, filepath.Join(dir, "config.json"), cfg)
	c := install(t, "copilot")
	p := filepath.Join(dir, "settings.json")
	if len(c) != 1 || c[0].Path != p {
		t.Fatalf("%+v", c)
	}
	if got := get(t, p); !strings.Contains(got, `"command":"`+fakeBin+` --agent copilot"`) || !strings.Contains(got, `"refreshInterval":1`) {
		t.Errorf("got %s", got)
	}
	if got := get(t, filepath.Join(dir, "config.json")); got != cfg {
		t.Errorf("config.json touched: %s", got)
	}
	uninstall(t)
	gone(t, p)
}

func TestOpenCodeInstallAndUninstall(t *testing.T) {
	home := sandbox(t)
	xdg := filepath.Join(home, "xdg cfg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	tui := filepath.Join(xdg, "opencode", "tui.json")
	orig := "{\n  \"plugin\": [\n    \"file:///other/tui.js\",\n    [\"tuple\", {\"a\": 1}]\n  ],\n  \"theme\": \"x\"\n}\n"
	put(t, tui, orig)
	changes := install(t, "opencode")
	if len(changes) != 2 {
		t.Fatalf("%+v", changes)
	}
	file := filepath.Join(xdg, "opencode", "plugins", "rainbow-statusline", "tui.tsx")
	src := get(t, file)
	if !strings.Contains(src, `const INSTALLED_BIN = "`+fakeBin+`"`) {
		t.Errorf("binary path not pinned in the copied plugin")
	}
	url := fileURL(runtime.GOOS, file)
	want := "{\n  \"plugin\": [\n    \"file:///other/tui.js\",\n    [\"tuple\", {\"a\": 1}],\n    \"" + url + "\"\n  ],\n  \"theme\": \"x\"\n}\n"
	if got := get(t, tui); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	before := tree(t, home)
	install(t, "opencode")
	sameTree(t, before, tree(t, home))
	uninstall(t)
	if got := get(t, tui); got != orig {
		t.Errorf("got %q", got)
	}
	gone(t, filepath.Dir(file))
}

func TestOpenCodeLaterEditsAndPluginEdits(t *testing.T) {
	home := sandbox(t)
	tui := filepath.Join(home, ".config", "opencode", "tui.json")
	put(t, tui, `{"plugin":["a"]}`)
	install(t, "opencode")
	file := filepath.Join(home, ".config", "opencode", "plugins", "rainbow-statusline", "tui.tsx")
	put(t, tui, strings.Replace(get(t, tui), `"a"`, `"a","b"`, 1))
	put(t, file, "// mine\n")
	out := uninstall(t)
	if got := get(t, tui); got != `{"plugin":["a","b"]}` {
		t.Errorf("got %s", got)
	}
	if got := get(t, file); got != "// mine\n" {
		t.Errorf("plugin file changed: %s", got)
	}
	if !strings.Contains(out, file) {
		t.Errorf("report = %q", out)
	}
}

func TestOpenCodeNoPluginKeyAndNonArray(t *testing.T) {
	home := sandbox(t)
	tui := filepath.Join(home, ".config", "opencode", "tui.json")
	if err := os.MkdirAll(filepath.Dir(tui), 0o755); err != nil {
		t.Fatal(err)
	}
	install(t, "opencode")
	if !strings.Contains(get(t, tui), "file://") {
		t.Errorf("got %s", get(t, tui))
	}
	put(t, tui, strings.Replace(get(t, tui), "{", `{"theme":"x",`, 1))
	uninstall(t)
	if got := get(t, tui); strings.Contains(got, "plugin") || !strings.Contains(got, "theme") {
		t.Errorf("got %s", got)
	}

	put(t, tui, `{"plugin":"not-a-list"}`)
	if _, err := Plan([]string{"opencode"}); err == nil || !strings.Contains(err.Error(), tui) {
		t.Errorf("err = %v", err)
	}
}

func TestCorruptStateRefused(t *testing.T) {
	home := sandbox(t)
	put(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	sf := filepath.Join(home, "cfg", "rainbow-statusline", "state", "state.json")
	put(t, sf, "garbage")
	if _, err := Plan([]string{"claude"}); err == nil || !strings.Contains(err.Error(), sf) {
		t.Errorf("Plan err = %v", err)
	}
	if err := Uninstall(); err == nil || !strings.Contains(err.Error(), sf) {
		t.Errorf("Uninstall err = %v", err)
	}
	if got := get(t, sf); got != "garbage" {
		t.Errorf("state overwritten: %s", got)
	}
}

func TestCommandWithSpaces(t *testing.T) {
	for _, tc := range []struct {
		goos, path, want string
		args             []string
	}{
		{"windows", `C:\Users\Żółć Test\bin\rainbow-statusline.exe`, `"C:\Users\Żółć Test\bin\rainbow-statusline.exe" --agent claude`, []string{"--agent", "claude"}},
		{"windows", `C:\Program Files (x86)\r.exe`, `"C:\Program Files (x86)\r.exe" --agent copilot`, []string{"--agent", "copilot"}},
		{"windows", `C:\plain\r.exe`, `"C:\plain\r.exe" "a b"`, []string{"a b"}},
		{"darwin", "/Users/Żółć Test/bin/rainbow-statusline", `'/Users/Żółć Test/bin/rainbow-statusline' --agent claude`, []string{"--agent", "claude"}},
		{"darwin", "/opt/bin/rs", "/opt/bin/rs --agent claude", []string{"--agent", "claude"}},
		{"linux", "/o'brien/$x/rs", `'/o'\''brien/$x/rs' 'a b'`, []string{"a b"}},
	} {
		got, err := shellCommand(tc.goos, tc.path, tc.args...)
		if err != nil || got != tc.want {
			t.Errorf("%s %q: got %q (%v), want %q", tc.goos, tc.path, got, err, tc.want)
		}
	}
	// cmd.exe expands %VAR% inside quotes and has no escape on a /c command line.
	// Git Bash runs $(...) and backticks inside double quotes; a bare cmd /c drops
	// the quotes around a string with & and the like.
	for _, bad := range []string{`C:\100%\r.exe`, `C:\a"b\r.exe`, "C:\\a\nb\\r.exe", `C:\$(x)\r.exe`, "C:\\`x`\\r.exe", `C:\a&b\r.exe`, `C:\a|b\r.exe`, `C:\a<b\r.exe`, `C:\a>b\r.exe`, `C:\a^b\r.exe`, `C:\a!b\r.exe`} {
		if got, err := shellCommand("windows", bad, "--agent", "claude"); err == nil {
			t.Errorf("%q accepted as %q", bad, got)
		}
	}
}

func TestFileURL(t *testing.T) {
	for _, tc := range []struct{ goos, path, want string }{
		{"darwin", "/Users/me/.config/opencode/plugins/rainbow-statusline/tui.tsx", "file:///Users/me/.config/opencode/plugins/rainbow-statusline/tui.tsx"},
		{"darwin", "/Users/Łu kasz/#1/tui.tsx", "file:///Users/%C5%81u%20kasz/%231/tui.tsx"},
		{"windows", `C:\Users\Żółć Test\.config\tui.tsx`, "file:///C:/Users/%C5%BB%C3%B3%C5%82%C4%87%20Test/.config/tui.tsx"},
		{"windows", `\\srv\share\tui.tsx`, "file://srv/share/tui.tsx"},
	} {
		if got := fileURL(tc.goos, tc.path); got != tc.want {
			t.Errorf("%s %q: got %q want %q", tc.goos, tc.path, got, tc.want)
		}
	}
}

// The quoting is run through a real sh, not only compared as text.
func TestQuotedCommandRunsInSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	dir := filepath.Join(t.TempDir(), "Żółć Test's $HOME")
	script := filepath.Join(dir, "rainbow-statusline")
	put(t, script, "#!/bin/sh\necho ran \"$@\"\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := shellCommand(runtime.GOOS, script, "--agent", "claude")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", cmd).CombinedOutput()
	if err != nil || string(out) != "ran --agent claude\n" {
		t.Errorf("%q: %q, %v", cmd, out, err)
	}
}

func stateFile(home string) string {
	return filepath.Join(home, "cfg", "rainbow-statusline", "state", "state.json")
}

func TestUninstallWithLostStateRemovesOurEntry(t *testing.T) {
	for name, st := range map[string]*string{"missing": nil, "null": ptr("null"), "no records": ptr(`{"Records":{}}`)} {
		t.Run(name, func(t *testing.T) {
			home := sandbox(t)
			claude := filepath.Join(home, ".claude", "settings.json")
			tui := filepath.Join(home, ".config", "opencode", "tui.json")
			file := filepath.Join(home, ".config", "opencode", "plugins", "rainbow-statusline", "tui.tsx")
			put(t, claude, "{\n  \"a\": 1\n}\n")
			put(t, tui, `{"plugin":["other"]}`)
			install(t, "claude", "opencode")
			if err := os.Remove(stateFile(home)); err != nil {
				t.Fatal(err)
			}
			if st != nil {
				put(t, stateFile(home), *st)
			}
			out := uninstall(t)
			if got := get(t, claude); got != "{\n  \"a\": 1\n}\n" {
				t.Errorf("claude = %q", got)
			}
			if got := get(t, tui); got != `{"plugin":["other"]}` {
				t.Errorf("tui = %q", got)
			}
			gone(t, file)
			for _, want := range []string{claude, tui, "no original", filepath.Join(filepath.Dir(stateFile(home)), "backups")} {
				if !strings.Contains(out, want) {
					t.Errorf("report misses %q: %s", want, out)
				}
			}
			if strings.Contains(out, "nothing to uninstall") {
				t.Errorf("report = %s", out)
			}
			if !strings.Contains(strings.Join(backups(t, home), "|"), ourCmd) {
				t.Error("no backup of the file holding our entry")
			}
			if out := uninstall(t); !strings.Contains(out, "nothing to uninstall") {
				t.Errorf("second run: %s", out)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestLostStateLeavesWhatIsNotOurs(t *testing.T) {
	home := sandbox(t)
	claude := filepath.Join(home, ".claude", "settings.json")
	copilot := filepath.Join(home, ".copilot", "settings.json")
	tui := filepath.Join(home, ".config", "opencode", "tui.json")
	file := filepath.Join(home, ".config", "opencode", "plugins", "rainbow-statusline", "tui.tsx")
	wrapper := `{"statusLine":{"command":"my-wrap /opt/rainbow-statusline --agent claude"}}`
	put(t, claude, wrapper)
	// Comments in a file that never mentions us are no reason to stop.
	put(t, copilot, "// mine\n{}")
	put(t, tui, `{"plugin":["file:///elsewhere/rainbow-statusline/tui.tsx"]}`)
	put(t, file, "// the user's own\n")
	before := tree(t, home)
	delete(before, stateFile(home))
	out := uninstall(t)
	after := tree(t, home)
	delete(after, stateFile(home))
	for k, v := range before {
		if strings.HasPrefix(k, filepath.Dir(stateFile(home))) {
			delete(before, k)
			continue
		}
		if after[k] != v {
			t.Errorf("%s changed: %q", k, after[k])
		}
	}
	if !strings.Contains(out, file) || !strings.Contains(out, "left alone") {
		t.Errorf("report = %s", out)
	}
}

func TestLostStateBrokenFileWithOurNameRefused(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	body := "// x\n" + `{"statusLine":{"command":"` + ourCmd + `"}}`
	put(t, p, body)
	if err := Uninstall(); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err = %v", err)
	}
	if got := get(t, p); got != body {
		t.Errorf("changed: %s", got)
	}
}

func TestNullRecordIsCorruptState(t *testing.T) {
	home := sandbox(t)
	put(t, stateFile(home), `{"Records":{"p":null}}`)
	if err := Uninstall(); err == nil || !strings.Contains(err.Error(), stateFile(home)) {
		t.Errorf("err = %v", err)
	}
	put(t, filepath.Join(home, ".claude", "settings.json"), `{}`)
	if _, err := Plan([]string{"claude"}); err == nil || !strings.Contains(err.Error(), stateFile(home)) {
		t.Errorf("Plan err = %v", err)
	}
}

func TestPreviousKeepsInvalidUTF8(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "{\"statusLine\":{\"command\":\"a\xffb\"},\"a\":1}")
	install(t, "claude")
	put(t, p, strings.Replace(get(t, p), `"a":1`, `"a":1,"mine":true`, 1))
	uninstall(t)
	want := "{\"statusLine\":{\"command\":\"a\xffb\"},\"a\":1,\"mine\":true}"
	if got := get(t, p); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBOMNamedInError(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, "\xef\xbb\xbf{}")
	_, err := Plan([]string{"claude"})
	if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "BOM") || strings.Contains(err.Error(), "comments") {
		t.Errorf("err = %v", err)
	}
}

// A kill after the files are restored must leave the records, and a rerun over
// them changes nothing.
func TestUninstallSavesStateAfterWrites(t *testing.T) {
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	orig := `{"a":1}`
	put(t, p, orig)
	install(t, "claude")
	snapshot := get(t, stateFile(home))
	ran := false
	beforeStateSave = func() {
		ran = true
		if got := get(t, stateFile(home)); got != snapshot {
			t.Errorf("state already saved: %s", got)
		}
		if got := get(t, p); got != orig {
			t.Errorf("file not written yet: %s", got)
		}
	}
	t.Cleanup(func() { beforeStateSave = func() {} })
	uninstall(t)
	beforeStateSave = func() {}
	if !ran {
		t.Fatal("hook did not run")
	}
	put(t, stateFile(home), snapshot) // what the kill would have left
	uninstall(t)
	if got := get(t, p); got != orig {
		t.Errorf("rerun changed the file: %s", got)
	}
	if out := uninstall(t); !strings.Contains(out, "nothing to uninstall") {
		t.Errorf("third run: %s", out)
	}
}

func TestUninstallRemovesOnlyFoldersWeCreated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup needs .exe")
	}
	home := sandbox(t)
	bin := filepath.Join(home, "bin")
	for _, name := range []string{"copilot", "opencode"} {
		put(t, filepath.Join(bin, name), "#!/bin/sh\n")
		if err := os.Chmod(filepath.Join(bin, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	install(t, "copilot", "opencode")
	uninstall(t)
	gone(t, filepath.Join(home, ".copilot"))
	gone(t, filepath.Join(home, ".config"))
	t.Setenv("PATH", "")
	for _, a := range Detect() {
		if a.Found {
			t.Errorf("%s found after uninstall", a.ID)
		}
	}

	// A folder that was there stays, and so does one the user filled.
	t.Setenv("PATH", bin)
	keep := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	install(t, "copilot", "opencode")
	put(t, filepath.Join(home, ".config", "opencode", "plugins", "mine.js"), "x")
	uninstall(t)
	if _, err := os.Stat(keep); err != nil {
		t.Error("existing folder removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "plugins", "mine.js")); err != nil {
		t.Error("user's file lost")
	}
	gone(t, filepath.Join(home, ".config", "opencode", "plugins", "rainbow-statusline"))
}

func TestIsOurCommand(t *testing.T) {
	for cmd, want := range map[string]bool{
		`/opt/rainbow-statusline --agent claude`:                            true,
		`'/Users/Łu k/bin/rainbow-statusline' --agent claude`:               true,
		`'/o'\''b/rainbow-statusline' --agent claude`:                       true,
		`"C:\Program Files (x86)\rs\rainbow-statusline.exe" --agent claude`: true,
		`/opt/rainbow-statusline --agent copilot`:                           false,
		`my-wrap /opt/rainbow-statusline --agent claude`:                    false,
		`/opt/rainbow-statusline-old --agent claude`:                        false,
		`/opt/rainbow-statusline --agent claude; rm x`:                      false,
		`/opt/rainbow-statusline --agent claude --width 80`:                 false,
	} {
		if got := isOurCommand(cmd, "claude"); got != want {
			t.Errorf("%q: got %v", cmd, got)
		}
	}
}

func TestUninstallRestoresFilesWhenStateCannotBeSaved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions differ on Windows")
	}
	home := sandbox(t)
	p := filepath.Join(home, ".claude", "settings.json")
	put(t, p, `{"a":1}`)
	install(t, "claude")
	written := get(t, p)
	dir := filepath.Dir(stateFile(home))
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if err := Uninstall(); err == nil {
		t.Fatal("no error")
	}
	if got := get(t, p); got != written {
		t.Errorf("got %q want %q", got, written)
	}
}

func TestDetectInstalled(t *testing.T) {
	home := sandbox(t)
	put(t, filepath.Join(home, ".claude", "settings.json"), `{"statusLine":{"command":"my-bar --note rainbow-statusline --agent claude-ish"}}`)
	put(t, filepath.Join(home, ".config", "opencode", "tui.json"), `{}`)
	installed := func() map[string]bool {
		got := map[string]bool{}
		for _, a := range Detect() {
			got[a.ID] = a.Installed
		}
		return got
	}
	if got := installed(); got["claude"] || got["opencode"] || got["copilot"] {
		t.Errorf("nothing of ours is installed yet: %v", got)
	}
	install(t, "claude", "opencode")
	if got := installed(); !got["claude"] || !got["opencode"] || got["copilot"] {
		t.Errorf("after install: %v", got)
	}
}
