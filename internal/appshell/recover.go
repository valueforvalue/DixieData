package appshell

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

// requestLog is a synchronous, non-deferred file used to record
// every request that reaches ServeHTTP. Survives os.Exit crashes
// (writes are flushed before LogCrash returns). The user's
// reported 'crash without warning' produces no crash.log entry,
// which suggests either a native crash (segfault) or an early
// os.Exit before our recover middleware runs. A request log
// surfaces the latter case because we'll see the last request
// that was in flight.
var (
	requestLogOnce sync.Once
	requestLogPath string
	requestLogFile *os.File
	requestLogMu   sync.Mutex
)

func resolveRequestLogPath() string {
	requestLogOnce.Do(func() {
		dir := appdata.DefaultDir()
		if dir == "" || os.MkdirAll(dir, 0o755) != nil {
			dir = filepath.Join(os.TempDir(), "dixiedata-crash")
			_ = os.MkdirAll(dir, 0o755)
		}
		requestLogPath = filepath.Join(dir, "request.log")
	})
	return requestLogPath
}

// LogRequest records a single line per request, synchronously.
// Format: <RFC3339Nano> <method> <url> <status-or-pending>.
// Used to leave breadcrumbs before a crash so we know what the
// last in-flight request was.
func LogRequest(r *http.Request, status int) {
	if r == nil {
		return
	}
	path := resolveRequestLogPath()
	requestLogMu.Lock()
	defer requestLogMu.Unlock()
	if requestLogFile == nil {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		requestLogFile = f
	}
	line := fmt.Sprintf("%s %s %s %d\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		r.Method,
		r.URL.String(),
		status,
	)
	_, _ = requestLogFile.WriteString(line)
	_ = requestLogFile.Sync()
}

// LogDebugEvent writes a tagged line to request.log for crash
// diagnosis. Tag prefix keeps it greppable and removable. Safe
// to call from inside handler code; writes are synchronous.
func LogDebugEvent(r *http.Request, msg string) {
	if r == nil {
		return
	}
	path := resolveRequestLogPath()
	requestLogMu.Lock()
	defer requestLogMu.Unlock()
	if requestLogFile == nil {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		requestLogFile = f
	}
	line := fmt.Sprintf("%s [DEBUG] %s %s %s\n",
		time.Now().UTC().Format(time.RFC3339Nano),
		r.Method,
		r.URL.String(),
		msg,
	)
	_, _ = requestLogFile.WriteString(line)
	_ = requestLogFile.Sync()
}

// crashLogOnce ensures we compute the crash-log path lazily, on first
// panic, so the App's dataDir has been set by startup() before we try
// to open a file there. Tests that trigger panics before startup will
// fall back to a temp dir.
var crashLogOnce sync.Once

var crashLogPath string

func resolveCrashLogPath() string {
	crashLogOnce.Do(func() {
		dir := appdata.DefaultDir()
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "dixiedata-crash")
			_ = os.MkdirAll(dir, 0o755)
		} else if err := os.MkdirAll(dir, 0o755); err != nil {
			dir = filepath.Join(os.TempDir(), "dixiedata-crash")
			_ = os.MkdirAll(dir, 0o755)
		}
		crashLogPath = filepath.Join(dir, "crash.log")
	})
	return crashLogPath
}

// LogCrash appends a structured crash entry to the crash log. Called
// from ServeHTTP's deferred recover so a handler panic is captured
// with the request context, the panic value, and the full goroutine
// stack trace. Returns the path of the log file so the 500 response
// can mention it.
func LogCrash(r *http.Request, panicValue any) string {
	path := resolveCrashLogPath()
	entry := buildCrashEntry(r, panicValue)
	// Best-effort append. If the log cannot be written we still want
	// to surface the panic to stderr so the operator sees it.
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_, _ = f.WriteString(entry)
		_ = f.Close()
	}
	fmt.Fprintln(os.Stderr, entry)
	return path
}

func buildCrashEntry(r *http.Request, panicValue any) string {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	var url string
	var method string
	if r != nil {
		method = r.Method
		url = r.URL.String()
	}
	return fmt.Sprintf(
		"\n===== CRASH %s =====\n"+
			"method:  %s\n"+
			"url:     %s\n"+
			"panic:   %v\n"+
			"stack:\n%s\n",
		ts, method, url, panicValue, debug.Stack(),
	)
}

// recoverMiddleware wraps an http.Handler so any panic from the
// inner handler is logged to crash.log and converted into a 500
// response. Keeps the app process alive so the user can see the
// error in the UI (or via the log file) instead of having the Wails
// window vanish.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if pv := recover(); pv != nil {
				path := LogCrash(r, pv)
				// Avoid clobbering a partial response by using a
				// fresh writer state. http.Error sets the status
				// and writes a plain body; if headers were already
				// sent the Write will be a no-op which is fine.
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Internal server error. See %s for details.\n", path)
			}
		}()
		next.ServeHTTP(w, r)
	})
}