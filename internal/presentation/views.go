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

// CalendarDayDetailPage wraps the bare CalendarDayDetail
// fragment in the Layout shell so direct navigation to
// /anniversary/<month>/<day> loads app.css + the top-nav
// + the floating dock instead of shipping an unstyled
// fragment. The HTMX swap path (where the parent calendar
// page already loaded the stylesheets) continues to use
// the bare CalendarDayDetail component via the HX-Request
// branch in appshell.
func CalendarDayDetailPage(day records.CalendarDay, editingID int64, itemType, title, notes, errorMessage, statusKind, statusMessage string) templ.Component {
	return templates.CalendarDayDetailPage(viewmodel.CalendarDayDetailFromDomain(day, viewmodel.CalendarItemForm{
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

func BrowseView(soldiers []models.Soldier, state records.BrowseRequest, total int, suggestions models.SoldierFormSuggestions, exportRecords []models.Soldier, availableTags []records.Tag, tagMap map[int64][]viewmodel.TagOption) templ.Component {
	return templates.BrowseView(
		viewmodel.PersonRecordsFromModelsWithTags(soldiers, tagMap),
		viewmodel.BrowseStateFromDomain(state, total),
		viewmodel.PersonRecordFormSuggestionsFromModel(suggestions),
		viewmodel.ExportRecordOptionsFromModels(exportRecords),
		viewmodel.TagsFromModels(availableTags),
	)
}

func BrowseResults(soldiers []models.Soldier, state records.BrowseRequest, total int, tagMap map[int64][]viewmodel.TagOption, availableTags []viewmodel.TagOption) templ.Component {
	return templates.BrowseResults(
		viewmodel.PersonRecordsFromModelsWithTags(soldiers, tagMap),
		viewmodel.BrowseStateFromDomain(state, total),
		availableTags,
	)
}

func SearchResults(soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int) templ.Component {
	return templates.SearchResults(viewmodel.PersonRecordsFromModels(soldiers), viewmodel.PersonRecordSearchFromModel(search), page, total, pageSize)
}

func SoldierDetail(soldier models.Soldier, tags []records.Tag) templ.Component {
	return templates.SoldierDetail(viewmodel.PersonRecordFromModel(soldier), tags)
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

func ShareView(conflicts []models.MergeReviewConflict, counts models.ArchiveCounts, recentJobs []viewmodel.RecentJobEntry) templ.Component {
	return templates.ShareView(
		viewmodel.MergeReviewConflictsFromModels(conflicts),
		viewmodelCountsFromModels(counts),
		recentJobs,
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

func EntryForm(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, isEdit bool) templ.Component {
	return templates.EntryForm(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), isEdit)
}

func EntryFormWithError(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormWithError(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), isEdit, errorMessage)
}

func EntryFormFragment(soldier models.Soldier, spouseCandidates []models.Soldier, suggestions models.SoldierFormSuggestions, isEdit bool, errorMessage string) templ.Component {
	return templates.EntryFormFragment(viewmodel.PersonRecordFromModel(soldier), viewmodel.PersonRecordsFromModels(spouseCandidates), viewmodel.PersonRecordFormSuggestionsFromModel(suggestions), isEdit, errorMessage)
}

// v60 (issue #320): Event Record presentation helpers. Each
// adapts the EventService's domain types (models.Soldier rows
// for the Event itself, records.EventWithLinks for the read
// side) into the viewmodel shape the Event templates consume.
// Mirrors the SoldierList / SoldierDetail / EntryForm pattern
// the rest of the app uses — handlers stay on the domain
// surface; presentation owns the viewmodel translation.

// EventList wraps templates.EventList. Used by the /events
// landing page.
func EventList(events []models.Soldier, page, total int) templ.Component {
	return templates.EventList(viewmodel.PersonRecordsFromModels(events), page, total)
}

// EventDetail wraps templates.EventDetail. The links slice
// (records.EventLink rows) is projected to the viewmodel
// shape the Linked Person Records section renders. The
// model.Soldier rows in the links are mapped via the existing
// PersonRecordFromModel helper.
// EventDetail wraps templates.EventDetail. The links slice
// (records.EventLink rows) is projected to the viewmodel shape
// the Linked Person Records section renders. The tags slice is
// pulled via a tag query so the section has chip data; the
// Event's `Records` projection flows through PersonRecordFromModel.
func EventDetail(tags []records.Tag, event *records.EventWithLinks) templ.Component {
	linked := make([]viewmodel.PersonRecord, 0, len(event.Links))
	for _, link := range event.Links {
		linked = append(linked, viewmodel.PersonRecord{
			ID:        link.PersonID,
			DisplayID: link.PersonDisplay,
		})
	}
	vm := viewmodel.PersonRecordFromModel(event.Event)
	if len(tags) > 0 {
		vm.Tags = make([]viewmodel.TagOption, 0, len(tags))
		for _, t := range tags {
			vm.Tags = append(vm.Tags, viewmodel.TagOption{ID: t.ID, Name: t.Name})
		}
	}
	return templates.EventDetail(vm, linked)
}

// EventForm wraps templates.EventForm. The handler builds the
// defaults from newEventDefaults (or the existing row on edit)
// and hands the resulting models.Soldier to this helper.
func EventForm(event models.Soldier, isEdit bool) templ.Component {
	return templates.EventForm(viewmodel.PersonRecordFromModel(event), isEdit)
}

// EventFormWithLinks (issue #361 slice 2) renders the Event
// edit form with the inline Linked Persons section populated.
// The handler fetches the links via ListForEvent (slice-2
// path) and passes them in; the presentation layer maps them
// to viewmodel and renders. New-event callers continue to
// use EventForm (no links possible for an un-persisted id).
func EventFormWithLinks(event models.Soldier, linked []models.Soldier, isEdit bool) templ.Component {
	vm := viewmodel.PersonRecordFromModel(event)
	vm.LinkedPersons = viewmodel.PersonRecordsFromModels(linked)
	return templates.EventForm(vm, isEdit)
}

// EventFormWithLinksAndTags (issue #361 slice 3) renders the
// Event edit form with BOTH the inline Linked Persons section
// (slice 2) AND the inline Tags section (slice 3) populated.
// The handler fetches both via the eventsFacade; the
// presentation layer maps them to viewmodel fields. The
// detail page has its own event-detail rendering pipeline
// (handlers load tags separately there); this is the
// edit-form-only population.
func EventFormWithLinksAndTags(event models.Soldier, linked []models.Soldier, tags []records.Tag, isEdit bool) templ.Component {
	vm := viewmodel.PersonRecordFromModel(event)
	vm.LinkedPersons = viewmodel.PersonRecordsFromModels(linked)
	vm.Tags = viewmodel.TagsFromModels(tags)
	return templates.EventForm(vm, isEdit)
}

// EventFormWithError mirrors EntryFormWithError: same body,
// toast header for the form-level validation message.
func EventFormWithError(event models.Soldier, isEdit bool, errorMessage string) templ.Component {
	return templates.EventFormWithError(viewmodel.PersonRecordFromModel(event), isEdit, errorMessage)
}

// EventFormWithErrorAndLinks (issue #361 slice 2) mirrors
// EventFormWithError but populates the Linked Persons section.
// Used by the POST error-rendering path so the user sees their
// existing links + the validation error on the same surface.
func EventFormWithErrorAndLinks(event models.Soldier, linked []models.Soldier, isEdit bool, errorMessage string) templ.Component {
	vm := viewmodel.PersonRecordFromModel(event)
	vm.LinkedPersons = viewmodel.PersonRecordsFromModels(linked)
	return templates.EventFormWithError(vm, isEdit, errorMessage)
}

// EventFormWithErrorAndLinksAndTags (issue #361 slice 3)
// mirrors EventFormWithErrorAndLinks but ALSO populates the
// inline Tags section. The POST error-rendering path needs
// both so the user sees their existing links + tags alongside
// the validation error.
func EventFormWithErrorAndLinksAndTags(event models.Soldier, linked []models.Soldier, tags []records.Tag, isEdit bool, errorMessage string) templ.Component {
	vm := viewmodel.PersonRecordFromModel(event)
	vm.LinkedPersons = viewmodel.PersonRecordsFromModels(linked)
	vm.Tags = viewmodel.TagsFromModels(tags)
	return templates.EventFormWithError(vm, isEdit, errorMessage)
}

// PersonEventsTab wraps templates.PersonEventsTab. The lazy-load
// fragment for the Person Record → Events tab (issue #320 slice
// #324). The linked slice is the per-Person projection from
// eventsFacade.ListForPerson; the template renders it as a
// table of Display ID + Kind + Date Range (D2 of #322 applied
// here too — no biography excerpt).
func PersonEventsTab(personID int64, linked []models.Soldier) templ.Component {
	return templates.PersonEventsTab(personID, viewmodel.PersonRecordsFromModels(linked))
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

// ShareQueuePage (issue #193) wraps the
// templates.ShareQueuePage component so the handler can call
// it through the same presentation facade the rest of the
// app uses.
func ShareQueuePage(rows []viewmodel.ShareQueueRow) templ.Component {
	return templates.ShareQueuePage(rows)
}

// Issue #284: wrappers for the dedicated /share/exports,
// /share/imports, /share/sync subpages. Each is a thin
// adapter from the handler's domain types into the
// viewmodel shape the templ component expects, mirroring
// the pattern set by ShareView above.

// ShareExportsView wraps templates.ShareExportsView. The
// exports subpage needs the export-records list (for the
// PrintConfigModal partial) and the include-tags flag
// (for the .ddshare checkbox).
func ShareExportsView(exportRecords []models.Soldier, shareIncludeTags bool) templ.Component {
	return templates.ShareExportsView(
		viewmodel.ExportRecordOptionsFromModels(exportRecords),
		shareIncludeTags,
	)
}

// ShareImportsView wraps templates.ShareImportsView. The
// imports subpage is a static launchpad — no domain
// data, no viewmodel adaptation needed.
func ShareImportsView() templ.Component {
	return templates.ShareImportsView()
}

// ShareSyncView wraps templates.ShareSyncView. The sync
// subpage renders the same Google Integration card the
// /share landing used to render, so the same domain
// model + viewmodel adapter is reused.
func ShareSyncView(status models.GoogleStatus) templ.Component {
	return templates.ShareSyncView(viewmodel.GoogleStatusFromModel(status))
}
