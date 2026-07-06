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
	"strings"
	"testing"

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