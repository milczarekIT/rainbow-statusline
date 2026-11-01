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
	for _, want := range []string{"RAINBOW_STATUSLINE_BIN", "windowsHide: true", `"--format", "json"`, "app_bottom"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("missing %q", want)
		}
	}
}
