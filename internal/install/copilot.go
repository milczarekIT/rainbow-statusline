package install

import (
	"os"
	"path/filepath"
)

func defaultCopilotDir() string {
	if d := os.Getenv("COPILOT_HOME"); filepath.IsAbs(d) {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".copilot")
}

// prepareCopilot edits settings.json, which is strict JSON. Next to it,
// config.json has comments and Copilot manages it, so it is never touched.
func prepareCopilot(bin string, st *state) ([]action, error) {
	return prepareStatusLine("copilot", filepath.Join(defaultCopilotDir(), "settings.json"), bin, st)
}
