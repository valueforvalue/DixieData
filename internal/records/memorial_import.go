package records

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const memorialRecordType = "Find a Grave"

type MemorialImportIssue struct {
	Row        int
	MemorialID string
	Name       string
	Error      string
}

type MemorialImportPreview struct {
	FilePath    string
	TotalRows   int
	WouldCreate int
	WouldSkip   int
	WouldFail   int
	Issues      []MemorialImportIssue
}

type MemorialImportSummary struct {
	FilePath  string
	BatchID   string
	TotalRows int
	Created   int
	Skipped   int
	Failed    int
	Issues    []MemorialImportIssue
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

func (s *SoldierService) PreviewMemorialArchive(path string) (MemorialImportPreview, error) {
	entries, err := loadMemorialArchive(path)
	if err != nil {
		return MemorialImportPreview{}, err
	}
	preview := MemorialImportPreview{
		FilePath:  strings.TrimSpace(path),
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

func (s *SoldierService) ImportMemorialArchive(path string) (MemorialImportSummary, error) {
	entries, err := loadMemorialArchive(path)
	if err != nil {
		return MemorialImportSummary{}, err
	}
	summary := MemorialImportSummary{
		FilePath:  strings.TrimSpace(path),
		TotalRows: len(entries),
		Issues:    make([]MemorialImportIssue, 0),
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

func loadMemorialArchive(path string) ([]memorialArchiveEntry, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, fmt.Errorf("memorial archive path is required")
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, err
	}
	var entries []memorialArchiveEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse memorial archive: %w", err)
	}
	return entries, nil
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
