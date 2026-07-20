package records

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/db/repo"
	sqliterepo "github.com/valueforvalue/DixieData/internal/db/repo/sqlite"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

const (
	soldierSelectColumns     = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, biography, pdf_excerpt_override, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, kind, begin_date, end_date, description, created_by_version, created_by_import_path, restored_at`
	soldierListSelectColumns = soldierSelectColumns + `, COALESCE((SELECT display_id FROM soldiers linked WHERE linked.id = soldiers.spouse_soldier_id), ''), (SELECT COUNT(*) FROM records WHERE records.person_record_id = soldiers.id), (SELECT COUNT(*) FROM images WHERE images.person_record_id = soldiers.id)`
	recordSelectColumns      = `id, sync_id, person_record_id, person_sync_id, record_type, app_id, details, sort_order`
	imageSelectColumns       = `id, sync_id, person_record_id, person_sync_id, file_name, file_path, caption, is_primary`
)

// SoldierService is the central domain service: CRUD on Soldiers
// (Person Records), the form-suggestions cache, the search
// facade the browse + soldiers-list pages use, and the merge
// pipeline that handles Local-vs-Incoming conflict resolution.
// Constructed by NewSoldierService and held by *App. Every other
// domain service (Backup, Export, Analytics) depends on
// *SoldierService for soldier lookups.
type SoldierService struct {
	db                *db.DB
	personRepo        repo.PersonRecordRepo
	qualityRepo       repo.QualityScanRepo
	memorialRepo      repo.MemorialImportRepo
	events            EventTimelineQuerier
	formSuggestionsMu sync.RWMutex
	formSuggestions   *models.SoldierFormSuggestions
}

// ServiceTimeline returns the per-soldier chronological service timeline (enlistment, transfer, wound, discharge, death).
type ServiceTimeline struct {
	Central            models.Soldier
	Events             []ServiceTimelineEvent
	UndatedRecords     []models.Record
	StartLabel         string
	EndLabel           string
	ExactEventCount    int
	InferredEventCount int
}

// ServiceTimelineEvent is a records-layer type used by the matching service.
type ServiceTimelineEvent struct {
	Title           string
	DateLabel       string
	Description     string
	SourceLabel     string
	Category        string
	ConfidenceLabel string
	Approximate     bool
	sortDate        dates.PartialDate
	sortOrder       int
}

// ResearchTask is a records-layer type used by the matching service.
type ResearchTask struct {
	ID           int64
	SoldierID    int64
	Title        string
	Notes        string
	EvidenceType string
	Status       string
	CreatedAt    string
	UpdatedAt    string
	ResolvedAt   string
}

// ResearchTaskSuggestion is a records-layer type used by the matching service.
type ResearchTaskSuggestion struct {
	Title        string
	Notes        string
	EvidenceType string
}

// ResearchLog returns the per-soldier Research Log: open tasks + dismissed suggestions + recently completed.
type ResearchLog struct {
	Central       models.Soldier
	Tasks         []ResearchTask
	Suggestions   []ResearchTaskSuggestion
	OpenCount     int
	ResolvedCount int
}

// ResearchCollection is a records-layer type used by the matching service.
type ResearchCollection struct {
	ID              int64
	Name            string
	Description     string
	CreatedAt       string
	UpdatedAt       string
	ItemCount       int
	ContainsCurrent bool
}

// ResearchCollectionHub is a records-layer type used by the matching service.
type ResearchCollectionHub struct {
	Current     *models.Soldier
	Collections []ResearchCollection
}

// ResearchCollectionDetail returns the per-collection detail page (the Soldiers in the collection, sorted).
type ResearchCollectionDetail struct {
	Collection ResearchCollection
	Current    *models.Soldier
	Members    []models.Soldier
}

// NewSoldierService constructs a SoldierService bound to the
// given database. The form-suggestions cache starts nil; it is
// populated lazily on the first form-suggestions request and
// refreshed when the soldier writes a new value.
func NewSoldierService(database *db.DB) *SoldierService {
	return &SoldierService{
		db:           database,
		personRepo:   sqliterepo.NewPersonRecordRepo(database),
		qualityRepo:  sqliterepo.NewQualityScanRepo(database),
		memorialRepo: sqliterepo.NewMemorialImportRepo(database),
	}
}

// EventTimelineQuerier is the narrow seam SoldierService
// consumes for the linked-events-for-timeline query. Defined
// at the SoldierService package boundary (per the two-adapter
// rule from codebase-design) so the consumer doesn't depend
// on the concrete EventService. *EventService satisfies this
// implicitly; tests can substitute a fake without dragging
// the whole Event facade.
//
// Issue #491 widens the seam with Count + KindRollup so the
// appshell inventory handler can call the same EventService
// instance SoldierService is wired to without reaching past
// the seam. The two methods are read-only inventory rollups
// and stay in the same file as the timeline querier.
type EventTimelineQuerier interface {
	LinkedEventsForTimeline(personID int64) ([]LinkedEventTimelineMarker, error)
	Count() (int, error)
	KindRollup() ([]EventKindCount, error)
}

// SetEvents wires the back-reference from SoldierService to
// the Event-side timeline querier. Issue #343 finding #5
// moved the linked-events-for-timeline query out of
// SoldierService (which otherwise doesn't touch the Event-
// side schema) into EventService (where the JOIN belongs).
// The back-reference lets ServiceTimeline delegate the query
// instead of carrying schema knowledge it shouldn't own.
//
// Production wiring (appshell/app.go) and the test bootstrap
// (records/*_test.go) must call SetEvents after constructing
// both services. If unset, ServiceTimeline skips the
// linked-event markers (no events to surface) — the rest of
// the timeline (Birth / Death / records / etc.) still renders.
func (s *SoldierService) SetEvents(events EventTimelineQuerier) {
	s.events = events
}

// Create persists a new Soldier and returns the assigned ID. Sets CreatedAt + UpdatedAt; the caller is responsible for the display ID.
func (s *SoldierService) Create(soldier models.Soldier) (*models.Soldier, error) {
	conn := s.db.Conn()
	generatedDisplayID := strings.TrimSpace(soldier.DisplayID) == ""
	if soldier.DisplayID == "" {
		id, err := s.db.NextDXDID()
		if err != nil {
			return nil, err
		}
		soldier.DisplayID = id
	}
	nodePrefix, err := s.db.NodePrefix()
	if err != nil {
		return nil, err
	}
	soldier.DisplayID = normalizeDisplayID(soldier.DisplayID, nodePrefix)
	if strings.TrimSpace(soldier.SyncID) == "" {
		soldier.SyncID, err = db.NewSyncID()
		if err != nil {
			return nil, err
		}
	}

	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := normalizeSoldierEntry(tx, &soldier); err != nil {
		return nil, err
	}
	normalizeConfederateHomeFields(&soldier)
	if err := normalizeSoldierDates(&soldier); err != nil {
		return nil, err
	}
	soldier.IsGenerated = generatedDisplayID || isGeneratedDisplayID(soldier.DisplayID)
	soldier.Rank = canonicalRank(soldier)
	if strings.TrimSpace(soldier.CreatedAt) == "" {
		soldier.CreatedAt = currentSQLiteTimestamp()
	}
	if strings.TrimSpace(soldier.UpdatedAt) == "" {
		soldier.UpdatedAt = soldier.CreatedAt
	}
	stampCreateAuditFields(s.currentAuditActor(), &soldier)

	// Issue #377 slice 2: default created_by_version to the
	// running binary's version + created_by_import_path to
	// "create_soldier" when the caller leaves them empty.
	// Slice 1 left them empty (backward compat); slice 2 flips
	// the policy so existing call sites stamp a sensible value
	// without code changes. Callers that want a more-specific
	// path (memorial_json_import, cli_export, etc.) keep
	// setting the field explicitly.
	if strings.TrimSpace(soldier.CreatedByVersion) == "" {
		soldier.CreatedByVersion = versioninfo.AppVersion()
	}
	if strings.TrimSpace(soldier.CreatedByImportPath) == "" {
		soldier.CreatedByImportPath = "create_soldier"
	}

	// Slice 2 of issue #613: the INSERT itself goes through
	// the repository seam. The pre-INSERT normalization
	// (Display ID generation, sync_id minting, audit
	// timestamps, entry_type canonicalization) stays in
	// this service layer — it composes domain rules; the
	// repo just executes the SQL.
	id, err := s.personRepo.Create(context.Background(), tx, soldier)
	if err != nil {
		return nil, err
	}
	soldier.ID = id

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.invalidateFormSuggestions()
	return &soldier, nil
}

func isGeneratedDisplayID(displayID string) bool {
	_, _, ok := db.CanonicalDisplayID(db.SanitizeID(displayID, ""))
	return ok
}

func isFiveDigitGeneratedSuffix(value string) bool {
	if len(value) != 5 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// GetByID returns the Soldier with the given primary-key ID, or ErrSoldierNotFound.
func (s *SoldierService) GetByID(id int64) (*models.Soldier, error) {
	// Slice 1 of issue #613: base row fetch goes through the
	// repository seam. Cross-table joins (records, images,
	// spouse lookup) stay inline until their own repos land
	// in slice 2+. The seam's value is proven on the single-
	// table read; extending it later is mechanical.
	row, err := s.personRepo.GetByID(context.Background(), id)
	if err != nil {
		return nil, err
	}
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}

	conn := s.db.Conn()
	rows, err := conn.Query(`SELECT `+recordSelectColumns+` FROM records WHERE person_record_id = ? ORDER BY sort_order, id`, id)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "GetByID.records")
	for rows.Next() {
		var r models.Record
		if err := rows.Scan(&r.ID, &r.SyncID, &r.PersonRecordID, &r.PersonSyncID, &r.RecordType, &r.AppID, &r.Details, &r.SortOrder); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, r)
	}

	imgRows, err := conn.Query(`SELECT `+imageSelectColumns+` FROM images WHERE person_record_id = ? ORDER BY is_primary DESC, id`, id)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(imgRows, "GetByID.images")
	for imgRows.Next() {
		var img models.Image
		if err := imgRows.Scan(&img.ID, &img.SyncID, &img.PersonRecordID, &img.PersonSyncID, &img.FileName, &img.FilePath, &img.Caption, &img.IsPrimary); err != nil {
			return nil, err
		}
		soldier.Images = append(soldier.Images, img)
	}
	soldier.SpouseName = spouseReference(conn, soldier.SpouseSoldierID)
	soldier.SpouseDisplayID = spouseDisplayID(conn, soldier.SpouseSoldierID)

	return soldier, nil
}

// GetByDisplayID returns the Soldier with the given display ID (e.g. 'P-0042'), or ErrSoldierNotFound.
func (s *SoldierService) GetByDisplayID(displayID string) (*models.Soldier, error) {
	trimmed := strings.TrimSpace(displayID)
	if trimmed == "" {
		return nil, os.ErrNotExist
	}

	conn := s.db.Conn()
	row := conn.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE upper(display_id) = upper(?)`, trimmed)
	soldier, err := scanSoldier(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	rows, err := conn.Query(`SELECT `+recordSelectColumns+` FROM records WHERE person_record_id = ? ORDER BY sort_order, id`, soldier.ID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "GetByDisplayID.records")
	for rows.Next() {
		var record models.Record
		if err := rows.Scan(&record.ID, &record.SyncID, &record.PersonRecordID, &record.PersonSyncID, &record.RecordType, &record.AppID, &record.Details, &record.SortOrder); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, record)
	}

	imgRows, err := conn.Query(`SELECT `+imageSelectColumns+` FROM images WHERE person_record_id = ? ORDER BY is_primary DESC, id`, soldier.ID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(imgRows, "GetByDisplayID.images")
	for imgRows.Next() {
		var img models.Image
		if err := imgRows.Scan(&img.ID, &img.SyncID, &img.PersonRecordID, &img.PersonSyncID, &img.FileName, &img.FilePath, &img.Caption, &img.IsPrimary); err != nil {
			return nil, err
		}
		soldier.Images = append(soldier.Images, img)
	}
	soldier.SpouseName = spouseReference(conn, soldier.SpouseSoldierID)
	soldier.SpouseDisplayID = spouseDisplayID(conn, soldier.SpouseSoldierID)

	return soldier, nil
}

// Update replaces the Soldier row identified by input.ID; touches UpdatedAt. Returns ErrSoldierNotFound if the row vanished.
func (s *SoldierService) Update(soldier models.Soldier) error {
	conn := s.db.Conn()
	nodePrefix, err := s.db.NodePrefix()
	if err != nil {
		return err
	}
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	before, err := loadSoldierAuditSnapshot(tx, soldier.ID)
	if err != nil {
		return err
	}

	soldier.Rank = canonicalRank(soldier)
	soldier.DisplayID = normalizeDisplayID(soldier.DisplayID, nodePrefix)
	if soldier.DisplayID == "" {
		if before != nil && strings.TrimSpace(before.DisplayID) != "" {
			soldier.DisplayID = before.DisplayID
		} else {
			return fmt.Errorf("soldier service: refusing to blank display_id for soldier %d", soldier.ID)
		}
	}
	if err := hydrateSoldierIdentity(tx, &soldier); err != nil {
		return err
	}
	if err := normalizeSoldierEntry(tx, &soldier); err != nil {
		return err
	}
	normalizeConfederateHomeFields(&soldier)
	if err := normalizeSoldierDates(&soldier); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.UpdatedAt) == "" {
		soldier.UpdatedAt = currentSQLiteTimestamp()
	}
	stampUpdateAuditFields(s.currentAuditActor(), before, &soldier)

	// Slice 2 of issue #613: the UPDATE itself goes through
	// the repository seam. Pre-UPDATE normalization (audit
	// snapshot, rank canonicalization, display_id
	// fallback, entry_type canonicalization) stays in this
	// service layer. The legacy behavior — Update on a
	// missing id is a silent no-op (rowsAffected=0, no
	// error) — is preserved for backwards compatibility
	// with existing callers (TestSoldierService_Update*).
	if _, err := s.personRepo.Update(context.Background(), tx, soldier); err != nil {
		return err
	}

	if err := replaceRecords(tx, soldier.ID, soldier.SyncID, soldier.Records); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateFormSuggestions()
	return nil
}

// Delete removes the Soldier and (via cascade) the attached records, images, and tag-join rows.
func (s *SoldierService) Delete(id int64) error {
	// Slice 2 of issue #613: the DELETE itself goes through
	// the repository seam. Like Update, the legacy behavior
	// (Delete on a missing id is a silent no-op) is preserved.
	// Delete stands alone (no surrounding transaction), so
	// the caller passes *sql.DB as the Execer.
	if _, err := s.personRepo.Delete(context.Background(), s.db.Conn(), id); err != nil {
		return err
	}
	s.invalidateFormSuggestions()
	return nil
}

// AddImage attaches an image to the Soldier; computes SHA-256 for dedup.
func (s *SoldierService) AddImage(soldierID int64, fileName, filePath, caption string) error {
	soldierSyncID, err := s.soldierSyncIDByID(soldierID)
	if err != nil {
		return err
	}
	imageSyncID, err := db.NewSyncID()
	if err != nil {
		return err
	}
	isPrimary, err := s.shouldAssignPrimaryImage(soldierID)
	if err != nil {
		return err
	}
	_, err = s.db.Conn().Exec(
		`INSERT INTO images (sync_id, person_record_id, person_sync_id, file_name, file_path, caption, is_primary) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		imageSyncID,
		soldierID,
		soldierSyncID,
		fileName,
		filePath,
		caption,
		isPrimary,
	)
	if err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "images")
}

// DeleteImages removes the image + tag-join rows for the given image IDs. Returns the count actually deleted.
func (s *SoldierService) DeleteImages(soldierID int64, imageIDs []int64) error {
	if len(imageIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(imageIDs))
	args := make([]interface{}, 0, len(imageIDs)+1)
	args = append(args, soldierID)
	for index, imageID := range imageIDs {
		placeholders[index] = "?"
		args = append(args, imageID)
	}

	_, err := s.db.Conn().Exec(
		fmt.Sprintf(`DELETE FROM images WHERE person_record_id = ? AND id IN (%s)`, strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return err
	}
	if err := s.ensurePrimaryImage(soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "images")
}

// SetPrimaryImage marks one image as the portrait shown on the Person Record header.
func (s *SoldierService) SetPrimaryImage(soldierID, imageID int64) error {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE person_record_id = ? AND id = ?`, soldierID, imageID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if _, err := s.db.Conn().Exec(`UPDATE images SET is_primary = CASE WHEN id = ? THEN 1 ELSE 0 END WHERE person_record_id = ?`, imageID, soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "primary_image")
}

// GetImageByID returns a single image row.
func (s *SoldierService) GetImageByID(imageID int64) (*models.Image, error) {
	row := s.db.Conn().QueryRow(`SELECT `+imageSelectColumns+` FROM images WHERE id = ?`, imageID)
	var image models.Image
	if err := row.Scan(&image.ID, &image.SyncID, &image.PersonRecordID, &image.PersonSyncID, &image.FileName, &image.FilePath, &image.Caption, &image.IsPrimary); err != nil {
		return nil, err
	}
	return &image, nil
}

// CountNeedsReview returns the number of records currently flagged
// for review. Used by the layout's top-nav badge (issue #180) so
// users see pending review work from any page without navigating
// to /review-queue. Single-digit ms with a partial index on
// needs_review; no caching (count changes whenever any page
// flags or resolves a record).
func (s *SoldierService) CountNeedsReview() (int, error) {
	var count int
	err := s.db.Conn().QueryRow(`SELECT COUNT(*) FROM soldiers WHERE needs_review = 1`).Scan(&count)
	return count, err
}

// InventoryMetricsRaw is the storage-side shape the activity
// rollup helper returns. The viewmodel layer (which cannot
// import the records package — cycle) gets a converted copy
// via appshell.inventory_handlers.go.
//
// Issue #580: the rollup covers primary entries (Person Record
// subtypes + Event Records + live Articles). Article Snapshots
// are excluded by the storage query so the activity count
// matches the existing Articles inventory headline number.
type InventoryMetricsRaw struct {
	EntriesPerDay  map[string]int
	// EntriesPerDayByKind is the per-kind per-day breakdown
	// the /inventory Activity metrics line graph renders
	// (issue #583). The outer map is keyed by kind
	// ("soldier" / "spouse" / "linked" / "event" / "article")
	// and always carries every kind even on an empty archive
	// so the templ partial never has to nil-check an inner
	// map. The inner map is keyed by YYYY-MM-DD with the
	// day's per-kind count; missing days are absent (not zero).
	EntriesPerDayByKind  map[string]map[string]int
	FirstEntryDate string
	LatestEntryDate string
	ActiveDayCount int
	TotalsByType   InventoryMetricTotalsRaw
}

// InventoryMetricTotalsRaw is the per-entry-type activity count
// the Metrics summary line renders. The numbers must match the
// headline counts in ArchiveCounts so a future storage
// regression that drops an entry type trips the smoke probe.
type InventoryMetricTotalsRaw struct {
	Soldiers       int
	SpouseRecords  int
	LinkedPersons  int
	EventRecords   int
	Articles       int
}

// ActivityMetrics returns the activity rollup that powers the
// /inventory page's Metrics section (issue #580 slice 1).
//
// The query unions the five primary-entry tables and groups
// by stored created_at date so:
//   - The day-bucketed entries-per-day map is the true
//     longitudinal activity shape of the Local Archive.
//   - The totals cross-check the headline counts in ArchiveCounts
//     so a render regression (or a storage migration that
//     dropped an entry type) trips the smoke probe.
//   - Article Snapshots are excluded by the is_snapshot = 0
//     predicate on articles, matching the headline Articles count
//     from ArchiveCounts (which already excludes snapshots).
//
// The query uses UNION ALL on subqueries instead of a single
// pass over the soldiers + articles tables because the
// schema-level split (Person Records / Spouse Records / Linked
// Persons / Event Records live on `soldiers`; live Articles
// live on `articles`) and the union is the simplest shape that
// preserves both buckets' created_at semantics. SQLite evaluates
// each side as a tiny indexed scan; on a 10k-row archive the
// whole call is well under 10ms.
func (s *SoldierService) ActivityMetrics(ctx context.Context) (InventoryMetricsRaw, error) {
	_ = ctx
	conn := s.db.Conn()
	rows, err := conn.Query(`
		WITH activity AS (
			SELECT date(created_at) AS day, 'soldier' AS kind FROM soldiers
			WHERE entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier'
			UNION ALL
			SELECT date(created_at), 'spouse' FROM soldiers
			WHERE LOWER(TRIM(entry_type)) IN ('wife', 'widow')
			UNION ALL
			SELECT date(created_at), 'linked' FROM soldiers
			WHERE LOWER(TRIM(entry_type)) = 'linked_person'
			UNION ALL
			SELECT date(created_at), 'event' FROM soldiers
			WHERE LOWER(TRIM(entry_type)) = 'event'
			UNION ALL
			SELECT date(created_at), 'article' FROM articles
			WHERE is_snapshot = 0
		)
		SELECT day, kind, COUNT(*) FROM activity WHERE day IS NOT NULL GROUP BY day, kind ORDER BY day ASC
	`)
	if err != nil {
		return InventoryMetricsRaw{}, err
	}
	defer debug.DeferCloseLog(rows, "ActivityMetrics.rows")

	out := InventoryMetricsRaw{
		EntriesPerDay: make(map[string]int),
		// Per-kind buckets always carry every kind so the
		// templ partial never nil-checks (issue #583 slice 1).
		EntriesPerDayByKind: map[string]map[string]int{
			"soldier": {},
			"spouse":  {},
			"linked":  {},
			"event":   {},
			"article": {},
		},
	}
	first := ""
	latest := ""
	for rows.Next() {
		var day string
		var kind string
		var count int
		if err := rows.Scan(&day, &kind, &count); err != nil {
			return InventoryMetricsRaw{}, err
		}
		out.EntriesPerDay[day] += count
		if bucket, ok := out.EntriesPerDayByKind[kind]; ok {
			bucket[day] += count
		}
		switch kind {
		case "soldier":
			out.TotalsByType.Soldiers += count
		case "spouse":
			out.TotalsByType.SpouseRecords += count
		case "linked":
			out.TotalsByType.LinkedPersons += count
		case "event":
			out.TotalsByType.EventRecords += count
		case "article":
			out.TotalsByType.Articles += count
		}
		if first == "" || day < first {
			first = day
		}
		if day > latest {
			latest = day
		}
	}
	if err := rows.Err(); err != nil {
		return InventoryMetricsRaw{}, err
	}
	out.FirstEntryDate = first
	out.LatestEntryDate = latest
	out.ActiveDayCount = len(out.EntriesPerDay)
	return out, nil
}

// ArchiveCounts returns the headline-number rollup (soldiers, wives/widows, linked people) for the Insights page header.
//
// Issue #491: the query also pulls Event Records + Articles + Tags counts
// in a single round-trip via scalar subqueries, so the same call site
// powers the Insights page header, the Calendar header archive rollup,
// and the new /inventory page. The three extra subqueries are O(1) on
// the tags + articles tables (table row count with the primary key
// index) and O(soldiers) on the event-count subquery — acceptable for
// the inventory rollup shape (a single-digit-millisecond sweep on a
// 10k-row archive).
func (s *SoldierService) ArchiveCounts() (models.ArchiveCounts, error) {
	row := s.db.Conn().QueryRow(`
		SELECT
			COALESCE(SUM(CASE
				WHEN entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier' THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN LOWER(TRIM(entry_type)) IN ('wife', 'widow') THEN 1
				ELSE 0
			END), 0),
			COALESCE(SUM(CASE
				WHEN LOWER(TRIM(entry_type)) = 'linked_person' THEN 1
				ELSE 0
			END), 0),
			(SELECT COUNT(*) FROM soldiers WHERE LOWER(TRIM(entry_type)) = 'event'),
			(SELECT COUNT(*) FROM articles WHERE is_snapshot = 0),
			(SELECT COUNT(*) FROM tags)
		FROM soldiers`)
	var counts models.ArchiveCounts
	if err := row.Scan(
		&counts.TotalSoldiers,
		&counts.TotalWivesWidows,
		&counts.TotalLinkedPeople,
		&counts.EventRecords,
		&counts.Articles,
		&counts.Tags,
	); err != nil {
		return models.ArchiveCounts{}, err
	}
	return counts, nil
}

// ReviewQueue returns the user's pending review items: unresolved duplicate-audit findings + unresolved merge-review conflicts.
func (s *SoldierService) ReviewQueue(page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	conn := s.db.Conn()
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM soldiers WHERE needs_review = 1`).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	rows, err := conn.Query(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE needs_review = 1 ORDER BY updated_at DESC, last_name, first_name LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "ReviewQueue.rows")
	soldiers, err := scanSoldiers(rows)
	return soldiers, total, err
}

// MarkReviewResolved marks one review item as resolved (kind: merge conflict or duplicate finding).
func (s *SoldierService) MarkReviewResolved(soldierID int64) error {
	if _, err := s.db.Conn().Exec(`UPDATE soldiers SET needs_review = 0, review_reason = '' WHERE id = ?`, soldierID); err != nil {
		return err
	}
	return s.touchAuditFields(soldierID, "review_status")
}

// SetReviewStatus sets the per-review-item status (open / dismissed / resolved).
func (s *SoldierService) SetReviewStatus(soldierID int64, needsReview bool, reason string) error {
	reason = strings.TrimSpace(reason)
	if !needsReview {
		reason = ""
	}
	if _, err := s.db.Conn().Exec(`UPDATE soldiers SET needs_review = ?, review_reason = ? WHERE id = ?`, needsReview, reason, soldierID); err != nil {
		return err
	}
	if needsReview {
		return s.touchAuditFields(soldierID, "needs_review")
	}
	return s.touchAuditFields(soldierID, "review_status")
}

// SearchPage returns the page of Soldiers matching the supplied filter + sort, plus the total count.
func (s *SoldierService) SearchPage(query string, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return s.List(page, pageSize)
	}

	offset := (page - 1) * pageSize
	soldiers, total, err := s.searchWithFTS(query, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	if total > 0 {
		return soldiers, total, nil
	}
	return s.searchWithLike(query, pageSize, offset)
}

func (s *SoldierService) searchWithFTS(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	matchQuery := ftsSearchExpression(query)
	if matchQuery == "" {
		return []models.Soldier{}, 0, nil
	}
	recordArgs := recordSearchLikeArgs(query)

	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT person_record_id AS id FROM soldiers_fts WHERE soldiers_fts MATCH ?
			UNION
			SELECT person_record_id AS id FROM records WHERE `+recordSearchLikeClause()+`
		) matches
	`, append([]interface{}{matchQuery}, recordArgs...)...).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	rowArgs := append(append([]interface{}{matchQuery}, recordArgs...), pageSize, offset)
	var rows *sql.Rows
	// The snippet picker is MAX-of-three-snippets, not a CASE WHEN.
	// Why this matters: SQLite's snippet() function returns non-empty
	// text for any FTS match in the same row, regardless of which
	// column the match actually landed in. Calling snippet(fts, 20,
	// ...) on a row that matched only the biography column returns
	// the start of the biography text, not the empty string a
	// reader of the SQL might expect. The downstream label picker
	// below uses snippetContainsQuery() to detect which snippet
	// actually contains the user's query; replacing MAX with a
	// `CASE WHEN biography_snippet != '' THEN biography_snippet
	// END` would pick the biography snippet for a notes-only match
	// and label the result "Biography" — wrong, and a regression
	// from the current behaviour. Audit issue #107 finding 7.11.
	if err := db.WithBusyRetry(3, func() error {
		r, qErr := conn.Query(`
		WITH matches AS (
			SELECT person_record_id,
				COALESCE(snippet(soldiers_fts, 19, '', '', '...', 12), '') AS biography_snippet,
				COALESCE(snippet(soldiers_fts, 20, '', '', '...', 12), '') AS notes_snippet,
				COALESCE(snippet(soldiers_fts, 21, '', '', '...', 12), '') AS scratch_snippet,
				bm25(soldiers_fts) AS score
			FROM soldiers_fts
			WHERE soldiers_fts MATCH ?
			UNION
			SELECT person_record_id, '', '', '', 1000.0
			FROM records
			WHERE `+recordSearchLikeClause()+`
		)
		SELECT `+soldierListSelectColumns+`, COALESCE(MAX(biography_snippet), ''), COALESCE(MAX(notes_snippet), ''), COALESCE(MAX(scratch_snippet), '')
		FROM soldiers
		JOIN matches ON matches.person_record_id = soldiers.id
		GROUP BY soldiers.id
		ORDER BY MIN(score), last_name, first_name
		LIMIT ? OFFSET ?
	`, rowArgs...)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	}); err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "searchWithFTS.rows")

	soldiers := []models.Soldier{}
	for rows.Next() {
		var (
			soldier          models.Soldier
			biographySnippet string
			notesSnippet     string
			scratchSnippet   string
		)
		if err := rows.Scan(append(soldierListScanDest(&soldier), &biographySnippet, &notesSnippet, &scratchSnippet)...); err != nil {
			return nil, 0, err
		}
		hydrateLegacyDeathParts(&soldier)
		normalizeConfederateHomeFields(&soldier)
		if snippetContainsQuery(biographySnippet, query) {
			soldier.SearchMatchField = "Biography"
			soldier.SearchMatchSnippet = strings.TrimSpace(biographySnippet)
		} else if snippetContainsQuery(notesSnippet, query) {
			soldier.SearchMatchField = "Notes"
			soldier.SearchMatchSnippet = strings.TrimSpace(notesSnippet)
		} else if snippetContainsQuery(scratchSnippet, query) {
			soldier.SearchMatchField = "Scratch Pad"
			soldier.SearchMatchSnippet = strings.TrimSpace(scratchSnippet)
		} else {
			soldier.SearchMatchField, soldier.SearchMatchSnippet = quickSearchMatch(soldier, quickSearchTerms(query))
		}
		soldiers = append(soldiers, soldier)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return soldiers, total, nil
}

func quickSearchLikeClause() string {
	return `display_id LIKE ? OR pension_id LIKE ? OR application_id LIKE ? OR prefix LIKE ? OR first_name LIKE ? OR middle_name LIKE ? OR last_name LIKE ? OR suffix LIKE ? OR unit LIKE ? OR rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ? OR pension_state LIKE ? OR confederate_home_status LIKE ? OR confederate_home_name LIKE ? OR buried_in LIKE ? OR maiden_name LIKE ? OR relationship_label LIKE ? OR biography LIKE ? OR notes LIKE ? OR EXISTS (
		SELECT 1 FROM records
		WHERE records.person_record_id = soldiers.id
			AND (record_type LIKE ? OR app_id LIKE ? OR details LIKE ?)
	)`
}

func quickSearchLikeArgs(query string) []interface{} {
	like := "%" + query + "%"
	return []interface{}{
		like, like, like,
		like, like, like, like, like, like,
		like, like, like, like,
		like, like, like, like,
		like, like, like,
		like, like, like,
	}
}

func (s *SoldierService) searchWithLike(query string, pageSize, offset int) ([]models.Soldier, int, error) {
	conn := s.db.Conn()
	args := quickSearchLikeArgs(query)

	var total int
	err := conn.QueryRow(`
		SELECT COUNT(*)
		FROM soldiers
		WHERE `+quickSearchLikeClause()+`
	`, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := conn.Query(`
		SELECT `+soldierListSelectColumns+`
		FROM soldiers
		WHERE `+quickSearchLikeClause()+`
		ORDER BY last_name, first_name
		LIMIT ? OFFSET ?
	`, append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "searchWithLike.rows")

	soldiers, err := scanListSoldiers(rows)
	return annotateQuickSearchMatches(soldiers, query), total, err
}

func recordSearchLikeClause() string {
	return `record_type LIKE ? OR app_id LIKE ? OR details LIKE ?`
}

func recordSearchLikeArgs(query string) []interface{} {
	like := "%" + query + "%"
	return []interface{}{like, like, like}
}

func ftsSearchExpression(query string) string {
	terms := quickSearchTerms(query)
	if len(terms) == 0 {
		return ""
	}
	parts := make([]string, 0, len(terms))
	for _, term := range terms {
		trimmed := strings.TrimSpace(term)
		if trimmed == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf(`"%s"*`, strings.ReplaceAll(trimmed, `"`, `""`)))
	}
	return strings.Join(parts, " AND ")
}

func snippetContainsQuery(snippet, query string) bool {
	if strings.TrimSpace(snippet) == "" {
		return false
	}
	lowerSnippet := strings.ToLower(snippet)
	for _, term := range quickSearchTerms(query) {
		if term != "" && strings.Contains(lowerSnippet, term) {
			return true
		}
	}
	return false
}

// AdvancedSearch returns the page of Soldiers matching an arbitrary query (built by the advanced-search form).
func (s *SoldierService) AdvancedSearch(search models.SoldierSearch, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	search.DisplayID = strings.TrimSpace(search.DisplayID)
	search.EntryType = strings.TrimSpace(strings.ToLower(search.EntryType))
	search.FirstName = strings.TrimSpace(search.FirstName)
	search.MiddleName = strings.TrimSpace(search.MiddleName)
	search.LastName = strings.TrimSpace(search.LastName)
	search.MaidenName = strings.TrimSpace(search.MaidenName)
	search.Rank = strings.TrimSpace(search.Rank)
	search.RankIn = strings.TrimSpace(search.RankIn)
	search.RankOut = strings.TrimSpace(search.RankOut)
	search.Unit = strings.TrimSpace(search.Unit)
	search.RecordType = strings.TrimSpace(search.RecordType)
	search.PensionState = normalizeOptionalPensionState(search.PensionState)
	search.ConfederateHomeStatus = normalizeOptionalConfederateHomeStatus(search.ConfederateHomeStatus)
	search.ConfederateHomeName = strings.TrimSpace(search.ConfederateHomeName)
	search.BuriedIn = strings.TrimSpace(search.BuriedIn)
	search.ReviewStatus = strings.TrimSpace(search.ReviewStatus)
	search.BirthDate = strings.TrimSpace(search.BirthDate)
	search.BirthYear = strings.TrimSpace(search.BirthYear)
	search.BirthYearTo = strings.TrimSpace(search.BirthYearTo)
	search.DeathDate = strings.TrimSpace(search.DeathDate)
	search.DeathYear = strings.TrimSpace(search.DeathYear)
	search.DeathYearTo = strings.TrimSpace(search.DeathYearTo)
	search.DeathMonth = strings.TrimSpace(search.DeathMonth)
	search.DeathDay = strings.TrimSpace(search.DeathDay)

	whereParts := []string{}
	args := []interface{}{}
	appendContainsFilter := func(column, value string) {
		whereParts = append(whereParts, column+" LIKE ?")
		args = append(args, "%"+value+"%")
	}
	appendYearFilter := func(expression, startValue, endValue, field string) error {
		if startValue == "" && endValue == "" {
			return nil
		}
		if startValue != "" && endValue == "" {
			parsed, err := strconv.Atoi(startValue)
			if err != nil {
				return fmt.Errorf("invalid %s", field)
			}
			whereParts = append(whereParts, expression+" = ?")
			args = append(args, parsed)
			return nil
		}

		var (
			start int
			end   int
			err   error
		)
		if startValue != "" {
			start, err = strconv.Atoi(startValue)
			if err != nil {
				return fmt.Errorf("invalid %s", field)
			}
		}
		if endValue != "" {
			end, err = strconv.Atoi(endValue)
			if err != nil {
				return fmt.Errorf("invalid %s_to", field)
			}
		}
		switch {
		case startValue == "":
			whereParts = append(whereParts, expression+" <= ?")
			args = append(args, end)
		case endValue == "":
			whereParts = append(whereParts, expression+" >= ?")
			args = append(args, start)
		default:
			if end < start {
				start, end = end, start
			}
			whereParts = append(whereParts, expression+" BETWEEN ? AND ?")
			args = append(args, start, end)
		}
		return nil
	}

	if search.DisplayID != "" {
		appendContainsFilter("display_id", search.DisplayID)
	}
	switch search.EntryType {
	case "soldier":
		whereParts = append(whereParts, "(entry_type IS NULL OR TRIM(entry_type) = '' OR LOWER(TRIM(entry_type)) = 'soldier')")
	case "wife", "widow", "linked_person":
		whereParts = append(whereParts, "LOWER(TRIM(entry_type)) = ?")
		args = append(args, search.EntryType)
	}
	if search.FirstName != "" {
		appendContainsFilter("first_name", search.FirstName)
	}
	if search.MiddleName != "" {
		appendContainsFilter("middle_name", search.MiddleName)
	}
	if search.LastName != "" {
		appendContainsFilter("last_name", search.LastName)
	}
	if search.MaidenName != "" {
		appendContainsFilter("maiden_name", search.MaidenName)
	}
	if search.RelationshipLabel != "" {
		appendContainsFilter("relationship_label", search.RelationshipLabel)
	}
	if search.Rank != "" {
		whereParts = append(whereParts, "(rank LIKE ? OR rank_in LIKE ? OR rank_out LIKE ?)")
		args = append(args, "%"+search.Rank+"%", "%"+search.Rank+"%", "%"+search.Rank+"%")
	}
	if search.RankIn != "" {
		appendContainsFilter("rank_in", search.RankIn)
	}
	if search.RankOut != "" {
		appendContainsFilter("rank_out", search.RankOut)
	}
	if search.Unit != "" {
		appendContainsFilter("unit", search.Unit)
	}
	if search.RecordType != "" {
		whereParts = append(whereParts, "EXISTS (SELECT 1 FROM records WHERE records.person_record_id = soldiers.id AND records.record_type LIKE ?)")
		args = append(args, "%"+search.RecordType+"%")
	}
	if search.PensionState != "" {
		whereParts = append(whereParts, normalizedPensionStateExpr+" = ?")
		args = append(args, search.PensionState)
	}
	if search.ConfederateHomeStatus != "" {
		whereParts = append(whereParts, normalizedConfederateHomeStatusExpr+" = ?")
		args = append(args, search.ConfederateHomeStatus)
	}
	if search.ConfederateHomeName != "" {
		appendContainsFilter("confederate_home_name", search.ConfederateHomeName)
	}
	if search.BuriedIn != "" {
		appendContainsFilter("buried_in", search.BuriedIn)
	}
	switch strings.ToLower(search.ReviewStatus) {
	case "clean":
		whereParts = append(whereParts, "needs_review = 0")
	case "review":
		whereParts = append(whereParts, "needs_review = 1")
	}
	if search.BirthDate != "" {
		normalized, err := dates.NormalizeCanonical(search.BirthDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid birth_date")
		}
		whereParts = append(whereParts, "birth_date = ?")
		args = append(args, normalized)
	}
	if err := appendYearFilter(`NULLIF(CAST(substr(trim(coalesce(birth_date, '')), -4) AS INTEGER), 0)`, search.BirthYear, search.BirthYearTo, "birth_year"); err != nil {
		return nil, 0, err
	}
	if search.DeathDate != "" {
		normalized, err := dates.NormalizeCanonical(search.DeathDate)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid death_date")
		}
		whereParts = append(whereParts, "death_date = ?")
		args = append(args, normalized)
	}
	if err := appendYearFilter("NULLIF(death_year, 0)", search.DeathYear, search.DeathYearTo, "death_year"); err != nil {
		return nil, 0, err
	}

	exactFilters := []struct {
		value  string
		field  string
		column string
	}{
		{search.DeathMonth, "death_month", "death_month"},
		{search.DeathDay, "death_day", "death_day"},
	}
	for _, filter := range exactFilters {
		if filter.value == "" {
			continue
		}
		parsed, err := strconv.Atoi(filter.value)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid %s", filter.field)
		}
		whereParts = append(whereParts, filter.column+" = ?")
		args = append(args, parsed)
	}

	if len(whereParts) == 0 {
		return s.List(page, pageSize)
	}

	whereClause := strings.Join(whereParts, " AND ")
	conn := s.db.Conn()

	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRow("SELECT COUNT(*) FROM soldiers WHERE "+whereClause, args...).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		r, qErr := conn.Query(
			"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE "+whereClause+" ORDER BY last_name, first_name LIMIT ? OFFSET ?",
			append(args, pageSize, offset)...,
		)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	}); err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "AdvancedSearch.rows")

	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

// List returns all Soldiers, paginated, in display-ID order.
func (s *SoldierService) List(page, pageSize int) ([]models.Soldier, int, error) {
	// Slice 1 of issue #613: paginated read goes through the
	// repository seam. The legacy inline SQL (count + order +
	// limit/offset) is now the SQLite repo's responsibility;
	// this service method becomes pure orchestration.
	rows, total, err := s.personRepo.List(context.Background(), page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "List.rows")
	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

// ListByEntryTypes returns Soldiers filtered by entry_type (soldier / wife / widow / linked person).
func (s *SoldierService) ListByEntryTypes(entryTypes []string, page, pageSize int) ([]models.Soldier, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	normalized := make([]string, 0, len(entryTypes))
	for _, entryType := range entryTypes {
		trimmed := strings.TrimSpace(strings.ToLower(entryType))
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	if len(normalized) == 0 {
		return []models.Soldier{}, 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(normalized)), ",")
	args := make([]interface{}, 0, len(normalized))
	for _, entryType := range normalized {
		args = append(args, entryType)
	}
	whereClause := fmt.Sprintf("LOWER(TRIM(entry_type)) IN (%s)", placeholders)
	conn := s.db.Conn()
	var total int
	if err := db.WithBusyRetry(3, func() error {
		return conn.QueryRow("SELECT COUNT(*) FROM soldiers WHERE "+whereClause, args...).Scan(&total)
	}); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	var rows *sql.Rows
	if err := db.WithBusyRetry(3, func() error {
		r, qErr := conn.Query(
			"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE "+whereClause+" ORDER BY last_name, first_name LIMIT ? OFFSET ?",
			append(args, pageSize, offset)...,
		)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	}); err != nil {
		return nil, 0, err
	}
	defer debug.DeferCloseLog(rows, "ListByEntryTypes.rows")
	soldiers, err := scanListSoldiers(rows)
	return soldiers, total, err
}

// recentSelectColumns is the column subset used by RecentByIDs. It
// drops the heavy record/image count subqueries and the long-form
// fields (biography, notes, pdf_excerpt_override, last_edited_fields,
// sync_id, audit timestamps) that the recent-search view never
// renders. Audit issue #119 (finding 7.2).
const recentSelectColumns = `id, display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, kind, begin_date, end_date, description`

// RecentByIDs returns the Soldiers with the given IDs in the order they appear in the input slice. Used to populate the Recent Edits list.
func (s *SoldierService) RecentByIDs(ids []int64, limit int) ([]models.Soldier, error) {
	if limit < 1 {
		limit = 10
	}
	if len(ids) == 0 {
		return []models.Soldier{}, nil
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Conn().Query(
		"SELECT "+recentSelectColumns+" FROM soldiers WHERE id IN ("+placeholders+")",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "RecentByIDs.rows")
	soldiers, err := scanRecentSoldiers(rows)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]models.Soldier, len(soldiers))
	for _, soldier := range soldiers {
		index[soldier.ID] = soldier
	}
	ordered := make([]models.Soldier, 0, len(ids))
	for _, id := range ids {
		if soldier, ok := index[id]; ok {
			ordered = append(ordered, soldier)
		}
	}
	return ordered, nil
}

// ServiceTimeline returns the per-soldier chronological service timeline (enlistment, transfer, wound, discharge, death).
func (s *SoldierService) ServiceTimeline(soldierID int64) (*ServiceTimeline, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	if normalizeEntryType(central.EntryType) != "soldier" {
		return nil, fmt.Errorf("service timeline is available for soldier records only")
	}

	timeline := &ServiceTimeline{Central: *central}
	if partial, approximate := soldierBirthTimelineDate(*central); partial.HasAny() {
		timeline.Events = append(timeline.Events, newServiceTimelineEvent(
			"Birth",
			partial,
			"Profile",
			strings.TrimSpace(central.BirthInfo),
			"life",
			approximate,
			0,
		))
	}
	for _, record := range central.Records {
		event, ok := serviceTimelineEventFromRecord(record)
		if ok {
			timeline.Events = append(timeline.Events, event)
			continue
		}
		if timelineRecordHasContent(record) {
			timeline.UndatedRecords = append(timeline.UndatedRecords, record)
		}
	}
	if partial, approximate := soldierDeathTimelineDate(*central); partial.HasAny() {
		timeline.Events = append(timeline.Events, newServiceTimelineEvent(
			"Death",
			partial,
			"Profile",
			"",
			"death",
			approximate,
			900,
		))
		if strings.TrimSpace(central.BuriedIn) != "" {
			timeline.Events = append(timeline.Events, newServiceTimelineEvent(
				"Burial recorded",
				partial,
				"Profile",
				central.BuriedIn,
				"burial",
				true,
				901,
			))
		}
	}

	// Issue #320 slice #337: append one Timeline Marker per
	// Event Record linked to this soldier via
	// event_person_links. Issue #343 finding #5: the JOIN +
	// projection now live on EventService (where the Event
	// schema belongs); ServiceTimeline delegates via the
	// back-reference set by SetEvents. If the back-reference
	// is nil (older wiring path or a test that didn't set it),
	// the linked-event markers are skipped — the rest of the
	// timeline still renders.
	var linkedMarkers []LinkedEventTimelineMarker
	if s.events != nil {
		linkedMarkers, err = s.events.LinkedEventsForTimeline(soldierID)
		if err != nil {
			return nil, err
		}
	}
	// Date sourcing: the Event's begin_date wins; when
	// begin_date is empty, end_date is used as the fallback.
	// Events with neither date are skipped (the user has not
	// yet back-filled the timeline fields). The marker is
	// sorted inline below alongside the existing Birth /
	// Death / record-derived markers, so the user sees a
	// single chronological view.
	for _, marker := range linkedMarkers {
		primary := strings.TrimSpace(marker.BeginDate)
		if primary == "" {
			primary = strings.TrimSpace(marker.EndDate)
		}
		partial, parseErr := dates.ParseCanonical(primary)
		if parseErr != nil || !partial.HasAny() {
			continue
		}
		timeline.Events = append(timeline.Events, newServiceTimelineEvent(
			"Linked Event: "+strings.TrimSpace(marker.Kind),
			partial,
			marker.DisplayID,
			strings.TrimSpace(marker.Description),
			"event",
			false,
			200,
		))
	}

	sort.SliceStable(timeline.Events, func(i, j int) bool {
		return serviceTimelineEventLess(timeline.Events[i], timeline.Events[j])
	})
	if len(timeline.Events) > 0 {
		timeline.StartLabel = timeline.Events[0].DateLabel
		timeline.EndLabel = timeline.Events[len(timeline.Events)-1].DateLabel
	}
	for _, event := range timeline.Events {
		if event.Approximate {
			timeline.InferredEventCount++
		} else {
			timeline.ExactEventCount++
		}
	}
	return timeline, nil
}

// ResearchLog returns the per-soldier Research Log: open tasks + dismissed suggestions + recently completed.
func (s *SoldierService) ResearchLog(soldierID int64) (*ResearchLog, error) {
	central, err := s.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Conn().Query(`
		SELECT id, person_record_id, title, notes, evidence_type, status, created_at, COALESCE(updated_at, ''), COALESCE(resolved_at, '')
		FROM research_tasks
		WHERE person_record_id = ?
		ORDER BY CASE status WHEN 'open' THEN 0 ELSE 1 END, created_at DESC, id DESC
	`, soldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ResearchLog.rows")

	log := &ResearchLog{
		Central:     *central,
		Suggestions: suggestedResearchTasks(*central),
	}
	for rows.Next() {
		var task ResearchTask
		if err := rows.Scan(&task.ID, &task.SoldierID, &task.Title, &task.Notes, &task.EvidenceType, &task.Status, &task.CreatedAt, &task.UpdatedAt, &task.ResolvedAt); err != nil {
			return nil, err
		}
		log.Tasks = append(log.Tasks, task)
		if strings.EqualFold(strings.TrimSpace(task.Status), "resolved") {
			log.ResolvedCount++
		} else {
			log.OpenCount++
		}
	}
	return log, rows.Err()
}

// AddResearchTask appends a new open task to the per-soldier Research Log.
func (s *SoldierService) AddResearchTask(soldierID int64, title, notes, evidenceType string) error {
	if _, err := s.GetByID(soldierID); err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return fmt.Errorf("research task title is required")
	}
	evidenceType = normalizeResearchEvidenceType(evidenceType)
	notes = strings.TrimSpace(notes)
	_, err := s.db.Conn().Exec(`
		INSERT INTO research_tasks (person_record_id, title, notes, evidence_type, status, updated_at)
		VALUES (?, ?, ?, ?, 'open', ?)
	`, soldierID, title, notes, evidenceType, currentSQLiteTimestamp())
	return err
}

// ResolveResearchTask marks an open task as resolved (status: done or dropped).
func (s *SoldierService) ResolveResearchTask(soldierID, taskID int64) error {
	result, err := s.db.Conn().Exec(`
		UPDATE research_tasks
		SET status = 'resolved', updated_at = ?, resolved_at = ?
		WHERE id = ? AND person_record_id = ? AND status <> 'resolved'
	`, currentSQLiteTimestamp(), currentSQLiteTimestamp(), taskID, soldierID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("research task not found")
	}
	return nil
}

// ResearchCollectionsHub returns the full Research Collections Hub page payload.
func (s *SoldierService) ResearchCollectionsHub(currentSoldierID int64) (*ResearchCollectionHub, error) {
	hub := &ResearchCollectionHub{}
	if currentSoldierID > 0 {
		current, err := s.GetByID(currentSoldierID)
		if err != nil {
			return nil, err
		}
		hub.Current = current
	}
	rows, err := s.db.Conn().Query(`
		SELECT c.id, c.name, COALESCE(c.description, ''), c.created_at, COALESCE(c.updated_at, ''), COUNT(i.person_record_id),
		       CASE WHEN ? > 0 AND EXISTS (SELECT 1 FROM research_collection_items existing WHERE existing.collection_id = c.id AND existing.person_record_id = ?) THEN 1 ELSE 0 END
		FROM research_collections c
		LEFT JOIN research_collection_items i ON i.collection_id = c.id
		GROUP BY c.id, c.name, c.description, c.created_at, c.updated_at
		ORDER BY LOWER(c.name) ASC
	`, currentSoldierID, currentSoldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ResearchCollectionsHub.rows")
	for rows.Next() {
		var (
			collection ResearchCollection
			contains   int
		)
		if err := rows.Scan(&collection.ID, &collection.Name, &collection.Description, &collection.CreatedAt, &collection.UpdatedAt, &collection.ItemCount, &contains); err != nil {
			return nil, err
		}
		collection.ContainsCurrent = contains == 1
		hub.Collections = append(hub.Collections, collection)
	}
	return hub, rows.Err()
}

// CreateResearchCollection creates a new named research collection.
func (s *SoldierService) CreateResearchCollection(name, description string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return fmt.Errorf("collection name is required")
	}
	_, err := s.db.Conn().Exec(`
		INSERT INTO research_collections (name, description, updated_at)
		VALUES (?, ?, ?)
	`, name, description, currentSQLiteTimestamp())
	return err
}

// AddSoldierToResearchCollection attaches a Soldier to a research
// collection. Returns (added=true, err=nil) on a fresh add,
// (added=false, err=nil) when the record was already in the
// collection (idempotent re-add is a no-op, not an error), and
// (added=false, err!=nil) on a real failure (soldier not found,
// DB error, etc.). The handler uses `added` to pick the toast
// copy: "Success: record added to collection" vs "Already in
// collection". Treating the already-in state as an error used
// to surface a red error toast on every double-click or stale
// post-redirect scenario.
func (s *SoldierService) AddSoldierToResearchCollection(collectionID, soldierID int64) (bool, error) {
	if _, err := s.GetByID(soldierID); err != nil {
		return false, err
	}
	result, err := s.db.Conn().Exec(`
		INSERT OR IGNORE INTO research_collection_items (collection_id, person_record_id, created_at)
		VALUES (?, ?, ?)
	`, collectionID, soldierID, currentSQLiteTimestamp())
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		// Already in the collection - idempotent no-op. The
		// updated_at bump below is still useful for "touched"
		// semantics but is conditional on the insert having
		// actually added a row. Skip the bump on the
		// already-in path to avoid misleading the UI into
		// thinking a real change happened.
		return false, nil
	}
	if _, err := s.db.Conn().Exec(`UPDATE research_collections SET updated_at = ? WHERE id = ?`, currentSQLiteTimestamp(), collectionID); err != nil {
		return true, err
	}
	return true, nil
}

// AddPersonRecordToResearchCollection is the glossary-name alias of AddSoldierToResearchCollection.
func (s *SoldierService) AddPersonRecordToResearchCollection(collectionID, personRecordID int64) (bool, error) {
	return s.AddSoldierToResearchCollection(collectionID, personRecordID)
}

// ResearchCollectionDetail returns the per-collection detail page (the Soldiers in the collection, sorted).
func (s *SoldierService) ResearchCollectionDetail(collectionID int64, currentSoldierID int64) (*ResearchCollectionDetail, error) {
	detail := &ResearchCollectionDetail{}
	if currentSoldierID > 0 {
		current, err := s.GetByID(currentSoldierID)
		if err != nil {
			return nil, err
		}
		detail.Current = current
	}
	if err := s.db.Conn().QueryRow(`
		SELECT c.id, c.name, COALESCE(c.description, ''), c.created_at, COALESCE(c.updated_at, ''), COUNT(i.person_record_id)
		FROM research_collections c
		LEFT JOIN research_collection_items i ON i.collection_id = c.id
		WHERE c.id = ?
		GROUP BY c.id, c.name, c.description, c.created_at, c.updated_at
	`, collectionID).Scan(&detail.Collection.ID, &detail.Collection.Name, &detail.Collection.Description, &detail.Collection.CreatedAt, &detail.Collection.UpdatedAt, &detail.Collection.ItemCount); err != nil {
		return nil, err
	}
	rows, err := s.db.Conn().Query(`
		SELECT `+soldierListSelectColumns+`
		FROM soldiers
		WHERE id IN (SELECT person_record_id FROM research_collection_items WHERE collection_id = ?)
		ORDER BY last_name, first_name
	`, collectionID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ResearchCollectionDetail.rows")
	members, err := scanListSoldiers(rows)
	if err != nil {
		return nil, err
	}
	detail.Members = members
	if currentSoldierID > 0 {
		for _, member := range members {
			if member.ID == currentSoldierID {
				detail.Collection.ContainsCurrent = true
				break
			}
		}
	}
	return detail, nil
}

func scanSoldier(row *sql.Row) (*models.Soldier, error) {
	var s models.Soldier
	if err := row.Scan(soldierScanDest(&s)...); err != nil {
		return nil, err
	}
	hydrateLegacyDeathParts(&s)
	s.PensionState = pensionstate.Normalize(s.PensionState)
	normalizeConfederateHomeFields(&s)
	return &s, nil
}

func scanSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(soldierScanDest(&s)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&s)
		s.PensionState = pensionstate.Normalize(s.PensionState)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}

func scanListSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(soldierListScanDest(&s)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&s)
		s.PensionState = pensionstate.Normalize(s.PensionState)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}

func annotateQuickSearchMatches(soldiers []models.Soldier, query string) []models.Soldier {
	terms := quickSearchTerms(query)
	if len(terms) == 0 {
		return soldiers
	}
	for index := range soldiers {
		soldiers[index].SearchMatchField, soldiers[index].SearchMatchSnippet = quickSearchMatch(soldiers[index], terms)
	}
	return soldiers
}

func quickSearchTerms(query string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.TrimSpace(query)), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

func quickSearchMatch(soldier models.Soldier, terms []string) (string, string) {
	for _, candidate := range []struct {
		label string
		value string
	}{
		{label: "Display ID", value: strings.TrimSpace(soldier.DisplayID)},
		{label: "Pension ID", value: strings.TrimSpace(soldier.PensionID)},
		{label: "Application ID", value: strings.TrimSpace(soldier.ApplicationID)},
		{label: "Name", value: strings.TrimSpace(soldier.GetFullName())},
		{label: "Rank", value: soldierSearchRank(soldier)},
		{label: "Unit", value: strings.TrimSpace(soldier.Unit)},
		{label: "Pension State", value: strings.TrimSpace(soldier.PensionState)},
		{label: "Buried In", value: strings.TrimSpace(soldier.BuriedIn)},
		{label: "Maiden Name", value: strings.TrimSpace(soldier.MaidenName)},
		{label: "Relationship", value: strings.TrimSpace(soldier.RelationshipLabel)},
	} {
		if candidate.value == "" {
			continue
		}
		lowerValue := strings.ToLower(candidate.value)
		for _, term := range terms {
			if term != "" && strings.Contains(lowerValue, term) {
				return candidate.label, candidate.value
			}
		}
	}
	return "", ""
}

func soldierSearchRank(soldier models.Soldier) string {
	for _, value := range []string{soldier.RankOut, soldier.Rank, soldier.RankIn} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var (
	slashTimelineDatePattern = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(\d{4})\b`)
)

// ErrDisplayIDNotEmpty is returned by RecoverDisplayID when the
// supplied soldier already has a display_id. The recovery
// affordance is for blank-id rows only; a healthy row must not
// be re-minted.
var ErrDisplayIDNotEmpty = errors.New("this record already has a display_id")

func soldierBirthTimelineDate(soldier models.Soldier) (dates.PartialDate, bool) {
	if partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.BirthDate)); err == nil && partial.HasAny() {
		return partial, false
	}
	if parsed := strings.TrimSpace(dates.ParseBirthInfo(soldier.BirthInfo)); parsed != "" {
		if partial, err := dates.ParseCanonical(parsed); err == nil && partial.HasAny() {
			return partial, true
		}
	}
	return dates.PartialDate{}, false
}

func soldierDeathTimelineDate(soldier models.Soldier) (dates.PartialDate, bool) {
	if partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.DeathDate)); err == nil && partial.HasAny() {
		return partial, false
	}
	partial := dates.PartialDate{
		Month: soldier.DeathMonth,
		Day:   soldier.DeathDay,
		Year:  soldier.DeathYear,
	}
	return partial, partial.HasAny()
}

func serviceTimelineEventFromRecord(record models.Record) (ServiceTimelineEvent, bool) {
	partial, approximate, ok := inferTimelineDateFromText(record.Details)
	if !ok {
		return ServiceTimelineEvent{}, false
	}
	description := strings.TrimSpace(record.Details)
	title := strings.TrimSpace(record.RecordType)
	if title == "" {
		title = "Archive record"
	}
	return newServiceTimelineEvent(
		title,
		partial,
		recordTimelineSourceLabel(record),
		description,
		recordTimelineCategory(record),
		approximate,
		100+recordTimelineOrder(record),
	), true
}

func inferTimelineDateFromText(value string) (dates.PartialDate, bool, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return dates.PartialDate{}, false, false
	}
	if match := slashTimelineDatePattern.FindStringSubmatch(trimmed); len(match) == 4 {
		month, monthErr := strconv.Atoi(match[1])
		day, dayErr := strconv.Atoi(match[2])
		year, yearErr := strconv.Atoi(match[3])
		if monthErr == nil && dayErr == nil && yearErr == nil {
			partial := dates.PartialDate{Month: month, Day: day, Year: year}
			if partial.HasAny() {
				return partial, false, true
			}
		}
	}
	if parsed := strings.TrimSpace(dates.ParseBirthInfo(trimmed)); parsed != "" {
		if partial, err := dates.ParseCanonical(parsed); err == nil && partial.HasAny() {
			return partial, true, true
		}
	}
	return dates.PartialDate{}, false, false
}

func newServiceTimelineEvent(title string, partial dates.PartialDate, sourceLabel, description, category string, approximate bool, order int) ServiceTimelineEvent {
	confidence := "Exact"
	if approximate {
		confidence = "Inferred"
	}
	return ServiceTimelineEvent{
		Title:           title,
		DateLabel:       dates.Display(partial.Format()),
		Description:     strings.TrimSpace(description),
		SourceLabel:     strings.TrimSpace(sourceLabel),
		Category:        strings.TrimSpace(category),
		ConfidenceLabel: confidence,
		Approximate:     approximate,
		sortDate:        partial,
		sortOrder:       order,
	}
}

func serviceTimelineEventLess(left, right ServiceTimelineEvent) bool {
	if left.sortDate.Year != right.sortDate.Year {
		return left.sortDate.Year < right.sortDate.Year
	}
	if left.sortDate.Month != right.sortDate.Month {
		return left.sortDate.Month < right.sortDate.Month
	}
	if left.sortDate.Day != right.sortDate.Day {
		return left.sortDate.Day < right.sortDate.Day
	}
	if left.Approximate != right.Approximate {
		return !left.Approximate
	}
	if left.sortOrder != right.sortOrder {
		return left.sortOrder < right.sortOrder
	}
	return strings.ToLower(left.Title) < strings.ToLower(right.Title)
}

func recordTimelineSourceLabel(record models.Record) string {
	label := strings.TrimSpace(record.RecordType)
	if label == "" {
		label = "Archive record"
	}
	if strings.TrimSpace(record.AppID) == "" {
		return label
	}
	return label + " · " + strings.TrimSpace(record.AppID)
}

func recordTimelineCategory(record models.Record) string {
	label := strings.ToLower(strings.TrimSpace(record.RecordType))
	switch {
	case strings.Contains(label, "pension"), strings.Contains(label, "application"):
		return "pension"
	case strings.Contains(label, "parole"), strings.Contains(label, "muster"), strings.Contains(label, "service"), strings.Contains(label, "roster"):
		return "service"
	case strings.Contains(label, "grave"), strings.Contains(label, "burial"), strings.Contains(label, "cemetery"):
		return "burial"
	default:
		return "archive"
	}
}

func recordTimelineOrder(record models.Record) int {
	switch recordTimelineCategory(record) {
	case "service":
		return 10
	case "pension":
		return 20
	case "burial":
		return 30
	default:
		return 40
	}
}

func timelineRecordHasContent(record models.Record) bool {
	return strings.TrimSpace(record.RecordType) != "" || strings.TrimSpace(record.AppID) != "" || strings.TrimSpace(record.Details) != ""
}

func suggestedResearchTasks(soldier models.Soldier) []ResearchTaskSuggestion {
	suggestions := make([]ResearchTaskSuggestion, 0, 8)
	if isSoldierEntryType(soldier.EntryType) && strings.TrimSpace(soldier.Unit) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm unit assignment",
			Notes:        "No unit is recorded yet. Verify regiment, company, or service branch from attached evidence.",
			EvidenceType: "service",
		})
	}
	if strings.TrimSpace(soldier.PensionID) == "" && strings.TrimSpace(soldier.ApplicationID) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Locate pension or application file",
			Notes:        "No pension ID or application ID is attached yet. Check pension indexes or state archive holdings.",
			EvidenceType: "pension",
		})
	}
	if strings.TrimSpace(soldier.BuriedIn) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm burial location",
			Notes:        "Burial place is still missing. Search cemetery registers, memorial sites, or obituary sources.",
			EvidenceType: "burial",
		})
	}
	if strings.TrimSpace(soldier.BirthDate) == "" && strings.TrimSpace(soldier.BirthInfo) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm birth details",
			Notes:        "Birth date and place context are missing. Look for census, family bible, or probate evidence.",
			EvidenceType: "vital",
		})
	}
	if strings.TrimSpace(soldier.DeathDate) == "" && soldier.DeathYear == 0 {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm death details",
			Notes:        "Death timing is incomplete. Search pension closure files, death certificates, and memorial records.",
			EvidenceType: "vital",
		})
	}
	if len(soldier.Records) == 0 {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Attach primary source records",
			Notes:        "No archive records are attached yet. Add muster, parole, pension, memorial, or correspondence sources.",
			EvidenceType: "archive",
		})
	}
	if !isSoldierEntryType(soldier.EntryType) && soldier.SpouseSoldierID == 0 {
		title := "Link spouse soldier record"
		notes := "Family relationship is not connected yet. Identify and link the matching soldier profile."
		if strings.TrimSpace(soldier.EntryType) == "linked_person" {
			title = "Link related soldier record"
			notes = "This person record still needs its anchor soldier record. Identify and link the matching soldier profile."
		}
		suggestions = append(suggestions, ResearchTaskSuggestion{Title: title, Notes: notes, EvidenceType: "family"})
	}
	if isPersonRecordEntryType(soldier.EntryType) && strings.TrimSpace(soldier.RelationshipLabel) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm relationship to soldier",
			Notes:        "Relationship to the linked soldier is still blank. Add the exact relationship label used in the source material.",
			EvidenceType: "family",
		})
	}
	if !isSoldierEntryType(soldier.EntryType) && !isPersonRecordEntryType(soldier.EntryType) && strings.TrimSpace(soldier.MaidenName) == "" {
		suggestions = append(suggestions, ResearchTaskSuggestion{
			Title:        "Confirm maiden name",
			Notes:        "Maiden name is still blank. Search marriage, pension, census, or obituary records for supporting evidence.",
			EvidenceType: "family",
		})
	}
	return suggestions
}

// normalizeResearchEvidenceType cleans a user-submitted
// research-tasks.evidence_type value.
//
// The column is free TEXT (see schema.go: research_tasks.evidence_type
// TEXT NOT NULL DEFAULT 'general'), so the function must not silently
// rewrite unknown values to a default — Issue #553 caught a user
// picking "Local Archive" and finding it persisted as "General"
// because the previous switch only knew the bare word "archive".
// The Research & Review form (internal/templates/research_log.templ)
// emits models.EvidenceTypeLocalArchive ("local_archive"), and any
// future evidence-type vocabulary must round-trip verbatim.
//
// Behaviour:
//   - whitespace is trimmed and the value is lowercased
//   - the result is returned as-is; there is no allow-list and no
//     silent fallback to "general"
func normalizeResearchEvidenceType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isSoldierEntryType(value string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	return trimmed == "" || trimmed == "soldier"
}

func isPersonRecordEntryType(value string) bool {
	return strings.ToLower(strings.TrimSpace(value)) == "linked_person"
}

// ManualComparison computes the per-field comparison for the user-initiated pair compare.
func (s *SoldierService) ManualComparison(leftID, rightID int64) (*DuplicateAuditComparison, error) {
	if leftID < 1 || rightID < 1 || leftID == rightID {
		return nil, fmt.Errorf("choose two different records to compare")
	}
	leftSoldier, err := s.GetByID(leftID)
	if err != nil {
		return nil, err
	}
	rightSoldier, err := s.GetByID(rightID)
	if err != nil {
		return nil, err
	}
	fields := buildDuplicateAuditComparisonFields(*leftSoldier, *rightSoldier, map[string]struct{}{})
	for index := range fields {
		fields[index].Highlighted = fields[index].LeftValue != fields[index].RightValue
	}
	return &DuplicateAuditComparison{
		PageTitle:    "Person Record Comparison",
		BackHref:     "/soldiers",
		BackLabel:    "Back",
		Reason:       "Manual side-by-side comparison of two selected person records.",
		Status:       "manual",
		LeftSoldier:  *leftSoldier,
		RightSoldier: *rightSoldier,
		Fields:       fields,
	}, nil
}

func replaceRecords(tx *sql.Tx, soldierID int64, soldierSyncID string, records []models.Record) error {
	if _, err := tx.Exec(`DELETE FROM records WHERE person_record_id = ?`, soldierID); err != nil {
		return err
	}
	for idx, record := range normalizeRecords(records) {
		if strings.TrimSpace(record.SyncID) == "" {
			syncID, err := db.NewSyncID()
			if err != nil {
				return err
			}
			record.SyncID = syncID
		}
		record.PersonSyncID = soldierSyncID
		// Issue #368 slice 2: write sort_order from the form-array
		// index so a re-save preserves the user's current display
		// order. The replaceRecords path deletes + reinserts, so
		// without this the new rows would all carry sort_order=0
		// (the column DEFAULT) and the secondary `id` tiebreak
		// would hide the user's reorder.
		if _, err := tx.Exec(
			`INSERT INTO records (sync_id, person_record_id, person_sync_id, record_type, app_id, details, sort_order) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			record.SyncID,
			soldierID,
			record.PersonSyncID,
			record.RecordType,
			record.AppID,
			record.Details,
			int64(idx),
		); err != nil {
			return err
		}
	}
	return nil
}

func normalizeRecords(records []models.Record) []models.Record {
	normalized := make([]models.Record, 0, len(records))
	for _, record := range records {
		record.RecordType = strings.TrimSpace(record.RecordType)
		record.AppID = strings.TrimSpace(record.AppID)
		record.Details = strings.TrimSpace(record.Details)
		if record.RecordType == "" && record.AppID == "" && record.Details == "" {
			continue
		}
		normalized = append(normalized, record)
	}
	return normalized
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
		createdByVersion      sql.NullString
		createdByImportPath   sql.NullString
		restoredAt            sql.NullString
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
		nullStringDest(&s.CreatedByVersion, &createdByVersion),
		nullStringDest(&s.CreatedByImportPath, &createdByImportPath),
		nullStringDest(&s.RestoredAt, &restoredAt),
	}
}

func soldierListScanDest(s *models.Soldier) []interface{} {
	dest := soldierScanDest(s)
	dest = append(dest, &s.SpouseDisplayID, &s.RecordCount, &s.ImageCount)
	return dest
}

// scanRecentSoldiers scans a query that returned the recentSelectColumns
// subset (drops the heavy record/image count subqueries and the
// long-form fields biography, notes, and pdf_excerpt_override).
// Biography / Notes / PDFExcerptOverride / SpouseDisplayID /
// RecordCount / ImageCount stay at their zero values because the
// recent-search view never renders them. Audit issue #119
// (finding 7.2).
func scanRecentSoldiers(rows *sql.Rows) ([]models.Soldier, error) {
	var soldiers []models.Soldier
	for rows.Next() {
		var s models.Soldier
		if err := rows.Scan(recentScanDest(&s)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&s)
		s.PensionState = pensionstate.Normalize(s.PensionState)
		normalizeConfederateHomeFields(&s)
		soldiers = append(soldiers, s)
	}
	if soldiers == nil {
		soldiers = []models.Soldier{}
	}
	return soldiers, rows.Err()
}

// recentScanDest mirrors soldierScanDest but skips the biography /
// notes / pdf_excerpt_override / spouse_display_id / record_count /
// image_count destinations. The recent-search view never reads them
// so the values stay at their model zero.
func recentScanDest(s *models.Soldier) []interface{} {
	var (
		displayID            sql.NullString
		syncID               sql.NullString
		entryType            sql.NullString
		spouseSoldierID      sql.NullInt64
		relationshipLabel    sql.NullString
		maidenName           sql.NullString
		pensionID            sql.NullString
		applicationID        sql.NullString
		prefix               sql.NullString
		showPrefixBeforeName sql.NullBool
		firstName            sql.NullString
		middleName           sql.NullString
		lastName             sql.NullString
		suffix               sql.NullString
		rank                 sql.NullString
		rankIn               sql.NullString
		rankOut              sql.NullString
		unit                 sql.NullString
		pensionState         sql.NullString
		confederateHomeStatus sql.NullString
		confederateHomeName  sql.NullString
		deathYear            sql.NullInt64
		deathMonth           sql.NullInt64
		deathDay             sql.NullInt64
		birthDate            sql.NullString
		deathDate            sql.NullString
		birthInfo            sql.NullString
		buriedIn             sql.NullString
		reviewReason         sql.NullString
		addedBy              sql.NullString
		lastEditedBy         sql.NullString
		lastEditedFields     sql.NullString
		lastEditedAt         sql.NullString
		createdAt            sql.NullString
		updatedAt            sql.NullString
		kind                 sql.NullString
		beginDate            sql.NullString
		endDate              sql.NullString
		description          sql.NullString
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

func normalizeSoldierEntry(tx *sql.Tx, soldier *models.Soldier) error {
	soldier.EntryType = normalizeEntryType(soldier.EntryType)
	soldier.Prefix = strings.TrimSpace(soldier.Prefix)
	soldier.PensionState = pensionstate.Normalize(soldier.PensionState)
	soldier.ConfederateHomeStatus = confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)
	soldier.FirstName = strings.TrimSpace(soldier.FirstName)
	soldier.MiddleName = strings.TrimSpace(soldier.MiddleName)
	soldier.Biography = strings.TrimSpace(soldier.Biography)
	soldier.PDFExcerptOverride = strings.TrimSpace(soldier.PDFExcerptOverride)
	soldier.Notes = strings.TrimSpace(soldier.Notes)
	soldier.LastName = strings.TrimSpace(soldier.LastName)
	soldier.Suffix = strings.TrimSpace(soldier.Suffix)
	soldier.MaidenName = strings.TrimSpace(soldier.MaidenName)
	soldier.RelationshipLabel = strings.TrimSpace(soldier.RelationshipLabel)
	if soldier.EntryType == "soldier" {
		soldier.SpouseSoldierID = 0
		soldier.RelationshipLabel = ""
		return nil
	}
	// v60 (issue #320): Event Records are not Persons — they have
	// no spouse. Bypass the spouse-required check the same way the
	// Soldier case does. Clear the spouse_soldier_id defensively
	// in case a form posts it for an Event.
	if soldier.EntryType == "event" {
		soldier.SpouseSoldierID = 0
		soldier.RelationshipLabel = ""
		soldier.MaidenName = ""
		return nil
	}
	if soldier.SpouseSoldierID < 1 {
		return fmt.Errorf("spouse_soldier_id required")
	}
	var spouseType string
	if err := tx.QueryRow(`SELECT entry_type FROM soldiers WHERE id = ?`, soldier.SpouseSoldierID).Scan(&spouseType); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("selected spouse record not found")
		}
		return err
	}
	if normalizeEntryType(spouseType) != "soldier" {
		return fmt.Errorf("selected spouse must be a soldier record")
	}
	if soldier.EntryType == "linked_person" {
		if soldier.RelationshipLabel == "" {
			return fmt.Errorf("relationship_label required")
		}
		return nil
	}
	soldier.RelationshipLabel = ""
	return nil
}

func normalizeEntryType(entryType string) string {
	switch strings.ToLower(strings.TrimSpace(entryType)) {
	case "wife":
		return "wife"
	case "widow":
		return "widow"
	case "linked_person":
		return "linked_person"
	// v60 (issue #320): Event Record subtype.
	case "event":
		return "event"
	default:
		return "soldier"
	}
}

func normalizeConfederateHomeFields(soldier *models.Soldier) {
	soldier.ConfederateHomeStatus = confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)
	soldier.ConfederateHomeName = strings.TrimSpace(soldier.ConfederateHomeName)
	if soldier.ConfederateHomeStatus == confederatehomestatus.NotApplicable {
		soldier.ConfederateHomeName = ""
	}
}

func (s *SoldierService) loadFormSuggestions() (models.SoldierFormSuggestions, error) {
	rankIn, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(rank_in) FROM soldiers WHERE rank_in IS NOT NULL AND TRIM(rank_in) <> '' ORDER BY TRIM(rank_in)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	rankOut, err := distinctTextValues(s.db.Conn(), `SELECT value FROM (
		SELECT DISTINCT TRIM(rank_out) AS value FROM soldiers WHERE rank_out IS NOT NULL AND TRIM(rank_out) <> ''
		UNION
		SELECT DISTINCT TRIM(rank) AS value FROM soldiers WHERE rank IS NOT NULL AND TRIM(rank) <> ''
	) ORDER BY value`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	unit, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(unit) FROM soldiers WHERE unit IS NOT NULL AND TRIM(unit) <> '' ORDER BY TRIM(unit)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	prefix, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(prefix) FROM soldiers WHERE prefix IS NOT NULL AND TRIM(prefix) <> '' ORDER BY TRIM(prefix)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	suffix, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(suffix) FROM soldiers WHERE suffix IS NOT NULL AND TRIM(suffix) <> '' ORDER BY TRIM(suffix)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	pensionState, err := distinctNormalizedTextValues(s.db.Conn(), `SELECT `+normalizedPensionStateExpr+` AS value FROM soldiers GROUP BY value ORDER BY value`, normalizeOptionalPensionState)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	buriedIn, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(buried_in) FROM soldiers WHERE buried_in IS NOT NULL AND TRIM(buried_in) <> '' ORDER BY TRIM(buried_in)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	confederateHomeName, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(confederate_home_name) FROM soldiers WHERE confederate_home_name IS NOT NULL AND TRIM(confederate_home_name) <> '' ORDER BY TRIM(confederate_home_name)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	recordType, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(record_type) FROM records WHERE record_type IS NOT NULL AND TRIM(record_type) <> '' ORDER BY TRIM(record_type)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	relationshipLabel, err := distinctTextValues(s.db.Conn(), `SELECT DISTINCT TRIM(relationship_label) FROM soldiers WHERE LOWER(TRIM(entry_type)) = 'linked_person' AND relationship_label IS NOT NULL AND TRIM(relationship_label) <> '' ORDER BY TRIM(relationship_label)`)
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}
	return models.SoldierFormSuggestions{
		RankIn:              rankIn,
		RankOut:             rankOut,
		Unit:                unit,
		Prefix:              prefix,
		Suffix:              suffix,
		PensionState:        pensionState,
		BuriedIn:            buriedIn,
		ConfederateHomeName: confederateHomeName,
		RecordType:          recordType,
		RelationshipLabel:   relationshipLabel,
	}, nil
}

func distinctTextValues(conn *sql.DB, query string) ([]string, error) {
	rows, err := conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "distinctTextValues.rows")

	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, strings.TrimSpace(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *SoldierService) invalidateFormSuggestions() {
	s.formSuggestionsMu.Lock()
	defer s.formSuggestionsMu.Unlock()
	s.formSuggestions = nil
}

func nullableInt64(value int64) interface{} {
	if value < 1 {
		return nil
	}
	return value
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

// MarriageCandidates returns the per-Soldier list of candidate marriage matches (used by the Marriage tab).
func (s *SoldierService) MarriageCandidates() ([]models.Soldier, error) {
	rows, err := s.db.Conn().Query(`SELECT ` + soldierSelectColumns + ` FROM soldiers WHERE entry_type = 'soldier' ORDER BY last_name, first_name`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "MarriageCandidates.rows")
	return scanSoldiers(rows)
}

// FormSuggestions returns the autocomplete payload for the Person Record form (units, cemeteries, ranks, etc.).
func (s *SoldierService) FormSuggestions() (models.SoldierFormSuggestions, error) {
	s.formSuggestionsMu.RLock()
	if s.formSuggestions != nil {
		cached := *s.formSuggestions
		s.formSuggestionsMu.RUnlock()
		return cached, nil
	}
	s.formSuggestionsMu.RUnlock()

	suggestions, err := s.loadFormSuggestions()
	if err != nil {
		return models.SoldierFormSuggestions{}, err
	}

	s.formSuggestionsMu.Lock()
	s.formSuggestions = &suggestions
	s.formSuggestionsMu.Unlock()
	return suggestions, nil
}

func spouseReference(conn *sql.DB, spouseSoldierID int64) string {
	if spouseSoldierID < 1 {
		return ""
	}
	// COALESCE is used because the prefix and middle_name columns
	// can be NULL in the DB, and the database/sql Scan cannot
	// convert NULL into a plain string. Without this the scan
	// returns an error, the function falls through to the
	// display_id lookup, and the rendered PDF shows the
	// display_id (e.g. \"DXD-00082\") instead of the actual
	// name (e.g. \"James H. Magness\").
	var prefix, first, middle, last, suffix string
	if err := conn.QueryRow(
		`SELECT COALESCE(prefix, ''), COALESCE(first_name, ''), COALESCE(middle_name, ''), COALESCE(last_name, ''), COALESCE(suffix, '') FROM soldiers WHERE id = ?`,
		spouseSoldierID,
	).Scan(&prefix, &first, &middle, &last, &suffix); err == nil {
		spouse := models.Soldier{
			Prefix:    prefix,
			FirstName: first,
			MiddleName: middle,
			LastName:  last,
			Suffix:    suffix,
		}
		if fullName := strings.TrimSpace(spouse.GetFullName()); fullName != "" {
			return fullName
		}
	}
	var displayID string
	if err := conn.QueryRow(`SELECT display_id FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&displayID); err == nil {
		return strings.TrimSpace(displayID)
	}
	return ""
}

func spouseDisplayID(conn *sql.DB, spouseSoldierID int64) string {
	if spouseSoldierID < 1 {
		return ""
	}
	var displayID string
	if err := conn.QueryRow(`SELECT display_id FROM soldiers WHERE id = ?`, spouseSoldierID).Scan(&displayID); err != nil {
		return ""
	}
	return strings.TrimSpace(displayID)
}

func normalizeDisplayID(displayID, nodePrefix string) string {
	return db.SanitizeID(displayID, nodePrefix)
}

func normalizeSoldierDates(soldier *models.Soldier) error {
	birthDate := strings.TrimSpace(soldier.BirthDate)
	if birthDate == "" {
		birthDate = dates.ParseBirthInfo(strings.TrimSpace(soldier.BirthInfo))
	}
	normalizedBirth, err := dates.NormalizeCanonical(birthDate)
	if err != nil {
		return fmt.Errorf("invalid birth_date")
	}
	soldier.BirthDate = normalizedBirth

	deathDate := strings.TrimSpace(soldier.DeathDate)
	if deathDate == "" {
		deathDate = dates.MustFormat(soldier.DeathMonth, soldier.DeathDay, soldier.DeathYear)
	}
	normalizedDeath, err := dates.NormalizeCanonical(deathDate)
	if err != nil {
		return fmt.Errorf("invalid death_date")
	}
	soldier.DeathDate = normalizedDeath
	hydrateLegacyDeathParts(soldier)
	return nil
}

func currentSQLiteTimestamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func hydrateLegacyDeathParts(soldier *models.Soldier) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.DeathDate))
	if err != nil {
		return
	}
	soldier.DeathMonth = partial.Month
	soldier.DeathDay = partial.Day
	soldier.DeathYear = partial.Year
}

func hydrateSoldierIdentity(tx *sql.Tx, soldier *models.Soldier) error {
	row := tx.QueryRow(`SELECT sync_id, added_by, created_at, needs_review, review_reason FROM soldiers WHERE id = ?`, soldier.ID)
	var currentSyncID sql.NullString
	var addedBy sql.NullString
	var createdAt sql.NullString
	var reviewReason sql.NullString
	var needsReview bool
	if err := row.Scan(&currentSyncID, &addedBy, &createdAt, &needsReview, &reviewReason); err != nil {
		return err
	}
	if strings.TrimSpace(soldier.SyncID) == "" {
		soldier.SyncID = currentSyncID.String
	}
	if strings.TrimSpace(soldier.AddedBy) == "" {
		soldier.AddedBy = addedBy.String
	}
	if strings.TrimSpace(soldier.CreatedAt) == "" {
		soldier.CreatedAt = createdAt.String
	}
	if !soldier.NeedsReview && strings.TrimSpace(soldier.ReviewReason) == "" {
		soldier.NeedsReview = needsReview
		soldier.ReviewReason = reviewReason.String
	}
	return nil
}

func (s *SoldierService) currentAuditActor() string {
	if identity, err := s.db.UserIdentity(); err == nil {
		if branding := strings.TrimSpace(identity.BrandingName()); branding != "" {
			return branding
		}
		fullName := strings.TrimSpace(strings.Join([]string{identity.FirstName, identity.MiddleName, identity.LastName}, " "))
		if fullName != "" {
			return fullName
		}
	}
	if nodePrefix, err := s.db.NodePrefix(); err == nil && strings.TrimSpace(nodePrefix) != "" {
		return strings.TrimSpace(nodePrefix)
	}
	return "Unknown"
}

func stampCreateAuditFields(actor string, soldier *models.Soldier) {
	if strings.TrimSpace(soldier.AddedBy) == "" {
		soldier.AddedBy = actor
	}
	if strings.TrimSpace(soldier.LastEditedBy) == "" {
		soldier.LastEditedBy = actor
	}
	if strings.TrimSpace(soldier.LastEditedFields) == "" {
		soldier.LastEditedFields = "created"
	}
	if strings.TrimSpace(soldier.LastEditedAt) == "" {
		soldier.LastEditedAt = soldier.UpdatedAt
	}
}

func stampUpdateAuditFields(actor string, before *models.Soldier, soldier *models.Soldier) {
	if strings.TrimSpace(soldier.AddedBy) == "" && before != nil {
		soldier.AddedBy = before.AddedBy
	}
	changed := diffSoldierFields(before, soldier)
	if len(changed) == 0 {
		changed = []string{"Metadata updated."}
	}
	soldier.LastEditedBy = actor
	soldier.LastEditedFields = strings.Join(changed, "\n")
	soldier.LastEditedAt = soldier.UpdatedAt
}

func diffSoldierFields(before *models.Soldier, after *models.Soldier) []string {
	if before == nil || after == nil {
		return []string{"Metadata updated."}
	}
	type comparedField struct {
		label  string
		before string
		after  string
	}
	fields := []comparedField{
		{"Display ID", auditDisplayID(strings.TrimSpace(before.DisplayID)), auditDisplayID(strings.TrimSpace(after.DisplayID))},
		{"Person Record Type", auditEntryType(strings.TrimSpace(before.EntryType)), auditEntryType(strings.TrimSpace(after.EntryType))},
		{"Linked Spouse Record", auditSpouseID(before.SpouseSoldierID), auditSpouseID(after.SpouseSoldierID)},
		{"Relationship to Soldier", auditTextValue(before.RelationshipLabel), auditTextValue(after.RelationshipLabel)},
		{"Maiden Name", auditTextValue(before.MaidenName), auditTextValue(after.MaidenName)},
		{"Pension ID", auditTextValue(before.PensionID), auditTextValue(after.PensionID)},
		{"Application ID", auditTextValue(before.ApplicationID), auditTextValue(after.ApplicationID)},
		{"Prefix", auditTextValue(before.Prefix), auditTextValue(after.Prefix)},
		{"Show Prefix Before Name", auditBoolValue(before.ShowPrefixBeforeName), auditBoolValue(after.ShowPrefixBeforeName)},
		{"First Name", auditTextValue(before.FirstName), auditTextValue(after.FirstName)},
		{"Middle Name", auditTextValue(before.MiddleName), auditTextValue(after.MiddleName)},
		{"Last Name", auditTextValue(before.LastName), auditTextValue(after.LastName)},
		{"Suffix", auditTextValue(before.Suffix), auditTextValue(after.Suffix)},
		{"Rank In", auditTextValue(before.RankIn), auditTextValue(after.RankIn)},
		{"Rank Out", auditTextValue(before.RankOut), auditTextValue(after.RankOut)},
		{"Unit", auditTextValue(before.Unit), auditTextValue(after.Unit)},
		{"Pension State", auditTextValue(before.PensionState), auditTextValue(after.PensionState)},
		{"Confederate Home Status", auditTextValue(before.ConfederateHomeStatus), auditTextValue(after.ConfederateHomeStatus)},
		{"Confederate Home Name", auditTextValue(before.ConfederateHomeName), auditTextValue(after.ConfederateHomeName)},
		{"Birth Date", auditDateValue(before.BirthDate), auditDateValue(after.BirthDate)},
		{"Death Date", auditDateValue(before.DeathDate), auditDateValue(after.DeathDate)},
		{"Birth Info", auditLongTextValue(before.BirthInfo), auditLongTextValue(after.BirthInfo)},
		{"Buried In", auditTextValue(before.BuriedIn), auditTextValue(after.BuriedIn)},
		{"Notes", auditLongTextValue(before.Notes), auditLongTextValue(after.Notes)},
		{"Needs Review", auditBoolValue(before.NeedsReview), auditBoolValue(after.NeedsReview)},
		{"Review Reason", auditLongTextValue(before.ReviewReason), auditLongTextValue(after.ReviewReason)},
	}
	changed := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		if field.before != field.after {
			changed = append(changed, fmt.Sprintf("%s changed from %s to %s.", field.label, field.before, field.after))
		}
	}
	if !recordsEqual(before.Records, after.Records) {
		changed = append(changed, "Records updated.")
	}
	return changed
}

func auditDisplayID(value string) string {
	if strings.TrimSpace(value) == "" {
		return "\"N/A\""
	}
	return fmt.Sprintf("%q", strings.TrimSpace(value))
}

func auditEntryType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "wife":
		return "\"Wife\""
	case "widow":
		return "\"Widow\""
	case "linked_person":
		return "\"Person Record\""
	default:
		return "\"Soldier\""
	}
}

func auditSpouseID(value int64) string {
	if value <= 0 {
		return "\"N/A\""
	}
	return fmt.Sprintf("%q", fmt.Sprintf("DB ID %d", value))
}

func auditTextValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "\"N/A\""
	}
	return fmt.Sprintf("%q", trimmed)
}

func auditLongTextValue(value string) string {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if normalized == "" {
		return "\"N/A\""
	}
	if len(normalized) > 72 {
		normalized = normalized[:69] + "..."
	}
	return fmt.Sprintf("%q", normalized)
}

func auditDateValue(value string) string {
	display := strings.TrimSpace(dates.Display(strings.TrimSpace(value)))
	if display == "" || display == "N/A" {
		return "\"N/A\""
	}
	return fmt.Sprintf("%q", display)
}

func auditBoolValue(value bool) string {
	if value {
		return `"Yes"`
	}
	return `"No"`
}

func recordsEqual(left, right []models.Record) bool {
	left = normalizeRecords(left)
	right = normalizeRecords(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if strings.TrimSpace(left[index].RecordType) != strings.TrimSpace(right[index].RecordType) ||
			strings.TrimSpace(left[index].AppID) != strings.TrimSpace(right[index].AppID) ||
			strings.TrimSpace(left[index].Details) != strings.TrimSpace(right[index].Details) {
			return false
		}
	}
	return true
}

func loadSoldierAuditSnapshot(tx *sql.Tx, soldierID int64) (*models.Soldier, error) {
	row := tx.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id = ?`, soldierID)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(`SELECT `+recordSelectColumns+` FROM records WHERE person_record_id = ? ORDER BY sort_order, id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadSoldierAuditSnapshot.rows")
	for rows.Next() {
		var record models.Record
		if err := rows.Scan(&record.ID, &record.SyncID, &record.PersonRecordID, &record.PersonSyncID, &record.RecordType, &record.AppID, &record.Details, &record.SortOrder); err != nil {
			return nil, err
		}
		soldier.Records = append(soldier.Records, record)
	}
	return soldier, rows.Err()
}

func (s *SoldierService) touchAuditFields(soldierID int64, fields ...string) error {
	actor := s.currentAuditActor()
	updatedAt := currentSQLiteTimestamp()
	changedFields := strings.Join(auditTouchDescriptions(fields), "\n")
	_, err := s.db.Conn().Exec(`UPDATE soldiers SET last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, updated_at = ? WHERE id = ?`,
		actor, changedFields, updatedAt, updatedAt, soldierID)
	return err
}

func auditTouchDescriptions(fields []string) []string {
	if len(fields) == 0 {
		return []string{"Metadata updated."}
	}
	descriptions := make([]string, 0, len(fields))
	for _, field := range fields {
		switch strings.TrimSpace(strings.ToLower(field)) {
		case "images":
			descriptions = append(descriptions, "Images updated.")
		case "primary_image":
			descriptions = append(descriptions, "Primary image updated.")
		case "records":
			descriptions = append(descriptions, "Records updated.")
		case "needs_review":
			descriptions = append(descriptions, "Review status updated.")
		case "review_status":
			descriptions = append(descriptions, "Review queue cleared.")
		default:
			label := strings.ReplaceAll(strings.TrimSpace(field), "_", " ")
			label = strings.TrimSpace(strings.Title(label))
			if label == "" {
				label = "Metadata"
			}
			descriptions = append(descriptions, label+" updated.")
		}
	}
	return descriptions
}

func (s *SoldierService) soldierSyncIDByID(soldierID int64) (string, error) {
	var syncID string
	if err := s.db.Conn().QueryRow(`SELECT sync_id FROM soldiers WHERE id = ?`, soldierID).Scan(&syncID); err != nil {
		return "", err
	}
	return syncID, nil
}

func (s *SoldierService) shouldAssignPrimaryImage(soldierID int64) (bool, error) {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE person_record_id = ?`, soldierID).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *SoldierService) ensurePrimaryImage(soldierID int64) error {
	var count int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE person_record_id = ?`, soldierID).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var primaryCount int
	if err := s.db.Conn().QueryRow(`SELECT COUNT(1) FROM images WHERE person_record_id = ? AND is_primary = 1`, soldierID).Scan(&primaryCount); err != nil {
		return err
	}
	if primaryCount > 0 {
		return nil
	}
	_, err := s.db.Conn().Exec(`UPDATE images SET is_primary = CASE WHEN id = (
		SELECT id FROM images WHERE person_record_id = ? ORDER BY id LIMIT 1
	) THEN 1 ELSE 0 END WHERE person_record_id = ?`, soldierID, soldierID)
	return err
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

type scannerFunc func(any) error

// Scan runs the data-quality scan over every Soldier in the archive; surfaces issues by kind.
func (f scannerFunc) Scan(value any) error {
	return f(value)
}

func canonicalRank(soldier models.Soldier) string {
	if strings.TrimSpace(soldier.RankOut) != "" {
		return strings.TrimSpace(soldier.RankOut)
	}
	if strings.TrimSpace(soldier.Rank) != "" {
		return strings.TrimSpace(soldier.Rank)
	}
	return strings.TrimSpace(soldier.RankIn)
}

// ByIDs (issue #182) returns the soldiers whose IDs are in the
// supplied slice, preserving the caller's order. Unknown IDs
// are silently dropped. Empty input returns an empty (non-nil)
// slice so callers can index without nil checks. Used by the
// Share Queue subset export to materialise a single staged
// shipment; mirrors RecentByIDs without the limit.
func (s *SoldierService) ByIDs(ids []int64) ([]models.Soldier, error) {
	if len(ids) == 0 {
		return []models.Soldier{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.Conn().Query(
		"SELECT "+soldierListSelectColumns+" FROM soldiers WHERE id IN ("+placeholders+")",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ByIDs.rows")
	var found []models.Soldier
	for rows.Next() {
		var soldier models.Soldier
		if err := rows.Scan(soldierListScanDest(&soldier)...); err != nil {
			return nil, err
		}
		hydrateLegacyDeathParts(&soldier)
		soldier.PensionState = pensionstate.Normalize(soldier.PensionState)
		normalizeConfederateHomeFields(&soldier)
		found = append(found, soldier)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	index := map[int64]models.Soldier{}
	for _, s := range found {
		index[s.ID] = s
	}
	out := make([]models.Soldier, 0, len(ids))
	for _, id := range ids {
		if soldier, ok := index[id]; ok {
			out = append(out, soldier)
		}
	}
	return out, nil
}


// LinkedEventTimelineMarker moved to event_service.go as part
// of issue #343 finding #5 — the JOIN against event_person_links
// belongs on EventService, not SoldierService. ServiceTimeline
// delegates to EventService.LinkedEventsForTimeline via the
// back-reference set by SetEvents.

func searchableFirstName(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.FirstName) + " " + strings.TrimSpace(soldier.MiddleName))
}

func searchableLastName(soldier models.Soldier) string {
	return strings.TrimSpace(soldier.LastName)
}

func searchableUnit(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.Unit) + " " + strings.TrimSpace(soldier.PensionState))
}

func searchableRank(soldier models.Soldier) string {
	return strings.TrimSpace(strings.TrimSpace(soldier.RankIn) + " " + strings.TrimSpace(soldier.RankOut))
}

// RecoverDisplayID mints a fresh DXDID for the soldier with the
// given id and writes it back to the row, but ONLY when the
// row's display_id is currently empty (issue #416). Returns
// ErrSoldierNotFound (wrapped sql.ErrNoRows) when no row exists;
// ErrDisplayIDNotEmpty when the row already has a non-empty id.
// The UPDATE includes the empty-guard in the WHERE clause so
// concurrent / stale clicks are no-ops (rows_affected == 0) rather
// than overwrites — the handler surfaces a 409 in that case.
// Stamps last_edited_by / last_edited_at / updated_at to mark
// the recovery in the audit trail.
func (s *SoldierService) RecoverDisplayID(id int64) (string, error) {
	row, err := s.GetByID(id)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.DisplayID) != "" {
		return "", fmt.Errorf("%w", ErrDisplayIDNotEmpty)
	}
	minted, err := s.db.NextDXDID()
	if err != nil {
		return "", err
	}
	now := currentSQLiteTimestamp()
	audit := s.currentAuditActor()
	res, err := s.db.Conn().Exec(
		`UPDATE soldiers SET display_id = ?, last_edited_by = ?, last_edited_at = ?, updated_at = ? WHERE id = ? AND display_id = ''`,
		minted, audit, now, now, id,
	)
	if err != nil {
		return "", err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		// Race: another caller mints between our GetByID and
		// our UPDATE. Re-read and return the now-existing id so
		// the handler can respond 200 with the real value.
		row, err := s.GetByID(id)
		if err != nil {
			return "", err
		}
		return row.DisplayID, nil
	}
	return minted, nil
}

// MoveRecordWithinPerson reorders a Source Record within its
// owning Person so it lands at `position` (1-indexed) in the
// sort_order sequence. The method writes a single transaction
// that shifts the other rows' sort_order so the requested row
// can take its new slot. Clamps `position` to [1, N] where N is
// the row count. Returns an error if the record doesn't exist
// OR doesn't belong to the supplied person (the WHERE clause
// scopes the UPDATE so a foreign record id is a no-op + 0 rows
// affected, which we surface as an error to the caller).
//
// Issue #368 slice 2.
func (s *SoldierService) MoveRecordWithinPerson(personID, recordID, position int64) error {
	if personID < 1 || recordID < 1 {
		return fmt.Errorf("person id and record id must be positive")
	}
	conn := s.db.Conn()
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Verify the record exists AND belongs to this person.
	var ownerID int64
	if err := tx.QueryRow(
		`SELECT person_record_id FROM records WHERE id = ?`,
		recordID,
	).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID != personID {
		return fmt.Errorf("record %d is not attached to person %d", recordID, personID)
	}

	// Count N (the current row count for this person) so we can
	// clamp position. N is stable within a single transaction.
	var n int64
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM records WHERE person_record_id = ?`,
		personID,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no records to reorder for person %d", personID)
	}
	if position < 1 {
		position = 1
	}
	if position > n {
		position = n
	}

	// Shift every other row's sort_order so the requested row
	// can take its new slot. Two strategies depending on whether
	// the row is moving up or down.
	//
	// The shift is +1 for every row between the requested row's
	// current position and the new position (inclusive on one
	// end, exclusive on the other), then the requested row gets
	// `position`. The single-transaction shape avoids an interim
	// state where two rows share a sort_order.
	//
	// First: read the requested row's current sort_order.
	var currentOrder int64
	if err := tx.QueryRow(
		`SELECT sort_order FROM records WHERE id = ?`,
		recordID,
	).Scan(&currentOrder); err != nil {
		return err
	}

	if currentOrder < position {
		// Moving down: rows between currentOrder+1 and position
		// shift up by -1.
		if _, err := tx.Exec(
			`UPDATE records SET sort_order = sort_order - 1 WHERE person_record_id = ? AND id <> ? AND sort_order > ? AND sort_order <= ?`,
			personID, recordID, currentOrder, position,
		); err != nil {
			return err
		}
	} else if currentOrder > position {
		// Moving up: rows between position and currentOrder-1
		// shift down by +1.
		if _, err := tx.Exec(
			`UPDATE records SET sort_order = sort_order + 1 WHERE person_record_id = ? AND id <> ? AND sort_order >= ? AND sort_order < ?`,
			personID, recordID, position, currentOrder,
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`UPDATE records SET sort_order = ? WHERE id = ?`,
		position, recordID,
	); err != nil {
		return err
	}

	return tx.Commit()
}
