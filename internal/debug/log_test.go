package debug

import (
	"bufio"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRingBuffer_PushAndSnapshot(t *testing.T) {
	rb := NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		rb.Push(Entry{Message: "m"})
	}
	snap := rb.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap))
	}
}

func TestRingBuffer_Since(t *testing.T) {
	rb := NewRingBuffer(10)
	for i := 0; i < 5; i++ {
		rb.Push(Entry{Message: "m"})
	}
	prev := rb.Total()
	rb.Push(Entry{Message: "new"})
	got := rb.Since(prev)
	if len(got) != 1 || got[0].Message != "new" {
		t.Errorf("Since() = %+v, want 1 new entry", got)
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Push(Entry{Message: "m"})
	rb.Push(Entry{Message: "n"})
	rb.Clear()
	if rb.Len() != 0 {
		t.Errorf("after Clear, Len = %d, want 0", rb.Len())
	}
}

func TestRingBuffer_ConcurrentSafe(t *testing.T) {
	rb := NewRingBuffer(100)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				rb.Push(Entry{Message: "x"})
			}
		}()
	}
	wg.Wait()
	if rb.Len() != 100 {
		t.Errorf("Len after 500 pushes into cap=100 = %d, want 100", rb.Len())
	}
}

// resetForTest clears package globals so each test gets a clean state.
func resetForTest(t *testing.T) {
	t.Helper()
	configureOnce = sync.Once{}
	mu = sync.RWMutex{}
	handler = nil
	logFile = nil
	bufWriter = nil
	ringBuf = nil
	sinksMu = sync.RWMutex{}
	sinks = nil
	debugMode.Store(false)
	t.Cleanup(func() {
		_ = Close()
		configureOnce = sync.Once{}
	})
}

func TestConfigure_WritesJSONL(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")

	if err := Configure(Config{LogPath: path, RingSize: 5, Debug: false}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	slog.Info("hello", "key", "value")
	if err := Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	_ = Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := splitLines(string(data))
	if len(lines) < 2 {
		t.Fatalf("want >=2 lines, got %d: %q", len(lines), data)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &obj); err != nil {
		t.Fatalf("last line not JSON: %v: %q", err, lines[len(lines)-1])
	}
	if obj["schema_version"] != float64(1) {
		t.Errorf("schema_version = %v, want 1", obj["schema_version"])
	}
	if obj["msg"] != "hello" || obj["key"] != "value" || obj["level"] != "INFO" {
		t.Errorf("log line wrong: %+v", obj)
	}
}

func TestConfigure_ComponentRoundTrip(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")
	if err := Configure(Config{LogPath: path, RingSize: 10}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	slog.Info("with component", "component", "test")
	if err := Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	rb := GetRingBuffer()
	if rb == nil || rb.Len() == 0 {
		t.Fatal("ring buffer empty after log")
	}
	last := rb.Snapshot()[rb.Len()-1]
	if last.Component != "test" {
		t.Errorf("Entry.Component = %q, want test", last.Component)
	}
	_ = Close()
}

func TestConfigure_DebugModeFromEnv(t *testing.T) {
	t.Setenv("DIXIEDATA_DEBUG", "1")
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")

	if err := Configure(Config{LogPath: path, RingSize: 5}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !IsDebugMode() {
		t.Errorf("IsDebugMode() with DIXIEDATA_DEBUG=1 = false, want true")
	}
	_ = Close()
}

func TestSetDebugMode_TogglesLevel(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")

	if err := Configure(Config{LogPath: path, RingSize: 5, Debug: false}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if IsDebugMode() {
		t.Fatal("expected IsDebugMode false initially")
	}
	SetDebugMode(true)
	if !IsDebugMode() {
		t.Fatal("SetDebugMode(true) did not flip flag")
	}
	SetDebugMode(false)
	if IsDebugMode() {
		t.Fatal("SetDebugMode(false) did not flip flag")
	}
	_ = Close()
}

// fakeSink records every entry it receives; used to verify the fanout
// reaches all registered sinks.
type fakeSink struct {
	mu      sync.Mutex
	entries []Entry
	failN   int // fail the first N writes
	calls   int
}

func (f *fakeSink) Write(e Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failN > 0 {
		f.failN--
		return errors.New("synthetic failure")
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeSink) Close() error { return nil }

func TestRegisterSink_Fanout(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")
	if err := Configure(Config{LogPath: path, RingSize: 10}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Configure already registered the default sink; add a fake one.
	fake := &fakeSink{}
	RegisterSink(fake)
	t.Cleanup(func() { _ = UnregisterSink(fake) })

	slog.Info("hello fanout")
	if err := Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	_ = Close()

	if fake.calls < 1 {
		t.Fatalf("fakeSink.calls = %d, want >=1", fake.calls)
	}
	var got Entry
	for _, e := range fake.entries {
		if e.Message == "hello fanout" {
			got = e
			break
		}
	}
	if got.Message != "hello fanout" {
		t.Fatalf("fakeSink did not receive entry; entries=%+v", fake.entries)
	}
}

func TestRegisterSink_Dedup(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log.jsonl")
	if err := Configure(Config{LogPath: path, RingSize: 5}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	fake := &fakeSink{}
	RegisterSink(fake)
	RegisterSink(fake) // duplicate, should be no-op
	count := len(sinks)
	if count != 2 { // default sink + fake
		t.Errorf("sinks len = %d, want 2", count)
	}
	_ = UnregisterSink(fake)
	_ = Close()
}

func TestUnregisterSink_Unknown(t *testing.T) {
	resetForTest(t)
	if err := UnregisterSink(&fakeSink{}); err == nil {
		t.Error("UnregisterSink on unknown sink should return error")
	}
}

func splitLines(s string) []string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}