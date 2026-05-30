package appshell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestHandleFeedbackSubmitAppendsFeedbackLog(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/feedback/submit", strings.NewReader(url.Values{
		"page_path":     {"/soldiers/12"},
		"category":      {"bug"},
		"contact_name":  {"A. Researcher"},
		"contact_email": {"test@example.com"},
		"message":       {"The timeline link should stay in context."},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-DixieData-Close-Feedback") != "true" {
		t.Fatalf("expected feedback modal close header")
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "logs", "feedback-log.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one feedback log line, got %d", len(lines))
	}

	var entry feedbackEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if entry.PagePath != "/soldiers/12" || entry.Category != "bug" || entry.Message == "" {
		t.Fatalf("unexpected feedback entry: %#v", entry)
	}
}

func TestHandleSoldierByDisplayIDRedirectsToRecord(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	created, err := app.soldiers.Create(models.Soldier{
		DisplayID: "JCM87-00011",
		FirstName: "Thomas",
		LastName:  "Cole",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/soldiers/display/JCM87-00011", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/soldiers/"+strconv.FormatInt(created.ID, 10) {
		t.Fatalf("location=%q", location)
	}
}

func TestHandleSoldierByDisplayIDFallsBackToSearch(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	app := NewApp()
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/soldiers/display/UNKNOWN-00001", nil)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusSeeOther)
	}
	if location := rec.Header().Get("Location"); location != "/soldiers/search?q=UNKNOWN-00001" {
		t.Fatalf("location=%q", location)
	}
}
