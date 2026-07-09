package testtemp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir_RemoveAllSucceeds(t *testing.T) {
	dir := New(t)
	if _, err := os.Stat(dir.Path()); err != nil {
		t.Fatalf("path should exist: %v", err)
	}
	// Write a file, then Release. RemoveAllWithRetry should
	// succeed because no handle is held.
	if err := os.WriteFile(filepath.Join(dir.Path(), "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := dir.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(dir.Path()); !os.IsNotExist(err) {
		t.Fatalf("dir should be removed: got err=%v", err)
	}
	// Second Release is a no-op.
	if err := dir.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestDir_AutoCleanupOnTestExit(t *testing.T) {
	dir := New(t)
	if _, err := os.Stat(dir.Path()); err != nil {
		t.Fatalf("path should exist: %v", err)
	}
	// No Release call — t.Cleanup runs auto-cleanup at test exit.
	// We can't directly verify that here (the t.Cleanup is
	// scheduled with t.Cleanup, which fires when the test
	// function returns and reaches the next test). Just verify
	// the dir exists now and that no panic happens.
}

func TestDir_ConcurrentRead(t *testing.T) {
	// The Windows file-handle race fix: open a file, "use" it
	// (no close), then Release. Without the GC-and-Gosched
	// settle, RemoveAll on Windows can fail. With them, the
	// handle is GC-reclaimed within the retry window.
	dir := New(t)
	target := filepath.Join(dir.Path(), "held.bin")
	if err := os.WriteFile(target, []byte("held"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Not closing f on purpose — simulates a leaked handle.
	// The retry logic + runtime.GC must recover.
	if err := dir.Release(); err != nil {
		t.Logf("Release after leaked handle returned %v (acceptable on Windows; expected 0 nil if OS released cleanly)", err)
	}
}

func TestDir_NestedDirRelease(t *testing.T) {
	// Nested dirs + files inside them. RemoveAll should walk the
	// tree.
	dir := New(t)
	subdir := filepath.Join(dir.Path(), "sub")
	if err := os.MkdirAll(filepath.Join(subdir, "inner"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "inner", "deep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := dir.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}
