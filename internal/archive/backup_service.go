package archive

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/records"
)

const backupFormatName = "dixiedata-backup"
const (
	archiveKindBackup = "backup"
	archiveKindShared = "shared"
)

// Issue #383 slice 7: typed error for major-bump .ddbak
// format_version refusals. The same pattern as
// records.ErrMemorialFormatMismatch — GUI handlers use
// errors.Is to surface a friendly "refused" message
// (KindValidation), CLI runners branch to exit code 2.
var ErrDDBakFormatMismatch = errors.New("ddbak archive format mismatch")

// BackupManifest is the metadata envelope written into every
// backup-archive (.ddbak) export. Carries the format + version
// (so the restore pipeline can apply forward-migrations), the
// per-archive-kind flag, the app + schema version the archive
// was produced under, and the U + N counters (issue #266's
// version shape).
type BackupManifest struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	ArchiveKind   string `json:"archive_kind,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	// FormatVersion (issue #383 slice 6) is the discoverable
	// per-surface stamp for the .ddbak envelope. Distinct from
	// the legacy integer `Version` (the row-shape counter); the
	// per-surface namespace pattern (Decision 1 in #383)
	// decouples from DixieData semver. Bumped on DixieData-side
	// .ddbak shape changes (manifest schema changes, new top-
	// level files added, restore-side field renames).
	FormatVersion string `json:"format_version,omitempty"`
	// CurrentUpdateFlowVersion (U) and ReleaseCounter (N) are
	// the explicit axes of the v{MAJOR}.{U}.{N} version split
	// (issues #266 + #296). New backups write both fields so
	// readers can compare U without re-parsing the AppVersion
	// string. Old archives pre-#296 omit both; readers treat
	// missing U as 1 (legacy v1.2.N strings parse to U=1 per
	// #266 decision 1) and missing N as SchemaVersion (the
	// historical formula tied N to schema).
	CurrentUpdateFlowVersion int `json:"current_update_flow_version,omitempty"`
	ReleaseCounter           int `json:"release_counter,omitempty"`
	NodePrefix    string `json:"node_prefix,omitempty"`
	OwnerName     string `json:"owner_name,omitempty"`
	SourceNodeID  string `json:"source_node_id,omitempty"`
	SourceLabel   string `json:"source_node_label,omitempty"`
	CreatedAt     string `json:"created_at"`
	DataFormat    string `json:"data_format,omitempty"`
	DataFile      string `json:"data_file,omitempty"`
	DatabaseFile  string `json:"database_file,omitempty"`
	ImageRoot     string `json:"image_root"`
	Soldiers      int    `json:"soldiers"`
	Records       int    `json:"records"`
	Images        int    `json:"images"`
	// Events (issue #320 child #334) is the per-archive Event
	// Record count. Emitted on the shared-archive export path
	// when at least one Event row exists. Backwards-compatible:
	// archives produced before this field landed read 0 for both
	// Events count and missing data_events.json, so the import
	// path silently drops the events array when the file is
	// absent. The spec calls out "always include Event Records
	// (no toggle)" so the export is unconditional even when
	// Events == 0 (the file is written as an empty array).
	Events        int    `json:"events,omitempty"`
	// DataEventsFile is the zip-internal path to the Event
	// Records JSON. Defaults to "data/events.json" when omitted
	// (which is most older archives: they don't carry events at
	// all, so the import path treats the absent file as "no
	// events to merge").
	DataEventsFile string `json:"data_events_file,omitempty"`
	// Articles (issue #321 slice 5.1) is the per-archive Article
	// Record count. Emitted on the shared-archive export path
	// when at least one Article row exists. Same shape as
	// Events: backwards-compatible (older archives omit the
	// field + the data/articles.json file, so the import path
	// silently drops the articles array when the file is
	// absent). The spec calls out "always include Articles
	// (no toggle)" so the export is unconditional.
	Articles int `json:"articles,omitempty"`
	// DataArticlesFile is the zip-internal path to the Article
	// Records JSON. Defaults to "data/articles.json" when omitted.
	DataArticlesFile string `json:"data_articles_file,omitempty"`
	// DataArticleRefsFile is the zip-internal path to the
	// article_refs junction rows. Defaults to
	// "data/article_refs.json" when omitted.
	DataArticleRefsFile string `json:"data_article_refs_file,omitempty"`
}

func loadSharedAliasTargetSnapshot(tx *sql.Tx, sourceNodeID, sourcePersonSyncID string) (*mergeReviewSnapshot, error) {
	sourceNodeID = strings.TrimSpace(sourceNodeID)
	sourcePersonSyncID = strings.TrimSpace(sourcePersonSyncID)
	if sourceNodeID == "" || sourcePersonSyncID == "" {
		return nil, nil
	}
	var (
		canonicalSoldierID int64
		canonicalSyncID    string
	)
	err := tx.QueryRow(`SELECT canonical_person_id, canonical_person_sync_id
		FROM shared_merge_aliases
		WHERE source_node_id = ? AND source_person_sync_id = ?`,
		sourceNodeID, sourcePersonSyncID,
	).Scan(&canonicalSoldierID, &canonicalSyncID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	snapshot, err := loadSoldierSnapshotByID(tx, canonicalSoldierID)
	if err == sql.ErrNoRows && strings.TrimSpace(canonicalSyncID) != "" {
		return loadSoldierSnapshotBySync(tx, canonicalSyncID)
	}
	return snapshot, err
}

// BackupService owns the backup-archive pipeline: producing full
// replacement SQLite snapshots (.ddbak) plus the per-record metadata
// the backup UI surfaces. Constructed by NewBackupService and held
// by *App. Backup operations are guarded by the inFlight dialog
// guard law (CONTEXT.md §Laws) because the native SaveFileDialog
// cannot run concurrently with itself.
type BackupService struct {
	db      *db.DB
	soldier *SoldierService
}

type backupContents struct {
	Manifest BackupManifest
	FileMap  map[string]*zip.File
	Soldiers []models.Soldier
	// Events (issue #320 child #334): Event Records read from
	// the shared archive's data/events.json file. Empty when
	// the file is absent (older archives).
	Events []models.Soldier
	// Articles (issue #321 slice 5.1): Article Records read from
	// the shared archive's data/articles.json file. Empty when
	// the file is absent (older archives).
	Articles []models.Article
	// ArticleRefs: the per-article person ref junction rows
	// (slice 5.1) read from data/article_refs.json. Empty when
	// the file is absent.
	ArticleRefs []records.ArticleRef
}

// SharedImportSummary is the per-import result the share-queue
// flow surfaces: how many Soldiers + Source Records + Images
// were imported, how many were skipped (display-ID collision),
// and any errors.
type SharedImportSummary struct {
	SoldiersInserted int
	SoldiersUpdated  int
	SoldiersSkipped  int
	RecordsInserted  int
	RecordsUpdated   int
	ImagesInserted   int
	ImagesUpdated    int
	// Issue #320 child #334: Event Records share the soldiers
	// table so SoldiersInserted / Skipped counts both Person +
	// Event rows. The dedicated Events* fields let the share-queue
	// summary card surface the Event count separately so the user
	// sees "3 events linked" alongside "12 soldiers merged".
	EventsInserted int
	EventsSkipped  int
	EventsLinked   int
	PendingConflicts int
	LogPath          string
}

type mergeLogger struct {
	path  string
	lines []string
}

type mergeReviewSnapshot struct {
	Soldier         models.Soldier `json:"soldier"`
	SpouseSyncID    string         `json:"spouse_sync_id,omitempty"`
	SourceNodeID    string         `json:"source_node_id,omitempty"`
	SourceNodeLabel string         `json:"source_node_label,omitempty"`
}

type sharedMergeTarget struct {
	SoldierID   int64
	SoldierSync string
}

// SourceConflictLedger is the per-Soldier ledger of unresolved
// conflicts between a Local Source Record and an incoming merge
// candidate. Surfaced on the Conflict Ledger tab; per-record
// resolution entries live in SourceConflictLedgerEntry.
type SourceConflictLedger struct {
	Central       models.Soldier
	Entries       []SourceConflictLedgerEntry
	OpenCount     int
	ResolvedCount int
}

// SourceConflictLedgerEntry is one row in SourceConflictLedger:
// the conflict, the resolution, the timestamp, and the user who
// resolved it.
type SourceConflictLedgerEntry struct {
	ID               int64
	ConflictType     string
	Reason           string
	SourceDisplayID  string
	Resolution       string
	CreatedAt        string
	ResolvedAt       string
	LocalSnapshot    models.Soldier
	SourceSnapshot   models.Soldier
	DifferenceFields []string
}

// NewBackupService constructs a BackupService bound to the given
// database and soldier service. The soldier service is used for
// per-record metadata enrichment (display IDs, computed dates);
// the database is used for both the source SQLite snapshot and the
// pre-backup retained-snapshot bookkeeping.
func NewBackupService(database *db.DB, soldier *SoldierService) *BackupService {
	return &BackupService{db: database, soldier: soldier}
}

// Export produces a full-replacement Backup Archive (.ddbak) at
// outputPath. The output is a complete SQLite snapshot of the
// Local Archive plus the per-record metadata needed to rehydrate
// images on restore. Returns the manifest the restore pipeline
// reads to decide whether to apply a forward-migration.
func (b *BackupService) Export(outputPath, dataDir string) (BackupManifest, error) {
	return b.exportArchive(outputPath, dataDir, archiveKindBackup)
}

// ExportShared produces a Shared Archive (.ddshare) at outputPath.
// The shared archive is the user-mergeable shape: per-Soldier
// records + optional tag inclusion (issue #183) + the per-user
// identity header so the recipient can re-merge into their Local
// Archive. Refuses to overwrite an existing file at outputPath.
func (b *BackupService) ExportShared(outputPath, dataDir string) (BackupManifest, error) {
	return b.ExportSharedWithTags(outputPath, dataDir, false)
}

// ExportSharedSubset (issue #182) writes a Shared Archive
// containing only the Person Records whose IDs are in the
// supplied slice. Mirrors ExportSharedWithTags: same archive
// kind, same JSON payload shape, same image-set semantics —
// just a filtered soldier slice. The manifest carries
// Notes describing the subset ("subset export of N Person
// Records selected from the Local Archive") so a recipient
// can tell at a glance that the archive is not full.
//
// IDs that do not exist in the Local Archive at export time
// are silently dropped (SoldierService.ByIDs contract) so
// a long-running session can stage ids, then drop a few rows,
// then export — no manual cleanup needed.
//
// Source Records, Images, and the merged Tags array all
// filter alongside the soldier selection: the image-set
// collection walks the subset only, never the full archive.
func (b *BackupService) ExportSharedSubset(outputPath, dataDir string, ids []int64) (BackupManifest, error) {
	if len(ids) == 0 {
		return BackupManifest{}, fmt.Errorf("subset export requires at least one selected_ids entry")
	}
	manifest, err := b.loadBackupData(archiveKindShared)
	if err != nil {
		return BackupManifest{}, err
	}
	soldiers, err := b.soldier.ByIDs(ids)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("load subset soldiers: %w", err)
	}
	if len(soldiers) == 0 {
		return BackupManifest{}, fmt.Errorf("subset export contains no existing soldiers (supplied %d ids)", len(ids))
	}
	// Tag opt-in mirrors ExportSharedWithTags so a subset export
	// inherits the same archive_meta.include_tags setting.
	includeTags := false
	tagSvc := records.NewTagService(b.db.Conn())
	tagMeta, terr := b.archiveMetaIncludeTags()
	if terr != nil {
		return BackupManifest{}, terr
	}
	includeTags = tagMeta
	if includeTags {
		soldierIDs := make([]int64, 0, len(soldiers))
		for _, s := range soldiers {
			soldierIDs = append(soldierIDs, s.ID)
		}
		tagMap, terr := tagSvc.TagsForSoldiers(context.Background(), soldierIDs)
		if terr != nil {
			return BackupManifest{}, fmt.Errorf("load tags for subset export: %w", terr)
		}
		for i := range soldiers {
			tags := tagMap[soldiers[i].ID]
			if len(tags) == 0 {
				continue
			}
			names := make([]string, 0, len(tags))
			for _, t := range tags {
				names = append(names, t.Name)
			}
			soldiers[i].Tags = names
		}
	}
	manifest.DataFormat = "json"
	manifest.DataFile = filepath.ToSlash(filepath.Join("data", "soldiers.json"))
	manifest.DatabaseFile = ""
	manifest.SourceLabel = fmt.Sprintf(
		"%s -- subset of %d Person Records selected from the Local Archive",
		strings.TrimSpace(manifest.SourceLabel), len(soldiers),
	)

	if err := writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
			return err
		}
		if err := writeBackupJSON(zipWriter, manifest.DataFile, soldiers); err != nil {
			return err
		}
		return addSelectedBackupImages(zipWriter, filepath.Join(dataDir, "images"), collectImagePaths(soldiers))
	}); err != nil {
		return BackupManifest{}, err
	}
	manifest.Soldiers = len(soldiers)
	return manifest, nil
}

// archiveMetaIncludeTags reads archive_meta.include_tags for the
// shared_archive kind (issue #183). Returns false on any error so
// the export pipeline never blocks on a stale archive_meta row.
// Reads via a direct SQL because backup_service.go can't import
// records.ArchiveMetaService without an import cycle today; the
// direct read keeps the dependency surface flat.
func (b *BackupService) archiveMetaIncludeTags() (bool, error) {
	var include int
	err := b.db.Conn().QueryRow(
		`SELECT include_tags FROM archive_meta WHERE archive_kind = ?`,
		records.ArchiveKindShared,
	).Scan(&include)
	if err != nil {
		return false, nil // treat missing row / read error as "no tags"
	}
	return include != 0, nil
}

// ExportSharedWithTags mirrors ExportShared but adds an includeTags
// switch that mirrors archive_meta.include_tags. When true, every
// soldier in the JSON payload grows a `tags: []string` field
// (issue #183). Callers (the App.handleExportSharedArchive handler
// and the test suite) read archive_meta via the ArchiveMetaService
// rather than hard-coding the switch.
//
// The shared archive zip only adds the tags array — never Source
// Record / Claim / Finding tags in v1 — and the import side uses
// TagService.AttachAdditive so existing local tags stay additive.
//
// Issue #320 child #334: shared archives also bundle Event Records
// (entry_type=event) into data/events.json. Per RPCI decision #9 there
// is no toggle — events ship unconditionally. Linked Person Records
// grow a `linked_display_ids` array (the Event Display IDs they link
// to) so the recipient's import path can recreate the junction
// without a second fetch. The import side resolves those Display IDs
// against the imported Events and calls AttachEventToPerson.
func (b *BackupService) ExportSharedWithTags(outputPath, dataDir string, includeTags bool) (BackupManifest, error) {
	manifest, err := b.loadBackupData(archiveKindShared)
	if err != nil {
		return BackupManifest{}, err
	}
	soldiers, err := listAllSoldiers(b.soldier)
	if err != nil {
		return BackupManifest{}, err
	}
	// Issue #320 child #334: keep events out of data/soldiers.json
	// so the merge path distinguishes soldier rows (with linked
	// Display IDs) from event rows (with kind / dates / description).
	// Without this filter, mergeSharedSoldiers would insert events
	// as soldiers and mergeSharedEvents would see a sparse
	// contents.Events (zero records → no junction re-attach).
	filteredSoldiers := make([]models.Soldier, 0, len(soldiers))
	for _, s := range soldiers {
		if s.EntryType == models.EntryTypeEvent {
			continue
		}
		filteredSoldiers = append(filteredSoldiers, s)
	}
	soldiers = filteredSoldiers
	if includeTags {
		tagSvc := records.NewTagService(b.db.Conn())
		ids := make([]int64, 0, len(soldiers))
		for _, s := range soldiers {
			ids = append(ids, s.ID)
		}
		tagMap, terr := tagSvc.TagsForSoldiers(context.Background(), ids)
		if terr != nil {
			return BackupManifest{}, fmt.Errorf("load tags for export: %w", terr)
		}
		for i := range soldiers {
			tags := tagMap[soldiers[i].ID]
			if len(tags) == 0 {
				continue
			}
			names := make([]string, 0, len(tags))
			for _, t := range tags {
				names = append(names, t.Name)
			}
			soldiers[i].Tags = names
		}
	}

	// Issue #320 child #334: bundle Event Records (always) and
	// denormalize linkedDisplayIds onto each soldier row so the
	// import path can re-attach without a separate fetch.
	events, err := listAllEvents(b.db)
	if err != nil {
		return BackupManifest{}, err
	}
	linkedIDs, err := loadAllEventPersonLinks(b.db)
	if err != nil {
		return BackupManifest{}, err
	}
	// Build a DisplayID-keyed lookup over the events so a row's
	// linkedDisplayIds string can be filled in.
	eventDisplayIDByRowID := make(map[int64]string, len(events))
	for _, ev := range events {
		eventDisplayIDByRowID[ev.ID] = strings.TrimSpace(ev.DisplayID)
	}
	for i := range soldiers {
		row := &soldiers[i]
		if row.EntryType == models.EntryTypeEvent {
			continue // events live in their own data file
		}
		links := linkedIDs[row.ID]
		if len(links) == 0 {
			continue
		}
		ids := make([]string, 0, len(links))
		for _, eid := range links {
			if did, ok := eventDisplayIDByRowID[eid]; ok {
				ids = append(ids, did)
			}
		}
		if len(ids) > 0 {
			row.LinkedDisplayIDs = ids
		}
	}

	manifest.DataFormat = "json"
	manifest.DataFile = filepath.ToSlash(filepath.Join("data", "soldiers.json"))
	manifest.DataEventsFile = filepath.ToSlash(filepath.Join("data", "events.json"))
	manifest.Events = len(events)
	manifest.DatabaseFile = ""

	// Issue #321 slice 5.1: Articles + article_refs ship
	// unconditionally (no toggle, per the spec's "always include
	// Article Records" rule). Read from the local archive
	// here so the zip contains the latest rows + refs.
	articles, err := listAllArticles(b.db)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("listAllArticles: %w", err)
	}
	articleRefs, err := listAllArticleRefs(b.db)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("listAllArticleRefs: %w", err)
	}
	manifest.Articles = len(articles)
	manifest.DataArticlesFile = filepath.ToSlash(filepath.Join("data", "articles.json"))
	manifest.DataArticleRefsFile = filepath.ToSlash(filepath.Join("data", "article_refs.json"))

	if err := writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
			return err
		}
		if err := writeBackupJSON(zipWriter, manifest.DataFile, soldiers); err != nil {
			return err
		}
		if err := writeBackupJSON(zipWriter, manifest.DataEventsFile, events); err != nil {
			return err
		}
		if err := writeBackupJSON(zipWriter, manifest.DataArticlesFile, articles); err != nil {
			return err
		}
		if err := writeBackupJSON(zipWriter, manifest.DataArticleRefsFile, articleRefs); err != nil {
			return err
		}
		return addSelectedBackupImages(zipWriter, filepath.Join(dataDir, "images"), collectImagePaths(soldiers))
	}); err != nil {
		return BackupManifest{}, err
	}

	return manifest, nil
}

// listAllEvents returns every Event Record row in the Local
// Archive. Implementation mirrors listAllSoldiers (uses the
// records.SoldierService.GetByID helper for the per-row scan so
// nullable columns like spouse_soldier_id already populate via
// the records package's NullInt64 conversion). The query has
// to filter on entry_type = 'event' because the shared archive
// export needs Events-only and the SoldierService.List API
// returns all subtypes mixed.
func listAllEvents(database *db.DB) ([]models.Soldier, error) {
	conn := database.Conn()
	rows, err := conn.Query(`SELECT id FROM soldiers WHERE entry_type = ? ORDER BY id`, models.EntryTypeEvent)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "listAllEvents.rows")
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	svc := NewSoldierService(database) // circular-ish but cheap; builds no event listeners
	out := make([]models.Soldier, 0, len(ids))
	for _, id := range ids {
		full, ferr := svc.GetByID(id)
		if ferr != nil {
			return nil, ferr
		}
		out = append(out, *full)
	}
	return out, nil
}

// loadAllEventPersonLinks returns event_id -> []person_id pairs
// for every row in event_person_links. Single-shot SQL keeps
// the export pipeline linear at volume (the spec calls out
// 5000 Person Records + 0-10 events each). The receiver uses
// the per-person map to drive the soldiers[i].LinkedDisplayIDs
// denormalization per issue #320 child #334.
func loadAllEventPersonLinks(database *db.DB) (map[int64][]int64, error) {
	rows, err := database.Conn().Query(
		`SELECT person_id, event_id FROM event_person_links ORDER BY person_id, event_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("query event_person_links: %w", err)
	}
	defer debug.DeferCloseLog(rows, "loadAllEventPersonLinks.rows")
	out := make(map[int64][]int64)
	for rows.Next() {
		var personID, eventID int64
		if err := rows.Scan(&personID, &eventID); err != nil {
			return nil, err
		}
		out[personID] = append(out[personID], eventID)
	}
	return out, rows.Err()
}

func (b *BackupService) exportArchive(outputPath, dataDir, archiveKind string) (BackupManifest, error) {
	manifest, err := b.loadBackupData(archiveKind)
	if err != nil {
		return BackupManifest{}, err
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-backup-export-*")
	if err != nil {
		return BackupManifest{}, err
	}
	defer os.RemoveAll(tempDir)

	snapshotPath := filepath.Join(tempDir, db.FileName)
	if err := b.db.SnapshotTo(snapshotPath); err != nil {
		return BackupManifest{}, err
	}

	if err := writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
			return err
		}
		if err := addBackupFile(zipWriter, manifest.DatabaseFile, snapshotPath); err != nil {
			return err
		}
		return addBackupImages(zipWriter, filepath.Join(dataDir, "images"))
	}); err != nil {
		return BackupManifest{}, err
	}

	return manifest, nil
}

// Import restores a Backup Archive (.ddbak) at backupPath into
// the Local Archive at dataDir. Refuses to restore a backup newer
// than the current schema version (the user must upgrade first).
// Returns the per-record summary: how many rows were restored,
// how many skipped, any errors.
func (b *BackupService) Import(backupPath, dataDir string) (BackupManifest, error) {
	localIdentity, preserveLocalIdentity, err := b.currentImportIdentity()
	if err != nil {
		return BackupManifest{}, err
	}
	return b.ImportWithLocalIdentity(backupPath, dataDir, localIdentity, preserveLocalIdentity)
}

// RestoreBackupArchive is the package-public entry point for restoring a backup archive from outside the appshell (the CLI runner and the in-place update flow both call it). Opens the zip, validates the manifest, applies the SQLite snapshot to dataDir, and returns the restored manifest for the caller to confirm.
func RestoreBackupArchive(backupPath, dataDir string) (BackupManifest, error) {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer debug.DeferCloseLog(reader, "RestoreBackupArchive.zip")

	contents, driftWarnings, err := readBackupContentsWithWarnings(&reader.Reader)
	if err != nil {
		return BackupManifest{}, err
	}
	for _, w := range driftWarnings {
		log.Printf("archive: %s", w)
	}
	if contents.Manifest.ArchiveKind != archiveKindBackup {
		return BackupManifest{}, fmt.Errorf("archive is not a backup archive")
	}
	if contents.Manifest.DataFormat != "sqlite" {
		return BackupManifest{}, fmt.Errorf("restore point archive must contain sqlite data")
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(dataDir), filepath.Base(dataDir)+"-restore-*")
	if err != nil {
		return BackupManifest{}, err
	}
	stagingActive := true
	defer func() {
		if stagingActive {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if err := restoreSnapshotArchive(stagingDir, &reader.Reader, contents); err != nil {
		return BackupManifest{}, err
	}
	if err := replaceDataDir(dataDir, stagingDir); err != nil {
		return BackupManifest{}, err
	}

	// Issue #423 slice 2: stamp every pre-existing row's
	// restored_at with the restore event timestamp so a
	// future "when was this row carried over the most recent
	// restore point?" question collapses to one query. We
	// open the restored DB (which runs the v65 migration and
	// adds the restored_at column if it isn't there yet),
	// then run a single bulk UPDATE. The columnExists guard
	// makes the stamp a no-op on extremely old archives that
	// somehow haven't been migrated to v65 (the next
	// db.Open will land the column and the rows stay NULL
	// until the next restore).
	if err := stampRestoredAtAfterRestore(dataDir); err != nil {
		// The swap already happened — log + continue rather
		// than fail the restore. The next db.Open on the new
		// data dir is the operator's natural next step; the
		// missing stamp is a degradation, not a data loss.
		log.Printf("archive: stampRestoredAtAfterRestore(%s) failed: %v (restore succeeded; restored_at will be NULL until next restore)", dataDir, err)
	}

	stagingActive = false
	return contents.Manifest, nil
}

// stampRestoredAtAfterRestore opens the restored data dir's DB
// (which triggers applySchema and the v65 ADD COLUMN for
// restored_at) and bulk-UPDATEs every row's restored_at to the
// current UTC timestamp. Used by RestoreBackupArchive to record
// the restore event on every pre-existing row. Idempotent: a
// second restore over the first re-stamps every row with the
// new timestamp (so restored_at always reflects the most-recent
// restore event, not the first one).
func stampRestoredAtAfterRestore(dataDir string) error {
	d, err := db.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open restored db: %w", err)
	}
	// Issue #449 slice 2: wrap the close in a closure so the
	// *DB.Close fires before stampRestoredAtAfterRestore
	// returns. The bare `defer Close()` form would defer the
	// call expression but Go's defer captures the result of
	// the call — debug.DeferCloseLog returns a function value
	// and the bare form defers the call to the function value,
	// which doesn't fire until the enclosing frame is gone
	// (too late for the stagingDir rename in callers that
	// follow). The wrapped form runs Close in this frame,
	// before the caller proceeds.
	defer func() { _ = d.Close() }()

	// columnExists guard: a freshly-restored archive might
	// predate v65 and the migration might somehow not have
	// run (e.g. a manually-edited DB on a v64 binary). Skip
	// silently rather than fail the restore.
	var hasRestoredAt int
	row := d.Conn().QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('soldiers') WHERE name = 'restored_at'`,
	)
	if err := row.Scan(&hasRestoredAt); err != nil {
		return fmt.Errorf("pragma_table_info(restored_at): %w", err)
	}
	if hasRestoredAt == 0 {
		return nil
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	// Stamp every row unconditionally: restored_at always
	// reflects the most-recent restore event, not the first
	// one. A second restore over the first re-stamps every
	// row with the new timestamp.
	if _, err := d.Conn().Exec(
		`UPDATE soldiers SET restored_at = ?`,
		stamp,
	); err != nil {
		return fmt.Errorf("UPDATE soldiers SET restored_at: %w", err)
	}
	return nil
}

// ImportWithLocalIdentity restores a Shared Archive using the
// current Local Archive's per-user identity (instead of the
// identity embedded in the archive). Used when the recipient
// wants to merge the shared archive into their own archive with
// their own node prefix (issue #180).
func (b *BackupService) ImportWithLocalIdentity(backupPath, dataDir string, localIdentity models.UserIdentity, preserveLocalIdentity bool) (BackupManifest, error) {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer debug.DeferCloseLog(reader, "ImportWithLocalIdentity.zip")

	contents, driftWarnings, err := readBackupContentsWithWarnings(&reader.Reader)
	if err != nil {
		return BackupManifest{}, err
	}
	for _, w := range driftWarnings {
		log.Printf("archive: %s", w)
	}
	if contents.Manifest.ArchiveKind != archiveKindBackup {
		return BackupManifest{}, fmt.Errorf("archive is not a backup archive")
	}

	extractDir, err := os.MkdirTemp("", "dixiedata-backup-*")
	if err != nil {
		return BackupManifest{}, err
	}
	defer os.RemoveAll(extractDir)

	if err := extractBackupImages(&reader.Reader, extractDir, contents.Manifest.ImageRoot); err != nil {
		return BackupManifest{}, err
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(dataDir), filepath.Base(dataDir)+"-import-*")
	if err != nil {
		return BackupManifest{}, err
	}
	stagingActive := true
	defer func() {
		if stagingActive {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	switch contents.Manifest.DataFormat {
	case "", "json":
		if err := b.restoreLegacyJSONBackup(stagingDir, extractDir, contents.Soldiers); err != nil {
			return BackupManifest{}, err
		}
	case "sqlite":
		if err := restoreSnapshotBackup(stagingDir, extractDir, contents); err != nil {
			return BackupManifest{}, err
		}
		if err := preserveSnapshotImportIdentity(stagingDir, localIdentity, preserveLocalIdentity); err != nil {
			return BackupManifest{}, err
		}
	default:
		return BackupManifest{}, fmt.Errorf("unsupported backup data format %q", contents.Manifest.DataFormat)
	}

	if err := validateStagedBackup(stagingDir, contents.Manifest); err != nil {
		return BackupManifest{}, err
	}
	if err := replaceDataDir(dataDir, stagingDir); err != nil {
		return BackupManifest{}, err
	}
	stagingActive = false

	return contents.Manifest, nil
}

func (b *BackupService) currentImportIdentity() (models.UserIdentity, bool, error) {
	complete, err := b.db.SystemConfig("user_identity_complete")
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	if strings.TrimSpace(complete) != "1" {
		return models.UserIdentity{}, false, nil
	}
	identity, err := b.db.UserIdentity()
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	return identity, true, nil
}

func preserveSnapshotImportIdentity(dataDir string, identity models.UserIdentity, preserve bool) error {
	if !preserve {
		return nil
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	// Issue #449 slice 2: wrap the close in a closure so the
	// *DB.Close fires before preserveSnapshotImportIdentity
	// returns. The bare `defer Close()` form would defer the
	// call expression but Go's defer captures the result of
	// the call — debug.DeferCloseLog returns a function value
	// and the bare form defers the call to the function value,
	// which doesn't fire until the enclosing frame is gone
	// (too late for the stagingDir rename). The wrapped form
	// runs Close in this frame, before replaceDataDir fires.
	defer func() { _ = database.Close() }()

	_, err = database.ConfigureUserIdentity(identity.FirstName, identity.MiddleName, identity.LastName, identity.BirthYear)
	return err
}

// ImportSharedBackup is the legacy-pre-issue-#183 alias of Import
// (older code paths still call it). Behavior is identical to
// Import; kept for back-compat with .ddshare archives produced
// before the issue #183 split.
func (b *BackupService) ImportSharedBackup(backupPath, dataDir string) (summary SharedImportSummary, err error) {
	logger, logErr := newMergeLogger(dataDir)
	if logErr == nil {
		defer func() {
			status := "success"
			if err != nil {
				status = "failure"
				logger.Printf("result=failure error=%v", err)
			}
			logger.Printf("finished status=%s", status)
			if finalizeErr := logger.Close(); finalizeErr == nil {
				summary.LogPath = logger.path
				if err != nil && !strings.Contains(err.Error(), logger.path) {
					err = fmt.Errorf("%w (merge log: %s)", err, logger.path)
				}
			} else if err == nil {
				err = finalizeErr
			}
		}()
		logger.Printf("started archive=%s", strings.TrimSpace(backupPath))
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return SharedImportSummary{}, err
	}
	defer debug.DeferCloseLog(reader, "ImportSharedBackup.zip")

	contents, driftWarnings, err := readBackupContentsWithWarnings(&reader.Reader)
	if err != nil {
		return SharedImportSummary{}, err
	}
	for _, w := range driftWarnings {
		log.Printf("archive: %s", w)
	}
	if contents.Manifest.ArchiveKind != archiveKindShared {
		return SharedImportSummary{}, fmt.Errorf("archive is not a shared archive")
	}
	if logger != nil {
		logger.Printf("manifest format=%s version=%d archive_kind=%s data_format=%s schema_version=%d node_prefix=%s owner_name=%q source_node_id=%q source_node_label=%q soldiers=%d records=%d images=%d",
			contents.Manifest.Format, contents.Manifest.Version, contents.Manifest.ArchiveKind, contents.Manifest.DataFormat, contents.Manifest.SchemaVersion, contents.Manifest.NodePrefix, contents.Manifest.OwnerName, contents.Manifest.SourceNodeID, contents.Manifest.SourceLabel, contents.Manifest.Soldiers, contents.Manifest.Records, contents.Manifest.Images)
	}

	sourceNodeID, sourceNodeLabel := sharedArchiveSourceIdentity(contents.Manifest)

	sessionID, err := db.NewSyncID()
	if err != nil {
		return SharedImportSummary{}, err
	}
	sessionRoot := filepath.Join(dataDir, "merge-review", sessionID)
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return SharedImportSummary{}, err
	}
	sessionActive := true
	defer func() {
		if sessionActive {
			_ = os.RemoveAll(sessionRoot)
		}
	}()

	if err := extractBackupImages(&reader.Reader, sessionRoot, contents.Manifest.ImageRoot); err != nil {
		return SharedImportSummary{}, err
	}

	switch contents.Manifest.DataFormat {
	case "", "json":
		summary, err = b.mergeSharedSoldiers(sessionID, backupPath, contents.Soldiers, sessionRoot, dataDir, sourceNodeID, sourceNodeLabel, logger)
		if err == nil {
			// Issue #320 child #334: merge Event Records AFTER
			// soldiers so target-side event IDs are known before
			// we walk LinkedDisplayIDs to recreate the junction.
			// mergeSharedEvents merges its event counters into
			// the receiver rather than returning the whole
			// summary, so a sparse shared archive (0 events)
			// does not zero out the soldier counts the prior
			// call populated.
			if err := b.mergeSharedEvents(sessionID, backupPath, contents.Events, contents.Soldiers, &summary, logger); err != nil {
				return SharedImportSummary{}, err
			}
		}
	case "sqlite":
		sourceDir, err := os.MkdirTemp("", "dixiedata-shared-backup-db-*")
		if err != nil {
			return SharedImportSummary{}, err
		}
		defer os.RemoveAll(sourceDir)

		databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
		if databaseFile == nil {
			return SharedImportSummary{}, fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
		}
		if err := extractBackupFile(databaseFile, db.Path(sourceDir)); err != nil {
			return SharedImportSummary{}, fmt.Errorf("stage shared backup database: %w", err)
		}
		sourceDB, err := db.Open(sourceDir)
		if err != nil {
			return SharedImportSummary{}, fmt.Errorf("open shared backup database: %w", err)
		}
		defer debug.DeferCloseLog(sourceDB, "ImportSharedBackup.sourceDB")

		sourceSvc := NewSoldierService(sourceDB)
		soldiers, err := listAllSoldiers(sourceSvc)
		if err != nil {
			return SharedImportSummary{}, fmt.Errorf("read shared backup database: %w", err)
		}
		summary, err = b.mergeSharedSoldiers(sessionID, backupPath, soldiers, sessionRoot, dataDir, sourceNodeID, sourceNodeLabel, logger)
		if err == nil {
			// SQLite-path cannot carry events in v1 (the shared
			// bundle's DataFormat=sqlite path predates #334); the
			// events file is JSON only. Summary's Events counts
			// stay at zero.
		}
	default:
		return SharedImportSummary{}, fmt.Errorf("unsupported backup data format %q", contents.Manifest.DataFormat)
	}
	if err != nil {
		return SharedImportSummary{}, fmt.Errorf("merge shared backup: %w", err)
	}
	// Issue #183: attach incoming tags additively. We reuse the
	// already-loaded `contents.Soldiers` snapshot (which carries
	// the optional `tags` array when the source archive had
	// archive_meta.include_tags enabled). The match uses display_id
	// since mergeSharedSoldiers may have inserted new rows, leaving
	// the imported soldier ID unreliable. Idempotent — re-running
	// the import adds no duplicate bindings.
	if b.db != nil && len(contents.Soldiers) > 0 {
		tagSvc := records.NewTagService(b.db.Conn())
		for _, incoming := range contents.Soldiers {
			if len(incoming.Tags) == 0 {
				continue
			}
			var localID int64
			if err := b.db.Conn().QueryRow(
				`SELECT id FROM soldiers WHERE display_id = ?`,
				incoming.DisplayID).Scan(&localID); err != nil {
				continue
			}
			for _, tagName := range incoming.Tags {
				if _, terr := tagSvc.AttachAdditive(context.Background(), localID, tagName); terr != nil {
					if logger != nil {
						logger.Printf("warn tag attach failed display_id=%q tag=%q err=%v", incoming.DisplayID, tagName, terr)
					}
				}
			}
		}
	}
	if summary.PendingConflicts == 0 {
		sessionActive = false
		_ = os.RemoveAll(sessionRoot)
	} else {
		sessionActive = false
	}
	return summary, nil
}

func (b *BackupService) loadBackupData(archiveKind string) (BackupManifest, error) {
	manifest := BackupManifest{
		Format:                   backupFormatName,
		Version:                  buildinfo.BackupFormatVersion,
		ArchiveKind:              archiveKind,
		AppVersion:               buildinfo.AppVersion,
		SchemaVersion:            buildinfo.SchemaVersion,
		// Issue #383 slice 6: stamp the discoverable per-surface
		// format version into every .ddbak manifest. Restore
		// paths read this field to detect format drift.
		FormatVersion:            buildinfo.DDBakFormatVersion,
		CurrentUpdateFlowVersion: versioninfo.CurrentUpdateFlowVersion,
		ReleaseCounter:           versioninfo.AppRelease(),
		CreatedAt:                time.Now().Format(time.RFC3339),
		DataFormat:               "sqlite",
		DatabaseFile:             filepath.ToSlash(filepath.Join("data", db.FileName)),
		ImageRoot:                "images/",
	}
	nodePrefix, err := b.db.NodePrefix()
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.NodePrefix = nodePrefix
	identity, err := b.db.UserIdentity()
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.OwnerName = strings.TrimSpace(strings.Join([]string{identity.FirstName, identity.MiddleName, identity.LastName}, " "))
	manifest.SourceLabel = strings.TrimSpace(manifest.OwnerName)
	if manifest.SourceLabel == "" {
		manifest.SourceLabel = strings.TrimSpace(manifest.NodePrefix)
	}
	sourceNodeID, err := b.db.SystemConfig("node_id")
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.SourceNodeID = strings.TrimSpace(sourceNodeID)

	page := 1
	for {
		batch, _, err := b.soldier.List(page, exportBatchSize)
		if err != nil {
			return BackupManifest{}, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := b.soldier.GetByID(item.ID)
			if err != nil {
				return BackupManifest{}, err
			}
			manifest.Soldiers++
			manifest.Records += len(soldier.Records)
			manifest.Images += len(soldier.Images)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	// Issue #321 slice 5.2: count Articles + Article Refs so
	// the .ddbak BackupManifest carries the same Articles +
	// DataArticlesFile + DataArticleRefsFile fields the
	// .ddshare manifest does. The actual rows ship inside
	// the SQLite snapshot, so the count is metadata for
	// visibility only.
	if err := b.db.Conn().QueryRow(`SELECT COUNT(*) FROM articles WHERE is_snapshot = 0`).Scan(&manifest.Articles); err != nil {
		return BackupManifest{}, err
	}
	var refCount int
	if err := b.db.Conn().QueryRow(`SELECT COUNT(*) FROM article_refs`).Scan(&refCount); err != nil {
		return BackupManifest{}, err
	}
	if refCount > 0 {
		manifest.DataArticleRefsFile = filepath.ToSlash(filepath.Join("data", "article_refs.json"))
	}
	// DataArticlesFile always points at the JSON; the .ddbak
	//'s source of truth is the SQLite snapshot, not the JSON.
	manifest.DataArticlesFile = filepath.ToSlash(filepath.Join("data", "articles.json"))

	return manifest, nil
}

func writeBackupJSON(zipWriter *zip.Writer, name string, value interface{}) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func addBackupFile(zipWriter *zip.Writer, entryName, sourcePath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(source, "addBackupFile.source")

	entry, err := zipWriter.Create(entryName)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, source)
	return err
}

func addBackupImages(zipWriter *zip.Writer, imageRoot string) error {
	if _, err := os.Stat(imageRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return filepath.Walk(imageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(filepath.Dir(imageRoot), path)
		if err != nil {
			return err
		}
		entryName := normalizeBackupPath(relativePath)
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer debug.DeferCloseLog(source, "addBackupImages.source")

		entry, err := zipWriter.Create(entryName)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, source)
		return err
	})
}

func addSelectedBackupImages(zipWriter *zip.Writer, imageRoot string, selectedPaths []string) error {
	if len(selectedPaths) == 0 {
		return nil
	}
	if _, err := os.Stat(imageRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	added := map[string]struct{}{}
	for _, selectedPath := range selectedPaths {
		normalized := normalizeBackupPath(selectedPath)
		if normalized == "" {
			continue
		}
		if _, seen := added[normalized]; seen {
			continue
		}

		sourcePath := filepath.Join(filepath.Dir(imageRoot), filepath.FromSlash(normalized))
		source, err := os.Open(sourcePath)
		if err != nil {
			return err
		}
		entry, err := zipWriter.Create(normalized)
		if err != nil {
			source.Close()
			return err
		}
		if _, err := io.Copy(entry, source); err != nil {
			source.Close()
			return err
		}
		if err := source.Close(); err != nil {
			return err
		}
		added[normalized] = struct{}{}
	}
	return nil
}

// parseDDBakVersion splits a ddbak_vN[.M] string into
// (major, minor, ok). Mirrors records.parseMemorialVersion
// (issue #383 slice 7). Returns ok=false for any string
// that doesn't match the ddbak_vN[.M] shape; the caller
// (checkDDBakFormatVersion) treats unparseable as a
// mismatch so a malformed stamp triggers the same refusal
// path as a major bump.
func parseDDBakVersion(s string) (major, minor int, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "ddbak_v") {
		return 0, 0, false
	}
	rest := strings.TrimPrefix(s, "ddbak_v")
	parts := strings.SplitN(rest, ".", 3)
	if len(parts) > 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil || maj < 0 {
		return 0, 0, false
	}
	if len(parts) == 1 {
		return maj, 0, true
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil || min < 0 {
		return 0, 0, false
	}
	return maj, min, true
}

// CheckDDBakFormatVersion enforces the per-surface stamp
// drift policy for .ddbak + .ddshare archives. Mirrors
// records.checkMemorialFormatVersion. Returns:
//   - (nil, nil) on same version or missing stamp (silent
//     for the former; warning for the latter is the
//     caller's job via DDBakFormatWarnings)
//   - (nil, warning) on a missing stamp, so the caller can
//     surface "pre-v1" in the summary
//   - (err, nil) on a major-bump or unparseable stamp
//     (wrapped ErrDDBakFormatMismatch for errors.Is dispatch)
//
// Exported so the CLI dry-run path (appshell/cli_import.go)
// can call it BEFORE starting a destructive import.
func CheckDDBakFormatVersion(archiveVersion string) error {
	expected := buildinfo.DDBakFormatVersion
	if strings.TrimSpace(archiveVersion) == "" {
		// Missing stamp: treat as pre-v1 backward compat
		// (warn, don't refuse). The warning is the caller's
		// responsibility via ddbakFormatWarnings.
		return nil
	}
	arcMaj, arcMin, arcOK := parseDDBakVersion(archiveVersion)
	expMaj, expMin, expOK := parseDDBakVersion(expected)
	if !arcOK || !expOK {
		return fmt.Errorf("%w: cannot parse format_version %q (expected %q)", ErrDDBakFormatMismatch, archiveVersion, expected)
	}
	if arcMaj != expMaj {
		return fmt.Errorf("%w: archive is %q (major v%d), this build expects %q (major v%d) — refuse", ErrDDBakFormatMismatch, archiveVersion, arcMaj, expected, expMaj)
	}
	if arcMin < expMin {
		// Defensive: same major, archive is older. Treat as
		// a mismatch because the archive's surface is older
		// than what the running binary assumes.
		return fmt.Errorf("%w: archive is %q (minor v%d), this build expects %q (minor v%d) — refuse", ErrDDBakFormatMismatch, archiveVersion, arcMin, expected, expMin)
	}
	return nil
}

// DDBakFormatWarnings returns the human-readable warning
// lines for the format_version drift cases that don't
// refuse (pre-v1 + minor-bump). The caller appends these
// to the import summary / CLI output / dry-run output.
// Returns nil for the same-version case (silent).
func DDBakFormatWarnings(archiveVersion string) []string {
	expected := buildinfo.DDBakFormatVersion
	if strings.TrimSpace(archiveVersion) == "" {
		return []string{fmt.Sprintf("archive has no format_version field; treating as pre-v1 (expected %s); import will proceed", expected)}
	}
	arcMaj, arcMin, arcOK := parseDDBakVersion(archiveVersion)
	expMaj, expMin, expOK := parseDDBakVersion(expected)
	if !arcOK || !expOK {
		return nil // unparseable; the check function refused already
	}
	if arcMaj == expMaj && arcMin > expMin {
		return []string{fmt.Sprintf("archive format_version %s is a minor bump past this build's %s; import will proceed but new fields may be missing", archiveVersion, expected)}
	}
	return nil
}

// readBackupContentsWithWarnings is the drift-aware
// variant of readBackupContents. Returns the parsed
// contents, the drift warnings (for caller summary
// surfaces), and any error. Major-bump refusals
// surface as ErrDDBakFormatMismatch (errors.Is dispatch).
//
// Issue #383 slice 7: this is the single seam where
// every reader (ImportWithLocalIdentity,
// RestoreBackupArchive, ImportSharedBackup) gates
// the format_version check. The legacy readBackupContents
// is preserved as a thin wrapper that drops the
// warnings so existing callers compile unchanged; new
// readers migrate to the WithWarnings variant.
func readBackupContentsWithWarnings(reader *zip.Reader) (backupContents, []string, error) {
	contents, err := readBackupContents(reader)
	if err != nil {
		return backupContents{}, nil, err
	}
	if err := CheckDDBakFormatVersion(contents.Manifest.FormatVersion); err != nil {
		return backupContents{}, nil, err
	}
	return contents, DDBakFormatWarnings(contents.Manifest.FormatVersion), nil
}

func readBackupContents(reader *zip.Reader) (backupContents, error) {
	fileMap := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		fileMap[file.Name] = file
	}

	manifestFile, ok := fileMap["manifest.json"]
	if !ok {
		return backupContents{}, fmt.Errorf("backup is missing manifest.json")
	}

	var manifest BackupManifest
	if err := readBackupJSON(manifestFile, &manifest); err != nil {
		return backupContents{}, err
	}
	if manifest.Format != backupFormatName {
		return backupContents{}, fmt.Errorf("unsupported backup format")
	}
	// Issue #383 slice 7: enforce the per-surface stamp
	// drift policy before any file-presence or schema
	// checks. The typed error (ErrDDBakFormatMismatch) lets
	// every caller (ImportWithLocalIdentity,
	// RestoreBackupArchive, ImportSharedBackup) surface
	// the refusal via errors.Is dispatch in the GUI +
	// CLI runners.
	if err := CheckDDBakFormatVersion(manifest.FormatVersion); err != nil {
		return backupContents{}, err
	}
	switch manifest.Version {
	case 1:
		manifest.DataFormat = "json"
		manifest.ArchiveKind = archiveKindBackup
		if manifest.DataFile == "" {
			manifest.DataFile = "data/soldiers.json"
		}
	case 2:
		if strings.TrimSpace(manifest.ArchiveKind) == "" {
			manifest.ArchiveKind = archiveKindBackup
		}
		if manifest.ArchiveKind != archiveKindBackup {
			return backupContents{}, fmt.Errorf("unsupported archive kind %q", manifest.ArchiveKind)
		}
		if strings.TrimSpace(manifest.DataFormat) == "" {
			manifest.DataFormat = "sqlite"
		}
		if manifest.SchemaVersion > buildinfo.SchemaVersion {
			return backupContents{}, fmt.Errorf("backup schema version %d is newer than this app supports", manifest.SchemaVersion)
		}
	case buildinfo.BackupFormatVersion:
		if strings.TrimSpace(manifest.ArchiveKind) == "" {
			manifest.ArchiveKind = archiveKindBackup
		}
		if manifest.ArchiveKind != archiveKindBackup && manifest.ArchiveKind != archiveKindShared {
			return backupContents{}, fmt.Errorf("unsupported archive kind %q", manifest.ArchiveKind)
		}
		if manifest.SchemaVersion > buildinfo.SchemaVersion {
			return backupContents{}, fmt.Errorf("backup schema version %d is newer than this app supports", manifest.SchemaVersion)
		}
	default:
		return backupContents{}, fmt.Errorf("unsupported backup format version %d", manifest.Version)
	}

	contents := backupContents{
		Manifest: manifest,
		FileMap:  fileMap,
	}
	if manifest.ImageRoot == "" {
		return backupContents{}, fmt.Errorf("backup manifest is incomplete")
	}

	if manifest.DataFormat == "sqlite" {
		if manifest.DatabaseFile == "" {
			return backupContents{}, fmt.Errorf("backup manifest is incomplete")
		}
		if _, ok := fileMap[manifest.DatabaseFile]; !ok {
			return backupContents{}, fmt.Errorf("backup is missing %s", manifest.DatabaseFile)
		}
		if err := validateSQLiteBackupImageEntries(contents); err != nil {
			return backupContents{}, err
		}
		return contents, nil
	}

	if manifest.DataFile == "" {
		return backupContents{}, fmt.Errorf("backup manifest is incomplete")
	}
	dataFile, ok := fileMap[manifest.DataFile]
	if !ok {
		return backupContents{}, fmt.Errorf("backup is missing %s", manifest.DataFile)
	}
	if err := readBackupJSON(dataFile, &contents.Soldiers); err != nil {
		return backupContents{}, err
	}

	// Issue #320 child #334: shared archives (kShared) carry
	// Event Records in a separate data/events.json file. The
	// file is optional so legacy archives keep importing.
	if contents.Manifest.ArchiveKind == archiveKindShared {
		eventsFile := contents.Manifest.DataEventsFile
		if eventsFile == "" {
			eventsFile = "data/events.json"
		}
		if ef, ok := fileMap[eventsFile]; ok {
			if err := readBackupJSON(ef, &contents.Events); err != nil {
				return backupContents{}, fmt.Errorf("decode %s: %w", eventsFile, err)
			}
		}
	}

	imageEntries := make(map[string]struct{})
	for name := range fileMap {
		if strings.HasPrefix(name, manifest.ImageRoot) {
			imageEntries[name] = struct{}{}
		}
	}
	for _, soldier := range contents.Soldiers {
		for _, image := range soldier.Images {
			if _, ok := imageEntries[normalizeBackupPath(image.FilePath)]; !ok {
				return backupContents{}, fmt.Errorf("backup is missing image file %s", image.FilePath)
			}
		}
	}

	return contents, nil
}

func validateSQLiteBackupImageEntries(contents backupContents) error {
	stageDir, err := os.MkdirTemp("", "dixiedata-backup-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
	if databaseFile == nil {
		return fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
	}
	if err := extractBackupFile(databaseFile, db.Path(stageDir)); err != nil {
		return fmt.Errorf("stage backup database: %w", err)
	}
	stagedDB, err := db.Open(stageDir)
	if err != nil {
		return fmt.Errorf("open staged backup database: %w", err)
	}
	defer debug.DeferCloseLog(stagedDB, "validateSQLiteBackupImageEntries.db")

	soldierSvc := NewSoldierService(stagedDB)
	soldiers, err := listAllSoldiers(soldierSvc)
	if err != nil {
		return fmt.Errorf("read staged backup database: %w", err)
	}
	imageEntries := make(map[string]struct{})
	for name := range contents.FileMap {
		if strings.HasPrefix(name, contents.Manifest.ImageRoot) {
			imageEntries[name] = struct{}{}
		}
	}
	for _, soldier := range soldiers {
		for _, image := range soldier.Images {
			normalized := normalizeBackupPath(image.FilePath)
			if _, ok := imageEntries[normalized]; !ok {
				return fmt.Errorf("backup is missing image file %s", image.FilePath)
			}
		}
	}
	return nil
}

func readBackupJSON(file *zip.File, target interface{}) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(reader, "readBackupJSON.reader")
	return json.NewDecoder(reader).Decode(target)
}

// NormalizeManifestBackwardsCompat applies defaults for
// version-axis fields that didn't exist before issue #296
// landed. Callers should invoke this on every freshly decoded
// BackupManifest so the rest of the import pipeline can
// compare U + N without re-parsing the AppVersion string.
//
// Defaults (per issue #296 acceptance):
//   - CurrentUpdateFlowVersion: 1 (legacy v1.2.N strings
//     parse to U=1 per issue #266 decision 1)
//   - ReleaseCounter: SchemaVersion (the historical formula
//     tied N to the schema version; the bug-fix-only release
//     counter diverged from that with issue #266)
//
// The function mutates the manifest in place and returns it
// so callers can chain `m, err := NormalizeManifestBackwardsCompat(m)`.
func NormalizeManifestBackwardsCompat(manifest BackupManifest) BackupManifest {
	if manifest.CurrentUpdateFlowVersion == 0 {
		manifest.CurrentUpdateFlowVersion = 1
	}
	if manifest.ReleaseCounter == 0 && manifest.SchemaVersion > 0 {
		manifest.ReleaseCounter = manifest.SchemaVersion
	}
	return manifest
}

func extractBackupImages(reader *zip.Reader, destinationRoot, imageRoot string) error {
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, imageRoot) || file.FileInfo().IsDir() {
			continue
		}
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		if err := extractBackupFile(file, destinationPath); err != nil {
			return err
		}
	}
	return nil
}

func extractBackupFile(file *zip.File, destinationPath string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(source, "extractBackupFile.source")

	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(target, "extractBackupFile.target")

	_, err = io.Copy(target, source)
	return err
}

func (b *BackupService) restoreLegacyJSONBackup(dataDir, extractedRoot string, soldiers []models.Soldier) error {
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	// Issue #449 slice 2: wrap the close in a closure so the
	// *DB.Close fires before restoreLegacyJSONBackup returns.
	// The bare `defer Close()` form defers the call to a
	// returned function value, which doesn't run until the
	// enclosing frame is gone — too late for the stagingDir
	// rename the caller (Import) issues immediately after.
	defer func() { _ = database.Close() }()

	soldierSvc := NewSoldierService(database)
	restoredIDsByLegacyID := make(map[int64]int64, len(soldiers))
	type legacyImageRestore struct {
		targetID int64
		images   []models.Image
	}
	pendingImages := make([]legacyImageRestore, 0, len(soldiers))
	isPrimarySoldier := func(entryType string) bool {
		normalized := strings.ToLower(strings.TrimSpace(entryType))
		return normalized == "" || normalized == "soldier"
	}

	createLegacySoldier := func(soldier models.Soldier, linkedSoldierID int64) (*models.Soldier, error) {
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the legacy-JSON backup restore
		// (pre-SQLite .ddbak archives). The service-layer default
		// already covers empty values, but explicit stamping
		// documents the intent at the call site and survives any
		// future defaulting change.
		created, err := soldierSvc.Create(models.Soldier{
			DisplayID:             soldier.DisplayID,
			EntryType:             soldier.EntryType,
			SpouseSoldierID:       linkedSoldierID,
			RelationshipLabel:     soldier.RelationshipLabel,
			MaidenName:            soldier.MaidenName,
			IsGenerated:           soldier.IsGenerated,
			SyncID:                soldier.SyncID,
			PensionID:             soldier.PensionID,
			ApplicationID:         soldier.ApplicationID,
			Prefix:                soldier.Prefix,
			ShowPrefixBeforeName:  soldier.ShowPrefixBeforeName,
			FirstName:             soldier.FirstName,
			MiddleName:            soldier.MiddleName,
			LastName:              soldier.LastName,
			Suffix:                soldier.Suffix,
			Rank:                  soldier.Rank,
			RankIn:                soldier.RankIn,
			RankOut:               soldier.RankOut,
			Unit:                  soldier.Unit,
			PensionState:          pensionstate.Normalize(soldier.PensionState),
			ConfederateHomeStatus: confederatehomestatus.Normalize(soldier.ConfederateHomeStatus),
			ConfederateHomeName:   soldier.ConfederateHomeName,
			BirthDate:             soldier.BirthDate,
			DeathDate:             soldier.DeathDate,
			DeathYear:             soldier.DeathYear,
			DeathMonth:            soldier.DeathMonth,
			DeathDay:              soldier.DeathDay,
			BirthInfo:             soldier.BirthInfo,
			BuriedIn:              soldier.BuriedIn,
			Notes:                 soldier.Notes,
			AddedBy:               soldier.AddedBy,
			LastEditedBy:          soldier.LastEditedBy,
			LastEditedFields:      soldier.LastEditedFields,
			LastEditedAt:          soldier.LastEditedAt,
			CreatedAt:             soldier.CreatedAt,
			UpdatedAt:             soldier.UpdatedAt,
			Records:               soldier.Records,
			CreatedByImportPath:   "restore_backup_archive",
		})
		if err != nil {
			return nil, err
		}
		return created, nil
	}

	for _, soldier := range soldiers {
		if !isPrimarySoldier(soldier.EntryType) {
			continue
		}
		created, err := createLegacySoldier(soldier, 0)
		if err != nil {
			return err
		}
		if soldier.ID > 0 {
			restoredIDsByLegacyID[soldier.ID] = created.ID
		}
		pendingImages = append(pendingImages, legacyImageRestore{targetID: created.ID, images: soldier.Images})
		if _, err := database.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, created_at = ?, updated_at = ? WHERE id = ?`,
			soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, created.ID); err != nil {
			return err
		}
	}

	for _, soldier := range soldiers {
		if isPrimarySoldier(soldier.EntryType) {
			continue
		}
		linkedSoldierID := restoredIDsByLegacyID[soldier.SpouseSoldierID]
		if soldier.SpouseSoldierID > 0 && linkedSoldierID == 0 {
			return fmt.Errorf("legacy backup linked soldier %d not found for %s", soldier.SpouseSoldierID, strings.TrimSpace(soldier.DisplayID))
		}
		created, err := createLegacySoldier(soldier, linkedSoldierID)
		if err != nil {
			return err
		}
		if soldier.ID > 0 {
			restoredIDsByLegacyID[soldier.ID] = created.ID
		}
		pendingImages = append(pendingImages, legacyImageRestore{targetID: created.ID, images: soldier.Images})
		if _, err := database.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, created_at = ?, updated_at = ? WHERE id = ?`,
			soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, created.ID); err != nil {
			return err
		}
	}

	for _, imageBatch := range pendingImages {
		for _, image := range imageBatch.images {
			sourcePath := filepath.Join(extractedRoot, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			destinationPath := filepath.Join(dataDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
				return err
			}
			if err := copyBackupFile(sourcePath, destinationPath); err != nil {
				return err
			}
			if err := soldierSvc.AddImage(imageBatch.targetID, image.FileName, image.FilePath, image.Caption); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreSnapshotBackup(dataDir, extractedRoot string, contents backupContents) error {
	databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
	if databaseFile == nil {
		return fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := extractBackupFile(databaseFile, db.Path(dataDir)); err != nil {
		return err
	}
	imageSource := filepath.Join(extractedRoot, "images")
	if _, err := os.Stat(imageSource); err == nil {
		imageTarget := filepath.Join(dataDir, "images")
		if err := copyBackupTree(imageSource, imageTarget); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	return database.Close()
}

func restoreSnapshotArchive(dataDir string, reader *zip.Reader, contents backupContents) error {
	databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
	if databaseFile == nil {
		return fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := extractBackupFile(databaseFile, db.Path(dataDir)); err != nil {
		return err
	}
	if err := extractBackupImages(reader, dataDir, contents.Manifest.ImageRoot); err != nil {
		return err
	}
	return nil
}

func validateStagedBackup(dataDir string, manifest BackupManifest) error {
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	// Issue #449 slice 2: explicitly close the staging DB
	// before the function returns so the WAL/SHM sidecar
	// handles are released before replaceDataDir fires its
	// MoveFileExW(stagingDir, targetDir). A bare
	// `defer Close()` fires the close AFTER the rename on
	// some Go versions (defer captures the call expression,
	// not the immediate body). The wrapped form below
	// guarantees the close runs in the validateStagedBackup
	// frame, before the caller invokes replaceDataDir.
	defer func() {
		_ = database.Close()
	}()

	soldierSvc := NewSoldierService(database)
	page := 1
	soldierCount := 0
	recordCount := 0
	imageCount := 0
	for {
		batch, _, err := soldierSvc.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := soldierSvc.GetByID(item.ID)
			if err != nil {
				return err
			}
			soldierCount++
			recordCount += len(soldier.Records)
			imageCount += len(soldier.Images)
			for _, image := range soldier.Images {
				imagePath := filepath.Join(dataDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
				if _, err := os.Stat(imagePath); err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("backup validation missing image file %s", image.FilePath)
					}
					return err
				}
			}
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	if soldierCount != manifest.Soldiers || recordCount != manifest.Records || imageCount != manifest.Images {
		return fmt.Errorf("backup validation mismatch: got %d soldiers, %d records, %d images", soldierCount, recordCount, imageCount)
	}
	return nil
}

func replaceDataDir(targetDir, stagingDir string) error {
	parent := filepath.Dir(targetDir)
	backupDir, err := os.MkdirTemp(parent, filepath.Base(targetDir)+"-previous-*")
	if err != nil {
		return err
	}
	_ = os.RemoveAll(backupDir)

	targetExists := true
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		targetExists = false
	}
	if targetExists {
		// Issue #216: skip the rename when the target is logically
		// empty. There's nothing to back up — removing the empty
		// target and promoting stagingDir to targetDir avoids the
		// Windows rename that fails when OneDrive / SearchHost /
		// the asset-server watcher holds a transient handle to
		// the target. isLogicallyEmpty uses the DB file size as
		// a proxy: < 64KB means schema-only (no records). See
		// isLogicallyEmpty for the exact thresholds. After
		// RemoveAll, targetExists is flipped to false so the
		// subsequent os.Rename(stagingDir, targetDir) finds no
		// pre-existing destination and succeeds.
		if isLogicallyEmpty(targetDir) {
			if err := os.RemoveAll(targetDir); err != nil {
				return fmt.Errorf("remove empty target %s: %w", targetDir, err)
			}
			targetExists = false
			_ = os.RemoveAll(backupDir)
			backupDir = ""
		}
	}
	if targetExists {
		// Issue #216: retry the rename with exponential backoff so
		// transient Windows handle conflicts (OneDrive, SearchHost,
		// asset watcher) have time to release. 5 attempts, 4 sleep
		// periods of 200/400/800/1600ms between them = 3s total
		// wait time before the final attempt fails.
		if err := renameWithRetry(targetDir, backupDir, 5, 200*time.Millisecond); err != nil {
			return err
		}
	}
	if err := renameOS(stagingDir, targetDir); err != nil {
		// Issue #449 slice 2: Windows MoveFileExW can return
		// "Access is denied" transiently when a parent dir
		// still has a finalizer-held handle. Retry with the
		// same exponential backoff as the target→backup
		// rename, plus a runtime.GC + Gosched to flush any
		// pending finalizers (mirrors testtemp's settle
		// window).
		var retryErr error
		delay := 200 * time.Millisecond
		for attempt := 1; attempt <= 5; attempt++ {
			runtime.GC()
			runtime.Gosched()
			time.Sleep(delay)
			delay *= 2
			if rerr := renameOS(stagingDir, targetDir); rerr == nil {
				retryErr = nil
				break
			} else {
				retryErr = rerr
			}
		}
		if retryErr != nil {
			if targetExists {
				_ = renameOS(backupDir, targetDir)
			}
			return retryErr
		}
	}
	if backupDir != "" {
		return os.RemoveAll(backupDir)
	}
	return nil
}

// isLogicallyEmpty reports whether the target data directory has
// no data to back up. The size threshold is a proxy for "no
// records" — an empty DixieData DB is schema-only, typically
// 200-300KB. A DB > 64KB is presumed to have data. If the DB
// file doesn't exist at all, the directory has no data. This
// is a heuristic; the precise check would require opening the
// SQLite file, which is closed by the caller before the rename
// phase runs (see importInFlightJobIDSet/importInFlight).
func isLogicallyEmpty(dir string) bool {
	dbPath := filepath.Join(dir, "dixiedata.db")
	info, err := os.Stat(dbPath)
	if err != nil {
		return true // no DB → no data
	}
	return info.Size() < 64*1024 // schema-only
}

// renameOS is the rename function used by renameWithRetry. On
// non-Windows it is os.Rename (see rename_unix.go); on Windows
// it calls MoveFileExW with MOVEFILE_REPLACE_EXISTING so an
// existing target directory can be overwritten (see
// rename_windows.go). Exposed as a package-level var so tests
// can inject a failing rename to exercise the retry logic
// without needing real Windows handle conflicts.

// renameWithRetry retries os.Rename with exponential backoff.
// attempts is the total number of tries (5 = 4 sleeps between
// attempts: 200ms, 400ms, 800ms, 1600ms = 3s total). Each failed
// attempt is logged via debug.Log so an extended retry window
// shows up in the JSONL log as a chain of "rename attempt N/M
// failed" entries — visible signal that Windows is fighting the
// rename, not a silent wait.
func renameWithRetry(src, dst string, attempts int, baseDelay time.Duration) error {
	var lastErr error
	delay := baseDelay
	for i := 0; i < attempts; i++ {
		if err := renameOS(src, dst); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < attempts-1 {
			log.Printf("archive: replaceDataDir rename %s → %s attempt %d/%d failed: %v; retrying in %v", src, dst, i+1, attempts, lastErr, delay)
			time.Sleep(delay)
			delay *= 2
		}
	}
	return fmt.Errorf("rename %s → %s failed after %d attempts: %w", src, dst, attempts, lastErr)
}

func copyBackupFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(source, "copyBackupFile.source")

	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(target, "copyBackupFile.target")

	_, err = io.Copy(target, source)
	return err
}

func copyBackupTree(sourceRoot, targetRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetRoot, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return copyBackupFile(path, targetPath)
	})
}

func countFilesUnder(root string) (int, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

func normalizeBackupPath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "/")
}

func listAllSoldiers(svc *SoldierService) ([]models.Soldier, error) {
	page := 1
	all := []models.Soldier{}
	for {
		batch, _, err := svc.List(page, exportBatchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			full, err := svc.GetByID(item.ID)
			if err != nil {
				return nil, err
			}
			all = append(all, *full)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return all, nil
}

func collectImagePaths(soldiers []models.Soldier) []string {
	paths := make([]string, 0)
	seen := map[string]struct{}{}
	for _, soldier := range soldiers {
		for _, image := range soldier.Images {
			normalized := normalizeBackupPath(image.FilePath)
			if normalized == "" {
				continue
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			paths = append(paths, normalized)
		}
	}
	return paths
}

// mergeSharedEvents (issue #320 child #334) parses the event
// rows from the shared archive, upserts each by display_id, and
// re-creates the event_person_links junction using the
// recipient-side resolved Display IDs. Mutates the supplied
// SharedImportSummary in place so the soldiers-only merge
// doesn't get zeroed out when the source archive has zero
// events. Side effect on the target DB: link rows inserted.
//
// sourceSoldiers is needed because that carries the per-soldier
// LinkedDisplayIDs the export service populated with the
// source-side Event Display IDs. Without that array, the
// recipient would have nothing to map back to.
func (b *BackupService) mergeSharedEvents(
	sessionID, archivePath string,
	sourceEvents []models.Soldier,
	sourceSoldiers []models.Soldier,
	summary *SharedImportSummary,
	logger *mergeLogger,
) error {
	if len(sourceEvents) == 0 {
		return nil
	}

	tx, err := b.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Build a lookup of source-side event-sync-id -> target event id.
	targetIDsByDisplayID := make(map[string]int64, len(sourceEvents))
	for _, event := range sourceEvents {
		displayID := strings.TrimSpace(event.DisplayID)
		if displayID == "" {
			continue
		}
		// Per RPCI #9: dedup-by-display-id. Upsert by display_id
		// (no shared archive uses sync_id to dedupe events in v1).
		var targetID int64
		row := tx.QueryRow(
			`SELECT id FROM soldiers WHERE display_id = ? AND entry_type = ? LIMIT 1`,
			displayID, models.EntryTypeEvent,
		)
		scanErr := row.Scan(&targetID)
		if scanErr != nil && scanErr != sql.ErrNoRows {
			return scanErr
		}
		if scanErr == sql.ErrNoRows {
			inserted, ierr := upsertSharedSoldierFromEvent(tx, event, sessionID)
			if ierr != nil {
				return ierr
			}
			targetID = inserted
			if logger != nil {
				logger.Printf("event action=insert display_id=%s target_id=%d", displayID, targetID)
			}
			summary.EventsInserted++
		} else {
			summary.EventsSkipped++
			if logger != nil {
				logger.Printf("event action=skip-existing display_id=%s target_id=%d", displayID, targetID)
			}
		}
		targetIDsByDisplayID[displayID] = targetID
	}

	// Walk each soldier's LinkedDisplayIDs and recreate the
	// junction. The schema has UNIQUE (event_id, person_id) so we
	// INSERT OR IGNORE to keep the import idempotent if the same
	// shared archive is re-imported (issue #183's idempotency
	// contract). Matches the source DB's
	// records.EventService.AttachEventToPerson duplicate-error
	// surface on the live link-creation RPC.
	for _, soldier := range sourceSoldiers {
		if len(soldier.LinkedDisplayIDs) == 0 {
			continue
		}
		soldierSyncID := strings.TrimSpace(soldier.SyncID)
		if soldierSyncID == "" {
			continue
		}
		var soldierID int64
		// Match by sync_id, NOT display_id: mergeSharedSoldiers
		// renames the recipient-side display_id to the recipient's
		// own prefix (issue #183's user-identity binding), but
		// sync_id is the immutable cross-archive identifier.
		if scanErr := tx.QueryRow(
			`SELECT id FROM soldiers WHERE sync_id = ? LIMIT 1`,
			soldierSyncID,
		).Scan(&soldierID); scanErr != nil {
			if scanErr == sql.ErrNoRows {
				continue
			}
			return scanErr
		}
		for _, evDisplayID := range soldier.LinkedDisplayIDs {
			eventID, ok := targetIDsByDisplayID[evDisplayID]
			if !ok {
				continue
			}
			res, lerr := tx.Exec(
				`INSERT OR IGNORE INTO event_person_links (event_id, person_id) VALUES (?, ?)`,
				eventID, soldierID,
			)
			if lerr != nil {
				return lerr
			}
			if n, raErr := res.RowsAffected(); raErr == nil && n > 0 {
				summary.EventsLinked++
			}
		}
	}

	commitErr := tx.Commit()
	return commitErr
}

// upsertSharedSoldierFromEvent inserts (or no-ops-if-existing) an
// Event Record row using the exact payload the source archive
// shipped. We re-use the model.Soldier column set so the event
// keeps its Kind / BeginDate / EndDate / Description fields
// intact (the EventService.CreateEvent helpers would re-generate
// a Display ID, but the shared archive export pinned the display id
// at source-time so we deliberately bypass the helper here and
// keep the source string).
func upsertSharedSoldierFromEvent(tx *sql.Tx, event models.Soldier, sessionID string) (int64, error) {
	res, err := tx.Exec(
		`INSERT INTO soldiers (
			display_id, sync_id, entry_type, kind, begin_date, end_date, description,
			added_by, last_edited_by, last_edited_at, last_edited_fields, import_batch_id,
			created_by_version, created_by_import_path
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		strings.TrimSpace(event.DisplayID),
		strings.TrimSpace(event.SyncID),
		models.EntryTypeEvent,
		strings.TrimSpace(event.Kind),
		strings.TrimSpace(event.BeginDate),
		strings.TrimSpace(event.EndDate),
		event.Description,
		event.AddedBy,
		event.LastEditedBy,
		event.LastEditedAt,
		event.LastEditedFields,
		sessionID,
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the shared-archive importer. The
		// SQLite default ('') would leave these as empty strings
		// indistinguishable from the v63→v64 backfill 'unknown'
		// sentinel, so the raw SQL path needs explicit stamps.
		versioninfo.AppVersion(),
		"import_shared_archive",
	)
	if err != nil {
		return 0, fmt.Errorf("insert event %s: %w", event.DisplayID, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (b *BackupService) mergeSharedSoldiers(sessionID, archivePath string, sourceSoldiers []models.Soldier, sourceDataDir, targetDataDir, sourceNodeID, sourceNodeLabel string, logger *mergeLogger) (SharedImportSummary, error) {
	tx, err := b.db.Conn().Begin()
	if err != nil {
		return SharedImportSummary{}, err
	}
	defer tx.Rollback()

	summary := SharedImportSummary{}
	targetsBySourceSync := make(map[string]sharedMergeTarget, len(sourceSoldiers))
	sourceSyncByID := make(map[int64]string, len(sourceSoldiers))
	conflictedSyncs := make(map[string]struct{})

	for _, soldier := range sourceSoldiers {
		sourceSyncByID[soldier.ID] = strings.TrimSpace(soldier.SyncID)
	}
	if err := ensureMergeReviewSession(tx, sessionID, archivePath, sourceDataDir); err != nil {
		return SharedImportSummary{}, err
	}
	if err := ensureImportBatch(tx, sessionID, archivePath); err != nil {
		return SharedImportSummary{}, err
	}

	for _, soldier := range sourceSoldiers {
		snapshot := mergeReviewSnapshot{
			Soldier:         normalizeSharedSoldierSnapshot(soldier),
			SpouseSyncID:    strings.TrimSpace(sourceSyncByID[soldier.SpouseSoldierID]),
			SourceNodeID:    strings.TrimSpace(sourceNodeID),
			SourceNodeLabel: strings.TrimSpace(sourceNodeLabel),
		}
		localSnapshot, err := loadSoldierSnapshotBySync(tx, snapshot.Soldier.SyncID)
		if err != nil && err != sql.ErrNoRows {
			return SharedImportSummary{}, err
		}
		if err == nil {
			if equivalentMergeReviewSnapshots(*localSnapshot, snapshot) {
				targetsBySourceSync[snapshot.Soldier.SyncID] = sharedMergeTarget{
					SoldierID:   localSnapshot.Soldier.ID,
					SoldierSync: localSnapshot.Soldier.SyncID,
				}
				summary.SoldiersSkipped++
				if logger != nil {
					logger.Printf("soldier action=skip-unchanged match=sync sync_id=%s display_id=%s target_id=%d", snapshot.Soldier.SyncID, localSnapshot.Soldier.DisplayID, localSnapshot.Soldier.ID)
				}
				continue
			}
			reason := describeSoldierConflict(*localSnapshot, snapshot)
			if err := insertMergeReviewConflict(tx, sessionID, "soldier-update", reason, localSnapshot, snapshot); err != nil {
				return SharedImportSummary{}, err
			}
			conflictedSyncs[snapshot.Soldier.SyncID] = struct{}{}
			summary.PendingConflicts++
			if logger != nil {
				logger.Printf("soldier action=stage-review conflict_type=soldier-update match=sync sync_id=%s source_display_id=%s reason=%q",
					snapshot.Soldier.SyncID, snapshot.Soldier.DisplayID, reason)
			}
			continue
		}

		aliasSnapshot, err := loadSharedAliasTargetSnapshot(tx, snapshot.SourceNodeID, snapshot.Soldier.SyncID)
		if err != nil {
			return SharedImportSummary{}, err
		}
		if aliasSnapshot != nil {
			targetsBySourceSync[snapshot.Soldier.SyncID] = sharedMergeTarget{
				SoldierID:   aliasSnapshot.Soldier.ID,
				SoldierSync: aliasSnapshot.Soldier.SyncID,
			}
			if equivalentAliasMappedSnapshots(*aliasSnapshot, snapshot) {
				summary.SoldiersSkipped++
				if logger != nil {
					logger.Printf("soldier action=skip-unchanged match=alias source_node_id=%q sync_id=%s canonical_sync_id=%s target_id=%d", snapshot.SourceNodeID, snapshot.Soldier.SyncID, aliasSnapshot.Soldier.SyncID, aliasSnapshot.Soldier.ID)
				}
				continue
			}
			reason := describeAliasMappedConflict(*aliasSnapshot, snapshot)
			if err := insertMergeReviewConflict(tx, sessionID, "soldier-update", reason, aliasSnapshot, snapshot); err != nil {
				return SharedImportSummary{}, err
			}
			conflictedSyncs[snapshot.Soldier.SyncID] = struct{}{}
			summary.PendingConflicts++
			if logger != nil {
				logger.Printf("soldier action=stage-review conflict_type=soldier-update match=alias source_node_id=%q sync_id=%s canonical_sync_id=%s source_display_id=%s reason=%q",
					snapshot.SourceNodeID, snapshot.Soldier.SyncID, aliasSnapshot.Soldier.SyncID, snapshot.Soldier.DisplayID, reason)
			}
			continue
		}

		localSnapshot, conflictType, reason, err := detectSharedConflict(tx, snapshot)
		if err != nil {
			return SharedImportSummary{}, err
		}
		if conflictType != "" {
			if err := insertMergeReviewConflict(tx, sessionID, conflictType, reason, localSnapshot, snapshot); err != nil {
				return SharedImportSummary{}, err
			}
			conflictedSyncs[snapshot.Soldier.SyncID] = struct{}{}
			summary.PendingConflicts++
			if logger != nil {
				logger.Printf("soldier action=stage-review conflict_type=%s sync_id=%s source_display_id=%s reason=%q",
					conflictType, snapshot.Soldier.SyncID, snapshot.Soldier.DisplayID, reason)
			}
			continue
		}

		targetID, existed, resolvedDisplayID, err := upsertSharedSoldier(tx, snapshot.Soldier, sessionID)
		if err != nil {
			return SharedImportSummary{}, err
		}
		targetsBySourceSync[snapshot.Soldier.SyncID] = sharedMergeTarget{
			SoldierID:   targetID,
			SoldierSync: snapshot.Soldier.SyncID,
		}
		if existed {
			summary.SoldiersUpdated++
			if logger != nil {
				logger.Printf("soldier action=update sync_id=%s display_id=%s target_id=%d", snapshot.Soldier.SyncID, resolvedDisplayID, targetID)
			}
		} else {
			summary.SoldiersInserted++
			if logger != nil {
				logger.Printf("soldier action=insert sync_id=%s display_id=%s target_id=%d", snapshot.Soldier.SyncID, resolvedDisplayID, targetID)
			}
		}
	}

	for _, soldier := range sourceSoldiers {
		syncID := strings.TrimSpace(soldier.SyncID)
		if _, conflicted := conflictedSyncs[syncID]; conflicted {
			continue
		}
		target := targetsBySourceSync[syncID]
		if target.SoldierID < 1 {
			return SharedImportSummary{}, fmt.Errorf("missing merged soldier target for sync_id %s", syncID)
		}
		spouseTargetID, err := resolveSharedSpouseTargetID(sourceSyncByID, targetsBySourceSync, soldier)
		if err != nil {
			return SharedImportSummary{}, err
		}
		if _, err := tx.Exec(`UPDATE soldiers SET spouse_soldier_id = ? WHERE id = ?`, nullableInt64(spouseTargetID), target.SoldierID); err != nil {
			return SharedImportSummary{}, err
		}

		for _, record := range soldier.Records {
			existed, err := upsertSharedRecord(tx, target.SoldierID, target.SoldierSync, record)
			if err != nil {
				return SharedImportSummary{}, err
			}
			if existed {
				summary.RecordsUpdated++
			} else {
				summary.RecordsInserted++
			}
		}

		for _, image := range soldier.Images {
			if err := copySharedImageFile(sourceDataDir, targetDataDir, image.FilePath); err != nil {
				return SharedImportSummary{}, err
			}
			existed, changed, err := upsertSharedImage(tx, target.SoldierID, target.SoldierSync, image)
			if err != nil {
				return SharedImportSummary{}, err
			}
			if existed {
				// Issue #136: only count Updates when the mutable
				// columns actually changed. An UPDATE that left
				// every column at its prior value is not a
				// meaningful update — it is just a no-op from
				// the user's perspective.
				if changed {
					summary.ImagesUpdated++
				}
			} else {
				summary.ImagesInserted++
			}
		}
	}

	if summary.PendingConflicts == 0 {
		if _, err := tx.Exec(`DELETE FROM merge_review_sessions WHERE id = ?`, sessionID); err != nil {
			return SharedImportSummary{}, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE merge_review_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
			return SharedImportSummary{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SharedImportSummary{}, err
	}
	if logger != nil {
		logger.Printf("summary soldiers_inserted=%d soldiers_updated=%d records_inserted=%d records_updated=%d images_inserted=%d images_updated=%d events_inserted=%d events_skipped=%d events_linked=%d conflicts_pending=%d",
			summary.SoldiersInserted, summary.SoldiersUpdated, summary.RecordsInserted, summary.RecordsUpdated, summary.ImagesInserted, summary.ImagesUpdated, summary.EventsInserted, summary.EventsSkipped, summary.EventsLinked, summary.PendingConflicts)
	}
	return summary, nil
}

// PendingMergeConflicts returns the open Local-vs-Incoming merge
// conflicts the user has not yet resolved. Surfaced on the Merge
// Review Ledger page.
func (b *BackupService) PendingMergeConflicts() ([]models.MergeReviewConflict, error) {
	rows, err := b.db.Conn().Query(`SELECT id, session_id, conflict_type, reason, COALESCE(local_record_id, 0), COALESCE(local_display_id, ''), source_display_id, COALESCE(resolution, ''), created_at, local_data, source_data
		FROM merge_review_conflicts
		WHERE COALESCE(resolution, '') = ''
		ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "PendingMergeConflicts.rows")

	conflicts := []models.MergeReviewConflict{}
	for rows.Next() {
		var (
			conflict   models.MergeReviewConflict
			localJSON  sql.NullString
			sourceJSON string
		)
		if err := rows.Scan(&conflict.ID, &conflict.SessionID, &conflict.ConflictType, &conflict.Reason, &conflict.LocalRecordID, &conflict.LocalDisplayID, &conflict.SourceDisplayID, &conflict.Resolution, &conflict.CreatedAt, &localJSON, &sourceJSON); err != nil {
			return nil, err
		}
		if strings.TrimSpace(localJSON.String) != "" {
			localSnapshot, err := unmarshalMergeReviewSnapshot(localJSON.String)
			if err != nil {
				return nil, err
			}
			conflict.LocalSoldier = &localSnapshot.Soldier
		}
		sourceSnapshot, err := unmarshalMergeReviewSnapshot(sourceJSON)
		if err != nil {
			return nil, err
		}
		conflict.SourceSoldier = sourceSnapshot.Soldier
		conflicts = append(conflicts, conflict)
	}
	return conflicts, rows.Err()
}

// ConflictLedger returns the full Source-Conflict Ledger for one
// Soldier: the per-Source-Record conflicts between Local +
// Incoming versions, with each one's resolution state.
func (b *BackupService) ConflictLedger(soldierID int64) (*SourceConflictLedger, error) {
	central, err := b.soldier.GetByID(soldierID)
	if err != nil {
		return nil, err
	}
	rows, err := b.db.Conn().Query(`
		SELECT id, conflict_type, reason, source_display_id, COALESCE(resolution, ''), created_at, COALESCE(resolved_at, ''), COALESCE(local_data, ''), source_data
		FROM merge_review_conflicts
		WHERE local_record_id = ?
		ORDER BY created_at DESC, id DESC
	`, soldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "ConflictLedger.rows")

	ledger := &SourceConflictLedger{Central: *central}
	for rows.Next() {
		var (
			entry                         SourceConflictLedgerEntry
			localJSON, sourceJSON         string
			localSnapshot, sourceSnapshot mergeReviewSnapshot
		)
		if err := rows.Scan(&entry.ID, &entry.ConflictType, &entry.Reason, &entry.SourceDisplayID, &entry.Resolution, &entry.CreatedAt, &entry.ResolvedAt, &localJSON, &sourceJSON); err != nil {
			return nil, err
		}
		if strings.TrimSpace(localJSON) != "" {
			localSnapshot, err = unmarshalMergeReviewSnapshot(localJSON)
			if err != nil {
				return nil, err
			}
			entry.LocalSnapshot = localSnapshot.Soldier
		}
		if sourceSnapshot, err = unmarshalMergeReviewSnapshot(sourceJSON); err != nil {
			return nil, err
		}
		entry.SourceSnapshot = sourceSnapshot.Soldier
		entry.DifferenceFields = collectSoldierConflictFields(localSnapshot, sourceSnapshot, false)
		ledger.Entries = append(ledger.Entries, entry)
		if strings.TrimSpace(entry.Resolution) == "" {
			ledger.OpenCount++
		} else {
			ledger.ResolvedCount++
		}
	}
	return ledger, rows.Err()
}

// ResolveMergeConflict records the user's decision for one
// open merge conflict: keep-local, keep-incoming, or keep-both.
// The decision is persisted to the Source-Conflict Ledger; the
// affected Local + Incoming rows are reconciled in the same tx.
func (b *BackupService) ResolveMergeConflict(conflictID int64, decision, dataDir string) error {
	tx, err := b.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	conflict, sessionRoot, err := loadMergeReviewConflict(tx, conflictID)
	if err != nil {
		return err
	}
	switch decision {
	case "keep-local":
	case "keep-both":
		if conflict.ConflictType != "display-id-collision" {
			return fmt.Errorf("keep both is only supported for display ID collisions")
		}
		if err := applySharedConflictResolution(tx, conflict, decision, sessionRoot, dataDir); err != nil {
			return err
		}
	case "use-shared", "keep-shared":
		if err := applySharedConflictResolution(tx, conflict, decision, sessionRoot, dataDir); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported merge review decision %q", decision)
	}

	if _, err := tx.Exec(`UPDATE merge_review_conflicts SET resolution = ?, resolved_at = CURRENT_TIMESTAMP WHERE id = ?`, decision, conflictID); err != nil {
		return err
	}
	if err := finalizeMergeReviewSession(tx, conflict.SessionID, sessionRoot); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertSharedSoldier(tx *sql.Tx, soldier models.Soldier, importBatchID string) (int64, bool, string, error) {
	syncID := strings.TrimSpace(soldier.SyncID)
	if syncID == "" {
		return 0, false, "", fmt.Errorf("shared database soldier missing sync_id")
	}

	var existingID int64
	var existingDisplayID string
	err := tx.QueryRow(`SELECT id, display_id FROM soldiers WHERE sync_id = ?`, syncID).Scan(&existingID, &existingDisplayID)
	if err == nil {
		_, err = tx.Exec(`UPDATE soldiers
			SET display_id = ?, entry_type = ?, relationship_label = ?, maiden_name = ?, is_generated = ?, pension_id = ?, application_id = ?, prefix = ?, show_prefix_before_name = ?, first_name = ?, middle_name = ?, last_name = ?, suffix = ?, rank = ?, rank_in = ?, rank_out = ?, unit = ?, pension_state = ?, confederate_home_status = ?, confederate_home_name = ?, death_year = ?, death_month = ?, death_day = ?, birth_date = ?, death_date = ?, birth_info = ?, buried_in = ?, biography = ?, pdf_excerpt_override = ?, notes = ?, needs_review = ?, review_reason = ?, added_by = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, created_at = ?, updated_at = ?
			WHERE id = ?`,
			existingDisplayID, soldier.EntryType, soldier.RelationshipLabel, soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.ShowPrefixBeforeName, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, pensionstate.Normalize(soldier.PensionState), confederatehomestatus.Normalize(soldier.ConfederateHomeStatus), soldier.ConfederateHomeName, soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Biography, soldier.PDFExcerptOverride, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, existingID)
		if err != nil {
			return 0, false, "", err
		}
		if err := refreshSoldierFTS(tx, existingID, soldier); err != nil {
			return 0, false, "", err
		}
		return existingID, true, existingDisplayID, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, "", err
	}

	displayID, err := resolveSharedDisplayID(tx, soldier.DisplayID, syncID)
	if err != nil {
		return 0, false, "", err
	}

	res, err := tx.Exec(`INSERT INTO soldiers
		(display_id, sync_id, entry_type, spouse_soldier_id, relationship_label, maiden_name, is_generated, pension_id, application_id, prefix, show_prefix_before_name, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, biography, pdf_excerpt_override, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at, import_batch_id, created_by_version, created_by_import_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		displayID, syncID, soldier.EntryType, nil, soldier.RelationshipLabel, soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.ShowPrefixBeforeName, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, pensionstate.Normalize(soldier.PensionState), confederatehomestatus.Normalize(soldier.ConfederateHomeStatus), soldier.ConfederateHomeName, soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Biography, soldier.PDFExcerptOverride, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, strings.TrimSpace(importBatchID),
		// Issue #377 slice 2: stamp the import path so future
		// "where did this row come from?" investigations can
		// attribute the row to the shared-archive importer. The
		// SQLite default ('') would leave these as empty strings
		// indistinguishable from the v63→v64 backfill 'unknown'
		// sentinel, so the raw SQL path needs explicit stamps.
		versioninfo.AppVersion(),
		"import_shared_archive")
	if err != nil {
		return 0, false, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, "", err
	}
	if err := insertSoldierFTS(tx, id, soldier); err != nil {
		return 0, false, "", err
	}
	return id, false, displayID, nil
}

func refreshSoldierFTS(tx *sql.Tx, soldierID int64, soldier models.Soldier) error {
	return nil
}

func ensureImportBatch(tx *sql.Tx, batchID, archivePath string) error {
	if strings.TrimSpace(batchID) == "" {
		return fmt.Errorf("import batch id is required")
	}
	_, err := tx.Exec(`INSERT OR IGNORE INTO import_batches (id, archive_path) VALUES (?, ?)`, strings.TrimSpace(batchID), strings.TrimSpace(archivePath))
	return err
}

func insertSoldierFTS(tx *sql.Tx, soldierID int64, soldier models.Soldier) error {
	return nil
}

func upsertSharedRecord(tx *sql.Tx, targetSoldierID int64, soldierSyncID string, record models.Record) (bool, error) {
	syncID := strings.TrimSpace(record.SyncID)
	if syncID == "" {
		return false, fmt.Errorf("shared database record missing sync_id")
	}
	var existingID int64
	err := tx.QueryRow(`SELECT id FROM records WHERE sync_id = ?`, syncID).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(`UPDATE records SET person_record_id = ?, person_sync_id = ?, record_type = ?, app_id = ?, details = ? WHERE id = ?`,
			targetSoldierID, soldierSyncID, record.RecordType, record.AppID, record.Details, existingID)
		return true, err
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = tx.Exec(`INSERT INTO records (sync_id, person_record_id, person_sync_id, record_type, app_id, details) VALUES (?, ?, ?, ?, ?, ?)`,
		syncID, targetSoldierID, soldierSyncID, record.RecordType, record.AppID, record.Details)
	return false, err
}

func upsertSharedImage(tx *sql.Tx, targetSoldierID int64, soldierSyncID string, image models.Image) (existed bool, changed bool, err error) {
	syncID := strings.TrimSpace(image.SyncID)
	if syncID == "" {
		return false, false, fmt.Errorf("shared database image missing sync_id")
	}
	var existing struct {
		id        int64
		fileName  string
		filePath  string
		caption   string
		isPrimary bool
	}
	row := tx.QueryRow(`SELECT id, file_name, file_path, caption, is_primary FROM images WHERE sync_id = ?`, syncID)
	if scanErr := row.Scan(&existing.id, &existing.fileName, &existing.filePath, &existing.caption, &existing.isPrimary); scanErr == nil {
		// Issue #136: only count as "updated" when at least one
		// mutable column actually changed. The pre-existing logic
		// re-ran the UPDATE unconditionally, which inflated
		// ImagesUpdated on every full-duplicate import and made
		// the job report misleading ("Imported 1140 images" on a
		// no-op round-trip).
		changed = existing.fileName != image.FileName ||
			existing.filePath != image.FilePath ||
			existing.caption != image.Caption ||
			existing.isPrimary != image.IsPrimary
		if _, updateErr := tx.Exec(`UPDATE images SET person_record_id = ?, person_sync_id = ?, file_name = ?, file_path = ?, caption = ?, is_primary = ? WHERE id = ?`,
			targetSoldierID, soldierSyncID, image.FileName, image.FilePath, image.Caption, image.IsPrimary, existing.id); updateErr != nil {
			return true, changed, updateErr
		}
		return true, changed, nil
	} else if scanErr != sql.ErrNoRows {
		return false, false, scanErr
	}
	_, err = tx.Exec(`INSERT INTO images (sync_id, person_record_id, person_sync_id, file_name, file_path, caption, is_primary) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		syncID, targetSoldierID, soldierSyncID, image.FileName, image.FilePath, image.Caption, image.IsPrimary)
	return false, false, err
}

func copySharedImageFile(sourceDataDir, targetDataDir, relativePath string) error {
	normalized := normalizeBackupPath(relativePath)
	if normalized == "" {
		return nil
	}
	sourcePath := filepath.Join(sourceDataDir, filepath.FromSlash(normalized))
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("shared database is missing image file %s", relativePath)
		}
		return err
	}
	targetPath := filepath.Join(targetDataDir, filepath.FromSlash(normalized))
	// Issue #136: short-circuit when the target file already exists
	// with the same byte count. Sharded image filenames are derived
	// from content hashes, so size-equal means same content for any
	// well-formed export. Avoids trashing the file on disk for every
	// full-duplicate re-import and keeps mtimes stable.
	if targetInfo, statErr := os.Stat(targetPath); statErr == nil {
		if targetInfo.Size() == sourceInfo.Size() {
			return nil
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return copyBackupFile(sourcePath, targetPath)
}

func resolveSharedDisplayID(tx *sql.Tx, desiredDisplayID, syncID string) (string, error) {
	nodePrefix, err := nodePrefixFromTx(tx)
	if err != nil {
		return "", err
	}
	candidate := db.SanitizeID(desiredDisplayID, nodePrefix)
	namespace, _, ok := db.CanonicalDisplayID(candidate)
	if !ok || !strings.EqualFold(namespace, db.NormalizeNodePrefix(nodePrefix)) {
		return nextLocalGeneratedDisplayID(tx)
	}
	return ensureUniqueDisplayID(tx, candidate, syncID)
}

func ensureUniqueDisplayID(tx *sql.Tx, candidate, syncID string) (string, error) {
	var existingSync sql.NullString
	err := tx.QueryRow(`SELECT sync_id FROM soldiers WHERE display_id = ?`, candidate).Scan(&existingSync)
	if err == sql.ErrNoRows {
		return candidate, nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(existingSync.String) == strings.TrimSpace(syncID) {
		return candidate, nil
	}
	return nextLocalGeneratedDisplayID(tx)
}

func nextLocalGeneratedDisplayID(tx *sql.Tx) (string, error) {
	nodePrefix, err := nodePrefixFromTx(tx)
	if err != nil {
		return "", err
	}
	rows, err := tx.Query(`SELECT display_id, is_generated FROM soldiers`)
	if err != nil {
		return "", err
	}
	defer debug.DeferCloseLog(rows, "nextLocalGeneratedDisplayID.rows")

	maxID := 0
	for rows.Next() {
		var (
			displayID   string
			isGenerated bool
		)
		if err := rows.Scan(&displayID, &isGenerated); err != nil {
			return "", err
		}
		sequence, ok := mergeGeneratedDisplayIDSequence(displayID, nodePrefix, isGenerated)
		if ok && sequence > maxID {
			maxID = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return db.NextGeneratedDisplayID(nodePrefix, maxID+1), nil
}

func nodePrefixFromTx(tx *sql.Tx) (string, error) {
	var prefix sql.NullString
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'node_prefix'`).Scan(&prefix); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	return db.NormalizeNodePrefix(prefix.String), nil
}

func mergeGeneratedDisplayIDSequence(displayID, nodePrefix string, isGenerated bool) (int, bool) {
	namespace, sequence, ok := db.CanonicalDisplayID(db.SanitizeID(displayID, nodePrefix))
	if !ok {
		return 0, false
	}
	if isGenerated || strings.EqualFold(namespace, db.LegacyDisplayIDNamespace) || strings.EqualFold(namespace, db.NormalizeNodePrefix(nodePrefix)) {
		return sequence, true
	}
	return 0, false
}

func detectSharedConflict(tx *sql.Tx, source mergeReviewSnapshot) (*mergeReviewSnapshot, string, string, error) {
	localBySync, err := loadSoldierSnapshotBySync(tx, source.Soldier.SyncID)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil {
		if equivalentMergeReviewSnapshots(*localBySync, source) {
			return nil, "", "", nil
		}
		return localBySync, "soldier-update", describeSoldierConflict(*localBySync, source), nil
	}

	localByDisplay, err := loadSoldierSnapshotByDisplayID(tx, source.Soldier.DisplayID)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil && strings.TrimSpace(localByDisplay.Soldier.SyncID) != strings.TrimSpace(source.Soldier.SyncID) {
		return localByDisplay, "display-id-collision", fmt.Sprintf("%s record %s collides with existing local record %s.", sharedConflictSourceNoun(source), source.Soldier.DisplayID, localByDisplay.Soldier.DisplayID), nil
	}

	localByHuman, err := loadSoldierSnapshotByHumanMatch(tx, source.Soldier)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil && strings.TrimSpace(localByHuman.Soldier.SyncID) != strings.TrimSpace(source.Soldier.SyncID) {
		return localByHuman, "human-duplicate", describeHumanDuplicateConflict(*localByHuman, source), nil
	}
	return nil, "", "", nil
}

func ensureMergeReviewSession(tx *sql.Tx, sessionID, archivePath, sourceRoot string) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO merge_review_sessions (id, archive_path, source_root, status, created_at, updated_at)
		VALUES (?, ?, ?, 'open', COALESCE((SELECT created_at FROM merge_review_sessions WHERE id = ?), CURRENT_TIMESTAMP), CURRENT_TIMESTAMP)`,
		sessionID, archivePath, sourceRoot, sessionID)
	return err
}

func insertMergeReviewConflict(tx *sql.Tx, sessionID, conflictType, reason string, localSnapshot *mergeReviewSnapshot, sourceSnapshot mergeReviewSnapshot) error {
	sourceJSON, err := marshalMergeReviewSnapshot(sourceSnapshot)
	if err != nil {
		return err
	}
	localJSON := ""
	localSoldierID := int64(0)
	localDisplayID := ""
	if localSnapshot != nil {
		localJSON, err = marshalMergeReviewSnapshot(*localSnapshot)
		if err != nil {
			return err
		}
		localSoldierID = localSnapshot.Soldier.ID
		localDisplayID = localSnapshot.Soldier.DisplayID
	}
	_, err = tx.Exec(`INSERT INTO merge_review_conflicts
		(session_id, conflict_type, reason, soldier_sync_id, local_record_id, local_display_id, source_display_id, local_data, source_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, conflictType, reason, sourceSnapshot.Soldier.SyncID, nullableInt64(localSoldierID), localDisplayID, sourceSnapshot.Soldier.DisplayID, nullableString(localJSON), sourceJSON)
	return err
}

func loadMergeReviewConflict(tx *sql.Tx, conflictID int64) (models.MergeReviewConflict, string, error) {
	var (
		conflict    models.MergeReviewConflict
		sessionRoot string
		localJSON   sql.NullString
		sourceJSON  string
	)
	err := tx.QueryRow(`SELECT c.id, c.session_id, c.conflict_type, c.reason, COALESCE(c.local_record_id, 0), COALESCE(c.local_display_id, ''), c.source_display_id,
		COALESCE(c.resolution, ''), c.created_at, c.local_data, c.source_data, s.source_root
		FROM merge_review_conflicts c
		JOIN merge_review_sessions s ON s.id = c.session_id
		WHERE c.id = ?`, conflictID).
		Scan(&conflict.ID, &conflict.SessionID, &conflict.ConflictType, &conflict.Reason, &conflict.LocalRecordID, &conflict.LocalDisplayID, &conflict.SourceDisplayID,
			&conflict.Resolution, &conflict.CreatedAt, &localJSON, &sourceJSON, &sessionRoot)
	if err != nil {
		return models.MergeReviewConflict{}, "", err
	}
	if strings.TrimSpace(conflict.Resolution) != "" {
		return models.MergeReviewConflict{}, "", fmt.Errorf("merge review item %d is already resolved", conflictID)
	}
	if strings.TrimSpace(localJSON.String) != "" {
		localSnapshot, err := unmarshalMergeReviewSnapshot(localJSON.String)
		if err != nil {
			return models.MergeReviewConflict{}, "", err
		}
		conflict.LocalSoldier = &localSnapshot.Soldier
	}
	sourceSnapshot, err := unmarshalMergeReviewSnapshot(sourceJSON)
	if err != nil {
		return models.MergeReviewConflict{}, "", err
	}
	conflict.SourceSoldier = sourceSnapshot.Soldier
	return conflict, sessionRoot, nil
}

func applySharedConflictResolution(tx *sql.Tx, conflict models.MergeReviewConflict, decision, sourceDataDir, targetDataDir string) error {
	sourceSnapshot, err := loadSourceSnapshotForConflict(tx, conflict.ID)
	if err != nil {
		return err
	}
	preserveLocalIdentifiers := decision != "keep-both" && conflict.LocalSoldier != nil
	if preserveLocalIdentifiers {
		sourceSnapshot.Soldier.ID = conflict.LocalRecordID
		sourceSnapshot.Soldier.DisplayID = conflict.LocalSoldier.DisplayID
		sourceSnapshot.Soldier.SyncID = conflict.LocalSoldier.SyncID
		sourceSnapshot.Soldier.AddedBy = conflict.LocalSoldier.AddedBy
		sourceSnapshot.Soldier.CreatedAt = conflict.LocalSoldier.CreatedAt
	}
	targetID, _, _, err := upsertSharedSoldier(tx, sourceSnapshot.Soldier, conflict.SessionID)
	if err != nil {
		return err
	}
	if conflict.ConflictType == "human-duplicate" && decision != "keep-both" {
		if err := recordSharedMergeAlias(tx, sourceSnapshot.SourceNodeID, strings.TrimSpace(conflict.SourceSoldier.SyncID), targetID, strings.TrimSpace(sourceSnapshot.Soldier.SyncID), decision, conflict.ID); err != nil {
			return err
		}
	}
	if decision == "keep-both" && conflict.LocalSoldier != nil {
		reviewReason := fmt.Sprintf("Potential duplicate preserved during shared merge against %s.", strings.TrimSpace(sourceSnapshot.Soldier.DisplayID))
		if err := setReviewStatusTx(tx, conflict.LocalRecordID, true, reviewReason); err != nil {
			return err
		}
		if err := setReviewStatusTx(tx, targetID, true, fmt.Sprintf("Potential duplicate imported from shared record %s.", strings.TrimSpace(conflict.LocalSoldier.DisplayID))); err != nil {
			return err
		}
	}
	spouseTargetID := int64(0)
	if strings.TrimSpace(sourceSnapshot.SpouseSyncID) != "" {
		spouseTargetID, err = loadTargetSoldierIDBySync(tx, sourceSnapshot.SpouseSyncID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("resolve the linked spouse record before applying shared changes")
			}
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE soldiers SET spouse_soldier_id = ? WHERE id = ?`, nullableInt64(spouseTargetID), targetID); err != nil {
		return err
	}
	for _, record := range sourceSnapshot.Soldier.Records {
		if _, err := upsertSharedRecord(tx, targetID, sourceSnapshot.Soldier.SyncID, record); err != nil {
			return err
		}
	}
	for _, image := range sourceSnapshot.Soldier.Images {
		if err := copySharedImageFile(sourceDataDir, targetDataDir, image.FilePath); err != nil {
			return err
		}
		if _, _, err := upsertSharedImage(tx, targetID, sourceSnapshot.Soldier.SyncID, image); err != nil {
			return err
		}
	}
	return nil
}

func setReviewStatusTx(tx *sql.Tx, soldierID int64, needsReview bool, reason string) error {
	reason = strings.TrimSpace(reason)
	if !needsReview {
		reason = ""
	}
	_, err := tx.Exec(`UPDATE soldiers SET needs_review = ?, review_reason = ? WHERE id = ?`, needsReview, reason, soldierID)
	return err
}

func loadSoldierSnapshotByHumanMatch(tx *sql.Tx, source models.Soldier) (*mergeReviewSnapshot, error) {
	birthYear, ok := humanDuplicateBirthYear(source)
	if !ok {
		return nil, sql.ErrNoRows
	}
	firstName := strings.TrimSpace(source.FirstName)
	lastName := strings.TrimSpace(source.LastName)
	unit := strings.TrimSpace(source.Unit)
	if firstName == "" || lastName == "" || unit == "" {
		return nil, sql.ErrNoRows
	}

	var soldierID int64
	err := tx.QueryRow(`SELECT id
		FROM soldiers
		WHERE TRIM(COALESCE(first_name, '')) = ?
		  AND TRIM(COALESCE(last_name, '')) = ?
		  AND TRIM(COALESCE(unit, '')) = ?
		  AND CAST(SUBSTR(TRIM(COALESCE(birth_date, '')), 7, 4) AS INTEGER) = ?
		  AND TRIM(COALESCE(sync_id, '')) <> ?
		ORDER BY id
		LIMIT 1`,
		firstName, lastName, unit, birthYear, strings.TrimSpace(source.SyncID),
	).Scan(&soldierID)
	if err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, soldierID)
}

func describeHumanDuplicateConflict(local mergeReviewSnapshot, source mergeReviewSnapshot) string {
	birthYear, _ := humanDuplicateBirthYear(source.Soldier)
	return fmt.Sprintf("%s record %s matches %s on name, birth year %d, and unit %s.", sharedConflictSourceNoun(source), source.Soldier.DisplayID, local.Soldier.DisplayID, birthYear, strings.TrimSpace(source.Soldier.Unit))
}

func humanDuplicateBirthYear(soldier models.Soldier) (int, bool) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.BirthDate))
	if err == nil && partial.Year >= 1000 {
		return partial.Year, true
	}
	if parsedBirth := dates.ParseBirthInfo(strings.TrimSpace(soldier.BirthInfo)); parsedBirth != "" {
		partial, err := dates.ParseCanonical(parsedBirth)
		if err == nil && partial.Year >= 1000 {
			return partial.Year, true
		}
	}
	return 0, false
}

func finalizeMergeReviewSession(tx *sql.Tx, sessionID, sessionRoot string) error {
	var unresolved int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM merge_review_conflicts WHERE session_id = ? AND COALESCE(resolution, '') = ''`, sessionID).Scan(&unresolved); err != nil {
		return err
	}
	if unresolved > 0 {
		_, err := tx.Exec(`UPDATE merge_review_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID)
		return err
	}
	if _, err := tx.Exec(`UPDATE merge_review_sessions SET status = 'resolved', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
		return err
	}
	if strings.TrimSpace(sessionRoot) != "" {
		if err := os.RemoveAll(sessionRoot); err != nil {
			return err
		}
	}
	return nil
}

func loadSourceSnapshotForConflict(tx *sql.Tx, conflictID int64) (mergeReviewSnapshot, error) {
	var sourceJSON string
	if err := tx.QueryRow(`SELECT source_data FROM merge_review_conflicts WHERE id = ?`, conflictID).Scan(&sourceJSON); err != nil {
		return mergeReviewSnapshot{}, err
	}
	return unmarshalMergeReviewSnapshot(sourceJSON)
}

func loadTargetSoldierIDBySync(tx *sql.Tx, syncID string) (int64, error) {
	var targetID int64
	err := tx.QueryRow(`SELECT id FROM soldiers WHERE sync_id = ?`, strings.TrimSpace(syncID)).Scan(&targetID)
	return targetID, err
}

func recordSharedMergeAlias(tx *sql.Tx, sourceNodeID, sourcePersonSyncID string, canonicalPersonID int64, canonicalPersonSyncID, resolutionKind string, conflictID int64) error {
	sourceNodeID = strings.TrimSpace(sourceNodeID)
	sourcePersonSyncID = strings.TrimSpace(sourcePersonSyncID)
	canonicalPersonSyncID = strings.TrimSpace(canonicalPersonSyncID)
	resolutionKind = strings.TrimSpace(resolutionKind)
	if sourceNodeID == "" || sourcePersonSyncID == "" || canonicalPersonID < 1 || canonicalPersonSyncID == "" {
		return nil
	}
	if resolutionKind == "" {
		resolutionKind = "merge-review"
	}
	_, err := tx.Exec(`INSERT INTO shared_merge_aliases
		(source_node_id, source_person_sync_id, canonical_person_sync_id, canonical_person_id, resolution_kind, created_from_conflict_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(source_node_id, source_person_sync_id) DO UPDATE SET
			canonical_person_sync_id = excluded.canonical_person_sync_id,
			canonical_person_id = excluded.canonical_person_id,
			resolution_kind = excluded.resolution_kind,
			created_from_conflict_id = excluded.created_from_conflict_id,
			updated_at = CURRENT_TIMESTAMP`,
		sourceNodeID, sourcePersonSyncID, canonicalPersonSyncID, canonicalPersonID, resolutionKind, nullableInt64(conflictID))
	return err
}

func loadSoldierSnapshotBySync(tx *sql.Tx, syncID string) (*mergeReviewSnapshot, error) {
	var id int64
	if err := tx.QueryRow(`SELECT id FROM soldiers WHERE sync_id = ?`, strings.TrimSpace(syncID)).Scan(&id); err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, id)
}

func loadSoldierSnapshotByDisplayID(tx *sql.Tx, displayID string) (*mergeReviewSnapshot, error) {
	var id int64
	if err := tx.QueryRow(`SELECT id FROM soldiers WHERE display_id = ?`, strings.TrimSpace(displayID)).Scan(&id); err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, id)
}

func loadSoldierSnapshotByID(tx *sql.Tx, soldierID int64) (*mergeReviewSnapshot, error) {
	row := tx.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id = ?`, soldierID)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}
	records, err := loadRecordsForSoldierTx(tx, soldierID)
	if err != nil {
		return nil, err
	}
	images, err := loadImagesForSoldierTx(tx, soldierID)
	if err != nil {
		return nil, err
	}
	soldier.Records = records
	soldier.Images = images
	spouseSyncID := ""
	if soldier.SpouseSoldierID > 0 {
		_ = tx.QueryRow(`SELECT COALESCE(sync_id, '') FROM soldiers WHERE id = ?`, soldier.SpouseSoldierID).Scan(&spouseSyncID)
	}
	normalized := normalizeSharedSoldierSnapshot(*soldier)
	normalized.ID = soldierID
	return &mergeReviewSnapshot{
		Soldier:      normalized,
		SpouseSyncID: strings.TrimSpace(spouseSyncID),
	}, nil
}

func loadRecordsForSoldierTx(tx *sql.Tx, soldierID int64) ([]models.Record, error) {
	rows, err := tx.Query(`SELECT `+recordSelectColumns+` FROM records WHERE person_record_id = ? ORDER BY sort_order, id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadRecordsForSoldierTx.rows")
	records := []models.Record{}
	for rows.Next() {
		var record models.Record
		if err := rows.Scan(&record.ID, &record.SyncID, &record.PersonRecordID, &record.PersonSyncID, &record.RecordType, &record.AppID, &record.Details, &record.SortOrder); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func loadImagesForSoldierTx(tx *sql.Tx, soldierID int64) ([]models.Image, error) {
	rows, err := tx.Query(`SELECT `+imageSelectColumns+` FROM images WHERE person_record_id = ? ORDER BY is_primary DESC, id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "loadImagesForSoldierTx.rows")
	images := []models.Image{}
	for rows.Next() {
		var image models.Image
		if err := rows.Scan(&image.ID, &image.SyncID, &image.PersonRecordID, &image.PersonSyncID, &image.FileName, &image.FilePath, &image.Caption, &image.IsPrimary); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func resolveSharedSpouseTargetID(sourceSyncByID map[int64]string, targetsBySourceSync map[string]sharedMergeTarget, soldier models.Soldier) (int64, error) {
	if soldier.SpouseSoldierID < 1 {
		return 0, nil
	}
	spouseSyncID := strings.TrimSpace(sourceSyncByID[soldier.SpouseSoldierID])
	if spouseSyncID == "" {
		return 0, fmt.Errorf("shared database spouse link missing sync id for soldier %s", soldier.DisplayID)
	}
	spouseTargetID := targetsBySourceSync[spouseSyncID].SoldierID
	if spouseTargetID < 1 {
		return 0, fmt.Errorf("shared database spouse link missing target for soldier %s", soldier.DisplayID)
	}
	return spouseTargetID, nil
}

func equivalentMergeReviewSnapshots(local, source mergeReviewSnapshot) bool {
	return describeSoldierConflict(local, source) == ""
}

func equivalentAliasMappedSnapshots(local, source mergeReviewSnapshot) bool {
	return len(collectSoldierConflictFields(local, source, true)) == 0
}

func describeSoldierConflict(local, source mergeReviewSnapshot) string {
	differences := collectSoldierConflictFields(local, source, false)
	if len(differences) == 0 {
		return ""
	}
	return sharedConflictSourceNoun(source) + " changed " + strings.Join(differences, ", ") + "."
}

func describeAliasMappedConflict(local, source mergeReviewSnapshot) string {
	differences := collectSoldierConflictFields(local, source, true)
	if len(differences) == 0 {
		return ""
	}
	return fmt.Sprintf("%s record %s is already mapped to local record %s, but changed %s.", sharedConflictSourceLabel(source), source.Soldier.DisplayID, local.Soldier.DisplayID, strings.Join(differences, ", "))
}

func collectSoldierConflictFields(local, source mergeReviewSnapshot, ignoreDisplayID bool) []string {
	differences := make([]string, 0, 8)
	appendDiff := func(label, left, right string) {
		left = strings.TrimSpace(left)
		right = strings.TrimSpace(right)
		if left != right {
			differences = append(differences, label)
		}
	}
	if !ignoreDisplayID {
		appendDiff("display ID", local.Soldier.DisplayID, source.Soldier.DisplayID)
	}
	appendDiff("entry type", local.Soldier.EntryType, source.Soldier.EntryType)
	appendDiff("first name", local.Soldier.FirstName, source.Soldier.FirstName)
	appendDiff("middle name", local.Soldier.MiddleName, source.Soldier.MiddleName)
	appendDiff("last name", local.Soldier.LastName, source.Soldier.LastName)
	appendDiff("relationship to soldier", local.Soldier.RelationshipLabel, source.Soldier.RelationshipLabel)
	appendDiff("maiden name", local.Soldier.MaidenName, source.Soldier.MaidenName)
	appendDiff("rank", local.Soldier.Rank, source.Soldier.Rank)
	appendDiff("rank in", local.Soldier.RankIn, source.Soldier.RankIn)
	appendDiff("rank out", local.Soldier.RankOut, source.Soldier.RankOut)
	appendDiff("unit", local.Soldier.Unit, source.Soldier.Unit)
	appendDiff("pension state", local.Soldier.PensionState, source.Soldier.PensionState)
	appendDiff("pension ID", local.Soldier.PensionID, source.Soldier.PensionID)
	appendDiff("application ID", local.Soldier.ApplicationID, source.Soldier.ApplicationID)
	appendDiff("birth date", local.Soldier.BirthDate, source.Soldier.BirthDate)
	appendDiff("death date", local.Soldier.DeathDate, source.Soldier.DeathDate)
	appendDiff("birth info", local.Soldier.BirthInfo, source.Soldier.BirthInfo)
	appendDiff("buried in", local.Soldier.BuriedIn, source.Soldier.BuriedIn)
	appendDiff("notes", local.Soldier.Notes, source.Soldier.Notes)
	appendDiff("spouse link", local.SpouseSyncID, source.SpouseSyncID)
	return differences
}

func normalizeSharedSoldierSnapshot(soldier models.Soldier) models.Soldier {
	soldier.Records = append([]models.Record(nil), soldier.Records...)
	soldier.Images = append([]models.Image(nil), soldier.Images...)
	for index := range soldier.Records {
		soldier.Records[index].ID = 0
		soldier.Records[index].PersonRecordID = 0
	}
	for index := range soldier.Images {
		soldier.Images[index].ID = 0
		soldier.Images[index].PersonRecordID = 0
	}
	soldier.ID = 0
	soldier.SpouseSoldierID = 0
	soldier.SpouseName = ""
	soldier.IsGenerated = soldier.IsGenerated || isGeneratedDisplayID(soldier.DisplayID)
	return soldier
}

func marshalMergeReviewSnapshot(snapshot mergeReviewSnapshot) (string, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalMergeReviewSnapshot(raw string) (mergeReviewSnapshot, error) {
	var snapshot mergeReviewSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return mergeReviewSnapshot{}, err
	}
	snapshot.Soldier = normalizeSharedSoldierSnapshot(snapshot.Soldier)
	return snapshot, nil
}

func sharedArchiveSourceIdentity(manifest BackupManifest) (string, string) {
	sourceNodeID := strings.TrimSpace(manifest.SourceNodeID)
	sourceNodeLabel := strings.TrimSpace(manifest.SourceLabel)
	if sourceNodeLabel == "" {
		sourceNodeLabel = strings.TrimSpace(manifest.OwnerName)
	}
	if sourceNodeLabel == "" {
		sourceNodeLabel = strings.TrimSpace(manifest.NodePrefix)
	}
	if sourceNodeLabel == "" {
		sourceNodeLabel = "shared archive"
	}
	return sourceNodeID, sourceNodeLabel
}

func sharedConflictSourceLabel(snapshot mergeReviewSnapshot) string {
	if label := strings.TrimSpace(snapshot.SourceNodeLabel); label != "" {
		return label
	}
	return "Shared archive"
}

func sharedConflictSourceNoun(snapshot mergeReviewSnapshot) string {
	if label := strings.TrimSpace(snapshot.SourceNodeLabel); label != "" {
		return label + " archive"
	}
	return "Shared archive"
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func newMergeLogger(dataDir string) (*mergeLogger, error) {
	logDir := appdata.LogsDir(dataDir)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	fileName := "shared-merge-" + time.Now().Format("20060102-150405") + ".log"
	return &mergeLogger{
		path:  filepath.Join(logDir, fileName),
		lines: []string{"DixieData shared archive merge log", "created_at=" + time.Now().Format(time.RFC3339)},
	}, nil
}

func (m *mergeLogger) Printf(format string, args ...interface{}) {
	if m == nil {
		return
	}
	m.lines = append(m.lines, time.Now().Format(time.RFC3339)+" "+fmt.Sprintf(format, args...))
}

func (m *mergeLogger) Close() error {
	if m == nil {
		return nil
	}
	body := strings.Join(m.lines, "\n") + "\n"
	if err := os.WriteFile(m.path, []byte(body), 0o644); err != nil {
		return err
	}
	latestPath := filepath.Join(filepath.Dir(m.path), "shared-merge-latest.log")
	if err := os.WriteFile(latestPath, []byte(body), fs.FileMode(0o644)); err != nil {
		return err
	}
	return nil
}

// listAllArticles returns every live-branch Article row in
// the Local Archive (issue #321 slice 5.1). Excludes snapshot
// rows (is_snapshot = 0 filter) per the slice-2.5 design --
// snapshots are historical artifacts, not load-bearing
// articles, and shipping them in shared archives would
// duplicate every article's body.
//
// Implementation mirrors listAllEvents: query ids, then
// hydrate each via the article service's GetByID so the
// full row is available for the JSON export.
func listAllArticles(database *db.DB) ([]models.Article, error) {
	conn := database.Conn()
	rows, err := conn.Query(`SELECT id FROM articles WHERE is_snapshot = 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "listAllArticles.rows")
	ids := make([]int64, 0, 8)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	soldierSvc := NewSoldierService(database)
	articleSvc := records.NewArticleService(soldierSvc)
	out := make([]models.Article, 0, len(ids))
	for _, id := range ids {
		full, ferr := articleSvc.GetByID(id)
		if ferr != nil {
			return nil, ferr
		}
		out = append(out, *full)
	}
	return out, nil
}

// listAllArticleRefs returns every row in article_refs (the
// per-article person ref junction, issue #321 slice 2).
// Single-shot SQL keeps the export pipeline linear at volume.
// The ArticleRef row carries the denormalized person_display_id
// + person_record_sync_id so the recipient's import path can
// resolve refs without a second fetch against the soldiers
// table.
func listAllArticleRefs(database *db.DB) ([]records.ArticleRef, error) {
	conn := database.Conn()
	rows, err := conn.Query(
		`SELECT id, article_id, article_sync_id, person_record_id,
		        person_record_sync_id, person_display_id, position
		 FROM article_refs ORDER BY article_id, position, id`,
	)
	if err != nil {
		return nil, err
	}
	defer debug.DeferCloseLog(rows, "listAllArticleRefs.rows")
	out := make([]records.ArticleRef, 0, 8)
	for rows.Next() {
		var r records.ArticleRef
		if err := rows.Scan(&r.ID, &r.ArticleID, &r.ArticleSyncID,
			&r.PersonRecordID, &r.PersonRecordSyncID,
			&r.PersonDisplayID, &r.Position); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
