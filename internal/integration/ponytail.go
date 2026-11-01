package integration

import (
	"errors"
	"io/fs"
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
//
// On OpenCode installed means ocPlugin and no flag means its default, full.
func (p ponytail) Badge(s model.Session) (render.Badge, bool) {
	var raw string
	if s.Agent == "opencode" {
		if !ocPlugin("ponytail") {
			return render.Badge{}, false
		}
		var err error
		raw, err = readFlagIn(openCodeDir(), ".ponytail-active")
		if errors.Is(err, fs.ErrNotExist) {
			raw = "full"
		} else if err != nil {
			return render.Badge{}, false
		}
	} else {
		if !p.Detected() {
			return render.Badge{}, false
		}
		var ok bool
		if raw, ok = readFlag(".ponytail-active"); !ok {
			return render.Badge{}, false
		}
	}
	line, _, _ := strings.Cut(raw, "\n")
	mode := strings.ToLower(strings.Join(strings.Fields(line), ""))
	if mode == "off" || (mode != "" && !modePattern.MatchString(mode)) {
		return render.Badge{}, false
	}
	label := "[PONYTAIL]"
	if mode != "" && mode != "full" {
		label = "[PONYTAIL:" + strings.ToUpper(mode) + "]"
	}
	return render.Badge{Name: "PONYTAIL", Lines: []render.Segment{{Text: label, FG: ponytailColour}}}, true
}
