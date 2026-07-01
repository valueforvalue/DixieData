//go:build debug

// Package trace provides a zero-cost Log function that emits
// debug-level slog entries only when compiled with -tags debug.
// In release builds the entire call tree is dead-code eliminated.
//
// Usage:
//
//	import "github.com/valueforvalue/DixieData/internal/debug/trace"
//
//	func someHandler() {
//	    trace.Log("handler_start", "param", val)
//	    // ...
//	    trace.Log("handler_done", "duration_ms", elapsed)
//	}
//
// Output flows through the existing debug.Configure() handler
// (JSONL file + ring buffer + stderr mirror). trace.Log calls
// are slog.Debug level — they are silent unless DIXIEDATA_DEBUG=1
// or SetDebugMode(true) is active.
//
// Do not call trace.Log before debug.Configure() — the default
// slog handler (stderr-only) will receive the entry but the
// file sink and ring buffer won't capture it.
package trace

import "log/slog"

// Log emits a debug-level trace entry. Compilation gated by
// //go:build debug — release builds use the no-op stub in
// trace_nodebug.go instead.
func Log(msg string, attrs ...any) {
	slog.Debug(msg, attrs...)
}
