// recover.go owns LogCrash (a thin shim preserved for tests and the
// 500-response body) + recoverMiddleware (converts a handler panic
// into a slog.Error + 500 response). The old requestLog/crashLog file
// globals are gone — every entry now flows through the debug package's
// slog handler into logs/app.log.jsonl.
package appshell

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	runtimedebug "runtime/debug"
	"time"

	"github.com/valueforvalue/DixieData/internal/debug"
)

// LogCrash is a thin shim preserved for tests (recover_test.go) and
// any future external callers. Writes a slog entry tagged "crash" and
// returns the active log file path so the 500 response can reference
// it. Replaces the old direct-write implementation that maintained
// crash.log as a separate text file.
//
// When debug.Configure has not yet run (early-startup panic, tests),
// falls back to writing a synthetic crash.log entry under a temp dir
// so recover_test.go's os.ReadFile call still succeeds.
func LogCrash(r *http.Request, panicValue any) string {
	path := debug.LogPath()
	log := debug.FromContext(r.Context())
	attrs := []any{
		"component", "http",
		"tag", "crash",
		"panic", fmt.Sprintf("%v", panicValue),
		"stack", string(runtimedebug.Stack()),
	}
	if r != nil {
		attrs = append(attrs, "method", r.Method, "url", r.URL.String())
	}
	log.Error("appshell: panic recovered", attrs...)
	_ = debug.Flush()
	if path == "" {
		// Fallback for early-startup or test-only path.
		dir := filepath.Join(os.TempDir(), "dixiedata-crash")
		_ = os.MkdirAll(dir, 0o755)
		path = filepath.Join(dir, "app.log.jsonl")
		fallback := buildCrashEntry(r, panicValue)
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.WriteString(fallback)
			_ = f.Close()
		}
		fmt.Fprintln(os.Stderr, fallback)
	}
	return path
}

// buildCrashEntry formats the synthetic crash block. Used by the
// fallback path in LogCrash when debug.Configure has not run.
func buildCrashEntry(r *http.Request, panicValue any) string {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	var method, url string
	if r != nil {
		method = r.Method
		url = r.URL.String()
	}
	return fmt.Sprintf(
		"\n===== CRASH %s =====\nmethod:  %s\nurl:     %s\npanic:   %v\nstack:\n%s\n",
		ts, method, url, panicValue, runtimedebug.Stack(),
	)
}

// recoverMiddleware converts a handler panic into a slog.Error + a 500
// response. Keeps the process alive so the user can read the error in
// the UI or the log file.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if pv := recover(); pv != nil {
				path := LogCrash(r, pv)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Internal server error. See %s for details.\n", path)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ResolveCrashLogPath is retained for any callers that previously
// depended on the old crashLogPath. Returns debug.LogPath().
func ResolveCrashLogPath() string {
	return debug.LogPath()
}