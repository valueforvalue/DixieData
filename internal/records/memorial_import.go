package records

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const memorialRecordType = "Find a Grave"

// ErrMemorialFormatMismatch is returned when a Memorial Archive's
// format_version field reflects a major bump from the importer's
// expected version (e.g. archive says "memorial_v2" but this
// DixieData build only understands "memorial_v1"). Per Decision 2
// of issue #383, the import refuses on major bumps; the user must
// re-export from an updated scraper that matches their DixieData
// build, or upgrade DixieData. Callers (handler + CLI) surface
// this differently from parse errors so the user sees a clear
// "your scraper produced a newer format than this build understands"
// message instead of a JSON-parse-failure trace.
var ErrMemorialFormatMismatch = errors.New("memorial archive format mismatch")

// MemorialImportIssue is a records-layer type used by the matching service.
type MemorialImportIssue struct {
	Row        int
	MemorialID string
	Name       string
	Error      string
}

// MemorialImportFormat captures the version metadata the FindAGrave
// scraper script stamps into every export (issue #383 Slice 1).
// FormatVersion is the per-surface namespace string (e.g.
// "memorial_v1"); ScriptVersion is the Tampermonkey @version the
// scraper was at when the file was exported; ScriptName is the
// @name metadata so the import summary UI can name the source
// tool. Empty strings mean the field was missing from the envelope
// (pre-v1 archives carry none of these).
type MemorialImportFormat struct {
	FormatVersion string
	ScriptVersion string
	ScriptName    string
}

// MemorialImportPreview is a records-layer type used by the matching service.
type MemorialImportPreview struct {
	FilePath    string
	Format      MemorialImportFormat
	Warnings    []string
	TotalRows   int
	WouldCreate int
	WouldSkip   int
	WouldFail   int
	Issues      []MemorialImportIssue
}

// MemorialImportSummary is a records-layer type used by the matching service.
type MemorialImportSummary struct {
	FilePath             string
	Format               MemorialImportFormat
	ImportedByAppVersion string
	Warnings             []string
	BatchID              string
	TotalRows            int
	Created              int
	Skipped              int
	Failed               int
	Issues               []MemorialImportIssue
}

type memorialArchiveEntry struct {
	MemorialID     string   `json:"memorial_id"`
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	BirthDate      string   `json:"birth_date"`
	BirthLocation  string   `json:"birth_location"`
	DeathDate      string   `json:"death_date"`
	DeathAge       int      `json:"death_age"`
	DeathLocation  string   `json:"death_location"`
	BurialCemetery string   `json:"burial_cemetery"`
	BurialLocation string   `json:"burial_location"`
	Biography      string   `json:"biography"`
	FamilyParents  []string `json:"family_parents"`
	FamilySpouse   string   `json:"family_spouse"`
	FamilyChildren []string `json:"family_children"`
	ScrapedAt      string   `json:"scraped_at"`
}

// PreviewMemorialArchive parses a memorial-archive upload and returns the rows the user must confirm before import.
func (s *SoldierService) PreviewMemorialArchive(path string) (MemorialImportPreview, error) {
	entries, format, err := loadMemorialArchive(path)
	if err != nil {
		return MemorialImportPreview{}, err
	}
	preview := MemorialImportPreview{
		FilePath:  strings.TrimSpace(path),
		Format:    format,
		Warnings:  memorialFormatWarnings(format),
		TotalRows: len(entries),
		Issues:    make([]MemorialImportIssue, 0),
	}
	seen := map[string]struct{}{}
	conn := s.db.Conn()
	for idx, entry := range entries {
		row := idx + 1
		mapped, mapErr := mapMemorialEntry(entry)
		if mapErr != nil {
			preview.WouldFail++
			preview.Issues = append(preview.Issues, importIssue(row, entry, mapErr))
			continue
		}
		memorialID := strings.TrimSpace(mapped.Records[0].AppID)
		if _, duplicateInFile := seen[memorialID]; duplicateInFile {
			preview.WouldSkip++
			continue
		}
		exists, existsErr := memorialIDExists(conn, memorialID)
		if existsErr != nil {
			preview.WouldFail++
			preview.Issues = append(preview.Issues, importIssue(row, entry, existsErr))
			continue
		}
		if exists {
			preview.WouldSkip++
			continue
		}
		seen[memorialID] = struct{}{}
		preview.WouldCreate++
	}
	return preview, nil
}

// ImportMemorialArchive imports the confirmed memorial-archive rows into the Local Archive.
func (s *SoldierService) ImportMemorialArchive(path string) (MemorialImportSummary, error) {
	entries, format, err := loadMemorialArchive(path)
	if err != nil {
		return MemorialImportSummary{}, err
	}
	summary := MemorialImportSummary{
		FilePath:             strings.TrimSpace(path),
		Format:               format,
		ImportedByAppVersion: buildinfo.AppVersion,
		Warnings:             memorialFormatWarnings(format),
		TotalRows:            len(entries),
		Issues:               make([]MemorialImportIssue, 0),
	}
	batchID, err := db.NewSyncID()
	if err != nil {
		return MemorialImportSummary{}, err
	}
	summary.BatchID = batchID
	if err := ensureImportBatchRecord(s.db.Conn(), batchID, path); err != nil {
		return MemorialImportSummary{}, err
	}
	seen := map[string]struct{}{}
	conn := s.db.Conn()
	for idx, entry := range entries {
		row := idx + 1
		mapped, mapErr := mapMemorialEntry(entry)
		if mapErr != nil {
			summary.Failed++
			summary.Issues = append(summary.Issues, importIssue(row, entry, mapErr))
			continue
		}
		memorialID := strings.TrimSpace(mapped.Records[0].AppID)
		if _, duplicateInFile := seen[memorialID]; duplicateInFile {
			summary.Skipped++
			continue
		}
		exists, existsErr := memorialIDExists(conn, memorialID)
		if existsErr != nil {
			summary.Failed++
			summary.Issues = append(summary.Issues, importIssue(row, entry, existsErr))
			continue
		}
		if exists {
			summary.Skipped++
			continue
		}
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the Memorial JSON importer. The
		// service-layer default already covers empty values, but
		// explicit stamping documents the intent at the call site
		// and survives any future defaulting change.
		mapped.CreatedByImportPath = "memorial_json_import"
		created, createErr := s.Create(mapped)
		if createErr != nil {
			summary.Failed++
			summary.Issues = append(summary.Issues, importIssue(row, entry, createErr))
			continue
		}
		if err := setSoldierImportBatch(conn, created.ID, batchID); err != nil {
			_ = s.Delete(created.ID)
			summary.Failed++
			summary.Issues = append(summary.Issues, importIssue(row, entry, err))
			continue
		}
		seen[memorialID] = struct{}{}
		summary.Created++
	}
	return summary, nil
}

// loadMemorialArchive reads + parses a Memorial Archive file,
// returning the entries + the format metadata the scraper
// stamped (if any). Pre-v1 archives (bare JSON arrays with no
// envelope) parse cleanly: the entries decode, the format
// metadata is zero-valued, and the caller can detect the
// pre-v1 case via FormatVersion == "". Caller treats
// FormatVersion == "" as "missing / pre-v1" and warns in the
// summary (per Decision 2 of #383: warn + import).
func loadMemorialArchive(path string) ([]memorialArchiveEntry, MemorialImportFormat, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, MemorialImportFormat{}, fmt.Errorf("memorial archive path is required")
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, MemorialImportFormat{}, err
	}

	// Try the envelope shape first: {"format_version": "...", "entries": [...]}.
	// Detect by attempting to decode into a struct that has a
	// known envelope field; on failure, fall back to bare-array
	// decoding for pre-v1 backward compat.
	var envelope struct {
		FormatVersion string             `json:"format_version"`
		ScriptVersion string             `json:"script_version"`
		ScriptName    string             `json:"script_name"`
		Entries       []memorialArchiveEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.FormatVersion != "" {
		format := MemorialImportFormat{
			FormatVersion: envelope.FormatVersion,
			ScriptVersion: envelope.ScriptVersion,
			ScriptName:    envelope.ScriptName,
		}
		if err := checkMemorialFormatVersion(format); err != nil {
			return nil, format, err
		}
		return envelope.Entries, format, nil
	}

	// Fall back to the pre-v1 bare-array shape: [{...}, {...}].
	var entries []memorialArchiveEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, MemorialImportFormat{}, fmt.Errorf("parse memorial archive: %w", err)
	}
	return entries, MemorialImportFormat{}, nil
}

// checkMemorialFormatVersion implements Decision 2 of #383:
//   - same major + minor → no error
//   - missing / empty format_version (handled by caller, not here) → caller warns
//   - major bump → ErrMemorialFormatMismatch (caller surfaces refusal)
//   - minor bump → no error here; caller warns via the Warnings slice
func checkMemorialFormatVersion(format MemorialImportFormat) error {
	archiveMajor, archiveMinor, archiveOK := parseMemorialVersion(format.FormatVersion)
	expectedMajor, expectedMinor, expectedOK := parseMemorialVersion(buildinfo.MemorialArchiveFormatVersion)
	if !archiveOK || !expectedOK {
		// Either side unparseable; treat as mismatch so the user
		// notices. ErrMemorialFormatMismatch wraps the raw strings
		// so the UI can show what version was expected vs found.
		return fmt.Errorf("%w: archive format_version %q does not match the expected %q (unparseable)", ErrMemorialFormatMismatch, format.FormatVersion, buildinfo.MemorialArchiveFormatVersion)
	}
	if archiveMajor != expectedMajor {
		return fmt.Errorf("%w: archive format_version %q is a major bump from the expected %q — re-export from a scraper version that matches this DixieData build, or upgrade DixieData", ErrMemorialFormatMismatch, format.FormatVersion, buildinfo.MemorialArchiveFormatVersion)
	}
	_ = archiveMinor
	_ = expectedMinor
	return nil
}

// parseMemorialVersion splits "memorial_v1", "memorial_v1.2", or
// "memorial_v2.0" into (1, 0, true), (1, 2, true), (2, 0, true).
// Returns (0, 0, false) for empty / unrecognised shapes.
func parseMemorialVersion(s string) (major, minor int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	const prefix = "memorial_v"
	if !strings.HasPrefix(s, prefix) {
		return 0, 0, false
	}
	body := strings.TrimPrefix(s, prefix)
	parts := strings.SplitN(body, ".", 2)
	mj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	major = mj
	if len(parts) == 2 {
		mn, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
		minor = mn
	}
	return major, minor, true
}

func mapMemorialEntry(entry memorialArchiveEntry) (models.Soldier, error) {
	memorialID := strings.TrimSpace(entry.MemorialID)
	if memorialID == "" {
		return models.Soldier{}, fmt.Errorf("memorial_id is required")
	}
	firstName, middleName, lastName := splitImportedName(entry.Name)
	if firstName == "" && lastName == "" {
		return models.Soldier{}, fmt.Errorf("name is required")
	}
	birthDate := dates.ParseBirthInfo(entry.BirthDate)
	deathDate := dates.ParseBirthInfo(entry.DeathDate)
	buriedIn := joinNonBlank(", ", entry.BurialCemetery, entry.BurialLocation)
	notes := buildImportNotes(entry, birthDate, deathDate)
	details := strings.TrimSpace(entry.URL)
	if details == "" {
		details = "Find a Grave memorial"
	}
	return models.Soldier{
		EntryType:    "soldier",
		FirstName:    firstName,
		MiddleName:   middleName,
		LastName:     lastName,
		BirthDate:    birthDate,
		DeathDate:    deathDate,
		BirthInfo:    strings.TrimSpace(entry.BirthLocation),
		BuriedIn:     buriedIn,
		Biography:    strings.TrimSpace(entry.Biography),
		Notes:        notes,
		NeedsReview:  true,
		ReviewReason: "Imported from memorial JSON. Verify mapped details and relationships.",
		Records: []models.Record{{
			RecordType: memorialRecordType,
			AppID:      memorialID,
			Details:    details,
		}},
	}, nil
}

func splitImportedName(raw string) (string, string, string) {
	parts := strings.Fields(strings.TrimSpace(raw))
	switch len(parts) {
	case 0:
		return "", "", ""
	case 1:
		return parts[0], "", ""
	case 2:
		return parts[0], "", parts[1]
	default:
		return parts[0], strings.Join(parts[1:len(parts)-1], " "), parts[len(parts)-1]
	}
}

func buildImportNotes(entry memorialArchiveEntry, birthDate, deathDate string) string {
	lines := []string{
		"Imported via Memorial JSON",
	}
	if strings.TrimSpace(entry.BirthDate) != "" && birthDate == "" {
		lines = append(lines, "Birth Date (raw): "+strings.TrimSpace(entry.BirthDate))
	}
	if strings.TrimSpace(entry.DeathDate) != "" && deathDate == "" {
		lines = append(lines, "Death Date (raw): "+strings.TrimSpace(entry.DeathDate))
	}
	if strings.TrimSpace(entry.DeathLocation) != "" {
		lines = append(lines, "Death Location: "+strings.TrimSpace(entry.DeathLocation))
	}
	if len(entry.FamilyParents) > 0 {
		lines = append(lines, "Family Parents: "+strings.Join(compactStrings(entry.FamilyParents), "; "))
	}
	if strings.TrimSpace(entry.FamilySpouse) != "" {
		lines = append(lines, "Family Spouse: "+strings.TrimSpace(strings.ReplaceAll(entry.FamilySpouse, "\n", " ")))
	}
	if len(entry.FamilyChildren) > 0 {
		lines = append(lines, "Family Children: "+strings.Join(compactStrings(entry.FamilyChildren), "; "))
	}
	if strings.TrimSpace(entry.ScrapedAt) != "" {
		lines = append(lines, "Scraped At: "+strings.TrimSpace(entry.ScrapedAt))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func compactStrings(values []string) []string {
	next := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
		if trimmed != "" {
			next = append(next, trimmed)
		}
	}
	return next
}

func joinNonBlank(separator string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, separator)
}

func importIssue(row int, entry memorialArchiveEntry, err error) MemorialImportIssue {
	return MemorialImportIssue{
		Row:        row,
		MemorialID: strings.TrimSpace(entry.MemorialID),
		Name:       strings.TrimSpace(entry.Name),
		Error:      strings.TrimSpace(err.Error()),
	}
}

func memorialIDExists(conn *sql.DB, memorialID string) (bool, error) {
	trimmed := strings.TrimSpace(memorialID)
	if trimmed == "" {
		return false, fmt.Errorf("memorial_id is required")
	}
	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM records WHERE LOWER(TRIM(record_type)) = LOWER(TRIM(?)) AND TRIM(app_id) = ?`,
		memorialRecordType, trimmed,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func ensureImportBatchRecord(conn *sql.DB, batchID, archivePath string) error {
	_, err := conn.Exec(`INSERT OR IGNORE INTO import_batches (id, archive_path) VALUES (?, ?)`, strings.TrimSpace(batchID), filepath.Clean(strings.TrimSpace(archivePath)))
	return err
}

func setSoldierImportBatch(conn *sql.DB, soldierID int64, batchID string) error {
	if soldierID < 1 {
		return fmt.Errorf("invalid soldier id")
	}
	_, err := conn.Exec(`UPDATE soldiers SET import_batch_id = ? WHERE id = ?`, strings.TrimSpace(batchID), soldierID)
	return err
}

// memorialFormatWarnings returns the user-facing warning strings
// the preview / summary should surface based on the archive's
// format metadata. Decision 2 of #383: pre-v1 archives warn but
// import; minor bumps warn but import; major bumps refuse
// (handled by checkMemorialFormatVersion before this runs). The
// pre-v1 warning names the upgrade path (re-export from the
// updated scraper) so the user knows how to silence it.
func memorialFormatWarnings(format MemorialImportFormat) []string {
	out := make([]string, 0)
	if format.FormatVersion == "" {
		out = append(out, "archive has no format_version field — treating as pre-v1. Re-export from an updated FindAGrave scraper to silence this warning and enable future format-drift detection.")
		return out
	}
	if format.FormatVersion != buildinfo.MemorialArchiveFormatVersion {
		_, _, ok := parseMemorialVersion(format.FormatVersion)
		if ok {
			out = append(out, fmt.Sprintf("archive format_version %q differs from this build's expected %q — proceeding with a best-effort import. Review the imported rows before continuing.", format.FormatVersion, buildinfo.MemorialArchiveFormatVersion))
		}
	}
	return out
}
