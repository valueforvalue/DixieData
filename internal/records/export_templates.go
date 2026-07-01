// Printable-export template storage (issue #178). Local-only SQLite
// table with one row per saved template. CRUD is intentionally
// minimal — name uniqueness is enforced by the UNIQUE constraint
// at the DB level, not by application logic, so a racing save
// surfaces as a SQLITE_CONSTRAINT error that the handler can map
// to 409.
//
// JSON-encoded columns for filters and group_by avoid the need for
// a normalized child table. Both arrays are small (typically 0-50
// entries) and rarely read in bulk; a relational layout would cost
// more in JOINs than it saves in storage.
package records

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ExportTemplate is the row shape returned by Get/List. The
// Filters + GroupBy slices are JSON-decoded from the *_json
// columns on read. JSON tags match the modal's field naming so
// the apply endpoint can return the row directly with no rename.
//
// SelectedIDs is populated only when scope == "selected" (issue
// #181); other scopes store an empty slice. The apply handler
// cross-checks each ID against the current archive and emits a
// stale-id warning for any that no longer exist.
type ExportTemplate struct {
	ID                int64                `json:"id"`
	Name              string               `json:"name"`
	Scope             string               `json:"scope"`
	Filters           map[string][]string  `json:"filters"`
	SortBy            string               `json:"sort_by"`
	GroupBy           []string             `json:"group_by"`
	Orientation       string               `json:"orientation"`
	SelectedIDs       []int64              `json:"selected_ids"`
	PrinterFriendly   bool                 `json:"printer_friendly"`
	FullBiographyPage bool                 `json:"full_biography_page"`
	CreatedAt         time.Time            `json:"created_at"`
	LastUsedAt        time.Time            `json:"last_used_at"`
}

// ExportTemplateService provides CRUD on the export_templates table.
// Construct via NewExportTemplateService(db) — usually wired into
// *App at startup.
type ExportTemplateService struct {
	db *sql.DB
}

func NewExportTemplateService(db *sql.DB) *ExportTemplateService {
	return &ExportTemplateService{db: db}
}

// Create inserts a new template. Returns ErrExportTemplateNameTaken
// if a row with the same name already exists (mapped to HTTP 409
// by the handler).
func (s *ExportTemplateService) Create(t ExportTemplate) (ExportTemplate, error) {
	if strings.TrimSpace(t.Name) == "" {
		return ExportTemplate{}, errors.New("template name is required")
	}
	if strings.TrimSpace(t.Scope) == "" {
		return ExportTemplate{}, errors.New("template scope is required")
	}
	if t.Filters == nil {
		t.Filters = map[string][]string{}
	}
	if t.GroupBy == nil {
		t.GroupBy = []string{}
	}
	if t.SelectedIDs == nil {
		t.SelectedIDs = []int64{}
	}
	if strings.TrimSpace(t.SortBy) == "" {
		t.SortBy = "last_name"
	}
	if strings.TrimSpace(t.Orientation) == "" {
		t.Orientation = "L"
	}
	filtersJSON, err := json.Marshal(t.Filters)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode filters: %w", err)
	}
	groupByJSON, err := json.Marshal(t.GroupBy)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode group_by: %w", err)
	}
	selectedIDsJSON, err := json.Marshal(t.SelectedIDs)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode selected_ids: %w", err)
	}
	res, err := s.db.Exec(`
		INSERT INTO export_templates (
			name, scope, filters_json, sort_by, group_by_json,
			orientation, selected_ids_json, printer_friendly, full_biography_page
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(t.Name),
		strings.TrimSpace(t.Scope),
		string(filtersJSON),
		t.SortBy,
		string(groupByJSON),
		t.Orientation,
		string(selectedIDsJSON),
		boolToInt(t.PrinterFriendly),
		boolToInt(t.FullBiographyPage),
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ExportTemplate{}, ErrExportTemplateNameTaken
		}
		return ExportTemplate{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ExportTemplate{}, err
	}
	return s.Get(id)
}

// Get returns a single template by id. Returns ErrExportTemplateNotFound
// if no row matches.
func (s *ExportTemplateService) Get(id int64) (ExportTemplate, error) {
	row := s.db.QueryRow(`
		SELECT id, name, scope, filters_json, sort_by, group_by_json,
			orientation, selected_ids_json, printer_friendly, full_biography_page,
			created_at, last_used_at
		FROM export_templates WHERE id = ?`, id)
	return scanExportTemplate(row)
}

// Update (issue #186) replaces the mutable fields on an existing
// template, preserving created_at + last_used_at. The caller is the
// "Save Changes" button on the print-config modal. The same
// normalisation as Create applies (trim, default sort/orientation),
// so the only difference from Create is the SQL statement (UPDATE
// instead of INSERT) + the existence check.
//
// Returns ErrExportTemplateNotFound when no row matches the id;
// ErrExportTemplateNameTaken when the supplied name collides with
// another row's normalised name. errors.Is checks both.
func (s *ExportTemplateService) Update(id int64, fields ExportTemplate) (ExportTemplate, error) {
	if strings.TrimSpace(fields.Name) == "" {
		return ExportTemplate{}, errors.New("template name is required")
	}
	if strings.TrimSpace(fields.Scope) == "" {
		return ExportTemplate{}, errors.New("template scope is required")
	}
	if fields.Filters == nil {
		fields.Filters = map[string][]string{}
	}
	if fields.GroupBy == nil {
		fields.GroupBy = []string{}
	}
	if fields.SelectedIDs == nil {
		fields.SelectedIDs = []int64{}
	}
	if strings.TrimSpace(fields.SortBy) == "" {
		fields.SortBy = "last_name"
	}
	if strings.TrimSpace(fields.Orientation) == "" {
		fields.Orientation = "L"
	}
	filtersJSON, err := json.Marshal(fields.Filters)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode filters: %w", err)
	}
	groupByJSON, err := json.Marshal(fields.GroupBy)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode group_by: %w", err)
	}
	selectedIDsJSON, err := json.Marshal(fields.SelectedIDs)
	if err != nil {
		return ExportTemplate{}, fmt.Errorf("encode selected_ids: %w", err)
	}
	res, err := s.db.Exec(`
		UPDATE export_templates SET
			name = ?, scope = ?, filters_json = ?, sort_by = ?,
			group_by_json = ?, orientation = ?, selected_ids_json = ?,
			printer_friendly = ?, full_biography_page = ?
		WHERE id = ?`,
		strings.TrimSpace(fields.Name),
		strings.TrimSpace(fields.Scope),
		string(filtersJSON),
		fields.SortBy,
		string(groupByJSON),
		fields.Orientation,
		string(selectedIDsJSON),
		boolToInt(fields.PrinterFriendly),
		boolToInt(fields.FullBiographyPage),
		id,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ExportTemplate{}, ErrExportTemplateNameTaken
		}
		return ExportTemplate{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ExportTemplate{}, err
	}
	if n == 0 {
		return ExportTemplate{}, ErrExportTemplateNotFound
	}
	return s.Get(id)
}

// List returns all templates ordered by last-used then name. Cheap
// enough that pagination isn't needed (typical archives have <50
// saved templates).
func (s *ExportTemplateService) List() ([]ExportTemplate, error) {
	rows, err := s.db.Query(`
		SELECT id, name, scope, filters_json, sort_by, group_by_json,
			orientation, selected_ids_json, printer_friendly, full_biography_page,
			created_at, last_used_at
		FROM export_templates
		ORDER BY datetime(last_used_at) DESC, name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportTemplate{}
	for rows.Next() {
		t, err := scanExportTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes a template by id. Missing rows are not an error
// (idempotent).
func (s *ExportTemplateService) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM export_templates WHERE id = ?`, id)
	return err
}

// TouchLastUsed bumps last_used_at to the current time. Called
// from the apply handler whenever a template is loaded into the
// modal so the dropdown ordering reflects recent use.
func (s *ExportTemplateService) TouchLastUsed(id int64) error {
	_, err := s.db.Exec(`UPDATE export_templates SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

func scanExportTemplate(scanner interface {
	Scan(dest ...any) error
}) (ExportTemplate, error) {
	var (
		t              ExportTemplate
		filtersJSON    string
		groupByJSON    string
		selectedIDsJSON string
		printer        int
		fullBio        int
		createdAt      string
		lastUsedAt     string
	)
	if err := scanner.Scan(
		&t.ID, &t.Name, &t.Scope, &filtersJSON, &t.SortBy, &groupByJSON,
		&t.Orientation, &selectedIDsJSON, &printer, &fullBio, &createdAt, &lastUsedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExportTemplate{}, ErrExportTemplateNotFound
		}
		return ExportTemplate{}, err
	}
	if err := json.Unmarshal([]byte(filtersJSON), &t.Filters); err != nil {
		return ExportTemplate{}, fmt.Errorf("decode filters: %w", err)
	}
	if t.Filters == nil {
		t.Filters = map[string][]string{}
	}
	if err := json.Unmarshal([]byte(groupByJSON), &t.GroupBy); err != nil {
		return ExportTemplate{}, fmt.Errorf("decode group_by: %w", err)
	}
	if t.GroupBy == nil {
		t.GroupBy = []string{}
	}
	if err := json.Unmarshal([]byte(selectedIDsJSON), &t.SelectedIDs); err != nil {
		return ExportTemplate{}, fmt.Errorf("decode selected_ids: %w", err)
	}
	if t.SelectedIDs == nil {
		t.SelectedIDs = []int64{}
	}
	t.PrinterFriendly = printer != 0
	t.FullBiographyPage = fullBio != 0
	t.CreatedAt = parseSQLiteTimestamp(createdAt)
	t.LastUsedAt = parseSQLiteTimestamp(lastUsedAt)
	return t, nil
}

// parseSQLiteTimestamp accepts the two timestamp shapes SQLite
// commonly returns: RFC3339 (when written by Go via time.Time) and
// the space-separated "YYYY-MM-DD HH:MM:SS" produced by
// CURRENT_TIMESTAMP. Returns zero time on parse failure rather
// than erroring — these are display-only fields and an unparseable
// value should not block the rest of the row from loading.
func parseSQLiteTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueConstraintError inspects the SQLite error for the
// UNIQUE constraint marker so the service can return
// ErrExportTemplateNameTaken without leaking sql.Error to handlers.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// ErrExportTemplateNameTaken is returned by Create when the name
// already exists. Handlers map this to HTTP 409.
var ErrExportTemplateNameTaken = errors.New("export template name already taken")

// ErrExportTemplateNotFound is returned by Get when no row matches
// the id. Handlers map this to HTTP 404.
var ErrExportTemplateNotFound = errors.New("export template not found")