package presentation

import (
	"github.com/a-h/templ"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

func Calendar(month int, calendar map[int][]models.Soldier, counts models.ArchiveCounts, quote models.Quote) templ.Component {
	return templates.Calendar(month, viewmodel.CalendarFromModels(calendar), viewmodel.ArchiveCountsFromModel(counts), viewmodel.QuoteFromModel(quote))
}

func InitialSetupView(form models.InitialSetupForm) templ.Component {
	return templates.InitialSetupView(viewmodel.InitialSetupFormFromModel(form))
}

func AnniversaryPartial(soldiers []models.Soldier, month, day int) templ.Component {
	return templates.AnniversaryPartial(viewmodel.SoldiersFromModels(soldiers), month, day)
}

func SoldierList(soldiers []models.Soldier, page, total int, query string, suggestions models.SoldierFormSuggestions) templ.Component {
	return templates.SoldierList(viewmodel.SoldiersFromModels(soldiers), page, total, query, viewmodel.SoldierFormSuggestionsFromModel(suggestions))
}

func SearchResults(soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int) templ.Component {
	return templates.SearchResults(viewmodel.SoldiersFromModels(soldiers), viewmodel.SoldierSearchFromModel(search), page, total, pageSize)
}

func SoldierDetail(soldier models.Soldier) templ.Component {
	return templates.SoldierDetail(viewmodel.SoldierFromModel(soldier))
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

func SourceConflictLedgerView(ledger archive.SourceConflictLedger) templ.Component {
	return templates.SourceConflictLedgerView(viewmodel.SourceConflictLedgerFromDomain(ledger))
}

func ResearchPackView(pack records.ResearchPack) templ.Component {
	return templates.ResearchPackView(viewmodel.ResearchPackFromDomain(pack))
}

func ShareView(status models.GoogleStatus, conflicts []models.MergeReviewConflict) templ.Component {
	return templates.ShareView(viewmodel.GoogleStatusFromModel(status), viewmodel.MergeReviewConflictsFromModels(conflicts))
}

func ResearchCollectionsHubView(hub records.ResearchCollectionHub) templ.Component {
	return templates.ResearchCollectionsHubView(viewmodel.ResearchCollectionHubFromDomain(hub))
}

func ResearchCollectionDetailView(detail records.ResearchCollectionDetail) templ.Component {
	return templates.ResearchCollectionDetailView(viewmodel.ResearchCollectionDetailFromDomain(detail))
}

func ReviewQueueView(soldiers []models.Soldier, findings map[int64][]records.DuplicateAuditFindingSummary, page, total, pageSize int) templ.Component {
	return templates.ReviewQueueView(viewmodel.ReviewQueueEntriesFromDomain(soldiers, findings), page, total, pageSize)
}

func InsightsView(snapshot records.AnalyticsSnapshot) templ.Component {
	return templates.InsightsView(viewmodel.AnalyticsSnapshotFromDomain(snapshot))
}

func InsightsDrilldownView(title, description string, soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int, scope, value string) templ.Component {
	return templates.InsightsDrilldownView(title, description, viewmodel.SoldiersFromModels(soldiers), viewmodel.SoldierSearchFromModel(search), page, total, pageSize, scope, value)
}

func SettingsView(confirmationWord string) templ.Component {
	return templates.SettingsView(confirmationWord)
}

func SettingsOrphanedImages(orphans []archive.OrphanedImage) templ.Component {
	return templates.SettingsOrphanedImages(viewmodel.OrphanedImagesFromDomain(orphans))
}

func SettingsOrphanCleanupResult(moved int, trashRoot string) templ.Component {
	return templates.SettingsOrphanCleanupResult(moved, trashRoot)
}

func ReviewQueueCompareView(comparison records.DuplicateAuditComparison) templ.Component {
	return templates.ReviewQueueCompareView(viewmodel.DuplicateAuditComparisonFromDomain(comparison))
}

func EntryForm(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool) templ.Component {
	return templates.EntryForm(viewmodel.SoldierFromModel(soldier), viewmodel.SoldiersFromModels(spouseCandidates), viewmodel.SoldierFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit)
}

func EntryFormWithError(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormWithError(viewmodel.SoldierFromModel(soldier), viewmodel.SoldiersFromModels(spouseCandidates), viewmodel.SoldierFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit, errorMessage)
}

func EntryFormFragment(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, scrape models.FindAGraveScrapeState, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormFragment(viewmodel.SoldierFromModel(soldier), viewmodel.SoldiersFromModels(spouseCandidates), viewmodel.SoldierFormSuggestionsFromModel(suggestions), viewmodel.FindAGraveScrapeStateFromModel(scrape), isEdit, errorMessage)
}
