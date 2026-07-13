package trace

import (
	"io"
	"log/slog"
	"testing"
)

// These benchmarks cover both build-tag modes. The package compiles
// one of {trace.go (active slog.Debug), trace_nodebug.go (no-op)} per
// build tag; we observe whichever path is in effect.
//
// Run order:
//   go test -bench=. -benchtime=1s -benchmem -run='^$' ./internal/debug/trace/...
//     -> measures the no-op stub (//go:build !debug path).
//   go test -tags debug -bench=. -benchtime=1s -benchmem -run='^$' ./internal/debug/trace/...
//     -> measures the active slog.Debug body (//go:build debug path).

func BenchmarkTraceLog_AttachedDebugHandler(b *testing.B) {
	h := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log("msg", "k", "v")
	}
}

func BenchmarkTraceLog_DropsAtInfoFloor(b *testing.B) {
	// Realistic release-with-debug-build-tag-off scenario: slog floor
	// is Info, so Debug entries are dropped at the Enabled check.
	h := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log("msg", "k", "v")
	}
}

func BenchmarkTraceLog_TwoAttrs(b *testing.B) {
	h := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log("handler_start", "id", 42, "stage", "begin")
	}
}