// Package uiver holds the UI version flag for issue #74.
//
// `?ui=v2` opt-in flag routes requests through v2-only code paths
// (initially a passthrough). Default off. Wails production builds never
// send ?ui=v2 so production behavior is unchanged. The flag exists so
// every Phase 1/2 component-primitive refactor in issue #74 can ship
// behind a kill switch without forcing a binary rollback.
//
// Lives in its own leaf package so templates can import it without
// pulling in appshell (which would create an import cycle: appshell
// already imports templates for the rendered views).
package uiver

import (
	"context"
	"net/http"
)

// Key is the unexported context key for the UI version flag.
// Stored as a typed empty struct to avoid collisions with other packages'
// context keys.
type Key struct{}

// IsV2 reports whether the current request context opted into the v2 UI.
// Returns false (v1) for any context that did not flow through Middleware.
func IsV2(ctx context.Context) bool {
	v, _ := ctx.Value(Key{}).(bool)
	return v
}

// Middleware reads `?ui=v2` from the query string and stores the boolean
// on the request context via IsV2. Anything other than the literal `v2`
// value is treated as v1.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v2 := r.URL.Query().Get("ui") == "v2"
		ctx := context.WithValue(r.Context(), Key{}, v2)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// DebugKey is the unexported context key for the debug-mode flag.
// Separate type from Key so callers can't confuse v2-UI opt-in with
// debug mode.
type DebugKey struct{}

// WithDebugMode returns a child context carrying the debug-mode flag.
func WithDebugMode(ctx context.Context, enabled bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, DebugKey{}, enabled)
}

// IsDebugMode reports whether the current request context had
// debug-mode enabled by the appshell wiring.
func IsDebugMode(ctx context.Context) bool {
	v, _ := ctx.Value(DebugKey{}).(bool)
	return v
}