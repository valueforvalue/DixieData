package services

import (
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/integrations"
	"github.com/valueforvalue/DixieData/internal/records"
)

type SoldierService = records.SoldierService
type AnniversaryService = records.AnniversaryService

type UnitCamaraderieGraph = records.UnitCamaraderieGraph
type UnitCamaraderieConnection = records.UnitCamaraderieConnection
type ServiceTimeline = records.ServiceTimeline
type ServiceTimelineEvent = records.ServiceTimelineEvent
type ResearchTask = records.ResearchTask
type ResearchTaskSuggestion = records.ResearchTaskSuggestion
type ResearchLog = records.ResearchLog
type ResearchPack = records.ResearchPack
type ResearchCollection = records.ResearchCollection
type ResearchCollectionHub = records.ResearchCollectionHub
type ResearchCollectionDetail = records.ResearchCollectionDetail

type AuditService = records.AuditService
type DuplicateAuditSummary = records.DuplicateAuditSummary
type DuplicateAuditRunResult = records.DuplicateAuditRunResult
type DuplicateAuditFindingSummary = records.DuplicateAuditFindingSummary
type ReviewQueueEntry = records.ReviewQueueEntry
type DuplicateAuditComparisonField = records.DuplicateAuditComparisonField
type DuplicateAuditComparison = records.DuplicateAuditComparison

type AnalyticsService = records.AnalyticsService
type AnalyticsCount = records.AnalyticsCount
type AnalyticsSnapshot = records.AnalyticsSnapshot

type BackupService = archive.BackupService
type BackupManifest = archive.BackupManifest
type SharedImportSummary = archive.SharedImportSummary
type SourceConflictLedger = archive.SourceConflictLedger
type SourceConflictLedgerEntry = archive.SourceConflictLedgerEntry
type DiagnosticsService = archive.DiagnosticsService
type DiagnosticsManifest = archive.DiagnosticsManifest
type ImageService = archive.ImageService
type OrphanedImage = archive.OrphanedImage
type ExportService = archive.ExportService
type PrintSettings = archive.PrintSettings

type ExportMetadata = archive.ExportMetadata
type JSONExportDocument = archive.JSONExportDocument
type StaticArchiveRecord = archive.StaticArchiveRecord
type StaticArchiveImage = archive.StaticArchiveImage
type StaticArchiveRecordEntry = archive.StaticArchiveRecordEntry

type GoogleCalendarSyncState = integrations.GoogleCalendarSyncState
type GoogleDriveUploadResult = integrations.GoogleDriveUploadResult
type GoogleCalendarSyncResult = integrations.GoogleCalendarSyncResult
type GoogleCalendarUnsyncResult = integrations.GoogleCalendarUnsyncResult
type GoogleService = integrations.GoogleService

const (
	PrintSortLastName  = archive.PrintSortLastName
	PrintSortBirthYear = archive.PrintSortBirthYear
	PrintSortDeathYear = archive.PrintSortDeathYear
)

func NewSoldierService(database *db.DB) *SoldierService { return records.NewSoldierService(database) }
func NewAnniversaryService(database *db.DB) *AnniversaryService {
	return records.NewAnniversaryService(database)
}
func NewAuditService(database *db.DB) *AuditService { return records.NewAuditService(database) }
func NewAnalyticsService(database *db.DB) *AnalyticsService {
	return records.NewAnalyticsService(database)
}
func NewImageService(database *db.DB) *ImageService { return archive.NewImageService(database) }
func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return archive.NewExportService(database, soldier)
}
func NewBackupService(database *db.DB, soldier *SoldierService) *BackupService {
	return archive.NewBackupService(database, soldier)
}
func NewDiagnosticsService(database *db.DB, soldier *SoldierService) *DiagnosticsService {
	return archive.NewDiagnosticsService(database, soldier)
}
func NewGoogleService(dataDir string) *GoogleService { return integrations.NewGoogleService(dataDir) }

func DiagnosticsBundleName(now time.Time) string { return archive.DiagnosticsBundleName(now) }
