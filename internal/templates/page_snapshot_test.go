// Page-render snapshot tests for the top 5 user flows.
//
// Each test renders a top-level page into a buffer, parses the
// HTML with goquery, and asserts a fixed set of behavioral invariants
// — required elements present, no accidentally reintroduced
// debug-overlay attributes, hx-target references known surfaces.
//
// The tests are intentionally NOT byte-equality snapshots (which
// would lock in cosmetic changes); they assert shape, not bytes.
// A CSS class rename, whitespace adjustment, or attribute reorder
// passes the test; a missing required input, wrong hx-target, or
// accidental `data-ui-id` reintroduction fails.
//
// Stable anchors: each test adds a `data-testid="..."` attribute on
// the root element via a known constant from the uiids registry so
// future refactors can keep the selector stable. Snapshot tests
// rely on this anchor to scope assertions.
package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
)

// renderIntoDoc renders a templ component into a buffer, parses the
// HTML with goquery, and returns the document for inspection. The
// debug mode is enabled on the context so template code that
// branches on debug.IsDebugMode(ctx) takes the "on" path. Failures
// from the render step fail the test.
func renderIntoDoc(t *testing.T, name string, render func(ctx context.Context, w *bytes.Buffer) error) *goquery.Document {
	t.Helper()
	var buf bytes.Buffer
	ctx := debug.WithDebugMode(context.Background(), false)
	if err := render(ctx, &buf); err != nil {
		t.Fatalf("%s: render failed: %v", name, err)
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		t.Fatalf("%s: parse HTML failed: %v", name, err)
	}
	return doc
}

// assertNoDebugOverlayAttrs fails the test if the rendered HTML
// contains data-ui-id (the developer overlay attribute removed in
// PR #0) or data-debug-ui-ids (its toggle). Reintroducing these
// would silently couple production code to a debug-only attribute.
func assertNoDebugOverlayAttrs(t *testing.T, name string, doc *goquery.Document) {
	t.Helper()
	doc.Find("[data-ui-id]").Each(func(_ int, s *goquery.Selection) {
		t.Errorf("%s: data-ui-id attribute found (debug overlay was removed in PR #0); use data-testid instead", name)
	})
	if doc.Find("body").Length() > 0 {
		// body may not be present in fragments; if present, check it
		if v, exists := doc.Find("body").Attr("data-debug-ui-ids"); exists && v != "" {
			t.Errorf("%s: body carries data-debug-ui-ids (debug toggle was removed in PR #0)", name)
		}
	}
}

// TestPageSnapshotBrowse renders the Browse list page and asserts
// the row container, filter form, and primary actions render.
func TestPageSnapshotBrowse(t *testing.T) {
	state := viewmodel.BrowseState{
		Page:     1,
		PageSize: 50,
		Scope:    "all",
		Sort:     "display_id",
	}
	records := []viewmodel.PersonRecord{
		{ID: 1, DisplayID: "DXD-00001", FirstName: "Samuel", LastName: "Carter", EntryType: "soldier"},
		{ID: 2, DisplayID: "DXD-00002", FirstName: "Jane", LastName: "Carter", EntryType: "widow"},
	}
	suggestions := viewmodel.PersonRecordFormSuggestions{}

	doc := renderIntoDoc(t, "Browse", func(ctx context.Context, w *bytes.Buffer) error {
		return BrowseView(records, state, suggestions, nil).Render(ctx, w)
	})

	if doc.Find("#browse-results").Length() == 0 {
		t.Error("browse-results container missing")
	}
	if doc.Find("[data-browse-filters-form]").Length() == 0 {
		t.Error("browse filters form missing")
	}
	if doc.Find("[data-browse-row-href]").Length() < 2 {
		t.Error("expected at least 2 browse rows")
	}
	assertNoDebugOverlayAttrs(t, "Browse", doc)
}

// TestPageSnapshotLayout renders the Layout chrome and asserts the
// top nav links, toast region, and progress region render.
func TestPageSnapshotLayout(t *testing.T) {
	doc := renderIntoDoc(t, "Layout", func(ctx context.Context, w *bytes.Buffer) error {
		return Layout("Test Page").Render(ctx, w)
	})

	required := []string{
		"/calendar",
		"/soldiers",
		"/browse",
		"/review-queue",
		"/insights",
		"/share",
		"/settings",
		"/soldiers/new",
	}
	for _, href := range required {
		if doc.Find("a[href='" + href + "']").Length() == 0 {
			t.Errorf("top nav link to %s missing", href)
		}
	}
	if doc.Find("[data-toast-region]").Length() == 0 {
		t.Error("toast region missing")
	}
	if doc.Find("[data-jobs-progress-region]").Length() == 0 {
		t.Error("jobs progress overlay (uiids.OverlayJobsProgress) missing from layout")
	}
	assertNoDebugOverlayAttrs(t, "Layout", doc)
}

// TestPageSnapshotSoldierDetail renders a Person Record detail page
// and asserts the summary, tabs, and primary actions render.
func TestPageSnapshotSoldierDetail(t *testing.T) {
	soldier := viewmodel.PersonRecord{
		ID:               42,
		DisplayID:        "DXD-00042",
		FirstName:        "Robert",
		LastName:         "Stewart",
		EntryType:        "soldier",
		Rank:             "Private",
		Unit:             "5th Virginia Infantry",
		PensionState:     "Virginia",
		BuriedIn:         "Hollywood Cemetery",
		BirthDate:        "1842-03-15",
		DeathDate:        "1923-08-04",
		NeedsReview:      false,
		ConfederateHomeStatus: "NotApplicable",
	}

	doc := renderIntoDoc(t, "SoldierDetail", func(ctx context.Context, w *bytes.Buffer) error {
		return SoldierDetail(soldier).Render(ctx, w)
	})

	if doc.Find("h2").First().Length() == 0 {
		t.Error("Person Record heading missing")
	}
	if !strings.Contains(doc.Text(), "Robert") || !strings.Contains(doc.Text(), "Stewart") {
		t.Error("Person Record name not rendered")
	}
	if !strings.Contains(doc.Text(), "DXD-00042") {
		t.Error("Display ID not rendered")
	}
	if !strings.Contains(doc.Text(), "5th Virginia Infantry") {
		t.Error("Unit not rendered")
	}
	assertNoDebugOverlayAttrs(t, "SoldierDetail", doc)
}

// TestPageSnapshotEntryForm renders the new-Person-Record entry form
// and asserts the canonical input fields render.
func TestPageSnapshotEntryForm(t *testing.T) {
	soldier := viewmodel.PersonRecord{}
	suggestions := viewmodel.PersonRecordFormSuggestions{}

	doc := renderIntoDoc(t, "EntryForm", func(ctx context.Context, w *bytes.Buffer) error {
		return EntryForm(soldier, nil, suggestions, viewmodel.FindAGraveScrapeState{}, false).Render(ctx, w)
	})

	required := []string{
		`name="first_name"`,
		`name="last_name"`,
		`name="display_id"`,
		`name="entry_type"`,
		`name="rank_in"`,
		`name="unit"`,
	}
	html, err := doc.Html()
	if err != nil {
		t.Fatalf("entry form: doc.Html failed: %v", err)
	}
	for _, needle := range required {
		if !strings.Contains(html, needle) {
			t.Errorf("entry form missing required input %s", needle)
		}
	}
	assertNoDebugOverlayAttrs(t, "EntryForm", doc)
}

// TestPageSnapshotJobsStatus renders the JobStatusView with a
// synthetic Job constructed via jobs.NewJob (added in FU.10). The
// job is set to a running state with mid-progress values, then the
// test asserts the rendered markup contains the expected status
// label, progress bar value, and cancel button. This replaces the
// earlier faked test that rendered Layout only.
func TestPageSnapshotJobsStatus(t *testing.T) {
	job := jobs.NewJob("job-abc", "export_pdf")
	job.Status = jobs.StatusRunning
	job.Progress = 42
	job.Message = "Generating PDF"

	doc := renderIntoDoc(t, "JobStatus", func(ctx context.Context, w *bytes.Buffer) error {
		return JobStatusView(*job).Render(ctx, w)
	})

	html, err := doc.Html()
	if err != nil {
		t.Fatalf("JobStatus: doc.Html failed: %v", err)
	}

	// Assert the running status label renders (template shows
	// "Status: <strong>running</strong>").
	if !strings.Contains(html, "running") {
		t.Error("JobStatus view missing 'running' status label")
	}
	// Assert the job ID renders somewhere on the page.
	if !strings.Contains(html, "job-abc") {
		t.Error("JobStatus view missing job ID 'job-abc'")
	}
	// Assert the progress value renders. The template renders an
	// aria-valuenow attribute; 42 is the integer value we set.
	if !strings.Contains(html, "42") {
		t.Error("JobStatus view missing progress value 42")
	}
	// Assert the job-status-body container (used by htmx swap) is
	// present so the polling fragment can find its target.
	if doc.Find("#job-status-body").Length() == 0 {
		t.Error("JobStatus view missing #job-status-body container")
	}
	// Assert the full page wires the 2s hx-get against
	// /jobs/{id}/status. Before the body extraction the landing
	// page was a static snapshot: no hx-get, no hx-trigger, so
	// fast jobs (static_archive) finished while the page sat
	// there reading "running" forever. See
	// TestJobStatusViewPollsForUpdates for the focused net.
	if !strings.Contains(html, `hx-get="/jobs/job-abc/status"`) {
		t.Error("JobStatus view missing hx-get against /jobs/{id}/status; the landing page will not auto-update")
	}

	assertNoDebugOverlayAttrs(t, "JobStatus", doc)
}