package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := ConfigDir
	ConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { ConfigDir = old })
	return filepath.Join(dir, "rainbow-statusline", "config.json")
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNoFile(t *testing.T) {
	useTempDir(t)
	c, ok := Load()
	if ok {
		t.Error("want false without a file")
	}
	if c.Density != "full" {
		t.Errorf("want default density full, got %q", c.Density)
	}
}

func TestRoundTrip(t *testing.T) {
	useTempDir(t)
	want := Config{
		View:   View{Density: "compact", Order: []string{"rtk", "caveman"}, Hidden: []string{"cache"}, Variants: map[string]string{"rtk": "short"}},
		Agents: []string{"claude", "opencode"},
	}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, ok := Load()
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v ok=%v, want %+v", got, ok, want)
	}
}

func TestBrokenFileGivesDefaults(t *testing.T) {
	path := useTempDir(t)
	for _, body := range []string{"{not json", "", "[]", `{"density": 5}`, "null"} {
		write(t, path, body)
		c, _ := Load()
		if c.Density != "full" || len(c.Hidden) != 0 {
			t.Errorf("%q: want defaults, got %+v", body, c)
		}
	}
}

func TestUnknownDensityFallsBackToFull(t *testing.T) {
	path := useTempDir(t)
	write(t, path, `{"density":"huge","hidden":["rtk"]}`)
	c, ok := Load()
	if !ok || c.Density != "full" || !reflect.DeepEqual(c.Hidden, []string{"rtk"}) {
		t.Errorf("got %+v ok=%v", c, ok)
	}
}

func TestSavePreservesUnknownKeys(t *testing.T) {
	path := useTempDir(t)
	write(t, path, `{"density":"full","future":{"a":1},"hidden":["rtk"]}`)
	c, _ := Load()
	c.Density = "minimal"
	c.Hidden = nil
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var future bytes.Buffer
	_ = json.Compact(&future, m["future"])
	if future.String() != `{"a":1}` {
		t.Errorf("unknown key lost: %s", raw)
	}
	if string(m["density"]) != `"minimal"` {
		t.Errorf("density not saved: %s", raw)
	}
	if _, ok := m["hidden"]; ok {
		t.Errorf("cleared hidden still stored: %s", raw)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestItems(t *testing.T) {
	want := []string{"ponytail", "caveman", "rtk", "claude-mem", "model", "context", "output", "cost", "cache"}
	if got := Items(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}
