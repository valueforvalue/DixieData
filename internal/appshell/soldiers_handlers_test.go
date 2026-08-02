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
	"time"

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

// TestHandleSoldierImagesDeleteRetriesOnFileLock (issue #709)
// pins the file-locking retry on Windows. The per-card
// Delete handler calls os.Remove on the image file. On
// Windows, anti-virus scanners / OS file indexers /
// headless-chromium's image cache can briefly hold a read
// handle on the file (no FILE_SHARE_DELETE on os.Create).
// os.Remove returns ERROR_SHARING_VIOLATION while any other
// handle is open; the handler must retry with short
// backoff so a fast click right after upload doesn't fail
// with a 500.
//
// This test simulates the contention by opening a read
// handle on the file in the test process before POSTing the
// delete. On Linux, os.Remove on an open file succeeds
// (unlink-while-open is allowed), so the test passes
// trivially. On Windows, the test would race the
// contention; with the retry loop in place the handler
// still succeeds because the test holds the handle open
// for the entire POST + handler runtime (1-2s typical),
// which exceeds the 1.55s retry budget. To make the test
// deterministic on both platforms, the test releases the
// read handle after 50ms — well within the retry budget —
// so the handler's next retry attempt succeeds.
//
// The deterministic part of the test is the FIRST os.Remove
// attempt that the handler makes while the read handle is
// still open: on Windows this returns ERROR_SHARING_VIOLATION;
// on Linux it succeeds. The test confirms that EITHER outcome
// (first-attempt success on Linux, retry-then-success on
// Windows) produces a 200 response. If the retry loop is
// removed, this test still passes on Linux (unlink succeeds)
// but fails on Windows (the first os.Remove would 500).
func TestHandleSoldierImagesDeleteRetriesOnFileLock(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	s := createSoldier(t, app, "ImagesDeleteRetry")
	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, s.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll imageDir: %v", err)
	}
	targetPath := filepath.Join(imageDir, "locked.png")
	if err := os.WriteFile(targetPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile target: %v", err)
	}
	if err := app.soldiers.AddImage(s.ID, "locked.png", filepath.Join(relativeDir, "locked.png"), "Locked"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}

	refreshed, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID pre-delete: %v", err)
	}
	dropID := refreshed.Images[0].ID

	// Open a read handle on the image file to simulate a
	// contending process. On Windows this prevents
	// os.Remove from succeeding until the handle is
	// released. On Linux unlink-while-open is allowed so
	// the delete succeeds on the first try.
	holdHandle, err := os.Open(targetPath)
	if err != nil {
		t.Fatalf("Open holdHandle: %v", err)
	}
	defer holdHandle.Close()

	// Release the handle after 50ms — inside the handler's
	// retry budget (5 attempts × 50/100/200/400/800ms
	// = 1.55s total). On Windows the first 1-2 attempts
	// fail with ERROR_SHARING_VIOLATION; the subsequent
	// attempts succeed once the test releases the handle.
	// On Linux the first attempt succeeds and the release
	// is a no-op.
	releaseTime := time.Now().Add(50 * time.Millisecond)
	go func() {
		time.Sleep(time.Until(releaseTime))
		holdHandle.Close()
	}()

	deleteStart := time.Now()
	resp, err := http.PostForm(server.URL+"/soldiers/"+intStr(s.ID)+"/images/delete", url.Values{
		"image_ids": {intStr(dropID)},
	})
	if err != nil {
		t.Fatalf("POST images/delete: %v", err)
	}
	deleteDuration := time.Since(deleteStart)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST delete status = %d, want 200 (handler retried past the 50ms hold); the file-locking retry logic on Windows is not working", resp.StatusCode)
	}
	// Sanity: the delete should have completed within the
	// retry budget (1.55s) plus a small margin. If the
	// handler returned 200 in <10ms, the retry loop was
	// not exercised; if it took >3s, the retry budget
	// overflowed. The expected window is 50-1600ms.
	if deleteDuration < 50*time.Millisecond {
		t.Logf("note: delete returned in %v; the retry loop may not have been exercised on this platform (Linux unlink-while-open is allowed; the loop exits on the first attempt)", deleteDuration)
	}
	if deleteDuration > 3*time.Second {
		t.Errorf("delete took %v; the 1.55s retry budget overflowed", deleteDuration)
	}

	// DB: image should be gone.
	afterDelete, err := app.soldiers.GetByID(s.ID)
	if err != nil {
		t.Fatalf("GetByID post-delete: %v", err)
	}
	for _, img := range afterDelete.Images {
		if img.ID == dropID {
			t.Errorf("image %d should be deleted from DB but is still present", dropID)
		}
	}
}

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

// TestSoldiersPagesRenderUIIDs is the consolidated table-driven
// UIID pin for every page wrapper + content panel rendered by
// the soldier browse / detail / new / edit routes. The 11
// individual tests it replaced (#397 wide.1 / wide.2 / wide.3)
// each booted newStressApp + httptest.NewServer + createSoldier
// to GET one path and assert one anchor. Sharing the App +
// soldier across the four subtests cuts ~10 cold boots from
// the CI hot path.
//
// Page wrapper UIIDs (PageSoldiersList / PageSoldierDetail /
// PageSoldierNew / PageSoldierEdit) and the content panel
// UIIDs (PanelSoldiersSearchBasic / PanelSoldiersSearchAdvanced
// / PanelSoldiersResults / PanelSoldierDetailSummary /
// PanelSoldierDetailRecords / PanelSoldierFormScratchpad /
// PanelSoldierFormRecords) are all asserted via the same
// `id="..."` substring pattern the prior tests used.
func TestSoldiersPagesRenderUIIDs(t *testing.T) {
	app := newStressApp(t)
	server := httptest.NewServer(app)
	defer server.Close()

	// Seed a soldier WITH a Source Record so
	// PanelSoldierDetailRecords renders (the wrapper is
	// conditional on len(s.Records) > 0 per soldier_card.templ
	// around the PanelSoldierDetailRecords section). Mirrors
	// the prior standalone TestHandleSoldierDetailRendersRecordsPanel
	// seed.
	seeded, err := app.soldiers.Create(models.Soldier{
		FirstName: "UIID",
		LastName:  "Test",
		Records: []models.Record{
			{RecordType: "TestSource", AppID: "APP-1", Details: "Seeded for PanelSoldierDetailRecords."},
		},
	})
	if err != nil {
		t.Fatalf("seed soldier with record: %v", err)
	}
	s := *seeded

	cases := []struct {
		name  string
		path  string
		uiids []string
	}{
		{
			name: "list",
			path: "/soldiers",
			uiids: []string{
				uiids.PageSoldiersList,
				uiids.PanelSoldiersSearchBasic,
				uiids.PanelSoldiersSearchAdvanced,
				uiids.PanelSoldiersResults,
			},
		},
		{
			name: "detail",
			path: "/soldiers/" + intStr(s.ID),
			uiids: []string{
				uiids.PageSoldierDetail,
				uiids.PanelSoldierDetailSummary,
				uiids.PanelSoldierDetailRecords,
			},
		},
		{
			name: "new",
			path: "/soldiers/new",
			uiids: []string{
				uiids.PageSoldierNew,
				uiids.PanelSoldierFormScratchpad,
				uiids.PanelSoldierFormRecords,
			},
		},
		{
			name: "edit",
			path: "/soldiers/" + intStr(s.ID) + "/edit",
			uiids: []string{
				uiids.PageSoldierEdit,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			body := readAll(t, resp)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", tc.path, resp.StatusCode)
			}
			for _, id := range tc.uiids {
				want := fmt.Sprintf(`id="%s"`, id)
				if !strings.Contains(body, want) {
					t.Errorf("%s missing #%s anchor", tc.path, id)
				}
			}
		})
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
