package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/milczarekIT/rainbow-statusline/internal/config"
	"github.com/milczarekIT/rainbow-statusline/internal/install"
)

func TestResolveWidth(t *testing.T) {
	size := func(n int) func() int { return func() int { return n } }
	never := func() int { t.Error("source must not be read"); return 0 }
	cases := []struct {
		name    string
		flag    int
		columns string
		sources []func() int
		want    int
	}{
		{"flag wins", 50, "60", []func() int{never, never}, 50},
		{"columns next", 0, "60", []func() int{never, never}, 60},
		{"stderr next", 0, "", []func() int{size(70), never}, 70},
		{"tty next", 0, "", []func() int{size(0), size(80)}, 80},
		{"nothing means no trimming", 0, "", []func() int{size(0), size(0)}, 0},
		{"bad columns falls through", 0, "wide", []func() int{size(70)}, 70},
		{"zero columns falls through", 0, "0", []func() int{size(70)}, 70},
		{"negative values fall through", -1, "-5", []func() int{size(-3), size(80)}, 80},
	}
	for _, c := range cases {
		if got := resolveWidth(c.flag, c.columns, c.sources...); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// binPath is where a test builds the tool; Windows only runs files ending in .exe.
func binPath(t *testing.T) string {
	name := "rainbow-statusline"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(t.TempDir(), name)
}

// runBinary builds the tool once per test and runs it in an empty home.
func runBinary(t *testing.T, stdin string, args ...string) []byte {
	t.Helper()
	bin := binPath(t)
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	cmd := exec.Command(bin, args...)
	cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "LOCALAPPDATA=" + home, "XDG_CONFIG_HOME=" + home + "/cfg", "XDG_DATA_HOME=" + home + "/data", "XDG_CACHE_HOME=" + home + "/cache", "PATH="}
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil || stderr.Len() != 0 {
		t.Fatalf("exit %v, stderr %q", err, stderr.String())
	}
	return out
}

func TestOpenCodeJSONHomeScreen(t *testing.T) {
	for _, in := range []string{``, `not json`, `{"cwd":"/w"}`, `{"cwd":"/w","sessionID":null,"messages":null}`} {
		out := runBinary(t, in, "--agent", "opencode", "--format", "json")
		var lines [][]struct {
			Text string `json:"text"`
			FG   string `json:"fg"`
			Bold bool   `json:"bold"`
			Dim  bool   `json:"dim"`
		}
		if err := json.Unmarshal(out, &lines); err != nil {
			t.Fatalf("%q: %v: %s", in, err, out)
		}
		if !strings.Contains(string(out), "—") || !strings.Contains(string(out), "[RTK off]") || !strings.Contains(string(out), "[CLAUDE-MEM off]") {
			t.Errorf("%q: %s", in, out)
		}
	}
}

func TestDefaultFormatStaysANSI(t *testing.T) {
	out := runBinary(t, `{}`, "--agent", "opencode")
	if !strings.Contains(string(out), "\x1b[") || json.Valid(out) {
		t.Errorf("got %q", out)
	}
}

func TestUninstallSubcommand(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "COPILOT_HOME", "CLAUDE_CONFIG_DIR", "PATH"} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	old := config.ConfigDir
	config.ConfigDir = func() (string, error) { return filepath.Join(home, "cfg"), nil }
	t.Cleanup(func() { config.ConfigDir = old })

	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := `{"a":1}`
	if err := os.WriteFile(p, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, err := install.Plan([]string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := install.Apply(changes); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := runUninstall(&out, &errOut); code != 0 || errOut.Len() != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
	if !strings.Contains(out.String(), p) {
		t.Errorf("stdout = %q", out.String())
	}
	if got, _ := os.ReadFile(p); string(got) != orig {
		t.Errorf("got %q", got)
	}

	// A broken state is an error with a non-zero exit.
	sf := filepath.Join(home, "cfg", "rainbow-statusline", "state", "state.json")
	if err := os.WriteFile(sf, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := runUninstall(&out, &errOut); code != 1 || !strings.Contains(errOut.String(), sf) {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) { panic("boom") }

func TestRunUninstallTurnsPanicIntoExitOne(t *testing.T) {
	home := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "COPILOT_HOME", "CLAUDE_CONFIG_DIR"} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	old := config.ConfigDir
	config.ConfigDir = func() (string, error) { return filepath.Join(home, "cfg"), nil }
	t.Cleanup(func() { config.ConfigDir = old })
	var errOut bytes.Buffer
	if code := runUninstall(panicWriter{}, &errOut); code != 1 || !strings.Contains(errOut.String(), "boom") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestNullStdinDrawsNothingAndExitsZero(t *testing.T) {
	bin := binPath(t)
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	for _, args := range [][]string{{"--agent", "claude"}, {}} {
		cmd := exec.Command(bin, args...)
		cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "XDG_CONFIG_HOME=" + home + "/cfg", "PATH="}
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdin = null
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		null.Close()
		if err != nil || len(out) != 0 || stderr.Len() != 0 {
			t.Errorf("%v: err %v, stdout %q, stderr %q", args, err, out, stderr.String())
		}
	}
}

func TestWantsScreen(t *testing.T) {
	cases := []struct {
		stdinTTY, stdoutTTY bool
		flagsSet            int
		want                bool
	}{
		{true, true, 0, true},
		{true, true, 1, false}, // --agent, --width or --format was given
		{true, false, 0, false},
		{false, true, 0, false},
	}
	for _, c := range cases {
		if got := wantsScreen(c.stdinTTY, c.stdoutTTY, c.flagsSet); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

func TestOpenUIGuideOnlyWithoutConfig(t *testing.T) {
	var ran []string
	guide := func() error { ran = append(ran, "guide"); return nil }
	screen := func() error { ran = append(ran, "screen"); return nil }
	_ = openUI(false, guide, screen)
	_ = openUI(true, guide, screen)
	if strings.Join(ran, ",") != "guide,screen" {
		t.Errorf("ran %v", ran)
	}
}

func TestVersionAndHelpPrint(t *testing.T) {
	if got := strings.TrimSpace(string(runBinary(t, "", "--version"))); got != "dev" {
		t.Errorf("--version: %q", got)
	}
	for _, arg := range []string{"--help", "-h"} {
		if got := string(runBinary(t, "", arg)); !strings.Contains(got, "Usage:") || !strings.Contains(got, "uninstall") || !strings.Contains(got, "--replace") {
			t.Errorf("%s: %q", arg, got)
		}
	}
}

func TestVersionComesFromLdflags(t *testing.T) {
	bin := binPath(t)
	if out, err := exec.Command("go", "build", "-ldflags", "-X main.version=9.9.9", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "9.9.9" {
		t.Errorf("got %q, %v", out, err)
	}
}

func TestUnknownArgumentStaysSilentWhenAnAssistantRunsIt(t *testing.T) {
	out := runBinary(t, "{}", "--bogus")
	if strings.Contains(string(out), "Usage:") {
		t.Errorf("usage printed to an assistant: %q", out)
	}
}

func TestBadArguments(t *testing.T) {
	bad := errors.New("flag provided but not defined")
	cases := []struct {
		stdinTTY, stdoutTTY bool
		err                 error
		extra               int
		want                bool
	}{
		{true, true, bad, 0, true},
		{true, true, nil, 1, true},
		{true, true, nil, 0, false},
		{false, true, bad, 0, false},
		{true, false, nil, 1, false},
	}
	for _, c := range cases {
		if got := badArguments(c.stdinTTY, c.stdoutTTY, c.err, c.extra); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}

// installHome gives runInstall an empty home where Claude Code is detected.
func installHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, k := range []string{"XDG_CONFIG_HOME", "CLAUDE_CONFIG_DIR", "PATH"} {
		t.Setenv(k, "")
	}
	t.Setenv("COPILOT_HOME", filepath.Join(home, "no-copilot"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	old := config.ConfigDir
	config.ConfigDir = func() (string, error) { return filepath.Join(home, "cfg"), nil }
	t.Cleanup(func() { config.ConfigDir = old })
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func detectCopilot(t *testing.T, home string) string {
	t.Helper()
	dir := filepath.Join(home, ".copilot")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COPILOT_HOME", dir)
	return filepath.Join(dir, "settings.json")
}

func runInstallOK(t *testing.T, args ...string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := runInstall(args, &out, &errOut); code != 0 || errOut.Len() != 0 {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
	return out.String()
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallSetsUpDetectedAssistantWithDefaults(t *testing.T) {
	home := installHome(t)
	out := runInstallOK(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if !strings.Contains(read(t, settings), "--agent claude") {
		t.Errorf("settings: %s", read(t, settings))
	}
	if !strings.Contains(out, settings) {
		t.Errorf("stdout = %q", out)
	}
	cfg, found := config.Load()
	if !found || cfg.Density != "full" || strings.Join(cfg.Agents, ",") != "claude" {
		t.Errorf("config %+v, found %v", cfg, found)
	}
}

func TestInstallFailures(t *testing.T) {
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"unknown assistant", []string{"--agent", "nope"}, 1, "nope"},
		{"assistant not detected", []string{"--agent", "copilot"}, 1, "not detected"},
		{"unknown flag", []string{"--nope"}, 2, "Usage:"},
		{"stray argument", []string{"claude"}, 2, "Usage:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			installHome(t)
			var out, errOut bytes.Buffer
			if code := runInstall(c.args, &out, &errOut); code != c.code || !strings.Contains(errOut.String(), c.want) {
				t.Errorf("code %d, stderr %q", code, errOut.String())
			}
			if _, found := config.Load(); found {
				t.Error("a failed install saved a configuration")
			}
		})
	}
}

func TestInstallFailsWhenNoAssistantIsDetected(t *testing.T) {
	home := installHome(t)
	if err := os.Remove(filepath.Join(home, ".claude")); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runInstall(nil, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "no assistant") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestInstallRefusesForeignStatusLineWithoutReplace(t *testing.T) {
	home := installHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	orig := `{"statusLine":{"type":"command","command":"my-line"}}`
	if err := os.WriteFile(settings, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runInstall(nil, &out, &errOut); code != 1 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{settings, "my-line", "--replace"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr %q lacks %q", errOut.String(), want)
		}
	}
	if got := read(t, settings); got != orig {
		t.Errorf("settings changed: %s", got)
	}
	if _, found := config.Load(); found {
		t.Error("a refused install saved a configuration")
	}

	runInstallOK(t, "--replace")
	if got := read(t, settings); strings.Contains(got, "my-line") || !strings.Contains(got, "--agent claude") {
		t.Errorf("settings after --replace: %s", got)
	}
}

func TestInstallSanitizesTheLineItRefuses(t *testing.T) {
	home := installHome(t)
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.WriteFile(settings, []byte(`{"statusLine":{"command":"a\u001b[31mb"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runInstall(nil, &out, &errOut); code != 1 || strings.Contains(errOut.String(), "\x1b") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestInstallAgainReportsNothingAndKeepsTheView(t *testing.T) {
	installHome(t)
	if out := runInstallOK(t); strings.Contains(out, "Nothing to change") {
		t.Errorf("first run: %q", out)
	}
	cfg, _ := config.Load()
	cfg.Density = "compact"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if out := runInstallOK(t); out != "Nothing to change.\n" {
		t.Errorf("second run: %q", out)
	}
	if cfg, _ := config.Load(); cfg.Density != "compact" {
		t.Errorf("density = %q", cfg.Density)
	}
}

func TestInstallAgainKeepsAnAssistantTheUserLeftOut(t *testing.T) {
	home := installHome(t)
	copilot := detectCopilot(t, home)
	if err := config.Save(config.Config{Agents: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	runInstallOK(t)
	if _, err := os.Stat(copilot); !os.IsNotExist(err) {
		t.Errorf("copilot was installed: %v", err)
	}
}

func TestInstallAgentFlagAddsToTheChosenAssistants(t *testing.T) {
	home := installHome(t)
	copilot := detectCopilot(t, home)
	runInstallOK(t, "--agent", "claude,claude")
	if _, err := os.Stat(copilot); !os.IsNotExist(err) {
		t.Errorf("copilot was installed: %v", err)
	}
	if cfg, _ := config.Load(); strings.Join(cfg.Agents, ",") != "claude" {
		t.Errorf("agents = %v", cfg.Agents)
	}
	runInstallOK(t, "--agent", "copilot")
	if !strings.Contains(read(t, copilot), "--agent copilot") {
		t.Errorf("copilot settings: %s", read(t, copilot))
	}
	if cfg, _ := config.Load(); strings.Join(cfg.Agents, ",") != "claude,copilot" {
		t.Errorf("agents = %v", cfg.Agents)
	}
}

func TestRunInstallTurnsPanicIntoExitOne(t *testing.T) {
	installHome(t)
	var errOut bytes.Buffer
	if code := runInstall(nil, panicWriter{}, &errOut); code != 1 || !strings.Contains(errOut.String(), "boom") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

// Automation has no terminal: the command must work with no stdin at all.
func TestInstallSubcommandRunsWithoutATerminal(t *testing.T) {
	bin := binPath(t)
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "install")
	cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + home + "/appdata", "XDG_CONFIG_HOME=" + home + "/cfg", "PATH="}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(read(t, settings), "--agent claude") {
		t.Errorf("settings: %s", read(t, settings))
	}
}
