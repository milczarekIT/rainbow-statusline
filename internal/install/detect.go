package install

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tidwall/gjson"
)

// Agent is one assistant as found on this computer.
type Agent struct {
	ID, Name                        string
	Found                           bool
	SettingsPath, CurrentStatusLine string
}

type definition struct {
	id, name, command string
	dir               func() string
	settings          string
	prepare           func(bin string, st *state) ([]action, error)
}

func definitions() []definition {
	return []definition{
		{"claude", "Claude Code", "claude", defaultClaudeDir, "settings.json", prepareClaude},
		{"copilot", "Copilot CLI", "copilot", defaultCopilotDir, "settings.json", prepareCopilot},
		{"opencode", "OpenCode", "opencode", defaultOpenCodeDir, "tui.json", prepareOpenCode},
	}
}

func (d definition) found() bool {
	if info, err := os.Stat(d.dir()); err == nil && info.IsDir() {
		return true
	}
	_, err := exec.LookPath(d.command)
	return err == nil
}

// Detect lists the assistants and what each one draws now. It only reads.
func Detect() []Agent {
	var out []Agent
	for _, d := range definitions() {
		a := Agent{ID: d.id, Name: d.name, Found: d.found(), SettingsPath: filepath.Join(d.dir(), d.settings)}
		if raw, err := os.ReadFile(a.SettingsPath); err == nil {
			a.CurrentStatusLine = statusLineText(gjson.GetBytes(raw, "statusLine"))
		}
		out = append(out, a)
	}
	return out
}
