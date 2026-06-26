// debug_handlers.go owns the /debug/* HTTP endpoints. Phase 4
// implements /debug/state and /debug/client-logs (foundations). Phase
// 6 adds /debug/console, /debug/console/tail, /debug/console/clear,
// and /debug/open-folder.
package appshell

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
)

// handleDebugState returns a small JSON snapshot of debug config so
// the frontend Debug Console UI can decide what to render.
func (a *App) handleDebugState(w http.ResponseWriter, r *http.Request) {
	rb := debug.GetRingBuffer()
	size := 0
	if rb != nil {
		size = rb.Len()
	}
	resp := map[string]any{
		"debug_mode":     a.debugMode.Load(),
		"ring_tail_size": size,
		"ring_capacity":  rb.Cap() != 0,
		"log_path":       debug.LogPath(),
		"app_version":    buildinfo.AppVersion,
		"build_identity": buildinfo.BuildIdentity(),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleClientLogs ingests a batch of frontend log entries and writes
// each as a slog entry with source=frontend. Endpoint accepts POST
// with JSON body {"entries":[{ts,level,msg,stack,url}, ...]}.
func (a *App) handleClientLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.debugMode.Load() {
		// Drop silently when debug mode is off so a misconfigured client
		// doesn't 4xx-spam the log.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var payload struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondValidation(w, r, "Could not parse client log payload.", err)
		return
	}
	log := debug.FromContext(r.Context()).With(
		"source", "frontend",
		"component", "frontend",
	)
	for _, e := range payload.Entries {
		ts, _ := e["ts"].(string)
		level, _ := e["level"].(string)
		msg, _ := e["msg"].(string)
		stack, _ := e["stack"].(string)
		url, _ := e["url"].(string)
		attrs := []any{"client_ts", ts, "client_url", url}
		if stack != "" {
			attrs = append(attrs, "client_stack", stack)
		}
		switch strings.ToLower(level) {
		case "debug":
			log.Debug(msg, attrs...)
		case "info", "log":
			log.Info(msg, attrs...)
		case "warn", "warning":
			log.Warn(msg, attrs...)
		case "error":
			log.Error(msg, attrs...)
		default:
			log.Info(msg, attrs...)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDebugModeToggle reads the new debug_mode from a POST form or
// JSON body, persists to local_settings.json, and updates the in-memory
// flag + slog level live.
func (a *App) handleDebugModeToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var enabled bool
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var payload struct {
			DebugMode string `json:"debug_mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondValidation(w, r, "Could not parse JSON toggle body.", err)
			return
		}
		enabled = strings.ToLower(strings.TrimSpace(payload.DebugMode)) == "on"
	} else {
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the debug-mode form.", err)
			return
		}
		enabled = strings.TrimSpace(r.FormValue("debug_mode")) == "on"
	}
	settings, err := records.LoadLocalSettings(a.dataDir)
	if err != nil {
		respondInternal(w, r, "Could not load local settings.", err)
		return
	}
	settings.DebugMode = enabled
	if err := records.SaveLocalSettings(a.dataDir, settings); err != nil {
		respondInternal(w, r, "Could not save local settings.", err)
		return
	}
	a.debugMode.Store(enabled)
	debug.SetDebugMode(enabled)
	log := debug.FromContext(r.Context())
	if enabled {
		log.Info("debug mode enabled via settings")
		setToastHeader(w, "Debug mode is on. Logs will include DEBUG-level entries and stderr.")
	} else {
		log.Info("debug mode disabled via settings")
		setToastHeader(w, "Debug mode is off.")
	}
	fmt.Fprint(w, "")
}

// handleDebugConsole renders the Debug Console panel via the templ
// fragment defined in internal/presentation/debug_console.templ.
func (a *App) handleDebugConsole(w http.ResponseWriter, r *http.Request) {
	if !a.debugMode.Load() {
		http.Error(w, "Debug mode is off.", http.StatusForbidden)
		return
	}
	rb := debug.GetRingBuffer()
	if rb == nil {
		http.Error(w, "Ring buffer not initialized.", http.StatusServiceUnavailable)
		return
	}
	levelFilter := strings.TrimSpace(r.URL.Query().Get("level"))
	entries := rb.Snapshot()
	if levelFilter != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.Level == levelFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	presentation.DebugConsole(entries, rb.Total(), debug.LogPath()).Render(r.Context(), w)
}

// Phase 6 stubs (added in Phase 6; declared here so route registration
// in routes.go has stable references).
func (a *App) handleDebugConsoleTail(w http.ResponseWriter, r *http.Request) {
	if !a.debugMode.Load() {
		http.Error(w, "Debug mode is off.", http.StatusForbidden)
		return
	}
	since := uint64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			since = n
		}
	}
	rb := debug.GetRingBuffer()
	if rb == nil {
		http.Error(w, "Ring buffer not initialized.", http.StatusServiceUnavailable)
		return
	}
	entries := rb.Since(since)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"entries": entries,
		"total":   rb.Total(),
	})
}

func (a *App) handleDebugConsoleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.debugMode.Load() {
		http.Error(w, "Debug mode is off.", http.StatusForbidden)
		return
	}
	if rb := debug.GetRingBuffer(); rb != nil {
		rb.Clear()
	}
	a.handleDebugConsole(w, r)
}

func (a *App) handleDebugOpenFolder(w http.ResponseWriter, r *http.Request) {
	if !a.debugMode.Load() {
		http.Error(w, "Debug mode is off.", http.StatusForbidden)
		return
	}
	dir := filepath.Dir(debug.LogPath())
	if dir == "" {
		http.Error(w, "Log directory not known.", http.StatusServiceUnavailable)
		return
	}
	url := "file:///" + filepath.ToSlash(dir)
	if err := a.BrowserOpenURL(url); err != nil {
		debug.FromContext(r.Context()).Warn("could not open log folder",
			"component", "debug", "err", err.Error())
	}
	w.WriteHeader(http.StatusNoContent)
}