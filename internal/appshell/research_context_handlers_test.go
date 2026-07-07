package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/uiids"
)

func TestHandleResearchPickerRendersShellNoCookie(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="`+uiids.PageResearchPicker+`"`) {
		t.Fatalf("picker page wrapper missing")
	}
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerSearch+`"`) {
		t.Fatalf("picker search panel missing")
	}
	if !strings.Contains(body, `id="`+uiids.PanelResearchPickerRecent+`"`) {
		t.Fatalf("picker recent panel missing")
	}
}

func TestHandleResearchPickerRendersContinueFromCookie(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	writePersonCtxCookie(t, req, app, 411)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Continue") {
		t.Fatalf("expected Continue shortcut in body")
	}
}

func TestHandleResearchSelectWritesCookieAndRedirects(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("person_id", "411")
	form.Set("next", "camaraderie")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got, want := rec.Header().Get("X-DixieData-Redirect"), "/soldiers/411/camaraderie"; got != want {
		t.Fatalf("X-DixieData-Redirect = %q; want %q", got, want)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dd_person_ctx cookie not emitted")
	}
}

func TestHandleResearchSelectRejectsMissingPersonID(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("next", "camaraderie")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestHandleResearchSelectRejectsBadNext(t *testing.T) {
	app := newPickerApp(t)
	form := url.Values{}
	form.Set("person_id", "1")
	form.Set("next", "../../etc/passwd")
	req := httptest.NewRequest(http.MethodPost, "/research/select", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestHandleResearchClearEmitsMaxAgeMinusOne(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodPost, "/research/clear", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := rec.Header().Get("X-DixieData-Redirect"); got != "/research" {
		t.Fatalf("X-DixieData-Redirect = %q; want /research", got)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dd_person_ctx" && c.MaxAge >= 0 {
			t.Fatalf("dd_person_ctx cookie not cleared")
		}
	}
}

func TestRouteOrderResearchBeatsSoldiersWildcard(t *testing.T) {
	app := newPickerApp(t)
	req := httptest.NewRequest(http.MethodGet, "/research", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Unknown soldier") {
		t.Fatalf("dispatched to handleSoldierByID")
	}
	if !strings.Contains(body, `id="`+uiids.PageResearchPicker+`"`) {
		t.Fatalf("did not render picker")
	}
}