package install

import (
	"errors"
	"testing"
)

func stubPath(t *testing.T, os string, look map[string]string, same func(a, b string) bool, exists ...string) {
	t.Helper()
	oldGoos, oldLook, oldSame, oldExists := goos, lookPath, sameFile, fileExists
	t.Cleanup(func() { goos, lookPath, sameFile, fileExists = oldGoos, oldLook, oldSame, oldExists })
	goos = os
	lookPath = func(name string) (string, error) {
		if p, ok := look[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
	sameFile = same
	fileExists = func(p string) bool {
		for _, e := range exists {
			if e == p {
				return true
			}
		}
		return false
	}
}

func always(a, b string) bool { return true }
func never(a, b string) bool  { return false }

func TestStablePathKeepsHomebrewLinkInsteadOfCellar(t *testing.T) {
	stubPath(t, "darwin", map[string]string{"rainbow-statusline": "/opt/homebrew/bin/rainbow-statusline"}, always)
	got := stablePath("/opt/homebrew/Cellar/rainbow-statusline/1.0.0/bin/rainbow-statusline")
	if got != "/opt/homebrew/bin/rainbow-statusline" {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathKeepsExecutableWhenLookupIsAnotherFile(t *testing.T) {
	stubPath(t, "darwin", map[string]string{"rainbow-statusline": "/usr/local/bin/rainbow-statusline"}, never)
	exe := "/home/me/build/rainbow-statusline"
	if got := stablePath(exe); got != exe {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathKeepsExecutableWhenNotOnPath(t *testing.T) {
	stubPath(t, "darwin", nil, always)
	exe := "/home/me/.local/bin/rainbow-statusline"
	if got := stablePath(exe); got != exe {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathPrefersScoopShim(t *testing.T) {
	shim := `C:\Users\me\scoop\shims\rainbow-statusline.exe`
	stubPath(t, "windows", nil, never, shim)
	got := stablePath(`C:\Users\me\scoop\apps\rainbow-statusline\1.0.0\rainbow-statusline.exe`)
	if got != shim {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathIgnoresMissingScoopShim(t *testing.T) {
	stubPath(t, "windows", nil, never)
	exe := `C:\Users\me\scoop\apps\rainbow-statusline\1.0.0\rainbow-statusline.exe`
	if got := stablePath(exe); got != exe {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathUsesPathLookupOnWindows(t *testing.T) {
	stubPath(t, "windows", map[string]string{`rainbow-statusline.exe`: `C:\Tools\rainbow-statusline.exe`}, always)
	got := stablePath(`C:\Tools\..\Tools\rainbow-statusline.exe`)
	if got != `C:\Tools\rainbow-statusline.exe` {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathScoopLayoutIsCaseInsensitiveAndNonASCII(t *testing.T) {
	shim := `D:\Użytkownik İ\Scoop\shims\rainbow-statusline.exe`
	stubPath(t, "windows", nil, never, shim)
	got := stablePath(`D:\Użytkownik İ\Scoop\APPS\Rainbow-Statusline\1.0.0\rainbow-statusline.exe`)
	if got != shim {
		t.Fatalf("got %q", got)
	}
}

func TestStablePathScoopShimNeedsTheRealLayout(t *testing.T) {
	for _, exe := range []string{
		`C:\tools\apps\rainbow-statusline.exe`,
		`C:\Users\me\apps\other\1.0.0\rainbow-statusline.exe`,
		`C:\Users\me\apps\rainbow-statusline\1.0.0\other.exe`,
		`C:\Users\me\apps\rainbow-statusline\1.0.0\bin\rainbow-statusline.exe`,
		`\apps\rainbow-statusline\1.0.0\rainbow-statusline.exe`,
	} {
		stubPath(t, "windows", nil, never, `\shims\rainbow-statusline.exe`, `C:\Users\me\shims\rainbow-statusline.exe`, `C:\tools\shims\rainbow-statusline.exe`)
		if got := stablePath(exe); got != exe {
			t.Errorf("%s: got %q", exe, got)
		}
	}
}
