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

// TestDiag_HandleCalendarPDF_NoDialog exercises the calendar PDF
// export end-to-end through the appshell handler without going
// through Wails' native SaveFileDialog (which crashes some
// WebView2 installs on Windows — see git history). The
// saveFileDialogOverride hook bypasses the dialog.
//
// This test only runs when the user's actual .dixiedata archive
// is present (so it stays useful as a manual smoke test against
// real data without forcing CI to seed it).
func TestDiag_HandleCalendarPDF_NoDialog(t *testing.T) {
	if testing.Short() {
		t.Skip("diag: requires real .dixiedata archive")
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

	outPath := filepath.Join(t.TempDir(), "diag.pdf")
	app.saveFileDialogOverride = func(_ any) (string, error) { return outPath, nil }

	form := strings.NewReader("orientation=P&printer_friendly=1")
	req := httptest.NewRequest(http.MethodPost,
		"/calendar/6/report/pdf", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PANIC: %v", r)
		}
	}()
	app.ServeHTTP(rec, req)

	t.Logf("status=%d body=%q", rec.Code, rec.Body.String())
	if rec.Code == 0 || rec.Code >= 500 {
		t.Fatalf("handler returned %d: %s", rec.Code, rec.Body.String())
	}
	st, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("PDF not written: %v", err)
	}
	t.Logf("PDF OK: %s (%d bytes)", outPath, st.Size())
}
