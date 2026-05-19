package appshell

import (
	"context"
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

type soldiersFacade interface {
	ArchiveCounts() (models.ArchiveCounts, error)
	FormSuggestions() (models.SoldierFormSuggestions, error)
	List(page, pageSize int) ([]models.Soldier, int, error)
	SearchPage(query string, page, pageSize int) ([]models.Soldier, int, error)
	RecentByIDs(ids []int64, limit int) ([]models.Soldier, error)
	AdvancedSearch(search models.SoldierSearch, page, pageSize int) ([]models.Soldier, int, error)
	Create(soldier models.Soldier) (*models.Soldier, error)
	GetByID(id int64) (*models.Soldier, error)
	Update(soldier models.Soldier) error
	Delete(id int64) error
	UnitCamaraderieGraph(soldierID int64) (*records.UnitCamaraderieGraph, error)
	ServiceTimeline(soldierID int64) (*records.ServiceTimeline, error)
	ResearchLog(soldierID int64) (*records.ResearchLog, error)
	AddResearchTask(soldierID int64, title, notes, evidenceType string) error
	ResolveResearchTask(soldierID, taskID int64) error
	ResearchPackForSoldier(soldierID int64, scope string) (*records.ResearchPack, error)
	ResearchCollectionsHub(currentSoldierID int64) (*records.ResearchCollectionHub, error)
	CreateResearchCollection(name, description string) error
	AddSoldierToResearchCollection(collectionID, soldierID int64) error
	ResearchCollectionDetail(collectionID, currentSoldierID int64) (*records.ResearchCollectionDetail, error)
	ReviewQueue(page, pageSize int) ([]models.Soldier, int, error)
	MarkReviewResolved(soldierID int64) error
	SetReviewStatus(soldierID int64, needsReview bool, reason string) error
	ListByEntryTypes(entryTypes []string, page, pageSize int) ([]models.Soldier, int, error)
	ManualComparison(leftID, rightID int64) (*records.DuplicateAuditComparison, error)
	GetImageByID(imageID int64) (*models.Image, error)
	DeleteImages(soldierID int64, imageIDs []int64) error
	SetPrimaryImage(soldierID, imageID int64) error
	MarriageCandidates() ([]models.Soldier, error)
	AddImage(soldierID int64, fileName, filePath, caption string) error
}

type anniversaryFacade interface {
	GetByMonthDay(month, day int) ([]models.Soldier, error)
	GetMonthCalendar(month int) (map[int][]models.Soldier, error)
}

type analyticsFacade interface {
	Snapshot() (records.AnalyticsSnapshot, error)
}

type reviewFacade interface {
	FindingsForSoldiers(soldierIDs []int64) (map[int64][]records.DuplicateAuditFindingSummary, error)
	ResolveFindingsForSoldier(soldierID int64) error
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
	ExportAnalyticsSummaryPDF(outputPath string, snapshot records.AnalyticsSnapshot) error
	ExportExcel(outputPath string) error
	ExportICalendar(outputPath string) error
	StaticArchiveFileName(now time.Time) (string, error)
	ExportStaticArchive(outputPath, dataDir string) error
	ExportFullDatabasePDF(outputPath string, settings archive.PrintSettings) error
	ExportCSV(outputPath string) error
	ExportSoldierPDF(outputPath string, soldier models.Soldier) error
	ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error
	ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier) error
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
