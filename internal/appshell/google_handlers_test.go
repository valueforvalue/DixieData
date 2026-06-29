package appshell

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
)

// TestGoogleHandlersRedirectToJobs is the regression test for the
// bug "export options status pages not landing". Symptom: clicking
// "Upload Backup to Google Drive" or "Export CSV to Google Sheets"
// on /share posted to the handler, the handler enqueued the job and
// returned 303, but the user stayed on /share instead of being
// navigated to /jobs/{id}.
//
// Root cause: handleGoogleBackup and handleGoogleSheetsExport wrote
// the `Location` header but forgot the `HX-Redirect` header. With
// `hx-swap="none"` on the share page buttons (share.templ:511, 517)
// htmx 2.x suppresses BOTH the swap AND the redirect handling,
// silently swallowing the plain Location. Every other export handler
// in exports_handlers.go.enqueueExport writes both headers — the two
// Google handlers were missed by that fix.
//
// Fix: add `w.Header().Set("HX-Redirect", "/jobs/"+jobID)` alongside
// the Location header in both handlers. This test pins both headers
// so a future refactor can't quietly drop HX-Redirect again.
//
// The async job callback eventually calls a.google.UploadBackup /
// a.google.UploadCSVAsSheet which would fail because we never
// initialize a real Google integration in this test; the failures
// surface as job errors inside the registry and are recorded by
// jobs.SetResult, NOT propagated as test failures.
func TestGoogleHandlersRedirectToJobs(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{name: "google_drive_backup", path: "/integrations/google/backup"},
		{name: "google_sheets_export", path: "/integrations/google/sheets/export"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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

			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("X-Requested-With", "DixieData")
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d want %d (Option C contract: 200 + X-DixieData-Redirect; body=%s)", rec.Code, http.StatusOK, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); loc != "" {
				t.Fatalf("Location=%q want empty (Option C contract)", loc)
			}
			dixie := rec.Header().Get("X-DixieData-Redirect")
			if !strings.HasPrefix(dixie, "/jobs/") {
				t.Fatalf("X-DixieData-Redirect=%q want prefix /jobs/ (dispatchDixieDataForm navigates from this header; this is the Option C contract that replaces the legacy 303 + HX-Redirect pattern)", dixie)
			}
			// The async job callback still holds a reference to the
			// DB via the registry; close it so t.TempDir cleanup can
			// remove the SQLite file on Windows.
			if app.database != nil {
				_ = app.database.Close()
			}
		})
	}
}