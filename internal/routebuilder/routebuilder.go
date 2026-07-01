// Package routebuilder provides typed URL builders for every route the
// app registers in internal/appshell/routes.go.
//
// Templates reference URLs in hx-get, hx-post, href, and form action
// attributes. Before this package existed, templates wrote those URLs as
// bare string literals (e.g. `hx-get="/jobs/active"`), which made route
// renames silently break templates — exactly the wrong-selector bug class
// the Stabilization Sprint set out to remove.
//
// Templates should call builders via templ.SafeURL, e.g.
//
//	<a href={ templ.SafeURL(routebuilder.JobStatus(jobID)) }>...
//	<div hx-get={ templ.SafeURL(routebuilder.BrowseResults()) }>...
//
// Each builder produces the canonical URL the corresponding handler
// registers in setupRoutes(). When a route moves, only this file (and
// routes.go) need to change.
//
// This package is importable from both internal/appshell (where routes
// are registered) and internal/templates (where they're referenced) —
// it has no internal dependencies, which is the only reason the
// appshell -> templates import cycle doesn't close on itself.
package routebuilder

import (
	"fmt"
	"net/url"
	"strings"
)

// ActiveJobs returns the URL for the active-jobs polling endpoint that
// the layout uses to surface background-job progress in the top
// progress bar. Registered as GET /jobs/active in routes.go.
func ActiveJobs() string {
	return "/jobs/active"
}

// JobStatus returns the URL for the job-status fragment polling
// endpoint. Registered as GET /jobs/{id}/status in routes.go.
func JobStatus(jobID string) string {
	return "/jobs/" + url.PathEscape(strings.TrimSpace(jobID)) + "/status"
}

// JobStatusSlot returns the URL for the job-status slot fragment.
// Registered as GET /jobs/{id}/status?slot=1 in routes.go.
func JobStatusSlot(jobID string) string {
	return JobStatus(jobID) + "?slot=1"
}

// JobLog returns the URL for downloading a job's optional companion
// log file (memorial import error log, etc.). The endpoint serves
// the file from job.Result.LogPath with a Content-Disposition
// attachment header so the browser saves rather than renders.
// Registered as GET /jobs/{id}/log in jobs_handlers.go via the
// handleJobStatus sub-router switch.
func JobLog(jobID string) string {
	return "/jobs/" + url.PathEscape(strings.TrimSpace(jobID)) + "/log"
}

// Anniversary returns the URL for the anniversary list endpoint for a
// given calendar month/day. Registered as GET /anniversary/{m}/{d} in
// routes.go.
func Anniversary(month, day int) string {
	return fmt.Sprintf("/anniversary/%d/%d", month, day)
}

// AnniversaryEdit returns the URL for editing a single anniversary item
// (calendar entry) on a given month/day. Registered as
// GET /anniversary/{m}/{d}?edit={id} in routes.go.
func AnniversaryEdit(month, day int, id int64) string {
	return fmt.Sprintf("/anniversary/%d/%d?edit=%d", month, day, id)
}

// AnniversaryItemDelete returns the URL for deleting a single calendar
// item. Registered as DELETE /anniversary/{m}/{d}/items/{id}.
func AnniversaryItemDelete(month, day int, id int64) string {
	return fmt.Sprintf("/anniversary/%d/%d/items/%d", month, day, id)
}

// AnniversaryItemUpdate returns the URL for updating a calendar item.
// Registered as PUT /anniversary/{m}/{d}/items/{id}.
func AnniversaryItemUpdate(month, day int, id int64) string {
	return fmt.Sprintf("/anniversary/%d/%d/items/%d", month, day, id)
}

// AnniversaryItemCreate returns the URL for adding a calendar item.
// Registered as POST /anniversary/{m}/{d}/items.
func AnniversaryItemCreate(month, day int) string {
	return fmt.Sprintf("/anniversary/%d/%d/items", month, day)
}

// FeedbackSubmit returns the URL for posting feedback from the floating
// feedback modal. Registered as POST /feedback/submit in routes.go.
func FeedbackSubmit() string {
	return "/feedback/submit"
}

// DebugConsole returns the URL for the runtime log console panel.
// Registered as GET /debug/console in routes.go. Distinct from the
// developer overlay killed in PR #0; this is the live log viewer that
// users open from Settings → Debug.
func DebugConsole() string {
	return "/debug/console"
}

// BrowseResults returns the URL for the browse-results fragment
// endpoint. Registered as GET /browse/results in routes.go. State is
// intentionally not encoded here: callers pass query params via form
// fields or via hx-vals; the handler reads them from the request.
func BrowseResults() string {
	return "/browse/results"
}

// SoldierSearch returns the URL for the soldier search endpoint.
// Registered as GET /soldiers/search in routes.go. Pass browse=true to
// render the browse variant; otherwise the search variant is returned.
func SoldierSearch(browse bool) string {
	if browse {
		return "/soldiers/search?browse=1"
	}
	return "/soldiers/search"
}

// SoldierSearchAdvanced returns the URL for the advanced soldier
// search form. Registered as GET /soldiers/search/advanced in
// routes.go.
func SoldierSearchAdvanced() string {
	return "/soldiers/search/advanced"
}

// SoldierScrapeFindAGrave returns the URL for the Find a Grave scraper
// form. Registered as POST /soldiers/scrape-findagrave in routes.go.
func SoldierScrapeFindAGrave() string {
	return "/soldiers/scrape-findagrave"
}

// SoldierCreate returns the URL for the new-soldier POST endpoint.
// Registered as POST /soldiers in routes.go.
func SoldierCreate() string {
	return "/soldiers"
}

// SoldierPDF returns the URL for the per-soldier PDF export. Registered
// as POST /soldiers/{id}/pdf in routes.go.
func SoldierPDF(id int64) string {
	return fmt.Sprintf("/soldiers/%d/pdf", id)
}

// SoldierReviewFlag returns the URL for flagging a soldier for review
// queue. Registered as POST /soldiers/{id}/review/flag in routes.go.
func SoldierReviewFlag(id int64) string {
	return fmt.Sprintf("/soldiers/%d/review/flag", id)
}

// SoldierImagesDownload returns the URL for downloading a soldier's
// images. Registered as POST /soldiers/{id}/images/download in
// routes.go.
func SoldierImagesDownload(id int64) string {
	return fmt.Sprintf("/soldiers/%d/images/download", id)
}

// SoldierImagesPrimary returns the URL for setting a soldier image
// as primary. Registered as POST /soldiers/{id}/images/primary/{imgID}
// in routes.go.
func SoldierImagesPrimary(soldierID, imageID int64) string {
	return fmt.Sprintf("/soldiers/%d/images/primary/%d", soldierID, imageID)
}

// ResearchLogTasksCreate returns the URL for creating a research-log
// task. Registered as POST /soldiers/{id}/research-log/tasks in
// routes.go.
func ResearchLogTasksCreate(soldierID int64) string {
	return fmt.Sprintf("/soldiers/%d/research-log/tasks", soldierID)
}

// SoldierCamaraderie returns the URL for the camaraderie graph page.
// Registered as GET /soldiers/{id}/camaraderie in routes.go.
func SoldierCamaraderie(id int64) string {
	return fmt.Sprintf("/soldiers/%d/camaraderie", id)
}

// SoldierConflictLedger returns the URL for the merge conflict
// ledger page. Registered as GET /soldiers/{id}/conflict-ledger in
// routes.go.
func SoldierConflictLedger(id int64) string {
	return fmt.Sprintf("/soldiers/%d/conflict-ledger", id)
}

// SoldierTimeline returns the URL for the service timeline page.
// Registered as GET /soldiers/{id}/timeline in routes.go.
func SoldierTimeline(id int64) string {
	return fmt.Sprintf("/soldiers/%d/timeline", id)
}

// SoldierEdit returns the URL for the edit-soldier form page.
// Registered as GET /soldiers/{id}/edit in routes.go.
func SoldierEdit(id int64) string {
	return fmt.Sprintf("/soldiers/%d/edit", id)
}

// SoldierDetail returns the URL for the soldier detail page.
// Registered as GET /soldiers/{id} in routes.go (matched by the
// catch-all /soldiers/* handler that dispatches on the {id}
// suffix).
func SoldierDetail(id int64) string {
	return fmt.Sprintf("/soldiers/%d", id)
}

// SettingsDebugMode returns the URL for toggling debug mode. Registered
// as POST /settings/debug-mode in routes.go.
func SettingsDebugMode() string {
	return "/settings/debug-mode"
}

// SettingsInitialize returns the URL for initializing data (factory
// reset). Registered as POST /settings/initialize in routes.go.
func SettingsInitialize() string {
	return "/settings/initialize"
}

// SettingsUpdateSource returns the URL for saving the custom update
// feed URL. Registered as POST /settings/updates/source in routes.go.
func SettingsUpdateSource() string {
	return "/settings/updates/source"
}

// SettingsUpdateCheck returns the URL for the "check for updates"
// button. Registered as POST /settings/updates/check in routes.go.
func SettingsUpdateCheck() string {
	return "/settings/updates/check"
}

// SettingsUpdateApply returns the URL for the "apply update" button.
// Registered as POST /settings/updates/apply in routes.go.
func SettingsUpdateApply() string {
	return "/settings/updates/apply"
}

// SettingsImagesOrphansScan returns the URL for the orphaned-image
// scan button. Registered as POST /settings/images/orphans/scan in
// routes.go.
func SettingsImagesOrphansScan() string {
	return "/settings/images/orphans/scan"
}

// SettingsImagesOrphansCleanup returns the URL for the orphan cleanup
// button. Registered as POST /settings/images/orphans/cleanup in
// routes.go.
func SettingsImagesOrphansCleanup() string {
	return "/settings/images/orphans/cleanup"
}

// SettingsQualityScan returns the URL for the data-quality scan form.
// Registered as POST /settings/quality/scan in routes.go.
func SettingsQualityScan() string {
	return "/settings/quality/scan"
}

// SettingsQualityApply returns the URL for the data-quality apply
// form. Registered as POST /settings/quality/apply in routes.go.
func SettingsQualityApply() string {
	return "/settings/quality/apply"
}

// ReviewQueueBulk returns the URL for the bulk-action form on the
// review queue page. Registered as POST /review-queue/bulk in
// routes.go.
func ReviewQueueBulk() string {
	return "/review-queue/bulk"
}

// ResearchCollectionsCreate returns the URL for the new-collection
// form. Registered as POST /research-collections in routes.go.
func ResearchCollectionsCreate() string {
	return "/research-collections"
}

// ResearchCollectionAdd returns the URL for adding a soldier to a
// research collection. Registered as POST /research-collections/{id}/add
// in routes.go.
func ResearchCollectionAdd(id int64) string {
	return fmt.Sprintf("/research-collections/%d/add", id)
}

// CalendarReportPDF returns the URL for the calendar-month PDF report
// endpoint. Registered as POST /calendar/{m}/report/pdf in routes.go.
func CalendarReportPDF(month int) string {
	return fmt.Sprintf("/calendar/%d/report/pdf", month)
}

// InsightsReportPDF returns the URL for the insights PDF export.
// Registered as POST /insights/report/pdf in routes.go.
func InsightsReportPDF() string {
	return "/insights/report/pdf"
}

// ExportBackup returns the URL for the Backup Archive export button.
// Registered as POST /export/backup in routes.go.
func ExportBackup() string {
	return "/export/backup"
}

// ExportDatabasePDFAsync returns the URL for the async database PDF
// export. Registered as POST /export/database-pdf?async=1 in
// routes.go.
func ExportDatabasePDFAsync() string {
	return "/export/database-pdf?async=1"
}

// ExportPreview returns the URL for the print-config live preview
// endpoint. Returns a small HTML fragment describing what the
// current modal selection would generate. Registered as POST
// /export/preview in routes.go.
func ExportPreview() string {
	return "/export/preview"
}

// ExportTemplatesList returns the URL for the saved-templates
// listing endpoint. Registered as GET /export/templates in
// routes.go.
func ExportTemplatesList() string {
	return "/export/templates"
}

// ExportTemplateSave returns the URL for the saved-templates
// creation endpoint. Registered as POST /export/templates in
// routes.go.
func ExportTemplateSave() string {
	return "/export/templates"
}

// ExportTemplateDelete returns the URL for deleting one saved
// template. Caller appends the {id} segment. Registered as DELETE
// /export/templates/{id} in routes.go.
func ExportTemplateDelete(id int64) string {
	return fmt.Sprintf("/export/templates/%d", id)
}

// ExportTemplateApply returns the URL for loading one saved
// template back into the modal. Caller appends the {id} segment.
// Registered as POST /export/templates/{id}/apply in routes.go.
func ExportTemplateApply(id int64) string {
	return fmt.Sprintf("/export/templates/%d/apply", id)
}

// ExportTemplateUpdate returns the URL for the Save Changes
// form target on a loaded template (issue #186). Caller appends
// the {id} segment. Registered as PATCH /export/templates/{id}
// in routes.go; the handler also accepts POST (HTML forms do
// not natively issue PATCH).
func ExportTemplateUpdate(id int64) string {
	return fmt.Sprintf("/export/templates/%d", id)
}

// LayoutReviewCount returns the URL for the layout's review-queue
// badge fragment. Registered as GET /layout/review-count in
// routes.go. Polled by the layout at 30s cadence via htmx.
func LayoutReviewCount() string {
	return "/layout/review-count"
}

// TagsPage returns the URL for the /tags management page.
// Registered as GET /tags in routes.go.
func TagsPage() string {
	return "/tags"
}

// TagDetail returns the URL for a single tag's detail / member
// list. Registered as GET /tags/{id} in routes.go.
func TagDetail(id int64) string {
	return fmt.Sprintf("/tags/%d", id)
}

// TagRename returns the URL for the rename form target. Caller
// appends the {id} segment. Registered as POST /tags/{id}/rename
// in routes.go.
func TagRename(id int64) string {
	return fmt.Sprintf("/tags/%d/rename", id)
}

// TagMerge returns the URL for the merge form target. Caller
// appends the {id} segment. Registered as POST /tags/{id}/merge
// in routes.go.
func TagMerge(id int64) string {
	return fmt.Sprintf("/tags/%d/merge", id)
}

// TagDelete returns the URL for the DELETE form target (used
// with `data-dixie-submit` and a hidden _method=DELETE override
// because HTML forms do not natively issue DELETE). Caller
// appends the {id} segment. Registered as DELETE /tags/{id} in
// routes.go.
func TagDelete(id int64) string {
	return fmt.Sprintf("/tags/%d", id)
}

// SoldierTagAutocomplete returns the URL for the picker
// autocomplete fragment. Registered as GET
// /soldiers/{id}/tags?autocomplete={q} in routes.go.
func SoldierTagAutocomplete(id int64) string {
	return fmt.Sprintf("/soldiers/%d/tags", id)
}

// SoldierTagAttach returns the URL for the attach form. Caller
// appends the {id} segment. Registered as POST /soldiers/{id}/tags
// in routes.go.
func SoldierTagAttach(id int64) string {
	return fmt.Sprintf("/soldiers/%d/tags", id)
}

// SoldierTagDetach returns the URL for the detach form. Caller
// appends the {id} and {tagId} segments. Registered as POST
// /soldiers/{id}/tags/{tagId} in routes.go.
func SoldierTagDetach(id, tagId int64) string {
	return fmt.Sprintf("/soldiers/%d/tags/%d", id, tagId)
}

// BrowseBulkTag returns the URL for the bulk-tag form target.
// Registered as POST /browse/bulk-tag in routes.go.
func BrowseBulkTag() string {
	return "/browse/bulk-tag"
}

// ShareExportOptions returns the URL for the include-tags
// toggle on the share page. Registered as PATCH
// /share/export-options in routes.go.
func ShareExportOptions() string {
	return "/share/export-options"
}

// SharePage (issue #182) returns the URL for the Share
// landing page. Registered as GET /share in routes.go.
func SharePage() string {
	return "/share"
}

// ShareQueueModal returns the URL for the Share Build modal
// fragment. Registered as GET /share/queue/modal in routes.go.
func ShareQueueModal() string {
	return "/share/queue/modal"
}

// ShareQueuePreview returns the URL for the live preview
// fragment endpoint. Registered as POST /share/queue/preview
// in routes.go.
func ShareQueuePreview() string {
	return "/share/queue/preview"
}

// ShareQueueClear returns the URL for the Clear Queue action.
// Registered as POST /share/queue/clear in routes.go.
func ShareQueueClear() string {
	return "/share/queue/clear"
}

// ShareQueuePage (issue #193) returns the URL for the
// management page reachable from the layout nav. The caller
// appends the ?ids=1,2,3 query string the client populates
// from localStorage on page load.
func ShareQueuePage() string {
	return "/share/queue"
}

// ShareQueuePresets (issue #192) returns the URL for the
// saved-presets endpoints. The {id} is appended by the caller
// via fmt.Sprintf since chi needs the literal segment to win
// over the wildcard /share/queue/presets/{id:[0-9]+} route.
func ShareQueuePresets() string {
	return "/share/queue/presets"
}

// ShareQueuePresetDelete (issue #192) returns the URL for
// deleting a saved preset. Caller substitutes {id}.
func ShareQueuePresetDelete(id int64) string {
	return fmt.Sprintf("/share/queue/presets/%d", id)
}

// ShareQueuePresetApply (issue #192) returns the URL for
// loading a saved preset into the modal's localStorage queue.
// Caller substitutes {id}.
func ShareQueuePresetApply(id int64) string {
	return fmt.Sprintf("/share/queue/presets/%d/apply", id)
}

// ExportSharedArchiveSubset returns the URL for the subset
// export entrypoint. Caller appends the form's selected_ids
// fields; the handler is the existing /export/shared-archive
// with a `?subset=1` discriminator.
func ExportSharedArchiveSubset() string {
	return "/export/shared-archive?subset=1"
}

// GoogleCalendarPreferencesSave returns the URL for the Google
// Calendar preferences form. Registered as POST
// /integrations/google/calendar/preferences/save in routes.go.
func GoogleCalendarPreferencesSave() string {
	return "/integrations/google/calendar/preferences/save"
}