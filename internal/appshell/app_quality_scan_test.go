package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

func TestSettingsQualityScanEndpointRendersFindings(t *testing.T) {
	app := newStressApp(t)
	if _, err := app.soldiers.Create(models.Soldier{
		DisplayID: "DXD-00001",
		FirstName: "John",
		LastName:  "Doe",
		BirthInfo: "TODO verify county",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/quality/scan", strings.NewReader(url.Values{
		"quality_mode": {"high-confidence"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"/settings/quality/apply",
		`name="selected_ids"`,
		"Move Selected to Review Queue",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("scan body missing %q: %q", needle, body)
		}
	}
}

func TestSettingsQualityApplyMovesSelectedRecordToReviewQueue(t *testing.T) {
	app := newStressApp(t)
	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "DXD-00001",
		FirstName: "John",
		LastName:  "Doe",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/quality/apply", strings.NewReader(url.Values{
		"selected_ids": {strconv.FormatInt(created.ID, 10)},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	updated, err := app.soldiers.GetByID(created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated == nil || !updated.NeedsReview {
		t.Fatalf("expected record to be in review queue: %#v", updated)
	}
	if !strings.Contains(updated.ReviewReason, "Heuristic scan flagged data-quality issues.") {
		t.Fatalf("review reason should include heuristic marker, got %q", updated.ReviewReason)
	}
}

func TestSettingsQualityApplyRequiresSelection(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodPost, "/settings/quality/apply", strings.NewReader(url.Values{}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Select at least one finding first.") {
		t.Fatalf("expected warning body, got %q", rec.Body.String())
	}
}
