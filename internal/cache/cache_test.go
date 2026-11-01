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

func TestFailedRefreshBacksOff(t *testing.T) {
	spawns := setup(t)
	Register("bad", func(ctx context.Context) ([]byte, error) { return nil, errors.New("boom") })
	Get("bad", time.Minute)
	_ = Refresh("bad")
	Get("bad", time.Minute)
	if n := atomic.LoadInt32(spawns); n != 1 {
		t.Fatalf("spawns after failure = %d", n)
	}
	dir, _ := baseDir()
	lock := filepath.Join(dir, "bad.lock")
	old := time.Now().Add(-time.Minute)
	_ = os.Chtimes(lock, old, old)
	Get("bad", time.Minute)
	if n := atomic.LoadInt32(spawns); n != 2 {
		t.Fatalf("spawns after stale lock = %d", n)
	}
}

func TestUnregisteredKeyKeepsLock(t *testing.T) {
	spawns := setup(t)
	Get("ghost", time.Minute)
	_ = Refresh("ghost")
	Get("ghost", time.Minute)
	if n := atomic.LoadInt32(spawns); n != 1 {
		t.Fatalf("spawns = %d", n)
	}
}

func TestSuccessReleasesLock(t *testing.T) {
	setup(t)
	Register("ok", func(ctx context.Context) ([]byte, error) { return []byte("v"), nil })
	Get("ok", time.Minute)
	_ = Refresh("ok")
	dir, _ := baseDir()
	if _, err := os.Stat(filepath.Join(dir, "ok.lock")); !os.IsNotExist(err) {
		t.Fatal("lock left after success")
	}
}

func TestEmptyResultKeepsPrevious(t *testing.T) {
	setup(t)
	val := []byte("old")
	Register("e", func(ctx context.Context) ([]byte, error) { return val, nil })
	_ = Refresh("e")
	val = nil
	if Refresh("e") == nil {
		t.Fatal("empty result accepted")
	}
	if v, ok := Get("e", time.Hour); !ok || string(v) != "old" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestValueOlderThanMaxStaleIsHidden(t *testing.T) {
	spawns := setup(t)
	Register("s", func(ctx context.Context) ([]byte, error) { return []byte("v"), nil })
	_ = Refresh("s")
	dir, _ := baseDir()
	p := filepath.Join(dir, "s")
	set := func(age time.Duration) {
		ts := time.Now().Add(-age)
		_ = os.Chtimes(p, ts, ts)
	}
	set(maxStale - time.Hour)
	if v, ok := Get("s", time.Minute); !ok || string(v) != "v" {
		t.Fatalf("within limit: %q %v", v, ok)
	}
	set(maxStale + time.Hour)
	_ = os.Remove(p + ".lock")
	before := atomic.LoadInt32(spawns)
	if _, ok := Get("s", time.Minute); ok {
		t.Fatal("value past maxStale returned")
	}
	if atomic.LoadInt32(spawns) != before+1 {
		t.Fatal("no refresh for hidden value")
	}
}

func TestRenameRetries(t *testing.T) {
	oldRename, oldDelay := rename, renameDelay
	t.Cleanup(func() { rename, renameDelay = oldRename, oldDelay })
	renameDelay = 0
	calls := 0
	rename = func(a, b string) error {
		calls++
		if calls < 3 {
			return errors.New("access denied")
		}
		return nil
	}
	if err := renameWithRetry("a", "b"); err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	calls = 0
	rename = func(a, b string) error { calls++; return errors.New("denied") }
	if renameWithRetry("a", "b") == nil || calls != 5 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestFailedRenameRemovesTemp(t *testing.T) {
	setup(t)
	oldRename, oldDelay := rename, renameDelay
	t.Cleanup(func() { rename, renameDelay = oldRename, oldDelay })
	renameDelay = 0
	rename = func(a, b string) error { return errors.New("denied") }
	Register("r", func(ctx context.Context) ([]byte, error) { return []byte("v"), nil })
	if Refresh("r") == nil {
		t.Fatal("expected error")
	}
	dir, _ := baseDir()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestPrefixKeyIsRefreshed(t *testing.T) {
	setup(t)
	RegisterPrefix("sess-", func(ctx context.Context, key string) ([]byte, error) {
		return []byte("got " + key), nil
	})
	if err := Refresh("sess-abc"); err != nil {
		t.Fatal(err)
	}
	if v, ok := Get("sess-abc", time.Minute); !ok || string(v) != "got sess-abc" {
		t.Fatalf("got %q %v", v, ok)
	}
}
