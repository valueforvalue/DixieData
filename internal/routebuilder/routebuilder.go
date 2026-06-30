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

// GoogleCalendarPreferencesSave returns the URL for the Google
// Calendar preferences form. Registered as POST
// /integrations/google/calendar/preferences/save in routes.go.
func GoogleCalendarPreferencesSave() string {
	return "/integrations/google/calendar/preferences/save"
}