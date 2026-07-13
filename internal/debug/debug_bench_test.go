package debug

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkRingBuffer_PushAtCapacity isolates the ring-buffer wrap path.
func BenchmarkRingBuffer_PushAtCapacity(b *testing.B) {
	rb := NewRingBuffer(500)
	// Fill to capacity so subsequent Push calls wrap.
	for i := 0; i < rb.Cap(); i++ {
		rb.Push(Entry{Time: time.Now(), Level: "DEBUG", Message: "fill"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(Entry{Time: time.Now(), Level: "DEBUG", Message: "hot"})
	}
}

// BenchmarkRingBuffer_PushEmpty isolates the empty (grow) path.
func BenchmarkRingBuffer_PushEmpty(b *testing.B) {
	rb := NewRingBuffer(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(Entry{Time: time.Now(), Level: "DEBUG", Message: "hot"})
	}
}

// BenchmarkSlogDebugDropsAtInfoFloor: slog-call overhead when the
// handler's level floor filters the entry out.
func BenchmarkSlogDebugDropsAtInfoFloor(b *testing.B) {
	h := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		slog.Debug("msg", "k", "v")
	}
}

// BenchmarkSlogDebugEmitsAtDebugFloor: full emit path (attrs flatten
// + JSON marshal + bufio write + ring push) against the project's
// real teeHandler.
func BenchmarkSlogDebugEmitsAtDebugFloor(b *testing.B) {
	dir := b.TempDir()
	logPath := filepath.Join(dir, "bench.log")
	if err := setup(Config{
		LogPath:       logPath,
		RingSize:      500,
		AppName:       "bench",
		AppVersion:    "0",
		BuildIdentity: "bench",
		Debug:         true,
	}); err != nil {
		b.Fatal(err)
	}
	// Pre-fill the global ring to capacity so we measure the wrap path.
	for i := 0; i < ringBuf.Cap(); i++ {
		ringBuf.Push(Entry{Time: time.Now(), Level: "DEBUG", Message: "fill"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		slog.Debug("msg", "k", "v")
	}
	b.StopTimer()
	// Close the file handle so TempDir cleanup can delete it.
	_ = Close()
	// Belt-and-suspenders for Windows file-lock flakiness.
	_ = os.Remove(logPath)
}