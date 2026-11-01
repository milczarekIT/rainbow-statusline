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

// Path is where the configuration file lives.
func Path() (string, error) { return path() }

func defaults() Config { return Config{View: View{Density: "full"}} }

// Load reads the file. It never fails: a missing or broken file gives the
// defaults, and false only when there is no file at all (the cue for the guide).
func Load() (Config, bool) {
	c := defaults()
	p, err := path()
	if err != nil {
		return c, false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return c, !os.IsNotExist(err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return c, true
	}
	var density string
	if json.Unmarshal(fields["density"], &density) == nil {
		switch density {
		case "full", "compact", "minimal":
			c.Density = density
		}
	}
	_ = json.Unmarshal(fields["order"], &c.Order)
	_ = json.Unmarshal(fields["hidden"], &c.Hidden)
	_ = json.Unmarshal(fields["variants"], &c.Variants)
	_ = json.Unmarshal(fields["agents"], &c.Agents)
	return c, true
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
