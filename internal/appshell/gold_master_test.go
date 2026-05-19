package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestGoldMasterCRUDLoopDetailSurface(t *testing.T) {
	app := newStressApp(t)

	soldier, err := app.soldiers.Create(models.Soldier{
		FirstName:    "Thomas",
		LastName:     "Carter",
		Prefix:       "Capt.",
		Unit:         "5th Virginia Infantry",
		BirthDate:    "11/09/1831",
		PensionState: "Virginia",
		Records: []models.Record{
			{RecordType: "Service Record", AppID: "GM-1", Details: "Captured in the gold master QA loop."},
		},
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	widow, err := app.soldiers.Create(models.Soldier{
		EntryType:       "widow",
		FirstName:       "Sarah",
		LastName:        "Carter",
		MaidenName:      "Cole",
		SpouseSoldierID: soldier.ID,
		PensionID:       "WP-1",
		ApplicationID:   "WA-1",
	})
	if err != nil {
		t.Fatalf("Create widow: %v", err)
	}

	imageDir, relativeDir := appdata.RecordImageDir(app.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	primaryPath := filepath.Join(imageDir, "primary.png")
	alternatePath := filepath.Join(imageDir, "alternate.png")
	if err := os.WriteFile(primaryPath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile primary: %v", err)
	}
	if err := os.WriteFile(alternatePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile alternate: %v", err)
	}
	if err := app.soldiers.AddImage(soldier.ID, "primary.png", filepath.Join(relativeDir, "primary.png"), "Primary portrait"); err != nil {
		t.Fatalf("AddImage primary: %v", err)
	}
	if err := app.soldiers.AddImage(soldier.ID, "alternate.png", filepath.Join(relativeDir, "alternate.png"), "Alternate portrait"); err != nil {
		t.Fatalf("AddImage alternate: %v", err)
	}
	refreshed, err := app.soldiers.GetByID(soldier.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	for _, image := range refreshed.Images {
		if image.FileName == "alternate.png" {
			if err := app.soldiers.SetPrimaryImage(soldier.ID, image.ID); err != nil {
				t.Fatalf("SetPrimaryImage: %v", err)
			}
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers/"+itoa(soldier.ID)+"?from="+itoa(widow.ID), nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"Primary Image",
		"Alternate portrait",
		"Back to Widow Record",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("detail page missing %q", needle)
		}
	}

	widowReq := httptest.NewRequest(http.MethodGet, "/soldiers/"+itoa(widow.ID), nil)
	widowRec := httptest.NewRecorder()
	app.ServeHTTP(widowRec, widowReq)
	if widowRec.Code != http.StatusOK {
		t.Fatalf("widow status=%d body=%q", widowRec.Code, widowRec.Body.String())
	}
	widowBody := widowRec.Body.String()
	for _, needle := range []string{
		"Married To",
		"Thomas Carter",
		"View Husband",
	} {
		if !strings.Contains(widowBody, needle) {
			t.Fatalf("widow detail missing %q", needle)
		}
	}
}

func TestGoldMasterSearchIndexesScratchpadViaHTTP(t *testing.T) {
	app := newStressApp(t)
	soldier, err := app.soldiers.Create(models.Soldier{
		DisplayID: "GM-SEARCH-001",
		FirstName: "Roswell",
		LastName:  "Depot",
	})
	if err != nil {
		t.Fatalf("Create soldier: %v", err)
	}
	textPath, _ := appdata.ScratchpadPaths(app.dataDir, soldier.DisplayID)
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(textPath, []byte("Ghosted rail depot notes for gold master FTS coverage."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers/search?q=rail+depot", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Scratch Pad") || !strings.Contains(body, "GM-SEARCH-001") {
		t.Fatalf("search response missing scratchpad hit: %q", body)
	}
}

func TestGoldMasterMissingMediaReturnsPlaceholder(t *testing.T) {
	app := newStressApp(t)
	req := httptest.NewRequest(http.MethodGet, "/media/images/missing-record/portrait.png", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("content-type=%q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "Image Missing") || !strings.Contains(rec.Body.String(), "portrait.png") {
		t.Fatalf("unexpected placeholder body: %q", rec.Body.String())
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
