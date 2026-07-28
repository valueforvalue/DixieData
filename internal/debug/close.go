package debug

// close.go holds the deferCloseLog helper used across the codebase
// to capture errors from deferred Close() calls that the bare
// `defer c.Close()` form silently drops.
//
// Issue #384 locked decision #5: every deferred Close() in a
// production path must capture the error and log via slog.Warn with
// structured context. The inline closure form is the canonical Go
// idiom but is verbose at 137 call sites; this helper preserves the
// intent (capture + log with structured context) in one line per
// site while keeping the call site readable:
//
//	defer debug.DeferCloseLog(f, "backup-zip-writer")
//
// vs the inline closure form:
//
//	defer func() {
//	    if err := f.Close(); err != nil {
//	        slog.Warn("close failed", "component", "backup-zip-writer", "err", err)
//	    }
//	}()
//
// The helper lives in internal/debug (not internal/appshell) because
// internal/archive and internal/records both need it and both are
// imported by internal/appshell — a cyclic import if the helper lived
// in appshell. internal/debug is a leaf package with no upstream deps.

import (
	"io"
	"log/slog"
)

// DeferCloseLog closes c and logs any failure. Intended for
// `defer debug.DeferCloseLog(c, "component-name")`.
//
// The component parameter is a short stable tag (e.g. "backup-zip",
// "soldier-row", "pdf-temp-file") that flows into the structured
// "component" log field so the debug-console dump can grep by
// component. The "audit" field is fixed to "close-error" so the
// regression-net probe can grep for it without parsing log lines.
//
// Nil-safety: c.Close() on a nil io.Closer interface panics, so
// callers must guard before deferring.
func DeferCloseLog(c io.Closer, component string) {
	if err := c.Close(); err != nil {
		slog.Warn("close failed",
			"audit", "close-error",
			"component", component,
			"err", err.Error(),
		)
	}
}
