package presentation

import (
	"github.com/a-h/templ"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func Calendar(month int, summary map[int]records.CalendarDaySummary, counts models.ArchiveCounts, quote models.Quote) templ.Component {
	return templates.Calendar(month, viewmodel.CalendarDaySummariesFromDomain(summary), viewmodel.ArchiveCountsFromModel(counts), viewmodel.QuoteFromModel(quote))
}

func JobStatusFragment(job jobs.Job) templ.Component {
	return templates.JobStatusFragment(job)
}

func JobStatusSlotFragment(job jobs.Job) templ.Component {
	return templates.JobStatusSlotFragment(job)
}

func CalendarGrid(month int, summary map[int]records.CalendarDaySummary) templ.Component {
	return templates.CalendarGrid(month, viewmodel.CalendarDaySummariesFromDomain(summary))
}

func InitialSetupView(form models.InitialSetupForm) templ.Component {
	return templates.InitialSetupView(viewmodel.InitialSetupFormFromModel(form))
}

func CalendarDayDetail(day records.CalendarDay, editingID int64, itemType, title, notes, errorMessage, statusKind, statusMessage string) templ.Component {
	return templates.CalendarDayDetail(viewmodel.CalendarDayDetailFromDomain(day, viewmodel.CalendarItemForm{
		EditingID:    editingID,
		ItemType:     itemType,
		Title:        title,
		Notes:        notes,
		ErrorMessage: errorMessage,
	}, statusKind, statusMessage))
}

func SoldierList(soldiers []models.Soldier, page, total int, query string, suggestions models.SoldierFormSuggestions) templ.Component {
	return templates.SoldierList(viewmodel.PersonRecordsFromModels(soldiers), page, total, query, viewmodel.PersonRecordFormSuggestionsFromModel(suggestions))
}

func BrowseView(soldiers []models.Soldier, state records.BrowseRequest, total int, suggestions models.SoldierFormSuggestions, exportRecords []models.Soldier) templ.Component {
	return templates.BrowseView(
		viewmodel.PersonRecordsFromModels(soldiers),
		viewmodel.BrowseStateFromDomain(state, total),
		viewmodel.PersonRecordFormSuggestionsFromModel(suggestions),
		viewmodel.ExportRecordOptionsFromModels(exportRecords),
	)
}

func BrowseResults(soldiers []models.Soldier, state records.BrowseRequest, total int) templ.Component {
	return templates.BrowseResults(
		viewmodel.PersonRecordsFromModels(soldiers),
		viewmodel.BrowseStateFromDomain(state, total),
	)
}

func SearchResults(soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int) templ.Component {
	return templates.SearchResults(viewmodel.PersonRecordsFromModels(soldiers), viewmodel.PersonRecordSearchFromModel(search), page, total, pageSize)
}

func SoldierDetail(soldier models.Soldier) templ.Component {
	return templates.SoldierDetail(viewmodel.PersonRecordFromModel(soldier))
}

func UnitCamaraderieView(graph records.UnitCamaraderieGraph) templ.Component {
	return templates.UnitCamaraderieView(viewmodel.UnitCamaraderieGraphFromDomain(graph))
}

func ServiceTimelineView(timeline records.ServiceTimeline) templ.Component {
	return templates.ServiceTimelineView(viewmodel.ServiceTimelineFromDomain(timeline))
}

func ResearchLogView(log records.ResearchLog) templ.Component {
	return templates.ResearchLogView(viewmodel.ResearchLogFromDomain(log))
}

func MergeReviewLedgerView(ledger archive.SourceConflictLedger) templ.Component {
	return templates.MergeReviewLedgerView(viewmodel.MergeReviewLedgerFromDomain(ledger))
}

func ResearchPackView(pack records.ResearchPack) templ.Component {
	return templates.ResearchPackView(viewmodel.ResearchPackFromDomain(pack))
}

func ShareView(status models.GoogleStatus, conflicts []models.MergeReviewConflict, exportRecords []models.Soldier, counts models.ArchiveCounts) templ.Component {
	return templates.ShareView(
		viewmodel.GoogleStatusFromModel(status),
		viewmodel.MergeReviewConflictsFromModels(conflicts),
		viewmodel.ExportRecordOptionsFromModels(exportRecords),
		viewmodelCountsFromModels(counts),
	)
}

func ResearchCollectionsHubView(hub records.ResearchCollectionHub) templ.Component {
	return templates.ResearchCollectionsHubView(viewmodel.ResearchCollectionHubFromDomain(hub))
}

func ResearchCollectionDetailView(detail records.ResearchCollectionDetail) templ.Component {
	return templates.ResearchCollectionDetailView(viewmodel.ResearchCollectionDetailFromDomain(detail))
}

func ReviewQueueView(soldiers []models.Soldier, findings map[int64][]records.DuplicateAuditFindingSummary, counts models.ArchiveCounts, page, total, pageSize int) templ.Component {
	return templates.ReviewQueueView(viewmodel.ReviewQueueEntriesFromDomain(soldiers, findings), viewmodelCountsFromModels(counts), page, total, pageSize)
}

func InsightsView(snapshot records.AnalyticsSnapshot, counts models.ArchiveCounts) templ.Component {
	return templates.InsightsView(viewmodel.AnalyticsSnapshotFromDomain(snapshot), viewmodelCountsFromModels(counts))
}

func InsightsDrilldownView(title, description string, soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int, scope, value string) templ.Component {
	return templates.InsightsDrilldownView(title, description, viewmodel.PersonRecordsFromModels(soldiers), viewmodel.PersonRecordSearchFromModel(search), page, total, pageSize, scope, value)
}

func SettingsView(confirmationWord string, updater update.SettingsState) templ.Component {
	return templates.SettingsView(confirmationWord, viewmodel.UpdateSettingsFromDomain(updater))
}

func SettingsUpdatePanel(updater update.SettingsState) templ.Component {
	return templates.SettingsUpdatePanel(viewmodel.UpdateSettingsFromDomain(updater))
}

func SettingsUpdateStatus(result update.CheckResult) templ.Component {
	return templates.SettingsUpdateStatus(viewmodel.UpdateCheckResultFromDomain(result))
}

func SettingsUpdateStatusMessage(kind, message string) templ.Component {
	return templates.SettingsUpdateStatusMessage(kind, message)
}

func SettingsUpdateApplyStarted(version string) templ.Component {
	return templates.SettingsUpdateApplyStarted(version)
}

func UpdateRecoveryPage(record update.RestorePointRecord, failureMessage string, rollbackStarted bool) templ.Component {
	return templates.UpdateRecoveryPage(record.CreatedAt, record.SourceAppVersion, record.TargetAppVersion, failureMessage, rollbackStarted)
}

func SettingsOrphanedImages(orphans []archive.OrphanedImage) templ.Component {
	return templates.SettingsOrphanedImages(viewmodel.OrphanedImagesFromDomain(orphans))
}

func SettingsOrphanCleanupResult(moved int, trashRoot string) templ.Component {
	return templates.SettingsOrphanCleanupResult(moved, trashRoot)
}

func SettingsQualityScanResults(result records.DataQualityScanResult) templ.Component {
	return templates.SettingsQualityScanResults(viewmodel.DataQualityScanResultFromDomain(result))
}

func SettingsQualityScanApplyResult(result records.DataQualityApplyResult) templ.Component {
	return templates.SettingsQualityScanApplyResult(viewmodel.DataQualityApplyResultFromDomain(result))
}

func ReviewQueueCompareView(comparison records.DuplicateAuditComparison) templ.Component {
	return templates.ReviewQueueCompareView(viewmodel.DuplicateAuditComparisonFromDomain(comparison))
}

func EntryForm(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool) templ.Component {
	return templates.EntryForm(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit)
}

func EntryFormWithError(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormWithError(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit, errorMessage)
}

func EntryFormFragment(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormFragment(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit, errorMessage)
}

// viewmodelCountsFromModels translates models.ArchiveCounts to the
// viewmodel-shaped counts struct the templates consume. Mirrors the
// pattern used for every other domain-to-viewmodel conversion in this
// file. Lives here (not in mappers.go) because the templates use the
// viewmodel type directly and this is the only place that needs both.
func viewmodelCountsFromModels(counts models.ArchiveCounts) viewmodel.ArchiveCounts {
	return viewmodel.ArchiveCounts{
		SoldierCount:      counts.TotalSoldiers,
		SpouseRecordCount: counts.TotalWivesWidows,
		PersonRecordCount: counts.TotalLinkedPeople,
	}
}
