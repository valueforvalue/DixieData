package templates

import (
	"context"
	"encoding/json"

	"github.com/valueforvalue/DixieData/internal/config"
)

// pagePathCtxKey + layoutHasOpenReviewCtxKey are the unexported
// context keys for the per-render page path + open-review flag.
// Issue #466: the prior design used package-level globals that
// raced under any concurrent ServeHTTP (the race-stress workflow
// exposes this with httptest.NewServer + a second request's defer
// still clearing state from the first). The Wails production path
// is single-window and serialized, but the test path isn't, so
// the package globals were a latent bug that -race surfaced as
// both a data race AND a 70x perf regression (the race detector
// slows contended writes by ~10-100x). Hoisting both values into
// context.Context follows the same pattern as debug.WithDebugMode
// in internal/debug/uictx.go: the appshell tags the request
// context in ServeHTTP, handlers + Layout read from it. No more
// Set/Clear brackets, no more global mutable state, no more
// race, no more race-detector slowdown on the GET path.
type pagePathCtxKey struct{}

type layoutHasOpenReviewCtxKey struct{}

// WithPagePath returns a child context carrying the per-render
// page path. The appshell tags this in ServeHTTP before the mux
// dispatches so Layout's breadcrumb + dev badge can render the
// current page without every page's templ signature growing a
// currentPath parameter (the issue #309 cost-cut). The helper
// returns a fresh ctx; pair with WithLayoutHasOpenReview when
// both flags need to land on the same request.
func WithPagePath(ctx context.Context, path string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, pagePathCtxKey{}, path)
}

// PagePathFromContext reports the page path tagged onto ctx via
// WithPagePath. Returns the empty string when no tag is present
// (e.g. raw template tests that render Layout directly without
// the appshell wrapper), which matches the prior package-global
// default state.
func PagePathFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(pagePathCtxKey{}).(string)
	return v
}

// WithLayoutHasOpenReview tags ctx with the per-render flag that
// drives the red review-state treatment on the Research & Review
// foldout's "Open Review Queue" menuitem (issue #460 follow-up).
// The appshell queries CountNeedsReview once per request and
// tags the result here so the treatment lands on the FIRST paint
// of every page that has pending review items, not only on
// /review-queue itself. Pages that know they are NOT a review-
// queue surface don't override the flag — the default (false)
// keeps the menuitem neutral.
func WithLayoutHasOpenReview(ctx context.Context, hasOpenReview bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, layoutHasOpenReviewCtxKey{}, hasOpenReview)
}

// LayoutHasOpenReviewFromContext reports the open-review flag
// tagged onto ctx via WithLayoutHasOpenReview. Returns false when
// no tag is present, matching the prior package-global default.
func LayoutHasOpenReviewFromContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(layoutHasOpenReviewCtxKey{}).(bool)
	return v
}

// layoutCurrentPath is the templ-callable helper Layout uses to
// render the current page path into the data-dixie-page attribute
// + breadcrumb + dev badge. Issue #466: takes ctx (read from the
// per-request tag) instead of a package global, so concurrent
// ServeHTTP calls don't race.
func layoutCurrentPath(ctx context.Context) string {
	return PagePathFromContext(ctx)
}

// layoutHasOpenReview mirrors layoutCurrentPath for the red
// treatment flag. Same templ-callable contract.
func layoutHasOpenReview(ctx context.Context) bool {
	return LayoutHasOpenReviewFromContext(ctx)
}

// ThemeFromContext is the templ-callable helper Layout uses to
// render the <html data-theme="..."> attribute. The appshell
// tags the resolved theme name (one of records.ThemeDefault /
// ThemeHighContrast / ThemeSoft) in ServeHTTP so the first
// paint of every page carries the right theme without a
// flash of default. Returns the default theme when no tag is
// present (raw template tests, error pages rendered before
// the request hook ran, etc.) so the attribute never
// serializes as an empty string.
//
// Issue #474 — the theme system is per-user, lives in
// .dixiedata-state/local_settings.json, and never travels with
// the archive on Share / Backup / Restore.
type layoutThemeCtxKey struct{}

// WithLayoutTheme tags ctx with the resolved theme name for
// the current request. Pair with PagePathFromContext-style
// accessors; the appshell calls this in ServeHTTP before
// the mux dispatches.
func WithLayoutTheme(ctx context.Context, theme string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, layoutThemeCtxKey{}, theme)
}

// LayoutThemeFromContext reports the theme name tagged onto
// ctx via WithLayoutTheme. Returns the empty string when no
// tag is present.
func LayoutThemeFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(layoutThemeCtxKey{}).(string)
	return v
}

// layoutTheme is the templ-callable shim. Returns the theme
// name or "default" as a safe fallback.
func layoutTheme(ctx context.Context) string {
	if t := LayoutThemeFromContext(ctx); t != "" {
		return t
	}
	return "default"
}

// layoutExportSurface mirrors layoutTheme for the per-user
// export.surface preference (issue #534). The dispatcher reads
// this attribute off <html> to decide whether to suppress the
// post-export navigation. The appshell tags the resolved value
// in ServeHTTP so the first paint carries the right preference
// without a flash of default.
type layoutExportSurfaceCtxKey struct{}

// WithLayoutExportSurface tags ctx with the resolved export
// surface preference for the current request. Pair with
// LayoutExportSurfaceFromContext; the appshell calls this in
// ServeHTTP before the mux dispatches.
func WithLayoutExportSurface(ctx context.Context, surface string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, layoutExportSurfaceCtxKey{}, surface)
}

// LayoutExportSurfaceFromContext reports the surface tagged
// onto ctx via WithLayoutExportSurface. Returns the empty
// string when no tag is present (templates rendered before the
// request hook ran, error pages, etc.).
func LayoutExportSurfaceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(layoutExportSurfaceCtxKey{}).(string)
	return v
}

// layoutExportSurface is the templ-callable shim. Returns the
// resolved preference or "jobs-page" as a safe fallback so
// the attribute never serializes as an empty string.
func layoutExportSurface(ctx context.Context) string {
	if s := LayoutExportSurfaceFromContext(ctx); s != "" {
		return s
	}
	return "jobs-page"
}

// layoutConfigCtxKey is the context key for the client-side config
// blob injected as window.__dixieConfig in the layout template.
type layoutConfigCtxKey struct{}

// WithLayoutConfig tags ctx with the client config for the
// current request. The appshell calls this in ServeHTTP before
// the mux dispatches. Pair with LayoutConfigJSON to render the
// <script> tag.
func WithLayoutConfig(ctx context.Context, cfg config.ClientConfig) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, layoutConfigCtxKey{}, cfg)
}

// LayoutConfigFromContext returns the client config tagged onto
// ctx. Returns zero-value ClientConfig when not set.
func LayoutConfigFromContext(ctx context.Context) config.ClientConfig {
	if ctx == nil {
		return config.ClientConfig{}
	}
	v, _ := ctx.Value(layoutConfigCtxKey{}).(config.ClientConfig)
	return v
}

// layoutConfigJSON is the templ-callable shim. Returns the
// client config serialized as a JSON blob for injection into
// a <script> tag. Returns "{}" when no config is set.
func layoutConfigJSON(ctx context.Context) string {
	cfg := LayoutConfigFromContext(ctx)
	data, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(data)
}