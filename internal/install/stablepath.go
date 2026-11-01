package install

import (
	"os"
	"os/exec"
	"strings"
)

// Replaced in tests.
var (
	lookPath   = exec.LookPath
	sameFile   = func(a, b string) bool { return fileIdentity(a, b) }
	fileExists = func(p string) bool { _, err := os.Stat(p); return err == nil }
)

func fileIdentity(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// stableExecutable is the path written into the assistants' settings. A package
// manager may keep the real file in a folder that changes on every upgrade
// (Homebrew's Cellar) or behind a shim (Scoop), so the path a person types is
// preferred when it leads to the same file.
func stableExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return stablePath(exe), nil
}

func stablePath(exe string) string {
	if goos == "windows" {
		if i := strings.Index(strings.ToLower(exe), `\apps\`); i >= 0 {
			shim := exe[:i] + `\shims` + exe[strings.LastIndex(exe, `\`):]
			if fileExists(shim) {
				return shim
			}
		}
	}
	name := exe[strings.LastIndexAny(exe, `/\`)+1:]
	if found, err := lookPath(name); err == nil && sameFile(found, exe) {
		return found
	}
	return exe
}
