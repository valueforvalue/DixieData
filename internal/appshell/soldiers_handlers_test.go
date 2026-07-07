// soldiers_handlers_test.go pins the soldier-side Images panel
// UIIDs declared in internal/uiids/uiids.go.
//
// Issue #392 (parallel of #390 for the Event gallery) closes
// the regression gap where the two gallery-relevant soldier
// UIIDs (PanelSoldierDetailImages + PanelSoldierFormImages)
// existed as constants but were NOT rendered as `id=` attributes
// in any templ. The smoke probe in audit/smoke_soldier_images.mjs
// drives the same shape; this test is the unit-level pin that
// fails if either wrapper is removed.
//
// The test only asserts the rendered HTML contains the canonical
// `id="panel.soldier.detail.images"` / `id="panel.soldier.form.images"`
// anchors. It does NOT touch handler code (no new routes, no
// fragment swap, no per-card Delete). Slice B (#391) owns those.
package appshell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/uiids"
)

// TestHandleSoldierByIDRendersImagesPanelAnchor pins the
// soldier-detail Images panel UIID. Mirrors
// events_handlers_test.go's
// TestHandleEventImages-panel anchor assertion (post-#390).
// Creates a soldier via the soldiers facade, GETs the detail
// page, and asserts the wrapper `id=panel.soldier.detail.images`
// appears in the rendered HTML.
func TestHandleSoldierByIDRendersImagesPanelAnchor(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "ImagesPanelAnchor")
	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d status = %d, want 200", s.ID, resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierDetailImages)
	if !strings.Contains(body, want) {
		t.Errorf(
			"soldier detail page missing #%s anchor; got %q",
			uiids.PanelSoldierDetailImages,
			bodyExtract(body, "Images", 200),
		)
	}
}

// TestHandleEditSoldierRendersFormImagesPanelAnchor pins
// the soldier-form Upload Images panel UIID. Creates a
// soldier, GETs /soldiers/{id}/edit, asserts the wrapper
// `id=panel.soldier.form.images` appears in the rendered
// HTML. The form is reachable for any persisted soldier
// (the entry_form renders the Images section unconditionally
// in edit mode per entry_form.templ line ~362).
func TestHandleEditSoldierRendersFormImagesPanelAnchor(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "FormImagesPanelAnchor")
	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID) + "/edit")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/edit: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d/edit status = %d, want 200", s.ID, resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierFormImages)
	if !strings.Contains(body, want) {
		t.Errorf(
			"soldier edit form missing #%s anchor; got %q",
			uiids.PanelSoldierFormImages,
			bodyExtract(body, "Upload Images", 200),
		)
	}
}

// TestHandleSoldierImagesFragmentGET (issue #391 Slice B.1)
// pins the new dedicated chi route
// GET /soldiers/{id}/images that returns the
// SoldierImagesListFragment (not a full page) so the
// fragment can be used as the lazy-load probe and the
// post-action swap target for the per-card Delete +
// Set-Primary forms. Mirrors
// TestHandleEventImages' shape in
// events_handlers_test.go (post-#332).
//
// Seeds two images via app.soldiers.AddImage (mirrors the
// gold_master_test.go image-seeding shape used by
// TestHandleEventImages), then asserts:
//   - GET /soldiers/{id}/images returns 200 with the
//     fragment wrapper id=panel.soldier.detail.images,
//   - body contains both seeded captions.
//   - the fragment preserves the data-image-card +
//     data-image-thumb-id attributes on each card.
func TestHandleSoldierImagesFragmentGET(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "ImagesFragmentGET")
	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, s.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll imageDir: %v", err)
	}
	firstPath := filepath.Join(imageDir, "first.png")
	secondPath := filepath.Join(imageDir, "second.png")
	if err := os.WriteFile(firstPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "first.png", filepath.Join(relativeDir, "first.png"), "First portrait"); err != nil {
		t.Fatalf("AddImage first: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "second.png", filepath.Join(relativeDir, "second.png"), "Second portrait"); err != nil {
		t.Fatalf("AddImage second: %v", err)
	}

	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID) + "/images")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/images: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET images status = %d, want 200", resp.StatusCode)
	}
	// The fragment is the inner content of the wrapper
	// <div id="panel.soldier.detail.images">; the wrapper
	// itself lives in soldier_card.templ (see Slice A) so
	// htmx's data-results-target can swap innerHTML on
	// Delete / Set-Primary without restitching the wrapper.
	// Asserting on the inner content (per-card div +
	// per-card caption) is what the swap target sees.
	if !strings.Contains(body, `data-image-card`) {
		t.Errorf("fragment missing per-card data-image-card markers; got %q", body)
	}
	if !strings.Contains(body, "First portrait") {
		t.Errorf("fragment missing first image caption; got %q", body)
	}
	if !strings.Contains(body, "Second portrait") {
		t.Errorf("fragment missing second image caption; got %q", body)
	}
	// The response must also expose the per-card selectors
	// Slice A pins (smoke_soldier_images.mjs step-03) so
	// the live probe keeps working post-migration.
	if !strings.Contains(body, `data-image-thumb-id="1"`) {
		t.Errorf("fragment missing first image data-image-thumb-id; got %q", body)
	}
	if !strings.Contains(body, `data-image-thumb-id="2"`) {
		t.Errorf("fragment missing second image data-image-thumb-id; got %q", body)
	}
	if strings.Contains(body, "No images are attached yet") {
		t.Errorf("fragment rendered empty state despite 2 seeded images; got %q", body)
	}
	// Belt-and-braces: the response must NOT be the full
	// detail page (which would be served by the catch-all
	// /soldiers/* dispatch on pre-B.1). The full page wraps
	// the fragment via templates.Layout() so it carries
	// <body> + <nav>; the fragment does not. If a future
	// regression lets the catch-all match first, the
	// response size jumps to detail-page scale.
	if strings.Contains(body, "<body") || strings.Contains(body, "<nav") {
		t.Errorf("fragment response leaked full-page layout; got %q", body[:min(2000, len(body))])
	}
}

// TestHandleSoldierImagesDeleteFragmentSwap (issue #391
// Slice B.2) pins the in-place fragment swap replacing the
// pre-B.2 X-Dixiedata-Redirect pattern. POST
// /soldiers/{id}/images/delete with one image_id must:
//   - drop that image from the DB,
//   - render the SoldierImagesListFragment (NOT set
//     X-Dixiedata-Redirect, NOT return the full page),
//   - leave the surviving card visible via the per-card
//     data-image-card / data-image-thumb-id markers.
//
// Mirrors the post-#341 TestHandleEventImages shape
// (events_handlers_test.go) but for the soldier facade.
// The outer bulk-delete form is preserved by the templ
// refactor (B.2 still uses the outer form for multi-select);
// this test exercises the per-card path that's new in B.2.
func TestHandleSoldierImagesDeleteFragmentSwap(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "ImagesDeleteSwap")
	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, s.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll imageDir: %v", err)
	}
	firstPath := filepath.Join(imageDir, "first.png")
	secondPath := filepath.Join(imageDir, "second.png")
	if err := os.WriteFile(firstPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "first.png", filepath.Join(relativeDir, "first.png"), "First portrait"); err != nil {
		t.Fatalf("AddImage first: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "second.png", filepath.Join(relativeDir, "second.png"), "Second portrait"); err != nil {
		t.Fatalf("AddImage second: %v", err)
	}

	refreshed, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID pre-delete: %v", err)
	}
	if len(refreshed.Images) != 2 {
		t.Fatalf("seeded 2 images, soldier has %d", len(refreshed.Images))
	}
	dropID := refreshed.Images[0].ID

	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(s.ID)+"/images/delete", url.Values{
		"image_ids": {intStr(dropID)},
	})
	if err != nil {
		t.Fatalf("POST images/delete: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST delete status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Dixiedata-Redirect"); got != "" {
		t.Errorf("POST delete set X-Dixiedata-Redirect=%q; want empty (issue #391 fragment-swap)", got)
	}
	if !strings.Contains(body, "data-image-card") {
		t.Errorf("POST delete response missing per-card data-image-card marker; got %q", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`data-image-thumb-id="%d"`, refreshed.Images[1].ID)) {
		t.Errorf("POST delete response missing surviving card data-image-thumb-id; got %q", body)
	}
	if strings.Contains(body, fmt.Sprintf(`data-image-thumb-id="%d"`, dropID)) {
		t.Errorf("POST delete response still renders dropped card; got %q", body)
	}
	// Belt-and-braces: response must be the fragment, NOT
	// the full detail page (which would re-emit Layout() +
	// top-nav).
	if strings.Contains(body, "<body") || strings.Contains(body, "<nav") {
		t.Errorf("delete response leaked full-page layout; got %q", body[:min(2000, len(body))])
	}

	afterDelete, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID post-delete: %v", err)
	}
	if len(afterDelete.Images) != 1 {
		t.Errorf("after delete want 1 image, got %d", len(afterDelete.Images))
	}
}

// TestHandleSoldierImagesSetPrimaryFragmentSwap (issue #391
// Slice B.2) pins the per-card Set-Primary fragment swap
// (NOT a full-page redirect). POST
// /soldiers/{id}/images/primary/{imageID} must:
//   - flip IsPrimary on the targeted image,
//   - render the SoldierImagesListFragment (NOT set
//     X-Dixiedata-Redirect).
//
// No template-only change exists for this path -- the
// existing data-image-primary-action button on each card
// already POSTs to it via the JS-handler-driven
// data-dixie-submit path. B.2 changes the handler so the
// response is the fragment that the data-results-target
// selector (set in B.1) swaps into place. Mirrors the
// post-#341 /events fragment-swap test pattern.
func TestHandleSoldierImagesSetPrimaryFragmentSwap(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "ImagesSetPrimarySwap")
	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, s.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll imageDir: %v", err)
	}
	firstPath := filepath.Join(imageDir, "first.png")
	secondPath := filepath.Join(imageDir, "second.png")
	if err := os.WriteFile(firstPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile first: %v", err)
	}
	if err := os.WriteFile(secondPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile second: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "first.png", filepath.Join(relativeDir, "first.png"), "First portrait"); err != nil {
		t.Fatalf("AddImage first: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "second.png", filepath.Join(relativeDir, "second.png"), "Second portrait"); err != nil {
		t.Fatalf("AddImage second: %v", err)
	}

	refreshed, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID pre-set: %v", err)
	}
	if len(refreshed.Images) != 2 {
		t.Fatalf("seeded 2 images, soldier has %d", len(refreshed.Images))
	}
	promoteID := refreshed.Images[1].ID

	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(s.ID)+"/images/primary/"+intStr(promoteID), url.Values{})
	if err != nil {
		t.Fatalf("POST images/primary: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST primary status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Dixiedata-Redirect"); got != "" {
		t.Errorf("POST primary set X-Dixiedata-Redirect=%q; want empty (issue #391 fragment-swap)", got)
	}
	if !strings.Contains(body, "data-image-card") {
		t.Errorf("POST primary response missing per-card data-image-card marker; got %q", body)
	}
	if strings.Contains(body, "<body") || strings.Contains(body, "<nav") {
		t.Errorf("set-primary response leaked full-page layout; got %q", body[:min(2000, len(body))])
	}

	after, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID post-set: %v", err)
	}
	var promoted *models.Image
	for i := range after.Images {
		if after.Images[i].ID == promoteID {
			promoted = &after.Images[i]
			break
		}
	}
	if promoted == nil {
		t.Fatalf("promoted image %d not found after set-primary", promoteID)
	}
	if !promoted.IsPrimary {
		t.Errorf("after set-primary want IsPrimary=true, got false")
	}
}

// TestHandleSoldiersListRendersPageWrapper pins the
// PageSoldiersList UIID (issue #397 wide.1). The /soldiers
// browse page must render the canonical `id="page.soldiers.list"`
// wrapper around the main content area so smoke selectors and
// goquery invariant tests can pin against the same registry
// that internal/uiids/uiids.go declares. Mirrors the Slice A
// pattern of canonicalizing only the content wrapper, not the
// full body (per #397 locked decision 1).
func TestHandleSoldiersListRendersPageWrapper(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers")
	if err != nil {
		t.Fatalf("GET /soldiers: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PageSoldiersList)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers missing #%s wrapper anchor; got %q",
			uiids.PageSoldiersList,
			bodyExtract(body, "Person Records", 200),
		)
	}
}

// TestHandleSoldiersListRendersSearchBasicPanel pins
// PanelSoldiersSearchBasic — the Quick Search tab panel on
// /soldiers.
func TestHandleSoldiersListRendersSearchBasicPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers")
	if err != nil {
		t.Fatalf("GET /soldiers: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldiersSearchBasic)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers missing #%s quick-search panel; got %q",
			uiids.PanelSoldiersSearchBasic,
			bodyExtract(body, "Quick search", 200),
		)
	}
}

// TestHandleSoldiersListRendersSearchAdvancedPanel pins
// PanelSoldiersSearchAdvanced — the Advanced Search form on
// /soldiers.
func TestHandleSoldiersListRendersSearchAdvancedPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers")
	if err != nil {
		t.Fatalf("GET /soldiers: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldiersSearchAdvanced)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers missing #%s advanced-search panel; got %q",
			uiids.PanelSoldiersSearchAdvanced,
			bodyExtract(body, "Advanced Search", 200),
		)
	}
}

// TestHandleSoldiersListRendersResultsPanel pins
// PanelSoldiersResults — the results wrapper around the
// SearchResults partial on /soldiers. Coexists with the
// existing `id="soldier-list"` (the htmx swap target) —
// adding the UIID wrapper is non-disruptive.
func TestHandleSoldiersListRendersResultsPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers")
	if err != nil {
		t.Fatalf("GET /soldiers: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldiersResults)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers missing #%s results panel; got %q",
			uiids.PanelSoldiersResults,
			bodyExtract(body, "soldier-list", 200),
		)
	}
}

// TestHandleSoldierDetailRendersPageWrapper pins the
// PageSoldierDetail UIID (issue #397 wide.2). The /soldiers/{id}
// detail page must render the canonical `id="page.soldier.detail"`
// wrapper around the main content area so smoke selectors and
// goquery invariant tests can pin against the same registry
// that internal/uiids/uiids.go declares. Per #397 locked
// decision 1, the page wrapper scopes the main content area
// only — not the full body.
func TestHandleSoldierDetailRendersPageWrapper(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "DetailPageWrapper")
	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d status = %d, want 200", s.ID, resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PageSoldierDetail)
	if !strings.Contains(body, want) {
		t.Errorf(
			"soldier detail page missing #%s wrapper anchor; got %q",
			uiids.PageSoldierDetail,
			bodyExtract(body, "Edit Person Record", 200),
		)
	}
}

// TestHandleSoldierDetailRendersSummaryPanel pins
// PanelSoldierDetailSummary — the summary card on
// /soldiers/{id} (the main card with title + field dl +
// biography block). Wraps the inner card div, not the
// outer page wrapper; class=contents on the wrapper so
// the card's relative+grid layout is preserved.
func TestHandleSoldierDetailRendersSummaryPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "DetailSummaryPanel")
	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d status = %d, want 200", s.ID, resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierDetailSummary)
	if !strings.Contains(body, want) {
		t.Errorf(
			"soldier detail page missing #%s summary panel; got %q",
			uiids.PanelSoldierDetailSummary,
			bodyExtract(body, "Display ID", 200),
		)
	}
}

// TestHandleSoldierDetailRendersRecordsPanel pins
// PanelSoldierDetailRecords — the Source Records section
// on /soldiers/{id}. The section is conditionally rendered
// (only when s.SourceRecords > 0), so the test seeds a
// record via the soldiers facade to ensure the wrapper
// anchors on a populated row.
func TestHandleSoldierDetailRendersRecordsPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s, err := app.soldiers.Create(models.Soldier{
		FirstName: "Robert",
		LastName:  "E Lee",
		Records: []models.Record{
			{RecordType: "TestSource", AppID: "APP-1", Details: "Sample source for panel anchor."},
		},
	})
	if err != nil {
		t.Fatalf("Create seeded soldier with record: %v", err)
	}

	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d status = %d, want 200", s.ID, resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierDetailRecords)
	if !strings.Contains(body, want) {
		t.Errorf(
			"soldier detail page missing #%s records panel; got %q",
			uiids.PanelSoldierDetailRecords,
			bodyExtract(body, "Source Records", 200),
		)
	}
}

// TestHandleNewSoldierRendersPageWrapper pins the
// PageSoldierNew UIID (issue #397 wide.3). GET /soldiers/new
// renders the EntryForm (entry_form.templ) which is the same
// templ the /soldiers/{id}/edit route uses — so the wrapper
// must conditional-render via templ's if isEdit branch to
// pin PageSoldierNew on the new page and PageSoldierEdit on
// the edit page (per #397 locked decision 3 — same templ,
// different page UIID).
func TestHandleNewSoldierRendersPageWrapper(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers/new")
	if err != nil {
		t.Fatalf("GET /soldiers/new: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/new status = %d, want 200", resp.StatusCode)
	}

	wantNew := fmt.Sprintf(`id="%s"`, uiids.PageSoldierNew)
	if !strings.Contains(body, wantNew) {
		t.Errorf(
			"/soldiers/new missing #%s wrapper anchor; got %q",
			uiids.PageSoldierNew,
			bodyExtract(body, "Add Person Record", 200),
		)
	}
}

// TestHandleEditSoldierRendersEditPageWrapper pins the
// PageSoldierEdit UIID. Mirrors the new-page test above —
// on /soldiers/{id}/edit, PageSoldierEdit must render and
// PageSoldierNew must NOT render (mutual exclusion via
// templ's if/else).
func TestHandleEditSoldierRendersEditPageWrapper(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "EditPageWrapper")
	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID) + "/edit")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/edit: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d/edit status = %d, want 200", s.ID, resp.StatusCode)
	}

	wantEdit := fmt.Sprintf(`id="%s"`, uiids.PageSoldierEdit)
	if !strings.Contains(body, wantEdit) {
		t.Errorf(
			"/soldiers/%d/edit missing #%s wrapper anchor; got %q",
			s.ID,
			uiids.PageSoldierEdit,
			bodyExtract(body, "Edit Person Record", 200),
		)
	}
}

// TestHandleNewSoldierRendersFormScratchpadPanel pins
// PanelSoldierFormScratchpad on the new-record form. Per
// #397 locked decision 3 the panel renders on BOTH new +
// edit because the same templ serves both routes; a new-page
// assertion is sufficient (the edit-page assertion lives
// alongside the TestHandleEditSoldierRendersFormScratchpadPanel
// test below — kept separate so a regression that breaks one
// route but not the other surfaces the right diagnostic).
func TestHandleNewSoldierRendersFormScratchpadPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers/new")
	if err != nil {
		t.Fatalf("GET /soldiers/new: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/new status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierFormScratchpad)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers/new missing #%s scratchpad panel; got %q",
			uiids.PanelSoldierFormScratchpad,
			bodyExtract(body, "record-persistence", 200),
		)
	}
}

// TestHandleNewSoldierRendersFormRecordsPanel pins
// PanelSoldierFormRecords on the new-record form — the
// Source Records editor section inside the entry form.
// Renders on BOTH new + edit (same templ, same UIID).
func TestHandleNewSoldierRendersFormRecordsPanel(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	resp, err := http.Get(server.URL + "/soldiers/new")
	if err != nil {
		t.Fatalf("GET /soldiers/new: %v", err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/new status = %d, want 200", resp.StatusCode)
	}

	want := fmt.Sprintf(`id="%s"`, uiids.PanelSoldierFormRecords)
	if !strings.Contains(body, want) {
		t.Errorf(
			"/soldiers/new missing #%s form-records panel; got %q",
			uiids.PanelSoldierFormRecords,
			bodyExtract(body, "Source Records", 200),
		)
	}
}

// TestHandleSoldierByID_RedirectsEventRowsToEventsDetail
// (issue #363) pins the server-gate that protects against
// an Event row ever rendering the Person Record shape via
// /soldiers/{id}*. The catch-all handleSoldierByID must
// 303-redirect GET /soldiers/{id} to /events/{id} when the
// row is an Event Record (entry_type="event").
func TestHandleSoldierByID_RedirectsEventRowsToEventsDetail(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "Battle", "07/01/1863", "07/03/1863", "Gettysburg")

	client := noRedirectClient()
	resp, err := client.Get(server.URL + "/soldiers/" + strconv.FormatInt(e.ID, 10))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", e.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /soldiers/%d status = %d, want 303 (issue #363)", e.ID, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/events/"+strconv.FormatInt(e.ID, 10) {
		t.Errorf("Location = %q, want /events/%d", loc, e.ID)
	}
}

// TestHandleSoldierByID_RedirectsEventRowsForEditSuffix
// (issue #363) pins the /soldiers/{id}/edit redirect
// target. The redirect goes to /events/{id} (the detail
// page) NOT /events/{id}/edit — the user can navigate
// from the detail to the editor.
func TestHandleSoldierByID_RedirectsEventRowsForEditSuffix(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "Battle", "07/01/1863", "07/03/1863", "Gettysburg")

	client := noRedirectClient()
	resp, err := client.Get(server.URL + "/soldiers/" + strconv.FormatInt(e.ID, 10) + "/edit")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/edit: %v", e.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /soldiers/%d/edit status = %d, want 303", e.ID, resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/events/"+strconv.FormatInt(e.ID, 10) {
		t.Errorf("Location = %q, want /events/%d (detail, not edit)", loc, e.ID)
	}
}

// TestHandleSoldierByID_PUTOnEventRowRedirectsAndDoesNotMutate
// (issue #363) pins the corruption-prevention guarantee.
// PUT on an Event row via /soldiers/{id} must:
//   - 303 to /events/{id}
//   - leave the row's kind / begin_date / description
//     unchanged from their pre-PUT values (i.e. the
//     redirect fired BEFORE parseSoldierForm could strip
//     them or rewrite entry_type).
func TestHandleSoldierByID_PUTOnEventRowRedirectsAndDoesNotMutate(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "OriginalKind", "01/15/1864", "", "Original description")

	form := url.Values{}
	form.Set("kind", "MaliciousOverride")
	form.Set("begin_date", "12/25/2025")
	form.Set("description", "PWNED")

	req, err := http.NewRequest(http.MethodPut,
		server.URL+"/soldiers/"+strconv.FormatInt(e.ID, 10),
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build PUT: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := noRedirectClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT /soldiers/%d: %v", e.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("PUT /soldiers/%d status = %d, want 303", e.ID, resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/events/"+strconv.FormatInt(e.ID, 10) {
		t.Errorf("Location = %q, want /events/%d", loc, e.ID)
	}

	after, err := app.soldiers.GetByID(e.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.Kind != "OriginalKind" {
		t.Errorf("after PUT kind = %q, want unchanged %q (issue #363 corruption guard)",
			after.Kind, "OriginalKind")
	}
	if after.BeginDate != "01/15/1864" {
		t.Errorf("after PUT begin_date = %q, want unchanged %q",
			after.BeginDate, "01/15/1864")
	}
	if after.Description != "Original description" {
		t.Errorf("after PUT description = %q, want unchanged",
			after.Description)
	}
	if after.EntryType != models.EntryTypeEvent {
		t.Errorf("after PUT entry_type = %q, want unchanged %q",
			after.EntryType, models.EntryTypeEvent)
	}
}

// TestHandleSoldierByID_DELETEOnEventRowRedirects (issue #363)
// is the defensive DELETE gate. An Event reached via
// /soldiers/{id} DELETE would route to a.soldiers.Delete(id)
// which cascades event_person_links references away. Wrong
// surface for the operation. The 303 sends the user to the
// canonical Event surface where DELETE is meaningless
// (events have no delete affordance).
func TestHandleSoldierByID_DELETEOnEventRowRedirects(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "ToNotDelete", "06/15/1863", "", "")

	req, err := http.NewRequest(http.MethodDelete,
		server.URL+"/soldiers/"+strconv.FormatInt(e.ID, 10), nil)
	if err != nil {
		t.Fatalf("build DELETE: %v", err)
	}
	client := noRedirectClient()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE /soldiers/%d: %v", e.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("DELETE /soldiers/%d status = %d, want 303", e.ID, resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/events/"+strconv.FormatInt(e.ID, 10) {
		t.Errorf("Location = %q, want /events/%d", loc, e.ID)
	}

	// Row must still exist.
	if _, err := app.soldiers.GetByID(e.ID); err != nil {
		t.Errorf("after DELETE GetByID failed: %v (Event row was deleted via /soldiers/!)", err)
	}
}

// TestHandleSoldierByID_PDFOnEventRowRedirects (issue #363)
// pins the PDF path redirect. Soldier PDF for an Event
// would render soldier_landscape.typ (wrong template).
// The redirect sends the user to the event's PDF surface.
func TestHandleSoldierByID_PDFOnEventRowRedirects(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	e := createEvent(t, app, "PdfEvent", "07/01/1863", "", "")

	client := noRedirectClient()
	resp, err := client.Get(server.URL + "/soldiers/" + strconv.FormatInt(e.ID, 10) + "/pdf")
	if err != nil {
		t.Fatalf("GET /soldiers/%d/pdf: %v", e.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /soldiers/%d/pdf status = %d, want 303", e.ID, resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/events/"+strconv.FormatInt(e.ID, 10)+"/pdf" {
		t.Errorf("Location = %q, want /events/%d/pdf", loc, e.ID)
	}
}

// TestHandleSoldierByID_PersonRowStillRendersSoldierCard is the
// protection test (issue #363): the gate must NOT regress
// non-Event rows. A Person Record reached via /soldiers/{id}
// must still render the detail card (status 200, body
// contains the SoldierDetail fragments).
func TestHandleSoldierByID_PersonRowStillRendersSoldierCard(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "PersonRowStillRendersCard")

	resp, err := http.Get(server.URL + "/soldiers/" + intStr(s.ID))
	if err != nil {
		t.Fatalf("GET /soldiers/%d: %v", s.ID, err)
	}
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /soldiers/%d status = %d, want 200 (Person row)", s.ID, resp.StatusCode)
	}
	// SoldierDetail renders 'Edit Person Record' button as a
	// smoke-pinned fragment; asserting on its presence is
	// enough to confirm Person Record shape rendered.
	if !strings.Contains(body, "Edit Person Record") {
		t.Errorf("Person row /soldiers/%d body missing 'Edit Person Record' fragment — gate may have over-reached", s.ID)
	}
}

// noRedirectClient returns an *http.Client that does NOT follow
// redirects. Used by the issue #363 gate tests so the 303
// response from handleSoldierByID is observed directly (default
// http.Get follows 3xx and reports the final 200/whatever on
// the destination, hiding the gate's status).
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}