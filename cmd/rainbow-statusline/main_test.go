package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// runBinary builds the tool once per test and runs it in an empty home.
func runBinary(t *testing.T, stdin string, args ...string) []byte {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "rainbow-statusline")
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
