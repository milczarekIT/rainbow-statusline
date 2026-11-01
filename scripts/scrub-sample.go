// scrub-sample reads a JSON sample on stdin (or from the file given as the first
// argument) and writes it to stdout, or to the file given as the second argument,
// with every string replaced by "redacted" except values at allowlisted key paths.
// Keys and non-string values are kept.
//
//	go run scripts/scrub-sample.go raw.json > testdata/claude/full.json
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const redacted = "redacted"

// allowlist holds key-path suffixes whose string values are kept (model names,
// plugin ids, status enums). A path matches when it ends with the entry on a
// key boundary, so "model.id" matches "model.id" and "x.model.id" but not "session_id".
var allowlist = []string{
	"model.id",
	"model.display_name",
	"effort.level",
	"output_style.name",
	"version",
	"prompt_cache.ttl",
	"prompt_cache.last_miss_cause",
	"model.providerID",
	"model.variant",
	"role",
	"mode",
	"agent",
	"finish",
	"type",
	"modelID",
	"providerID",
}

var timestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[\d:.]+(Z|[+-]\d{2}:\d{2})$`)

func allowed(path string) bool {
	for _, a := range allowlist {
		if path == a || strings.HasSuffix(path, "."+a) {
			return true
		}
	}
	return false
}

// private reports whether a value that would otherwise be kept looks personal.
func private(s string) bool {
	if strings.ContainsAny(s, `/\@`) {
		return true
	}
	for _, p := range privateTokens() {
		if strings.Contains(strings.ToLower(s), p) {
			return true
		}
	}
	return false
}

func privateTokens() []string {
	var out []string
	for _, v := range []string{os.Getenv("USER"), os.Getenv("USERNAME")} {
		if len(v) >= 3 {
			out = append(out, strings.ToLower(v))
		}
	}
	if h, err := os.Hostname(); err == nil && len(h) >= 3 {
		out = append(out, strings.ToLower(strings.SplitN(h, ".", 2)[0]))
	}
	return out
}

func walk(path string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			p := k
			if path != "" {
				p = path + "." + k
			}
			x[k] = walk(p, c)
		}
		return x
	case []any:
		for i, c := range x {
			x[i] = walk(path+"[]", c)
		}
		return x
	case string:
		if (allowed(path) || timestamp.MatchString(x)) && !private(x) {
			return x
		}
		return redacted
	default:
		return v
	}
}

// scrub returns the sample with private strings replaced.
func scrub(in []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(in))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(walk("", v), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func main() {
	var in []byte
	var err error
	if len(os.Args) > 1 {
		in, err = os.ReadFile(os.Args[1])
	} else {
		in, err = io.ReadAll(os.Stdin)
	}
	if err == nil {
		var out []byte
		if out, err = scrub(in); err == nil {
			if len(os.Args) > 2 {
				err = os.WriteFile(os.Args[2], out, 0o644)
			} else {
				_, err = os.Stdout.Write(out)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "scrub-sample:", err)
		os.Exit(1)
	}
}
