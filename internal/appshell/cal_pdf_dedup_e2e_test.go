package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestCalendarPDFDedupClearsAfterCancel verifies the dedup key is
// cleared when the user CANCELS the SaveFileDialog (not just when
// they pick a file). Repro for the user-reported "toast more often
// than not": the original patch correctly releases the key after a
// successful export, but if the key isn't released after a cancel,
// the user's NEXT legitimate click is rejected.
func TestCalendarPDFDedupClearsAfterCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("requires real .dixiedata archive")
	}
	dir, _ := os.Getwd()
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "wails.json")); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	dbPath := filepath.Join(dir, ".dixiedata")
	if _, err := os.Stat(filepath.Join(dbPath, "dixiedata.db")); err != nil {
		t.Skipf("no .dixiedata/dixiedata.db at %s", dbPath)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	app := NewApp()
	app.WithFrontendAssets(os.DirFS(filepath.Join(dir, "frontend")))
	app.dataDir = dbPath
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	// First call: user CANCELS the dialog (path="").
	app.saveFileDialogOverride = func(_ any) (string, error) {
		return "", nil // path="" simulates user clicking Cancel
	}
	formBody := "orientation=P&printer_friendly="
	req1 := httptest.NewRequest(http.MethodPost, "/calendar/6/report/pdf", strings.NewReader(formBody))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec1 := httptest.NewRecorder()
	app.ServeHTTP(rec1, req1)
	t.Logf("call 1 (cancelled): status=%d body=%s", rec1.Code, rec1.Body.String())

	// Second call: dialog returns a valid path. Should NOT be rejected.
	calls := 0
	app.saveFileDialogOverride = func(_ any) (string, error) {
		calls++
		return filepath.Join(t.TempDir(), "cal.pdf"), nil
	}
	req2 := httptest.NewRequest(http.MethodPost, "/calendar/6/report/pdf", strings.NewReader(formBody))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	app.ServeHTTP(rec2, req2)
	t.Logf("call 2 (after cancel): status=%d dialogs=%d body=%s", rec2.Code, calls, rec2.Body.String())

	body := rec2.Body.String()
	if strings.Contains(body, "Export already in progress") {
		t.Fatalf("call 2 returned dedup toast after cancel — inFlight key was NOT cleared:\nstatus=%d\nbody=%s", rec2.Code, body)
	}
	if calls != 1 {
		t.Fatalf("call 2 expected dialog to fire, got total dialogs=%d", calls)
	}
}