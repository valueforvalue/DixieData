// This file re-exports types from internal/records + pkg/render as
// type aliases so the appshell can depend on internal/archive alone
// (rather than importing both records + render). Each alias is
// documented at the canonical definition; the alias itself is
// non-canonical and exists for import-path convenience.
package archive

import (
	"database/sql"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/pkg/render"
)

const (
	soldierSelectColumns = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, biography, pdf_excerpt_override, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, kind, begin_date, end_date, description`
	recordSelectColumns  = `id, sync_id, person_record_id, person_sync_id, record_type, app_id, details`
	imageSelectColumns   = `id, sync_id, person_record_id, person_sync_id, file_name, file_path, caption, is_primary`
)

// SoldierService is a re-export of records.SoldierService (or pkg/render.SoldierService for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type SoldierService = records.SoldierService
// AnniversaryService is a re-export of records.AnniversaryService (or pkg/render.AnniversaryService for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type AnniversaryService = records.AnniversaryService
// AnalyticsService is a re-export of records.AnalyticsService (or pkg/render.AnalyticsService for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type AnalyticsService = records.AnalyticsService
// AnalyticsCount is a re-export of records.AnalyticsCount (or pkg/render.AnalyticsCount for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type AnalyticsCount = records.AnalyticsCount
// AnalyticsSnapshot is a re-export of records.AnalyticsSnapshot (or pkg/render.AnalyticsSnapshot for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type AnalyticsSnapshot = records.AnalyticsSnapshot
// PrintSettings is a re-export of records.PrintSettings (or pkg/render.PrintSettings for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type PrintSettings = render.PrintSettings
// PDFOptions is a re-export of records.PDFOptions (or pkg/render.PDFOptions for the render types). See the canonical definition in the source package for the contract; the alias exists for import-path convenience.
type PDFOptions = render.PDFOptions
const (
	PrintSortLastName  = render.PrintSortLastName
	PrintSortBirthYear = render.PrintSortBirthYear
	PrintSortDeathYear = render.PrintSortDeathYear
	PrintScopeAll      = render.PrintScopeAll
	PrintScopeFiltered = render.PrintScopeFiltered
	PrintScopeSelected = render.PrintScopeSelected
)

// NewSoldierService is a re-export of records.NewSoldierService (or pkg/render.NewSoldierService). See the source package for the canonical implementation.
func NewSoldierService(database *db.DB) *SoldierService { return records.NewSoldierService(database) }
// NewAnniversaryService is a re-export of records.NewAnniversaryService (or pkg/render.NewAnniversaryService). See the source package for the canonical implementation.
func NewAnniversaryService(database *db.DB) *AnniversaryService { return records.NewAnniversaryService(database) }
// NewAnalyticsService is a re-export of records.NewAnalyticsService (or pkg/render.NewAnalyticsService). See the source package for the canonical implementation.
func NewAnalyticsService(database *db.DB) *AnalyticsService { return records.NewAnalyticsService(database) }

func nullableInt64(value int64) interface{} {
	if value < 1 {
		return nil
	}
	return value
}

func isGeneratedDisplayID(displayID string) bool {
	_, _, ok := db.CanonicalDisplayID(db.SanitizeID(displayID, ""))
	return ok
}

func nextGoogleAnniversaryDate(soldier models.Soldier, now time.Time, location *time.Location) time.Time {
	// Build base + candidate in the caller's location, not time.Local.
	// Same rationale as the integrations copy: UTC CI runners would
	// otherwise shift the candidate's calendar day when the caller
	// subsequently converts to a non-UTC location.
	if location == nil {
		location = time.Local
	}
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	for i := 0; i < 8; i++ {
		year := now.Year() + i
		candidate := time.Date(year, time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, location)
		if candidate.Month() != time.Month(soldier.DeathMonth) || candidate.Day() != soldier.DeathDay {
			continue
		}
		if !candidate.Before(base) {
			return candidate
		}
	}
	return time.Date(now.Year(), time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, location)
}

func scanSoldier(row *sql.Row) (*models.Soldier, error) {
	var s models.Soldier
	err := row.Scan(soldierScanDest(&s)...)
	if err != nil {
		return nil, err
	}
	hydrateLegacyDeathParts(&s)
	normalizeConfederateHomeFields(&s)
	return &s, nil
}

func soldierScanDest(s *models.Soldier) []interface{} {
	var (
		displayID             sql.NullString
		syncID                sql.NullString
		entryType             sql.NullString
		maidenName            sql.NullString
		relationshipLabel     sql.NullString
		spouseSoldierID       sql.NullInt64
		pensionID             sql.NullString
		applicationID         sql.NullString
		prefix                sql.NullString
		showPrefixBeforeName  sql.NullBool
		firstName             sql.NullString
		middleName            sql.NullString
		lastName              sql.NullString
		suffix                sql.NullString
		rank                  sql.NullString
		rankIn                sql.NullString
		rankOut               sql.NullString
		unit                  sql.NullString
		pensionState          sql.NullString
		confederateHomeStatus sql.NullString
		confederateHomeName   sql.NullString
		birthInfo             sql.NullString
		buriedIn              sql.NullString
		biography             sql.NullString
		pdfExcerptOverride    sql.NullString
		notes                 sql.NullString
		reviewReason          sql.NullString
		addedBy               sql.NullString
		lastEditedBy          sql.NullString
		lastEditedFields      sql.NullString
		lastEditedAt          sql.NullString
		createdAt             sql.NullString
		kind                  sql.NullString
		beginDate             sql.NullString
		endDate               sql.NullString
		description           sql.NullString
		deathYear             sql.NullInt64
		deathMonth            sql.NullInt64
		deathDay              sql.NullInt64
		birthDate             sql.NullString
		deathDate             sql.NullString
		updatedAt             sql.NullString
	)

	return []interface{}{
		&s.ID,
		nullStringDest(&s.DisplayID, &displayID),
		nullStringDest(&s.SyncID, &syncID),
		nullStringDest(&s.EntryType, &entryType),
		nullInt64Dest(&s.SpouseSoldierID, &spouseSoldierID),
		nullStringDest(&s.RelationshipLabel, &relationshipLabel),
		nullStringDest(&s.MaidenName, &maidenName),
		&s.IsGenerated,
		nullStringDest(&s.PensionID, &pensionID),
		nullStringDest(&s.ApplicationID, &applicationID),
		nullStringDest(&s.Prefix, &prefix),
		nullBoolDest(&s.ShowPrefixBeforeName, &showPrefixBeforeName),
		nullStringDest(&s.FirstName, &firstName),
		nullStringDest(&s.MiddleName, &middleName),
		nullStringDest(&s.LastName, &lastName),
		nullStringDest(&s.Suffix, &suffix),
		nullStringDest(&s.Rank, &rank),
		nullStringDest(&s.RankIn, &rankIn),
		nullStringDest(&s.RankOut, &rankOut),
		nullStringDest(&s.Unit, &unit),
		nullStringDest(&s.PensionState, &pensionState),
		nullStringDest(&s.ConfederateHomeStatus, &confederateHomeStatus),
		nullStringDest(&s.ConfederateHomeName, &confederateHomeName),
		nullIntDest(&s.DeathYear, &deathYear),
		nullIntDest(&s.DeathMonth, &deathMonth),
		nullIntDest(&s.DeathDay, &deathDay),
		nullStringDest(&s.BirthDate, &birthDate),
		nullStringDest(&s.DeathDate, &deathDate),
		nullStringDest(&s.BirthInfo, &birthInfo),
		nullStringDest(&s.BuriedIn, &buriedIn),
		nullStringDest(&s.Biography, &biography),
		nullStringDest(&s.PDFExcerptOverride, &pdfExcerptOverride),
		nullStringDest(&s.Notes, &notes),
		&s.NeedsReview,
		nullStringDest(&s.ReviewReason, &reviewReason),
		nullStringDest(&s.AddedBy, &addedBy),
		nullStringDest(&s.LastEditedBy, &lastEditedBy),
		nullStringDest(&s.LastEditedFields, &lastEditedFields),
		nullStringDest(&s.LastEditedAt, &lastEditedAt),
		nullStringDest(&s.CreatedAt, &createdAt),
		nullStringDest(&s.UpdatedAt, &updatedAt),
		nullStringDest(&s.Kind, &kind),
		nullStringDest(&s.BeginDate, &beginDate),
		nullStringDest(&s.EndDate, &endDate),
		nullStringDest(&s.Description, &description),
	}
}

func normalizeConfederateHomeFields(soldier *models.Soldier) {
	soldier.ConfederateHomeStatus = confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)
	soldier.ConfederateHomeName = strings.TrimSpace(soldier.ConfederateHomeName)
	if soldier.ConfederateHomeStatus == confederatehomestatus.NotApplicable {
		soldier.ConfederateHomeName = ""
	}
}

func nullStringDest(target *string, holder *sql.NullString) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = holder.String
		} else {
			*target = ""
		}
		return nil
	})
}

func nullIntDest(target *int, holder *sql.NullInt64) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = int(holder.Int64)
		} else {
			*target = 0
		}
		return nil
	})
}

func nullBoolDest(target *bool, holder *sql.NullBool) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = holder.Bool
		} else {
			*target = false
		}
		return nil
	})
}

func nullInt64Dest(target *int64, holder *sql.NullInt64) interface{ Scan(any) error } {
	return scannerFunc(func(value any) error {
		if err := holder.Scan(value); err != nil {
			return err
		}
		if holder.Valid {
			*target = holder.Int64
		} else {
			*target = 0
		}
		return nil
	})
}

type scannerFunc func(any) error

// Scan is the archive-layer method matching its name.
func (f scannerFunc) Scan(value any) error { return f(value) }

func hydrateLegacyDeathParts(soldier *models.Soldier) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.DeathDate))
	if err != nil {
		return
	}
	soldier.DeathMonth = partial.Month
	soldier.DeathDay = partial.Day
	soldier.DeathYear = partial.Year
}
