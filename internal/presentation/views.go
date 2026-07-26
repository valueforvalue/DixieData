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
	return templates.SoldierDetail(viewmodel.PersonRecordFromModel(soldier), tags, nil)
}

// SoldierDetailWithCitedIn is the slice-3.8 wrapper over
// SoldierDetail -- it carries the CitedIn articles
// (reverse-lookup from article_refs.article_id where the
// row cites this person) so the detail page can render
// the "Cited in" panel. nil hides the panel (the slice-1
// detail surface; the slice-3.8 caller passes the slice).
func SoldierDetailWithCitedIn(soldier models.Soldier, tags []records.Tag, citedIn []models.Article) templ.Component {
	return templates.SoldierDetail(viewmodel.PersonRecordFromModel(soldier), tags, viewmodel.ArticlesFromModels(citedIn))
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

func ReviewQueueView(soldiers []models.Soldier, findings map[int64][]records.DuplicateAuditFindingSummary, counts models.ArchiveCounts, page, total, pageSize int, activeTab string) templ.Component {
	return templates.ReviewQueueView(viewmodel.ReviewQueueEntriesFromDomain(soldiers, findings), viewmodelCountsFromModels(counts), page, total, pageSize, activeTab)
}

// ResolvedReviewQueueView renders the Resolved tab (issue
// #461): a paginated history of resolved duplicate-audit
// findings + resolved merge-review conflicts. Each row links
// back to the Person Record so the user can re-open the
// original audit compare or merge-replay if they want to
// revisit a decision.
func ResolvedReviewQueueView(findings []records.ResolvedFindingSummary, conflicts []models.MergeReviewConflict, counts models.ArchiveCounts, page, totalFindings, totalConflicts, pageSize int) templ.Component {
	return templates.ResolvedReviewQueueView(
		viewmodel.ResolvedFindingEntriesFromDomain(findings),
		viewmodel.ResolvedConflictEntriesFromDomain(conflicts),
		viewmodelCountsFromModels(counts),
		page,
		totalFindings+totalConflicts,
		pageSize,
	)
}

func InsightsView(snapshot records.AnalyticsSnapshot, counts models.ArchiveCounts) templ.Component {
	return templates.InsightsView(viewmodel.AnalyticsSnapshotFromDomain(snapshot), viewmodelCountsFromModels(counts))
}

func InsightsDrilldownView(title, description string, soldiers []models.Soldier, search models.SoldierSearch, page, total, pageSize int, scope, value string) templ.Component {
	return templates.InsightsDrilldownView(title, description, viewmodel.PersonRecordsFromModels(soldiers), viewmodel.PersonRecordSearchFromModel(search), page, total, pageSize, scope, value)
}

func SettingsView(confirmationWord string, updater update.SettingsState, currentTheme, currentExportSurface string) templ.Component {
	return templates.SettingsView(confirmationWord, viewmodel.UpdateSettingsFromDomain(updater), currentTheme, currentExportSurface)
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

// SettingsUpdateApplyStarting renders the initial "Starting
// update…" fragment the apply handler returns when it spawns
// the prepare goroutine (issue #661). The fragment carries the
// data-poll-progress marker so the JS dispatcher starts polling
// /settings/updates/progress.
func SettingsUpdateApplyStarting() templ.Component {
	return templates.SettingsUpdateApplyStarting()
}

// SettingsUpdateProgress renders the live progress fragment
// for /settings/updates/progress. The fragment includes
// data-progress-phase so the JS poller can decide whether to
// stop (terminal phases) or continue.
func SettingsUpdateProgress(progress update.UpdateProgress) templ.Component {
	return templates.SettingsUpdateProgress(
		progress.Phase,
		progress.BytesDownloaded,
		progress.TotalBytes,
		progress.Message,
		progress.Error,
	)
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
	return templates.EventList(viewmodel.EventRecordsFromModels(events, nil, nil), page, total)
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
// Event's `Records` projection flows through EventRecordFromModel.
// #343 #1: the Event projection is now EventRecord, not
// PersonRecord. The Event-only fields (Kind, BeginDate,
// EndDate, Description, EventSources, LinkedPersons) belong
// on EventRecord; the base identity (DisplayID, Tags) lives
// on the embedded PersonRecord.
func EventDetail(tags []records.Tag, event *records.EventWithLinks) templ.Component {
	linked := make([]viewmodel.PersonRecord, 0, len(event.Links))
	for _, link := range event.Links {
		linked = append(linked, viewmodel.PersonRecord{
			ID:        link.PersonID,
			DisplayID: link.PersonDisplay,
		})
	}
	// Per-Event sources (event_sources table) live on the
	// records.EventWithLinks.Event.EventSources field, populated
	// by EventService.GetEventByID from the dedicated table.
	sources := event.Event.EventSources
	vm := viewmodel.EventRecordFromModel(event.Event, nil, sources)
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
// #343 #1: the Event projection is now EventRecord.
func EventForm(event models.Soldier, isEdit bool) templ.Component {
	return templates.EventForm(viewmodel.EventRecordFromModel(event, nil, nil), isEdit)
}

// EventFormWithLinks (issue #361 slice 2) renders the Event
// edit form with the inline Linked Persons section populated.
// The handler fetches the links via ListForEvent (slice-2
// path) and passes them in; the presentation layer maps them
// to viewmodel and renders. New-event callers continue to
// use EventForm (no links possible for an un-persisted id).
// #343 #1: projection is EventRecord.
func EventFormWithLinks(event models.Soldier, linked []models.Soldier, isEdit bool) templ.Component {
	vm := viewmodel.EventRecordFromModel(event, linked, nil)
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
// #343 #1: projection is EventRecord.
func EventFormWithLinksAndTags(event models.Soldier, linked []models.Soldier, tags []records.Tag, isEdit bool) templ.Component {
	vm := viewmodel.EventRecordFromModel(event, linked, nil)
	vm.Tags = viewmodel.TagsFromModels(tags)
	return templates.EventForm(vm, isEdit)
}

// EventFormWithError mirrors EntryFormWithError: same body,
// toast header for the form-level validation message.
// #343 #1: projection is EventRecord.
func EventFormWithError(event models.Soldier, isEdit bool, errorMessage string) templ.Component {
	return templates.EventFormWithError(viewmodel.EventRecordFromModel(event, nil, nil), isEdit, errorMessage)
}

// EventFormWithErrorAndLinks (issue #361 slice 2) mirrors
// EventFormWithError but populates the Linked Persons section.
// Used by the POST error-rendering path so the user sees their
// existing links + the validation error on the same surface.
// #343 #1: projection is EventRecord.
func EventFormWithErrorAndLinks(event models.Soldier, linked []models.Soldier, isEdit bool, errorMessage string) templ.Component {
	vm := viewmodel.EventRecordFromModel(event, linked, nil)
	return templates.EventFormWithError(vm, isEdit, errorMessage)
}

// EventFormWithErrorAndLinksAndTags (issue #361 slice 3)
// mirrors EventFormWithErrorAndLinks but ALSO populates the
// inline Tags section. The POST error-rendering path needs
// both so the user sees their existing links + tags alongside
// the validation error.
// #343 #1: projection is EventRecord.
func EventFormWithErrorAndLinksAndTags(event models.Soldier, linked []models.Soldier, tags []records.Tag, isEdit bool, errorMessage string) templ.Component {
	vm := viewmodel.EventRecordFromModel(event, linked, nil)
	vm.Tags = viewmodel.TagsFromModels(tags)
	return templates.EventFormWithError(vm, isEdit, errorMessage)
}

// PersonEventsTab wraps templates.PersonEventsTab. The lazy-load
// fragment for the Person Record → Events tab (issue #320 slice
// #324). The linked slice is the per-Person projection from
// eventsFacade.ListForPerson; the template renders it as a
// table of Display ID + Kind + Date Range (D2 of #322 applied
// here too — no biography excerpt).
// #343 #1: the linked rows are now projected as EventRecord so
// the Kind + BeginDate + EndDate fields render without a
// separate PersonRecord unmarshalling.
func PersonEventsTab(personID int64, linked []models.Soldier) templ.Component {
	return templates.PersonEventsTab(personID, viewmodel.EventRecordsFromModels(linked, nil, nil))
}

// viewmodelCountsFromModels translates models.ArchiveCounts to the
// viewmodel-shaped counts struct the templates consume. Mirrors the
// pattern used for every other domain-to-viewmodel conversion in this
// file. Lives here (not in mappers.go) because the templates use the
// viewmodel type directly and this is the only place that needs both.
func viewmodelCountsFromModels(counts models.ArchiveCounts) viewmodel.ArchiveCounts {
	return viewmodel.ArchiveCounts{
		SoldierCount:       counts.TotalSoldiers,
		SpouseRecordCount:  counts.TotalWivesWidows,
		PersonRecordCount:  counts.TotalLinkedPeople,
		EventRecordCount:   counts.EventRecords,
		ArticleRecordCount: counts.Articles,
		TagCount:           counts.Tags,
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


// ArticlesListShell (issue #321 slice 2) wraps
// templates.ArticlesListShell with the slice-2 per-row card
// list. The slice-1 surface was an empty-state placeholder
// because ArticleService had no List yet; slice 2 wires in
// viewmodel.ArticlesFromModels so the rendered HTML includes
// every non-snapshot article.
func ArticlesListShell(articles []viewmodel.Article) templ.Component {
	return templates.ArticlesListShell(articles)
}

// ArticleNewShell (issue #321 slice 1) wraps
// templates.ArticleNewShell. The slice-1 surface is a minimal
// title + subtitle + body form posting to /articles/new;
// slice 3 swaps this for the markdown editor + sanitized
// preview + local-draft-persistence block.
func ArticleNewShell() templ.Component {
	return templates.ArticleNewShell()
}

// ArticleDetailShell (issue #321 slice 1) wraps
// templates.ArticleDetailShell with the per-Article viewmodel.
// slice 1 renders title + body verbatim; slice 3 adds the
// Refs panel + the "Cited in" reverse-lookup section + the
// Revisions tab.
func ArticleDetailShell(view viewmodel.Article) templ.Component {
	return templates.ArticleDetailShell(view)
}

// ArticleEditShell (issue #321 slice 3.5) wraps
// templates.ArticleEditShell with the per-Article viewmodel.
// Slice 3.5 ships a minimal form; slice 3.7 swaps in the
// full markdown editor + sanitized preview + local-draft
// persistence block.
func ArticleEditShell(view *viewmodel.Article) templ.Component {
	if view == nil {
		return templates.ArticleEditShell(viewmodel.Article{})
	}
	return templates.ArticleEditShell(*view)
}

// ResearchPickerView wraps the bare picker templ with the supplied
// viewmodel. Slice 1 ships the page shell only — search-results swap
// (slice 2) and recents persistence (slice 3) land as follow-up slices.
func ResearchPickerView(view viewmodel.ResearchPickerView) templ.Component {
	return templates.ResearchPickerView(view)
}

// ResearchPickerSearchResults wraps the results-only fragment returned
// by the live htmx search (issue #378 slice 2). Renders into
// #panel.research.picker.results so the htmx swap replaces just that
// panel — the rest of the picker chrome (search input, Continue
// shortcut, Recent list) is unaffected.
func ResearchPickerSearchResults(view viewmodel.ResearchPickerView) templ.Component {
	return templates.ResearchPickerSearchResults(view)
}

// InventoryView wraps the archive-inventory page (issue #491).
// Carries the full Local Archive rollup (Person Record subtypes +
// Event Records + Articles + Tags) at a basic level than the
// per-attribute analytics on the Insights page. Each card on
// the page drilldowns into the matching listing page (browse /
// events / articles / tags).
func InventoryView(view viewmodel.InventoryView) templ.Component {
	return templates.InventoryView(view)
}

// AboutView wraps the templ AboutView so the appshell handler
// does not import internal/templates directly (the same
// indirection every page-level view uses).
func AboutView(view viewmodel.AboutView) templ.Component {
	return templates.AboutView(view)
}

// SettingsAppearanceView wraps the appearance sub-page templ.
func SettingsAppearanceView(currentTheme, currentExportSurface string) templ.Component {
	return templates.SettingsAppearancePage(currentTheme, currentExportSurface)
}

// SettingsUpdatesView wraps the updates sub-page templ.
func SettingsUpdatesView(state update.SettingsState) templ.Component {
	return templates.SettingsUpdatesPage(viewmodel.UpdateSettingsFromDomain(state))
}

// SettingsMaintenanceView wraps the maintenance sub-page templ.
func SettingsMaintenanceView() templ.Component {
	return templates.SettingsMaintenancePage()
}

// SettingsDataView wraps the data sub-page templ.
func SettingsDataView(confirmationWord string) templ.Component {
	return templates.SettingsDataPage(confirmationWord)
}

// SettingsDiagnosticsView wraps the diagnostics sub-page templ.
func SettingsDiagnosticsView(debugEnabled bool) templ.Component {
	return templates.SettingsDiagnosticsPage(debugEnabled)
}

// SettingsConfigView wraps the config sub-page templ (#638).
func SettingsConfigView(cfg viewmodel.ConfigView) templ.Component {
	return templates.SettingsConfigPage(cfg)
}

// ResearchPickerRecent wraps the recents-only fragment returned by
// the /research/recent endpoint (issue #378 slice 3, option C1). JS
// reads localStorage dixiedata.research.recents, fetches the
// fragment, and swaps #panel.research.picker.recent in place.
func ResearchPickerRecent(view viewmodel.ResearchPickerView) templ.Component {
	return templates.ResearchPickerRecent(view)
}
