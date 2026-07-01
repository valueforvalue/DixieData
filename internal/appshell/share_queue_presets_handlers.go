package appshell

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/records"
)

// handleListShareQueuePresets returns the saved presets as a
// small JSON array used by the modal's Saved Queues section.
// GET /share/queue/presets.
//
// Returns the presets ordered by last_used_at DESC, name ASC
// (the service does the sort); the modal renders the row
// straight from this payload.
func (a *App) handleListShareQueuePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	presets, err := a.shareQueuePresets.List()
	if err != nil {
		respondInternal(w, r, "Could not load saved Share Queue presets.", err)
		return
	}
	// Emit [] instead of null so the modal's JS can iterate
	// the response without a null-guard. List() returns nil
	// for an empty database so we coerce here.
	if presets == nil {
		presets = []records.ShareQueuePreset{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{"presets": presets}); err != nil {
		// Body already partially written; nothing useful to do.
		return
	}
}

// handleSaveShareQueuePreset creates a new preset from the
// modal's form fields. POST /share/queue/presets.
//
// Reads a single `name` field plus a repeating
// `soldier_ids` field. Trims whitespace, drops non-positive
// IDs, and maps the UNIQUE constraint violation to HTTP 409
// so the modal can render an inline error rather than a
// 500.
func (a *App) handleSaveShareQueuePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the preset form.", err)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		respondValidation(w, r, "Preset name is required.", errors.New("missing name"))
		return
	}
	soldierIDs := parseShareQueueIDs(r.Form["soldier_ids"])
	if len(soldierIDs) == 0 {
		respondValidation(w, r, "Preset must contain at least one Person Record.",
			errors.New("empty soldier_ids"))
		return
	}
	saved, err := a.shareQueuePresets.Create(records.ShareQueuePreset{
		Name:       name,
		SoldierIDs: soldierIDs,
	})
	if err != nil {
		if errors.Is(err, records.ErrShareQueuePresetNameTaken) {
			http.Error(w, "A preset with that name already exists. Pick a different name.", http.StatusConflict)
			return
		}
		respondInternal(w, r, "Could not save the preset.", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":   saved.ID,
		"name": saved.Name,
	}); err != nil {
		return
	}
}

// handleDeleteShareQueuePreset removes a saved preset.
// DELETE /share/queue/presets/{id}.
func (a *App) handleDeleteShareQueuePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/share/queue/presets/"), 10, 64)
	if err != nil {
		http.Error(w, "invalid preset id", http.StatusBadRequest)
		return
	}
	if err := a.shareQueuePresets.Delete(id); err != nil {
		if errors.Is(err, records.ErrShareQueuePresetNotFound) {
			http.Error(w, "preset not found", http.StatusNotFound)
			return
		}
		respondInternal(w, r, "Could not delete the preset.", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleApplyShareQueuePreset returns the preset's soldier_id
// list as JSON so the modal's Load handler can write it back
// to localStorage. GET /share/queue/presets/{id}/apply.
//
// Also bumps last_used_at so the preset floats to the top of
// the Saved Queues section the next time the modal opens.
func (a *App) handleApplyShareQueuePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/share/queue/presets/")
	path = strings.TrimSuffix(path, "/apply")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid preset id", http.StatusBadRequest)
		return
	}
	preset, err := a.shareQueuePresets.Get(id)
	if err != nil {
		if errors.Is(err, records.ErrShareQueuePresetNotFound) {
			http.Error(w, "preset not found", http.StatusNotFound)
			return
		}
		respondInternal(w, r, "Could not load the preset.", err)
		return
	}
	if err := a.shareQueuePresets.TouchLastUsed(id); err != nil {
		// Non-fatal: the preset was loaded, we just couldn't
		// record the touch. The next List() will surface the
		// older last_used_at; the user can re-load to retry.
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":          preset.ID,
		"name":        preset.Name,
		"soldier_ids": preset.SoldierIDs,
	}); err != nil {
		return
	}
}