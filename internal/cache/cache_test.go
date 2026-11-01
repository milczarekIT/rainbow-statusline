package cache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setup(t *testing.T) *int32 {
	t.Helper()
	dir := t.TempDir()
	oldBase, oldSpawn := baseDir, spawn
	var spawns int32
	baseDir = func() (string, error) { return dir, nil }
	spawn = func(key string) error { atomic.AddInt32(&spawns, 1); return nil }
	t.Cleanup(func() { baseDir, spawn = oldBase, oldSpawn })
	return &spawns
}

func TestGetNeverBlocks(t *testing.T) {
	setup(t)
	var running sync.WaitGroup
	release := make(chan struct{})
	t.Cleanup(func() { close(release); running.Wait() })
	spawn = func(key string) error {
		running.Add(1)
		go func() { defer running.Done(); _ = Refresh(key) }()
		return nil
	}
	Register("slow", func(ctx context.Context) ([]byte, error) {
		select {
		case <-time.After(3 * time.Second):
		case <-release:
		}
		return []byte("x"), nil
	})
	start := time.Now()
	if _, ok := Get("slow", time.Minute); ok {
		t.Fatal("no value expected yet")
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("Get took %v", d)
	}
}

func TestAtomicWrite(t *testing.T) {
	setup(t)
	fail := false
	Register("k", func(ctx context.Context) ([]byte, error) {
		if fail {
			return nil, errors.New("boom")
		}
		return []byte("old"), nil
	})
	if err := Refresh("k"); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := Refresh("k"); err == nil {
		t.Fatal("expected error")
	}
	v, ok := Get("k", time.Hour)
	if !ok || string(v) != "old" {
		t.Fatalf("got %q %v", v, ok)
	}
	dir, _ := baseDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "k" {
			t.Fatalf("leftover file %s", e.Name())
		}
	}
}

func TestOneRefreshAtATime(t *testing.T) {
	spawns := setup(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); Get("shared", time.Minute) }()
	}
	wg.Wait()
	if n := atomic.LoadInt32(spawns); n != 1 {
		t.Fatalf("spawns = %d", n)
	}
}

func TestStaleLockIsTakenOver(t *testing.T) {
	spawns := setup(t)
	dir, _ := baseDir()
	lock := filepath.Join(dir, "k.lock")
	_ = os.WriteFile(lock, nil, 0o644)
	old := time.Now().Add(-time.Minute)
	_ = os.Chtimes(lock, old, old)
	Get("k", time.Minute)
	if n := atomic.LoadInt32(spawns); n != 1 {
		t.Fatalf("spawns = %d", n)
	}
}

func TestFreshValueDoesNotSpawn(t *testing.T) {
	spawns := setup(t)
	Register("f", func(ctx context.Context) ([]byte, error) { return []byte("v"), nil })
	_ = Refresh("f")
	if v, ok := Get("f", time.Hour); !ok || string(v) != "v" || *spawns != 0 {
		t.Fatalf("v=%q ok=%v spawns=%d", v, ok, *spawns)
	}
}

func TestUnsafeKeysRejected(t *testing.T) {
	spawns := setup(t)
	for _, k := range []string{"", "../x", "a/b", `a\b`, "a..b"} {
		if _, ok := Get(k, time.Minute); ok {
			t.Fatalf("%q accepted", k)
		}
		if Refresh(k) == nil {
			t.Fatalf("Refresh accepted %q", k)
		}
	}
	if *spawns != 0 {
		t.Fatal("spawned for unsafe key")
	}
}
