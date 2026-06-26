package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestHandleImportBackupRedirectsAfterRestore is the regression test
// for the user-reported 'backup loaded but UI still shows old data'
// fallout.
//
// Symptom: the Load Backup button on /share successfully imports a
// .ddbak (the import code path works — TestBackupService_ImportSeededArchiveRoundTrip
// proves it), but the handler returns no X-DixieData-Redirect header.
// The user stays on /share, the page is not re-rendered, and every
// panel that depends on live DB state (counts, merge review, recent
// records) keeps showing the pre-import values. The fix: set
// X-DixieData-Redirect so the user lands on a page that reflects the
// restored archive.
//
// Without the fix this test fails because the header is missing.
// With the fix it passes and acts as a guard against future
// regressions if someone removes the redirect line.
func TestHandleImportBackupRedirectsAfterRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a real .ddbak archive")
	}

	// Locate the seeded .ddbak the dev left at the repo root.
	cwd, _ := os.Getwd()
	var backupPath string
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(cwd, "dixiedata-backup-2026-05-30.ddbak")
		if _, err := os.Stat(candidate); err == nil {
			backupPath = candidate
			break
		}
		cwd = filepath.Dir(cwd)
	}
	if backupPath == "" {
		t.Skip("no dixiedata-backup-2026-05-30.dddbak in repo root")
	}

	// Set up a minimal App with an open DB + identity.
	dir, _ := os.Getwd()
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(dir, "wails.json")); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	tmpDataDir := t.TempDir()
	database, err := db.Open(tmpDataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}

	app := NewApp()
	app.dataDir = tmpDataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)
	app.setupRoutes()

	// Inject the real .ddbak path via the open-file-dialog override.
	app.openFileDialogOverride = func(_ any) (string, error) {
		return backupPath, nil
	}

	// POST /import/backup exactly as the Load Backup button does.
	req := httptest.NewRequest(http.MethodPost, "/import/backup", nil)
	req.Header.Set("X-Requested-With", "DixieData")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Core assertion: the handler must set X-DixieData-Redirect on
	// success so the user lands on a fresh page that reflects the
	// restored archive.
	got := rec.Header().Get("X-DixieData-Redirect")
	if got == "" {
		t.Fatalf("handleImportBackup did NOT set X-DixieData-Redirect on success.\n"+
			"Symptom: user stays on /share, every panel keeps pre-import state.\n"+
			"Fix: set X-DixieData-Redirect (e.g. to '/') in handleImportBackup success path.\n"+
			"Body: %s", rec.Body.String())
	}
	t.Logf("X-DixieData-Redirect = %q (success body: %s)", got, rec.Body.String())

	// Toast header is still expected — sanity check we didn't break
	// the existing toast contract.
	if toast := rec.Header().Get("X-DixieData-Toast"); toast == "" {
		t.Errorf("expected X-DixieData-Toast on success, got empty")
	}

	// Close the database BEFORE temp dir teardown so SQLite WAL
	// sidecars (dixiedata.db-wal / dixiedata.db-shm) release their
	// Windows file handles. The handler closes + reopens the DB
	// during the import, so app.database is a fresh handle here.
	if app.database != nil {
		app.database.Close()
		app.database = nil
	}
}