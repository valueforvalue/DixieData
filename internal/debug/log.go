// Package debug owns DixieData's logging + debug harness.
//
// It exposes:
//
//   - Configure: call once at startup to install a slog handler that
//     fans output to a registry of Sinks (file + ring buffer + stderr
//     by default; future Syslog/OTLP/HTTP sinks register themselves).
//   - SetDebugMode: toggle debug mode at runtime; future slog calls
//     use the new level + mirror to stderr.
//   - IsDebugMode / LogPath / GetRingBuffer: accessors consumed by the
//     appshell layer (handlers, settings, debug console).
//   - FromContext: per-request logger that decorates every line with
//     the request_id set by Middleware.
//   - RegisterSink / UnregisterSink: extend the harness with new
//     output destinations without editing the handler.
//
// The package depends only on stdlib; no third-party deps.
//
// Designed for future generalization: see the Generalization Path
// section of the implementation plan for the seams (Sink interface,
// component attribute, schema_version field) that make this package
// lift-and-shift ready for other Go apps on the same stack.
package debug

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// SchemaVersion is the version of the JSONL entry format emitted by
// the handler. Bump when adding fields; parsers branch on it.
const SchemaVersion = 1

// Config controls Configure. All fields are read once at startup; later
// changes must go through SetDebugMode / SetLevel.
type Config struct {
	// LogPath is the JSONL file destination. Parent dirs are created.
	// Required.
	LogPath string
	// RingSize is the in-memory ring buffer capacity. <= 0 disables the
	// ring buffer.
	RingSize int
	// Debug mirrors output to stderr and uses Debug level as the floor.
	// Initial value can also be set via DIXIEDATA_DEBUG=1 in env.
	Debug bool
	// AppName + AppVersion + BuildIdentity are stamped into every entry
	// as stable fields for downstream filtering.
	AppName       string
	AppVersion    string
	BuildIdentity string
}

var (
	configureOnce sync.Once

	// mu guards handler, currentLevel, debugMode, logFile, bufWriter.
	mu            sync.RWMutex
	handler       *teeHandler
	currentLevel  slog.Level
	debugMode     atomic.Bool
	logFile       *os.File
	bufWriter     *bufio.Writer
	ringBuf       *RingBuffer
	stderrMirrored bool

	// sinksMu guards the sinks registry. Sinks receive every entry
	// from the time they are registered forward.
	sinksMu sync.RWMutex
	sinks   []Sink
)

// Sink is the destination interface for log entries. Implementations
// must be safe for concurrent use. Non-blocking preferred; failures
// are logged via stdlib log but never panic.
//
// The default registry contains one composite sink (file + ring buffer
// + stderr). Additional sinks (Syslog, OTLP, HTTP webhook) can be
// registered via RegisterSink without editing the handler.
type Sink interface {
	Write(e Entry) error
	Close() error
}

// RegisterSink adds a sink to the global registry. Safe to call
// before or after Configure. The sink receives every entry from the
// time it is registered forward. Duplicate registration of the same
// pointer is a no-op.
func RegisterSink(s Sink) {
	if s == nil {
		return
	}
	sinksMu.Lock()
	defer sinksMu.Unlock()
	for _, existing := range sinks {
		if existing == s {
			return // already registered
		}
	}
	sinks = append(sinks, s)
}

// UnregisterSink removes a sink from the registry and closes it.
// Returns an error if the sink was not registered.
func UnregisterSink(s Sink) error {
	if s == nil {
		return fmt.Errorf("debug: nil sink")
	}
	sinksMu.Lock()
	defer sinksMu.Unlock()
	for i, existing := range sinks {
		if existing == s {
			sinks = append(sinks[:i], sinks[i+1:]...)
			return s.Close()
		}
	}
	return fmt.Errorf("debug: sink not registered")
}

// fanout writes one entry to every registered sink. Errors are
// logged via stdlib log but never propagate.
func fanout(e Entry) {
	sinksMu.RLock()
	defer sinksMu.RUnlock()
	for _, s := range sinks {
		if err := s.Write(e); err != nil {
			fmt.Fprintf(os.Stderr, "debug: sink write failed: %v\n", err)
		}
	}
}

// closeSinks closes all registered sinks and clears the registry.
func closeSinks() {
	sinksMu.Lock()
	defer sinksMu.Unlock()
	for _, s := range sinks {
		_ = s.Close()
	}
	sinks = nil
}

// Configure installs the package's slog handler as slog.Default. Safe to
// call multiple times; the first call wins. Subsequent calls return
// nil without reconfiguring (sync.Once pattern). Errors from the first
// call are NOT re-surfaced on subsequent calls — callers that need to
// react to setup failure must observe it directly via the first return.
//
// Reads DIXIEDATA_DEBUG=1 / true from env to seed cfg.Debug when env is
// set, regardless of the value passed in cfg.
func Configure(cfg Config) error {
	var setupErr error
	configureOnce.Do(func() {
		setupErr = setup(cfg)
	})
	return setupErr
}

func setup(cfg Config) error {
	if strings.TrimSpace(cfg.LogPath) == "" {
		return fmt.Errorf("debug: LogPath is required")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return fmt.Errorf("debug: create log dir: %w", err)
	}
	f, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("debug: open log file: %w", err)
	}
	if cfg.RingSize <= 0 {
		cfg.RingSize = 500
	}
	rb := NewRingBuffer(cfg.RingSize)

	debugFromEnv := envBool("DIXIEDATA_DEBUG")
	debugMode.Store(cfg.Debug || debugFromEnv)

	mu.Lock()
	logFile = f
	bufWriter = bufio.NewWriterSize(f, 64*1024)
	ringBuf = rb
	stderrMirrored = false

	level := slog.LevelInfo
	if debugMode.Load() {
		level = slog.LevelDebug
	}
	currentLevel = level

	h := newTeeHandler(bufWriter, ringBuf, cfg.AppName, cfg.AppVersion, cfg.BuildIdentity, level)
	handler = h
	// Register the handler as a sink so future Syslog/OTLP/HTTP sinks
	// can be added via RegisterSink without editing this code.
	RegisterSink(h)
	mu.Unlock()

	slog.SetDefault(slog.New(h))
	slog.Info("debug: logging configured",
		"path", cfg.LogPath,
		"ring_size", cfg.RingSize,
		"debug_mode", debugMode.Load(),
		"level", level.String(),
	)
	return nil
}

// SetDebugMode flips the global debug flag at runtime. When enabled,
// stderr mirroring is activated (if it wasn't already) and the slog
// level floor drops to Debug. When disabled, stderr mirroring stops and
// the floor rises to Info.
func SetDebugMode(enabled bool) {
	debugMode.Store(enabled)
	mu.Lock()
	defer mu.Unlock()
	if handler == nil {
		return
	}
	if enabled {
		handler.level = slog.LevelDebug
		stderrMirrored = true
	} else {
		handler.level = slog.LevelInfo
		stderrMirrored = false
	}
	currentLevel = handler.level
}

// IsDebugMode returns the current debug-mode flag.
func IsDebugMode() bool { return debugMode.Load() }

// LogPath returns the configured log file path, or "" if Configure has
// not run.
func LogPath() string {
	mu.RLock()
	defer mu.RUnlock()
	if logFile == nil {
		return ""
	}
	return logFile.Name()
}

// GetRingBuffer returns the in-memory ring buffer, or nil if Configure has
// not run or RingSize was set to 0.
func GetRingBuffer() *RingBuffer {
	mu.RLock()
	defer mu.RUnlock()
	return ringBuf
}

// StderrWriter returns an io.Writer that mirrors stderr. When stress
// logging is active (appshell swaps os.Stderr), the slog handler's
// stderr output should be routed through this writer so it ends up in
// the stress log file alongside stdlib log output.
func StderrWriter() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	if handler == nil {
		return nil
	}
	return stderrMirror{}
}

// stderrMirror writes directly to the active slog handler's file
// buffer. Used by stress_logging.go to redirect os.Stderr into the
// same file as slog JSON output without double-formatting.
type stderrMirror struct{}

func (stderrMirror) Write(p []byte) (int, error) {
	mu.RLock()
	h := handler
	mu.RUnlock()
	if h == nil {
		return len(p), nil
	}
	p = append(p, '\n')
	n, err := h.buf.Write(p)
	if err != nil {
		return n, err
	}
	return len(p) - 1, nil
}

// Flush synchronously writes any buffered log output to disk.
func Flush() error {
	mu.Lock()
	defer mu.Unlock()
	if bufWriter == nil {
		return nil
	}
	return bufWriter.Flush()
}

// Close releases the log file handle and closes all registered sinks.
func Close() error {
	mu.Lock()
	if bufWriter != nil {
		_ = bufWriter.Flush()
		bufWriter = nil
	}
	var err error
	if logFile != nil {
		err = logFile.Close()
		logFile = nil
	}
	mu.Unlock()
	closeSinks()
	return err
}

// FromContext returns a *slog.Logger that emits the request_id stored
// on the context. If no request_id is present, the default logger is
// returned (still routed through our handler).
func FromContext(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if rid := RequestIDFromContext(ctx); rid != "" {
		return slog.Default().With("request_id", rid)
	}
	return slog.Default()
}

// envBool reads a boolean env var. Accepts 1, true, yes (case-insensitive).
func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}