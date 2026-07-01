// Saved Share Queue presets (issue #192). Local-only SQLite
// table with one row per named subset. Mirrors the printable-
// export template shape from issue #178 so power users get a
// consistent save/reuse surface across both subset pipelines.
//
// The payload is a JSON array of soldier IDs (the v1 schema
// trades a normalized join table for a JSON column because
// preset membership is small and we never need to query
// "all presets containing soldier X" in reverse).
package records

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ShareQueuePreset is the row shape returned by Get/List. The
// SoldierIDs slice is JSON-decoded from soldier_ids_json on
// read. JSON tags match the modal's field naming so the apply
// endpoint can return the row directly with no rename.
//
// LastUsedAt is bumped by the apply handler every time the
// preset is loaded into the modal; it surfaces as the modal's
// "most recently used" sort order so power users find their
// favourites without re-scanning the whole list.
type ShareQueuePreset struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	SoldierIDs []int64   `json:"soldier_ids"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// ErrShareQueuePresetNameTaken is returned when Create hits
// the UNIQUE constraint on name. The handler maps it to 409.
var ErrShareQueuePresetNameTaken = errors.New("share queue preset name already exists")

// ErrShareQueuePresetNotFound is returned when Get / Delete /
// Apply can't find a matching row. Mapped to 404 by the
// handler.
var ErrShareQueuePresetNotFound = errors.New("share queue preset not found")

// ShareQueuePresetService provides CRUD on the
// share_queue_presets table. Construct via
// NewShareQueuePresetService(db) -- usually wired into *App at
// startup.
type ShareQueuePresetService struct {
	db *sql.DB
}

func NewShareQueuePresetService(db *sql.DB) *ShareQueuePresetService {
	return &ShareQueuePresetService{db: db}
}

// Create inserts a new preset. Returns
// ErrShareQueuePresetNameTaken if a row with the same name
// already exists (mapped to HTTP 409 by the handler). The
// name is trimmed before insert so a leading/trailing space
// never silently creates a "different" preset.
func (s *ShareQueuePresetService) Create(p ShareQueuePreset) (ShareQueuePreset, error) {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return ShareQueuePreset{}, errors.New("preset name is required")
	}
	if p.SoldierIDs == nil {
		p.SoldierIDs = []int64{}
	}
	// Drop empty / negative IDs defensively. The handler should
	// have already filtered these, but a stray "" or 0 from a
	// bad form parse must never land in the database.
	cleaned := make([]int64, 0, len(p.SoldierIDs))
	for _, id := range p.SoldierIDs {
		if id > 0 {
			cleaned = append(cleaned, id)
		}
	}
	soldierIDsJSON, err := json.Marshal(cleaned)
	if err != nil {
		return ShareQueuePreset{}, fmt.Errorf("encode soldier_ids: %w", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO share_queue_presets (name, soldier_ids_json) VALUES (?, ?)`,
		name, string(soldierIDsJSON),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ShareQueuePreset{}, ErrShareQueuePresetNameTaken
		}
		return ShareQueuePreset{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ShareQueuePreset{}, err
	}
	return s.Get(id)
}

// Get returns a single preset by id. Returns
// ErrShareQueuePresetNotFound if no row matches.
func (s *ShareQueuePresetService) Get(id int64) (ShareQueuePreset, error) {
	row := s.db.QueryRow(`
		SELECT id, name, soldier_ids_json, created_at, last_used_at
		FROM share_queue_presets WHERE id = ?`, id)
	return scanShareQueuePreset(row)
}

// List returns every preset sorted by last_used_at DESC, name
// ASC. The modal's Saved Queues section relies on this order
// so recently-loaded presets float to the top.
func (s *ShareQueuePresetService) List() ([]ShareQueuePreset, error) {
	rows, err := s.db.Query(`
		SELECT id, name, soldier_ids_json, created_at, last_used_at
		FROM share_queue_presets
		ORDER BY last_used_at DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShareQueuePreset
	for rows.Next() {
		p, err := scanShareQueuePreset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes a preset by id. Returns
// ErrShareQueuePresetNotFound if no row matches.
func (s *ShareQueuePresetService) Delete(id int64) error {
	res, err := s.db.Exec(`DELETE FROM share_queue_presets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrShareQueuePresetNotFound
	}
	return nil
}

// TouchLastUsed bumps last_used_at to now so the modal's
// "most recently used" sort surfaces the preset the next time
// the Saved Queues list renders. Called by the apply handler
// every time a preset is loaded.
func (s *ShareQueuePresetService) TouchLastUsed(id int64) error {
	_, err := s.db.Exec(
		`UPDATE share_queue_presets SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	return err
}

type shareQueuePresetScanner interface {
	Scan(dest ...interface{}) error
}

func scanShareQueuePreset(row shareQueuePresetScanner) (ShareQueuePreset, error) {
	var (
		p         ShareQueuePreset
		idsJSON   string
		createdAt time.Time
		lastUsed  time.Time
	)
	if err := row.Scan(&p.ID, &p.Name, &idsJSON, &createdAt, &lastUsed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ShareQueuePreset{}, ErrShareQueuePresetNotFound
		}
		return ShareQueuePreset{}, err
	}
	if err := json.Unmarshal([]byte(idsJSON), &p.SoldierIDs); err != nil {
		return ShareQueuePreset{}, fmt.Errorf("decode soldier_ids: %w", err)
	}
	if p.SoldierIDs == nil {
		p.SoldierIDs = []int64{}
	}
	p.CreatedAt = createdAt
	p.LastUsedAt = lastUsed
	return p, nil
}