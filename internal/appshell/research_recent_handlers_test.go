package appshell

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/uiids"
)

// === Issue #378 slice 3 RED tests ===

func TestHandleResearchRecentReturnsRequestedPersons(t *testing.T) {
	app := newPickerApp(t)
	seedTimelinePerson(t, app, 601)
	seedTimelinePerson(t, app, 602)
	seedTimelinePerson(t, app, 603)
	req := httptest.NewRequest(http.MethodGet, "/research/recent?ids=601,602,603", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerRecent+`"`) &&
		!strings.Contains(body, `data-research-recent-list`) {
		t.Fatalf("recent fragment missing the #panel.research.picker.recent wrapper or the ul tag")
	}
	for _, id := range []int64{601, 602, 603} {
		if !strings.Contains(body, "Person"+strconv.FormatInt(id, 10)) {
			t.Fatalf("recent fragment did not include Person%d", id)
		}
	}
}

func TestHandleResearchRecentIgnoresUnknownIDs(t *testing.T) {
	app := newPickerApp(t)
	seedTimelinePerson(t, app, 701)
	req := httptest.NewRequest(http.MethodGet, "/research/recent?ids=701,9999,8888", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Person701") {
		t.Fatalf("expected Person701 in fragment body; unknown IDs should not break render")
	}
	if strings.Contains(body, "Person9999") || strings.Contains(body, "Person8888") {
		t.Fatalf("fragment unexpectedly rendered unknown IDs")
	}
}

func TestHandleResearchRecentEmptyIDsReturnsEmptyState(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research/recent?ids=", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-research-recent-empty`) {
		t.Fatalf("empty ids should render the recent-empty paragraph, not a populated ul")
	}
}

func TestHandleResearchRecentPreservesOrderFromIDs(t *testing.T) {
	app := newPickerApp(t)
	seedTimelinePerson(t, app, 801)
	seedTimelinePerson(t, app, 802)
	seedTimelinePerson(t, app, 803)
	req := httptest.NewRequest(http.MethodGet, "/research/recent?ids=803,802,801", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	pos803 := strings.Index(body, "Person803")
	pos802 := strings.Index(body, "Person802")
	pos801 := strings.Index(body, "Person801")
	if pos803 < 0 || pos802 < 0 || pos801 < 0 {
		t.Fatalf("all three rows should render: pos803=%d pos802=%d pos801=%d", pos803, pos802, pos801)
	}
	if !(pos803 < pos802 && pos802 < pos801) {
		t.Fatalf("ids order not preserved; want 803 < 802 < 801, got %d, %d, %d", pos803, pos802, pos801)
	}
}

func TestHandleResearchPickerRendersPackStateSubScreen(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research?next=research-pack", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="geography"`) &&
		!strings.Contains(body, "research-pack-state") &&
		!strings.Contains(body, "research-pack-county") {
		t.Fatalf("picker did not surface the state vs county sub-screen for ?next=research-pack")
	}
}

func TestPickerRecentFragmentEchoesNext(t *testing.T) {
	app := newPickerApp(t)
	seedTimelinePerson(t, app, 901)
	req := httptest.NewRequest(http.MethodGet, "/research/recent?ids=901&next=camaraderie", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="next" value="camaraderie"`) {
		t.Fatalf("recent fragment did not forward ?next=camaraderie to its hidden fields")
	}
}
