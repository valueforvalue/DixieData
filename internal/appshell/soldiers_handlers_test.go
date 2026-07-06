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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
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