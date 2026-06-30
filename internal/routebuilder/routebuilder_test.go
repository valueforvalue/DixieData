package routebuilder

import (
	"net/url"
	"testing"
)

func TestActiveJobs(t *testing.T) {
	if ActiveJobs() != "/jobs/active" {
		t.Fatalf("ActiveJobs = %q", ActiveJobs())
	}
}

func TestJobStatus(t *testing.T) {
	got := JobStatus("abc-123")
	want := "/jobs/abc-123/status"
	if got != want {
		t.Fatalf("JobStatus = %q, want %q", got, want)
	}
}

func TestJobStatusEscapesID(t *testing.T) {
	got := JobStatus("id with/slash")
	want := "/jobs/id%20with%2Fslash/status"
	if got != want {
		t.Fatalf("JobStatus = %q, want %q", got, want)
	}
}

func TestJobStatusTrimsWhitespace(t *testing.T) {
	got := JobStatus("  spaced  ")
	want := "/jobs/spaced/status"
	if got != want {
		t.Fatalf("JobStatus = %q, want %q", got, want)
	}
}

func TestJobStatusSlot(t *testing.T) {
	got := JobStatusSlot("xyz")
	want := "/jobs/xyz/status?slot=1"
	if got != want {
		t.Fatalf("JobStatusSlot = %q, want %q", got, want)
	}
}

func TestAnniversary(t *testing.T) {
	got := Anniversary(7, 4)
	want := "/anniversary/7/4"
	if got != want {
		t.Fatalf("Anniversary = %q, want %q", got, want)
	}
}

func TestAnniversaryEdit(t *testing.T) {
	got := AnniversaryEdit(12, 25, int64(42))
	want := "/anniversary/12/25?edit=42"
	if got != want {
		t.Fatalf("AnniversaryEdit = %q, want %q", got, want)
	}
}

func TestAnniversaryItemDelete(t *testing.T) {
	got := AnniversaryItemDelete(7, 4, int64(99))
	want := "/anniversary/7/4/items/99"
	if got != want {
		t.Fatalf("AnniversaryItemDelete = %q, want %q", got, want)
	}
}

func TestAnniversaryItemUpdate(t *testing.T) {
	got := AnniversaryItemUpdate(7, 4, int64(99))
	want := "/anniversary/7/4/items/99"
	if got != want {
		t.Fatalf("AnniversaryItemUpdate = %q, want %q", got, want)
	}
}

func TestAnniversaryItemCreate(t *testing.T) {
	got := AnniversaryItemCreate(7, 4)
	want := "/anniversary/7/4/items"
	if got != want {
		t.Fatalf("AnniversaryItemCreate = %q, want %q", got, want)
	}
}

func TestFeedbackSubmit(t *testing.T) {
	if FeedbackSubmit() != "/feedback/submit" {
		t.Fatalf("FeedbackSubmit = %q", FeedbackSubmit())
	}
}

func TestDebugConsole(t *testing.T) {
	if DebugConsole() != "/debug/console" {
		t.Fatalf("DebugConsole = %q", DebugConsole())
	}
}

func TestBrowseResults(t *testing.T) {
	if BrowseResults() != "/browse/results" {
		t.Fatalf("BrowseResults = %q", BrowseResults())
	}
}

func TestSoldierSearchBrowseVariant(t *testing.T) {
	if SoldierSearch(true) != "/soldiers/search?browse=1" {
		t.Fatalf("SoldierSearch(true) = %q", SoldierSearch(true))
	}
}

func TestSoldierSearchDefault(t *testing.T) {
	if SoldierSearch(false) != "/soldiers/search" {
		t.Fatalf("SoldierSearch(false) = %q", SoldierSearch(false))
	}
}

func TestJobStatusOutputIsValidURLPath(t *testing.T) {
	got := JobStatus("valid-id_123")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("JobStatus produced unparseable URL %q: %v", got, err)
	}
	if parsed.Path != "/jobs/valid-id_123/status" {
		t.Fatalf("parsed path = %q, want /jobs/valid-id_123/status", parsed.Path)
	}
}
func TestSoldierSearchAdvanced(t *testing.T) {
	if SoldierSearchAdvanced() != "/soldiers/search/advanced" {
		t.Fatalf("SoldierSearchAdvanced = %q", SoldierSearchAdvanced())
	}
}

func TestSoldierScrapeFindAGrave(t *testing.T) {
	if SoldierScrapeFindAGrave() != "/soldiers/scrape-findagrave" {
		t.Fatalf("SoldierScrapeFindAGrave = %q", SoldierScrapeFindAGrave())
	}
}

func TestSoldierCreate(t *testing.T) {
	if SoldierCreate() != "/soldiers" {
		t.Fatalf("SoldierCreate = %q", SoldierCreate())
	}
}

func TestSoldierPDF(t *testing.T) {
	got := SoldierPDF(42)
	want := "/soldiers/42/pdf"
	if got != want {
		t.Fatalf("SoldierPDF = %q, want %q", got, want)
	}
}

func TestSoldierReviewFlag(t *testing.T) {
	got := SoldierReviewFlag(99)
	want := "/soldiers/99/review/flag"
	if got != want {
		t.Fatalf("SoldierReviewFlag = %q, want %q", got, want)
	}
}

func TestSoldierImagesDownload(t *testing.T) {
	got := SoldierImagesDownload(7)
	want := "/soldiers/7/images/download"
	if got != want {
		t.Fatalf("SoldierImagesDownload = %q, want %q", got, want)
	}
}

func TestSoldierImagesPrimary(t *testing.T) {
	got := SoldierImagesPrimary(7, 13)
	want := "/soldiers/7/images/primary/13"
	if got != want {
		t.Fatalf("SoldierImagesPrimary = %q, want %q", got, want)
	}
}

func TestResearchLogTasksCreate(t *testing.T) {
	got := ResearchLogTasksCreate(11)
	want := "/soldiers/11/research-log/tasks"
	if got != want {
		t.Fatalf("ResearchLogTasksCreate = %q, want %q", got, want)
	}
}

func TestSoldierCamaraderie(t *testing.T) {
	got := SoldierCamaraderie(5)
	want := "/soldiers/5/camaraderie"
	if got != want {
		t.Fatalf("SoldierCamaraderie = %q, want %q", got, want)
	}
}

func TestSoldierConflictLedger(t *testing.T) {
	got := SoldierConflictLedger(5)
	want := "/soldiers/5/conflict-ledger"
	if got != want {
		t.Fatalf("SoldierConflictLedger = %q, want %q", got, want)
	}
}

func TestSoldierTimeline(t *testing.T) {
	got := SoldierTimeline(5)
	want := "/soldiers/5/timeline"
	if got != want {
		t.Fatalf("SoldierTimeline = %q, want %q", got, want)
	}
}

func TestSoldierEdit(t *testing.T) {
	got := SoldierEdit(5)
	want := "/soldiers/5/edit"
	if got != want {
		t.Fatalf("SoldierEdit = %q, want %q", got, want)
	}
}

func TestSettingsDebugMode(t *testing.T) {
	if SettingsDebugMode() != "/settings/debug-mode" {
		t.Fatalf("SettingsDebugMode = %q", SettingsDebugMode())
	}
}

func TestSettingsInitialize(t *testing.T) {
	if SettingsInitialize() != "/settings/initialize" {
		t.Fatalf("SettingsInitialize = %q", SettingsInitialize())
	}
}

func TestSettingsUpdateSource(t *testing.T) {
	if SettingsUpdateSource() != "/settings/updates/source" {
		t.Fatalf("SettingsUpdateSource = %q", SettingsUpdateSource())
	}
}

func TestSettingsUpdateCheck(t *testing.T) {
	if SettingsUpdateCheck() != "/settings/updates/check" {
		t.Fatalf("SettingsUpdateCheck = %q", SettingsUpdateCheck())
	}
}

func TestSettingsUpdateApply(t *testing.T) {
	if SettingsUpdateApply() != "/settings/updates/apply" {
		t.Fatalf("SettingsUpdateApply = %q", SettingsUpdateApply())
	}
}

func TestSettingsImagesOrphansScan(t *testing.T) {
	if SettingsImagesOrphansScan() != "/settings/images/orphans/scan" {
		t.Fatalf("SettingsImagesOrphansScan = %q", SettingsImagesOrphansScan())
	}
}

func TestSettingsImagesOrphansCleanup(t *testing.T) {
	if SettingsImagesOrphansCleanup() != "/settings/images/orphans/cleanup" {
		t.Fatalf("SettingsImagesOrphansCleanup = %q", SettingsImagesOrphansCleanup())
	}
}

func TestSettingsQualityScan(t *testing.T) {
	if SettingsQualityScan() != "/settings/quality/scan" {
		t.Fatalf("SettingsQualityScan = %q", SettingsQualityScan())
	}
}

func TestSettingsQualityApply(t *testing.T) {
	if SettingsQualityApply() != "/settings/quality/apply" {
		t.Fatalf("SettingsQualityApply = %q", SettingsQualityApply())
	}
}

func TestReviewQueueBulk(t *testing.T) {
	if ReviewQueueBulk() != "/review-queue/bulk" {
		t.Fatalf("ReviewQueueBulk = %q", ReviewQueueBulk())
	}
}

func TestResearchCollectionsCreate(t *testing.T) {
	if ResearchCollectionsCreate() != "/research-collections" {
		t.Fatalf("ResearchCollectionsCreate = %q", ResearchCollectionsCreate())
	}
}

func TestResearchCollectionAdd(t *testing.T) {
	got := ResearchCollectionAdd(42)
	want := "/research-collections/42/add"
	if got != want {
		t.Fatalf("ResearchCollectionAdd = %q, want %q", got, want)
	}
}

func TestCalendarReportPDF(t *testing.T) {
	got := CalendarReportPDF(7)
	want := "/calendar/7/report/pdf"
	if got != want {
		t.Fatalf("CalendarReportPDF = %q, want %q", got, want)
	}
}

func TestInsightsReportPDF(t *testing.T) {
	if InsightsReportPDF() != "/insights/report/pdf" {
		t.Fatalf("InsightsReportPDF = %q", InsightsReportPDF())
	}
}

func TestExportBackup(t *testing.T) {
	if ExportBackup() != "/export/backup" {
		t.Fatalf("ExportBackup = %q", ExportBackup())
	}
}

func TestExportDatabasePDFAsync(t *testing.T) {
	if ExportDatabasePDFAsync() != "/export/database-pdf?async=1" {
		t.Fatalf("ExportDatabasePDFAsync = %q", ExportDatabasePDFAsync())
	}
}

func TestExportPreview(t *testing.T) {
	if ExportPreview() != "/export/preview" {
		t.Fatalf("ExportPreview = %q", ExportPreview())
	}
}

func TestGoogleCalendarPreferencesSave(t *testing.T) {
	if GoogleCalendarPreferencesSave() != "/integrations/google/calendar/preferences/save" {
		t.Fatalf("GoogleCalendarPreferencesSave = %q", GoogleCalendarPreferencesSave())
	}
}
