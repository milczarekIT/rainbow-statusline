package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

// installPlugin makes a Claude Code plugin enabled and installed under e.dir.
func installPlugin(t *testing.T, e *env, id string, enabled bool) {
	t.Helper()
	root := filepath.Join(e.dir, "plugins", "cache", strings.ReplaceAll(id, "@", "-"))
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	flag := "false"
	if enabled {
		flag = "true"
	}
	write(t, filepath.Join(e.dir, "settings.json"), `{"enabledPlugins":{"`+id+`":`+flag+`}}`)
	write(t, filepath.Join(e.dir, "plugins", "installed_plugins.json"),
		`{"plugins":{"`+id+`":[{"installPath":"`+filepath.ToSlash(filepath.Join(e.dir, "gone"))+`"},{"installPath":"`+filepath.ToSlash(root)+`"}]}}`)
}

func ponytailSetup(t *testing.T, flag string) *env {
	t.Helper()
	e := setup(t)
	installPlugin(t, e, "ponytail@ponytail", true)
	if flag != "" {
		write(t, filepath.Join(e.dir, ".ponytail-active"), flag)
	}
	return e
}

func ponyBadge(t *testing.T) (string, bool, []render.Segment) {
	t.Helper()
	b, ok := ponytail{}.Badge(model.Session{Agent: "claude"})
	return text(b), ok, b.Lines
}

func TestPonytailBadge(t *testing.T) {
	for flag, want := range map[string]string{
		"full":         "[PONYTAIL]",
		"full\n":       "[PONYTAIL]",
		"":             "[PONYTAIL]", // an empty flag file is the default mode
		"lite":         "[PONYTAIL:LITE]",
		"Ultra\r\n":    "[PONYTAIL:ULTRA]",
		" li te \n":    "[PONYTAIL:LITE]", // whitespace is dropped, like tr -d '[:space:]'
		"lite\nfull\n": "[PONYTAIL:LITE]", // first line only
		"future-m0de":  "[PONYTAIL:FUTURE-M0DE]",
	} {
		e := ponytailSetup(t, "x")
		write(t, filepath.Join(e.dir, ".ponytail-active"), flag)
		got, ok, lines := ponyBadge(t)
		if !ok || got != want {
			t.Errorf("flag %q: got %q %v, want %q", flag, got, ok, want)
		}
		if len(lines) != 1 || lines[0].FG != "38;5;108" {
			t.Errorf("flag %q: colour %+v", flag, lines)
		}
	}
}

func TestPonytailHidden(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		ponytailSetup(t, "off\n")
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("off shows a badge")
		}
	})
	t.Run("no flag file", func(t *testing.T) {
		ponytailSetup(t, "")
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("badge without a flag file")
		}
	})
	t.Run("plugin disabled", func(t *testing.T) {
		e := ponytailSetup(t, "full")
		installPlugin(t, e, "ponytail@ponytail", false)
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("badge for a disabled plugin")
		}
	})
	t.Run("plugin not installed", func(t *testing.T) {
		e := ponytailSetup(t, "full")
		write(t, filepath.Join(e.dir, "plugins", "installed_plugins.json"), `{"plugins":{}}`)
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("badge for a plugin that is not installed")
		}
	})
	t.Run("install path gone", func(t *testing.T) {
		e := ponytailSetup(t, "full")
		write(t, filepath.Join(e.dir, "plugins", "installed_plugins.json"),
			`{"plugins":{"ponytail@ponytail":[{"installPath":"`+filepath.ToSlash(filepath.Join(e.dir, "gone"))+`"}]}}`)
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("badge for a plugin whose folder is gone")
		}
	})
	t.Run("flag is a symlink", func(t *testing.T) {
		e := ponytailSetup(t, "")
		secret := filepath.Join(e.dir, "secret")
		write(t, secret, "full")
		if err := os.Symlink(secret, filepath.Join(e.dir, ".ponytail-active")); err != nil {
			t.Skip("no symlinks:", err)
		}
		if _, ok, _ := ponyBadge(t); ok {
			t.Fatal("followed a symlink")
		}
	})
}

func TestPonytailUnknownModeNeverReachesTheBar(t *testing.T) {
	for _, flag := range []string{"\x1b]8;;http://x\x07full", "li\x1b[31mte", "lite;rm", "ül", "a.b", "<b>", "a_b"} {
		ponytailSetup(t, flag)
		if got, ok, _ := ponyBadge(t); ok {
			t.Errorf("flag %q rendered %q", flag, got)
		}
	}
}

func TestPonytailReadsOnlySixteenBytes(t *testing.T) {
	e := ponytailSetup(t, "")
	// 16 valid bytes then a byte that would be rejected: it must not be read.
	write(t, filepath.Join(e.dir, ".ponytail-active"), strings.Repeat("a", 16)+"\x1b[31m")
	got, ok, _ := ponyBadge(t)
	if !ok || got != "[PONYTAIL:"+strings.Repeat("A", 16)+"]" {
		t.Fatalf("got %q %v", got, ok)
	}
}

func TestClaudeDirFollowsClaudeConfigDir(t *testing.T) {
	abs := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", abs)
	if got := defaultClaudeDir(); got != abs {
		t.Fatalf("got %q want %q", got, abs)
	}
	// A relative value would let a project folder supply the flag.
	t.Setenv("CLAUDE_CONFIG_DIR", "rel/dir")
	if got := defaultClaudeDir(); !filepath.IsAbs(got) || strings.Contains(got, "rel") {
		t.Fatalf("relative CLAUDE_CONFIG_DIR used: %q", got)
	}
}
