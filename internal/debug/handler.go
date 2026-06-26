package debug

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"
)

// teeHandler writes each slog record to:
//  1. JSONL file (bufio.Writer, flushed periodically)
//  2. In-memory ring buffer (Debug Console UI)
//  3. stderr (when debug mode is on)
//
// It implements both slog.Handler (for slog.SetDefault) and Sink (so it
// can be registered in the sinks registry via RegisterSink).
type teeHandler struct {
	buf      *bufio.Writer
	ring     *RingBuffer
	appName  string
	version  string
	build    string
	level    slog.Level
	attrs    []slog.Attr
	groups   []string
	stderrMu sync.Mutex
}

var _ Sink = (*teeHandler)(nil)

// Write implements Sink. Renders the entry as a JSONL line and writes
// to the file buffer + ring + (optionally) stderr.
func (h *teeHandler) Write(e Entry) error {
	line, err := buildJSONLineFromEntry(h, e)
	if err != nil {
		return err
	}
	if _, err := h.buf.Write(line); err != nil {
		return err
	}
	if h.ring != nil {
		h.ring.Push(e)
	}
	if debugMode.Load() {
		h.stderrMu.Lock()
		_, _ = os.Stderr.Write([]byte(e.Time.Format(time.RFC3339Nano) + " " +
			e.Level + " " + e.Message + "\n"))
		h.stderrMu.Unlock()
	}
	return nil
}

// Close implements Sink. Flushes the buffer; does not close the
// underlying file (that's Close()'s job on the package level).
func (h *teeHandler) Close() error {
	if h.buf == nil {
		return nil
	}
	return h.buf.Flush()
}

func newTeeHandler(buf *bufio.Writer, ring *RingBuffer, appName, version, build string, level slog.Level) *teeHandler {
	return &teeHandler{
		buf: buf, ring: ring, appName: appName, version: version, build: build, level: level,
	}
}

func (h *teeHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *teeHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := flattenAttrs(r)
	entry := Entry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrs,
	}
	if v, ok := attrs["source"].(string); ok {
		entry.Source = v
	}
	if v, ok := attrs["component"].(string); ok {
		entry.Component = v
	}
	fanout(entry)
	return nil
}

// writeRaw writes a pre-formatted line directly to the buffer.
func (h *teeHandler) writeRaw(p []byte) {
	h.buf.Write(p)
}

func (h *teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *teeHandler) flush() error {
	if h.buf == nil {
		return nil
	}
	return h.buf.Flush()
}

func buildJSONLine(h *teeHandler, r slog.Record, attrs map[string]any) ([]byte, error) {
	return buildJSONLineFromEntry(h, Entry{
		Time:      r.Time,
		Level:     r.Level.String(),
		Message:   r.Message,
		Attrs:     attrs,
		Source:    stringAttr(attrs, "source"),
		Component: stringAttr(attrs, "component"),
	})
}

// buildJSONLineFromEntry renders an Entry as a single JSON object on
// one line. Adds schema_version + the standard envelope fields.
func buildJSONLineFromEntry(h *teeHandler, e Entry) ([]byte, error) {
	obj := make(map[string]any, 5+len(e.Attrs))
	obj["schema_version"] = SchemaVersion
	obj["time"] = e.Time.UTC().Format(time.RFC3339Nano)
	obj["level"] = e.Level
	obj["msg"] = e.Message
	if h != nil {
		if h.appName != "" {
			obj["app"] = h.appName
		}
		if h.version != "" {
			obj["version"] = h.version
		}
		if h.build != "" {
			obj["build"] = h.build
		}
	}
	if e.Component != "" {
		obj["component"] = e.Component
	}
	if e.Source != "" {
		obj["source"] = e.Source
	}
	for _, a := range h.attrs {
		obj[a.Key] = a.Value.Any()
	}
	for k, v := range e.Attrs {
		obj[k] = v
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func stringAttr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func flattenAttrs(r slog.Record) map[string]any {
	out := make(map[string]any, r.NumAttrs()+2)
	r.Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.Any()
		return true
	})
	return out
}

// PeriodicFlush flushes the buffer every interval. Started by appshell.
func PeriodicFlush(interval time.Duration, done <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-done:
			_ = Flush()
			return
		case <-t.C:
			_ = Flush()
		}
	}
}

// (teeHandler intentionally does not implement io.Writer; its Write
// method has the Sink.Entry signature. For raw byte writes, use
// writeRaw.)