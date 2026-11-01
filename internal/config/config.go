package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the whole configuration file.
type Config struct {
	View
	Agents []string `json:"agents,omitempty"`
}

// ConfigDir finds the user's config directory; tests replace it.
var ConfigDir = os.UserConfigDir

func path() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rainbow-statusline", "config.json"), nil
}

func defaults() Config { return Config{View: View{Density: "full"}} }

// Load reads the file. It never fails: a missing or broken file gives the
// defaults, and false only when there is no file at all (the cue for the guide).
func Load() (c Config, found bool) {
	c = defaults()
	defer func() {
		if recover() != nil {
			c, found = defaults(), true
		}
	}()
	p, err := path()
	if err != nil {
		return c, false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return c, !os.IsNotExist(err)
	}
	var parsed Config
	if json.Unmarshal(raw, &parsed) != nil {
		return c, true
	}
	switch parsed.Density {
	case "full", "compact", "minimal":
	default:
		parsed.Density = "full"
	}
	return parsed, true
}

// Save writes the file atomically and keeps keys it does not know.
func Save(c Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	keep := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(raw, &keep)
	}
	mine, err := json.Marshal(c)
	if err != nil {
		return err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(mine, &known); err != nil {
		return err
	}
	for _, k := range []string{"density", "order", "hidden", "variants", "agents"} {
		delete(keep, k)
	}
	for k, v := range known {
		keep[k] = v
	}
	out, err := json.MarshalIndent(keep, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(out, '\n'))
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
