//go:build !debug

package trace

// Log is a no-op when compiled without -tags debug.
// The Go compiler dead-code-eliminates every call site.
func Log(msg string, attrs ...any) {}
