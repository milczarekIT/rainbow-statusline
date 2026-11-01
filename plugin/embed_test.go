package plugin

import (
	"strings"
	"testing"
)

func TestOpenCodeFilesHoldThePlugin(t *testing.T) {
	src, err := OpenCodeFiles.ReadFile("opencode/tui.tsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"RAINBOW_STATUSLINE_BIN", `const INSTALLED_BIN = ""`, "windowsHide: true", `"--format", "json"`, "app_bottom", "isAbsolute(", "cwd: homedir()", `includes("rtk")`, `typeof s.text === "string"`} {
		if !strings.Contains(string(src), want) {
			t.Errorf("missing %q", want)
		}
	}
}
