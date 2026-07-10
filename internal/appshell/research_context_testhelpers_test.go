package appshell

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

"github.com/valueforvalue/DixieData/internal/testtemp"
	"github.com/valueforvalue/DixieData/internal/cookies"
	"github.com/valueforvalue/DixieData/internal/db"
"github.com/valueforvalue/DixieData/internal/appdata"
)

func newPickerApp(t *testing.T) *App {
	t.Helper()
	dataDir := filepath.Join(testtemp.New(t).Path(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	app := NewApp()
	app.WithFrontendAssets(os.DirFS(repoFixturePath(t, "frontend")))
	app.dataDir = dataDir
	app.database = database
	if err := app.reloadServices(); err != nil {
		t.Fatalf("reloadServices: %v", err)
	}
	configureTestIdentity(t, app)

	cookiesDir := appdata.CookiesDir(dataDir)
	key, err := cookies.EnsureKey(cookiesDir)
	if err != nil {
		t.Fatalf("cookies.EnsureKey: %v", err)
	}
	app.personCtxKey = key

	// Issue #378 slice 1: seed sentinel Person Record at id=411 so the
	// Continue shortcut has something to resolve against. The picker
	// test fixtures hardcode personID = 411 (see
	// TestHandleResearchPickerRendersContinueFromCookie +
	// TestHandleResearchSelectWritesCookieAndRedirects). Direct
	// INSERT bypasses auto-increment so we don't have to seed 411
	// rows per test.
	conn := app.database.Conn()
	if _, err := conn.Exec(
		`INSERT INTO soldiers (id, display_id, sync_id, entry_type, first_name, last_name, created_at, updated_at)
		 VALUES (?, ?, ?, 'soldier', ?, ?, ?, ?)`,
		int64(411),
		"CSA-PICKER-411",
		"picker-411",
		"Test",
		"PersonFourEleven",
		"2026-07-07T00:00:00Z",
		"2026-07-07T00:00:00Z",
	); err != nil {
		t.Fatalf("seed sentinel Person Record 411: %v", err)
	}

	app.setupRoutes()
	return app
}

func writePersonCtxCookie(t *testing.T, req *http.Request, app *App, personID int64) {
	t.Helper()
	if app.personCtxKey == nil {
		t.Fatalf("app.personCtxKey is nil")
	}
	rec := httptest.NewRecorder()
	if err := cookies.WritePersonCtx(rec, app.personCtxKey, personID); err != nil {
		t.Fatalf("WritePersonCtx: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookies.CookieName {
			req.AddCookie(c)
			return
		}
	}
	t.Fatalf("WritePersonCtx did not emit %q cookie", cookies.CookieName)
}