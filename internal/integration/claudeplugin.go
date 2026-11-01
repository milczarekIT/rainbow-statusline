package integration

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// claudeDir is where Claude Code keeps its settings, plugins and mode flags.
// Replaced in tests.
var claudeDir = defaultClaudeDir

// defaultClaudeDir honours CLAUDE_CONFIG_DIR like the plugins do. A relative
// value is ignored: a project folder could supply the flag files behind it.
func defaultClaudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); filepath.IsAbs(d) {
		return d
	}
	h, _ := homeDir()
	return filepath.Join(h, ".claude")
}

// pluginActive reports whether Claude Code has the plugin enabled and an
// install of it on disk.
func pluginActive(id string) bool {
	raw, err := os.ReadFile(claudeSettings())
	if err != nil {
		return false
	}
	var cfg struct {
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	if json.Unmarshal(raw, &cfg) != nil || !cfg.Enabled[id] {
		return false
	}
	raw, err = os.ReadFile(filepath.Join(claudeDir(), "plugins", "installed_plugins.json"))
	if err != nil {
		return false
	}
	var installed struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
		} `json:"plugins"`
	}
	if json.Unmarshal(raw, &installed) != nil {
		return false
	}
	for _, p := range installed.Plugins[id] {
		if info, err := os.Stat(p.InstallPath); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

const flagBytes = 16

// readFlag reads the start of a plugin's mode flag. A symlink or anything but
// a regular file is refused, so a link cannot make the bar print another file.
func readFlag(name string) (string, bool) {
	raw, err := readFlagIn(claudeDir(), name)
	return raw, err == nil
}

func readFlagIn(dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	info, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("flag: not a regular file")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, flagBytes))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
