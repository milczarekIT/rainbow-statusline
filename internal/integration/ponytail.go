package integration

import (
	"strings"

	"github.com/milczarekIT/rainbow-statusline/internal/model"
	"github.com/milczarekIT/rainbow-statusline/internal/render"
)

const (
	ponytailPlugin = "ponytail@ponytail"
	ponytailColour = "38;5;108"
)

type ponytail struct{}

func (ponytail) ID() string { return "ponytail" }

func (ponytail) Detected() bool { return pluginActive(ponytailPlugin) }

// Badge is the plugin's own: [PONYTAIL] for full or an empty flag, [PONYTAIL:MODE]
// otherwise, hidden when the flag is missing, "off" or not a plain mode word.
func (p ponytail) Badge(model.Session) (render.Badge, bool) {
	if !p.Detected() {
		return render.Badge{}, false
	}
	raw, ok := readFlag(".ponytail-active")
	if !ok {
		return render.Badge{}, false
	}
	line, _, _ := strings.Cut(raw, "\n")
	mode := strings.Join(strings.Fields(line), "")
	if mode == "off" || !plainMode(mode) {
		return render.Badge{}, false
	}
	label := "[PONYTAIL]"
	if mode != "" && mode != "full" {
		label = "[PONYTAIL:" + strings.ToUpper(mode) + "]"
	}
	return render.Badge{Name: "PONYTAIL", Lines: []render.Segment{{Text: label, FG: ponytailColour}}}, true
}

// plainMode accepts letters, digits, "-" and "_": a mode word, never markup.
func plainMode(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
