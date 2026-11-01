package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const sample = `{
  "session_id": "0b1c2d3e-aaaa-bbbb-cccc-1234567890ab",
  "transcript_path": "/Users/jdoe/.claude/projects/-Users-jdoe-dev-app/abc.jsonl",
  "cwd": "/Users/jdoe/dev/app",
  "user": {"name": "jdoe", "email": "jdoe@example.com"},
  "model": {"id": "claude-opus-4-7", "display_name": "Opus 4.7"},
  "message": "please delete my secret file",
  "context_window": {"used_percentage": 42.5, "total_input_tokens": 1234},
  "cost": {"total_cost_usd": 0.0123},
  "tags": ["one", "two"],
  "flag": true,
  "nothing": null
}`

func TestScrubRemovesPrivateStrings(t *testing.T) {
	out, err := scrub([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"jdoe", "example.com", "/Users", "secret file", "0b1c2d3e", "one", "two"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("output still contains %q:\n%s", leak, out)
		}
	}
}

func TestScrubKeepsKeysNumbersAndAllowlist(t *testing.T) {
	out, err := scrub([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	var in, got map[string]any
	if err := json.Unmarshal([]byte(sample), &in); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keysAndNumbers(in), keysAndNumbers(got)) {
		t.Errorf("shape differs:\n in: %v\nout: %v", keysAndNumbers(in), keysAndNumbers(got))
	}
	model := got["model"].(map[string]any)
	if model["id"] != "claude-opus-4-7" || model["display_name"] != "Opus 4.7" {
		t.Errorf("allowlisted model values changed: %v", model)
	}
}

func TestScrubAllowlistedValueWithPrivateDataIsStillScrubbed(t *testing.T) {
	out, err := scrub([]byte(`{"model":{"id":"/Users/jdoe/x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "jdoe") {
		t.Errorf("leak: %s", out)
	}
}

func TestScrubRejectsInvalidJSON(t *testing.T) {
	if _, err := scrub([]byte(`{nope`)); err == nil {
		t.Error("expected error")
	}
}

// keysAndNumbers flattens v into path -> number, path -> kind for non-strings, path -> "string" for strings.
func keysAndNumbers(v any) map[string]any {
	m := map[string]any{}
	var walk func(p string, v any)
	walk = func(p string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				walk(p+"."+k, c)
			}
		case []any:
			for i, c := range x {
				walk(p+"["+string(rune('0'+i))+"]", c)
			}
		case string:
			m[p] = "string"
		default:
			m[p] = x
		}
	}
	walk("", v)
	return m
}
