package debug

import "context"

// debugCtxKey is the unexported context key for the per-request
// debug-mode flag. Stored as a typed empty struct to avoid collisions
// with other packages' context keys.
type debugCtxKey struct{}

// WithDebugMode returns a child context carrying the per-request
// debug-mode flag. The appshell sets this in ServeHTTP via
// debug.WithDebugMode(r.Context(), a.debugMode.Load()) so templates
// can read it without needing the App struct.
func WithDebugMode(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, debugCtxKey{}, enabled)
}

// IsDebugMode reports whether the current request context had
// debug-mode enabled. Returns false when the context did not flow
// through ServeHTTP (e.g. raw template tests, prerender paths).
func IsDebugMode(ctx context.Context) bool {
	v, _ := ctx.Value(debugCtxKey{}).(bool)
	return v
}