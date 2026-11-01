// Package cache keeps slow numbers on disk so drawing never waits for them.
package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	refreshLimit = 20 * time.Second
	staleLock    = 30 * time.Second
)

var (
	mu       sync.Mutex
	registry = map[string]func(ctx context.Context) ([]byte, error){}

	// baseDir and spawn are replaced in tests.
	baseDir = defaultDir
	spawn   = spawnDetached
)

func defaultDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "rainbow-statusline"), nil
}

// Register tells the refresh subcommand how to compute the value for key.
func Register(key string, fn func(ctx context.Context) ([]byte, error)) {
	mu.Lock()
	defer mu.Unlock()
	registry[key] = fn
}

// validKey keeps keys usable as file names: no separators, no "..".
func validKey(key string) bool {
	return key != "" && !strings.Contains(key, "..") && !strings.ContainsAny(key, `/\:`+"\x00")
}

func path(key string) (string, error) {
	if !validKey(key) {
		return "", errors.New("cache: invalid key")
	}
	dir, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, key), nil
}

// Get returns the last stored value. When it is missing or older than maxAge,
// it starts one detached refresh and still returns immediately.
func Get(key string, maxAge time.Duration) ([]byte, bool) {
	p, err := path(key)
	if err != nil {
		return nil, false
	}
	value, readErr := os.ReadFile(p)
	info, statErr := os.Stat(p)
	fresh := readErr == nil && statErr == nil && time.Since(info.ModTime()) <= maxAge
	if !fresh && takeLock(p+".lock") {
		if spawn(key) != nil {
			_ = os.Remove(p + ".lock")
		}
	}
	if readErr != nil {
		return nil, false
	}
	return value, true
}

// takeLock creates the lock file; a lock older than staleLock is taken over.
func takeLock(lock string) bool {
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return false
	}
	for range 2 {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_ = f.Close()
			return true
		}
		info, statErr := os.Stat(lock)
		if statErr != nil || time.Since(info.ModTime()) <= staleLock {
			if statErr == nil {
				return false
			}
			continue
		}
		_ = os.Remove(lock)
	}
	return false
}

// Refresh computes the value for key and stores it atomically. A failure keeps
// the previous value. It always releases the lock.
func Refresh(key string) error {
	p, err := path(key)
	if err != nil {
		return err
	}
	defer os.Remove(p + ".lock")

	mu.Lock()
	fn := registry[key]
	mu.Unlock()
	if fn == nil {
		return errors.New("cache: key not registered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), refreshLimit)
	defer cancel()
	value, err := fn(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), key+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
