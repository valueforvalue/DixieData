package archive

import (
	"database/sql"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

const (
	soldierSelectColumns = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at`
	recordSelectColumns  = `id, sync_id, soldier_id, soldier_sync_id, record_type, app_id, details`
	imageSelectColumns   = `id, sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary`
)

type SoldierService = records.SoldierService
type AnalyticsCount = records.AnalyticsCount
type AnalyticsSnapshot = records.AnalyticsSnapshot

func NewSoldierService(database *db.DB) *SoldierService { return records.NewSoldierService(database) }

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

func nextGoogleAnniversaryDate(soldier models.Soldier, now time.Time) time.Time {
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	for i := 0; i < 8; i++ {
		year := now.Year() + i
		candidate := time.Date(year, time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.Local)
		if candidate.Month() != time.Month(soldier.DeathMonth) || candidate.Day() != soldier.DeathDay {
			continue
		}
		if !candidate.Before(base) {
			return candidate
		}
	}
	return time.Date(now.Year(), time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.Local)
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
		notes                 sql.NullString
		reviewReason          sql.NullString
		addedBy               sql.NullString
		lastEditedBy          sql.NullString
		lastEditedFields      sql.NullString
		lastEditedAt          sql.NullString
		createdAt             sql.NullString
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
		nullStringDest(&s.Notes, &notes),
		&s.NeedsReview,
		nullStringDest(&s.ReviewReason, &reviewReason),
		nullStringDest(&s.AddedBy, &addedBy),
		nullStringDest(&s.LastEditedBy, &lastEditedBy),
		nullStringDest(&s.LastEditedFields, &lastEditedFields),
		nullStringDest(&s.LastEditedAt, &lastEditedAt),
		nullStringDest(&s.CreatedAt, &createdAt),
		nullStringDest(&s.UpdatedAt, &updatedAt),
	}
}

func normalizeConfederateHomeFields(soldier *models.Soldier) {
	soldier.ConfederateHomeStatus = normalizeConfederateHomeStatus(soldier.ConfederateHomeStatus)
	soldier.ConfederateHomeName = strings.TrimSpace(soldier.ConfederateHomeName)
	if soldier.ConfederateHomeStatus == "None" {
		soldier.ConfederateHomeName = ""
	}
}

func normalizeConfederateHomeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inmate":
		return "Inmate"
	case "staffer":
		return "Staffer"
	case "trustee":
		return "Trustee"
	default:
		return "None"
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
