package appshell

import (
	"context"
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/update"
)

type personRecord = models.Soldier
type personRecordSearch = models.SoldierSearch
type personRecordSuggestions = models.SoldierFormSuggestions

type personRecordsFacade interface {
	ArchiveCounts() (models.ArchiveCounts, error)
	FormSuggestions() (personRecordSuggestions, error)
	List(page, pageSize int) ([]personRecord, int, error)
	BrowsePage(request records.BrowseRequest) ([]personRecord, int, records.BrowseRequest, error)
	SearchPage(query string, page, pageSize int) ([]personRecord, int, error)
	RecentByIDs(ids []int64, limit int) ([]personRecord, error)
	AdvancedSearch(search personRecordSearch, page, pageSize int) ([]personRecord, int, error)
	Create(personRecord personRecord) (*personRecord, error)
	GetByID(id int64) (*personRecord, error)
	GetByDisplayID(displayID string) (*personRecord, error)
	Update(personRecord personRecord) error
	Delete(id int64) error
	UnitCamaraderieGraph(personRecordID int64) (*records.UnitCamaraderieGraph, error)
	ServiceTimeline(personRecordID int64) (*records.ServiceTimeline, error)
	ResearchLog(personRecordID int64) (*records.ResearchLog, error)
	AddResearchTask(personRecordID int64, title, notes, evidenceType string) error
	ResolveResearchTask(personRecordID, taskID int64) error
	ResearchPackForPersonRecord(personRecordID int64, scope string) (*records.ResearchPack, error)
	ResearchCollectionsHub(currentPersonRecordID int64) (*records.ResearchCollectionHub, error)
	CreateResearchCollection(name, description string) error
	AddPersonRecordToResearchCollection(collectionID, personRecordID int64) error
	ResearchCollectionDetail(collectionID, currentPersonRecordID int64) (*records.ResearchCollectionDetail, error)
	ReviewQueue(page, pageSize int) ([]personRecord, int, error)
	MarkReviewResolved(personRecordID int64) error
	SetReviewStatus(personRecordID int64, needsReview bool, reason string) error
	ListByEntryTypes(entryTypes []string, page, pageSize int) ([]personRecord, int, error)
	ManualComparison(leftID, rightID int64) (*records.DuplicateAuditComparison, error)
	GetImageByID(imageID int64) (*models.Image, error)
	DeleteImages(personRecordID int64, imageIDs []int64) error
	SetPrimaryImage(personRecordID, imageID int64) error
	MarriageCandidates() ([]personRecord, error)
	AddImage(personRecordID int64, fileName, filePath, caption string) error
	PreviewMemorialArchive(path string) (records.MemorialImportPreview, error)
	ImportMemorialArchive(path string) (records.MemorialImportSummary, error)
}

type anniversaryFacade interface {
	GetByMonthDay(month, day int) ([]models.Soldier, error)
	GetMonthCalendar(month int) (map[int][]models.Soldier, error)
}

type calendarFacade interface {
	GetMonthSummary(month int) (map[int]records.CalendarDaySummary, error)
	GetDay(month, day int) (records.CalendarDay, error)
	CreateCalendarItem(month, day int, input records.CalendarItemInput) (models.CalendarItem, error)
	UpdateCalendarItem(itemID int64, input records.CalendarItemInput) (models.CalendarItem, error)
	DeleteCalendarItem(itemID int64) error
}

type analyticsFacade interface {
	Snapshot() (records.AnalyticsSnapshot, error)
}

type reviewFacade interface {
	FindingsForPersonRecords(personRecordIDs []int64) (map[int64][]records.DuplicateAuditFindingSummary, error)
	ResolveFindingsForPersonRecord(personRecordID int64) error
	RunDuplicateAudit() (records.DuplicateAuditRunResult, error)
	ResolveFinding(findingID int64) error
	Comparison(findingID int64) (*records.DuplicateAuditComparison, error)
}

type imageFacade interface {
	EnsureShardedStorage(dataDir string) error
	DiscoverOrphans(dataDir string) ([]archive.OrphanedImage, error)
	MoveOrphansToTrash(dataDir string, relativePaths []string) (int, string, error)
	PurgeExpiredTrash(dataDir string) error
}

type exportFacade interface {
	ExportJSON(outputPath string) error
	ExportAnalyticsSummaryPDF(outputPath string, snapshot records.AnalyticsSnapshot, options archive.PDFOptions) error
	ExportExcel(outputPath string) error
	ExportICalendar(outputPath string) error
	StaticArchiveFileName(now time.Time) (string, error)
	ExportStaticArchive(outputPath, dataDir string) error
	ExportFullDatabasePDF(outputPath string, settings archive.PrintSettings) error
	ExportCSV(outputPath string) error
	ExportSoldierPDF(outputPath string, soldier models.Soldier, options archive.PDFOptions) error
	ExportSoldierJPG(outputPath string, soldier models.Soldier, options archive.PDFOptions) ([]string, error)
	ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error
	ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier, options archive.PDFOptions) error
	ExportImages(outputPath string, images []models.Image) error
}

type backupFacade interface {
	Export(outputPath, dataDir string) (archive.BackupManifest, error)
	ExportShared(outputPath, dataDir string) (archive.BackupManifest, error)
	ImportWithLocalIdentity(backupPath, dataDir string, localIdentity models.UserIdentity, preserveLocalIdentity bool) (archive.BackupManifest, error)
	ImportSharedBackup(backupPath, dataDir string) (archive.SharedImportSummary, error)
	ResolveMergeConflict(conflictID int64, decision, dataDir string) error
	PendingMergeConflicts() ([]models.MergeReviewConflict, error)
	ConflictLedger(soldierID int64) (*archive.SourceConflictLedger, error)
}

type diagnosticsFacade interface {
	Export(outputPath, dataDir string) (archive.DiagnosticsManifest, error)
}

type integrationFacade interface {
	Status() (models.GoogleStatus, error)
	Connect(ctx context.Context) error
	Disconnect() error
	UploadBackup(ctx context.Context, backupPath string) (integrations.GoogleDriveUploadResult, error)
	UploadCSVAsSheet(ctx context.Context, csvPath, title string) (integrations.GoogleDriveUploadResult, error)
	LoadEffectiveSettings() (models.GoogleSettings, bool, bool, string, error)
	SyncCalendar(ctx context.Context, settings models.GoogleSettings, soldiers []models.Soldier) (integrations.GoogleCalendarSyncResult, error)
	UnsyncCalendar(ctx context.Context) (integrations.GoogleCalendarUnsyncResult, error)
}

type updaterFacade interface {
	Settings() (update.SettingsState, error)
	SaveSource(rawURL string) (update.SettingsState, error)
	Check() (update.CheckResult, error)
	PrepareLatest() (update.PreparedUpdate, error)
}
