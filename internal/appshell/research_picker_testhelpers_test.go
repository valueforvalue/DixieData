package appshell

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

// newPickerApp (issue #455 slice 2 successor) stands up a
// minimal App for picker-related tests. Replaces the
// newPickerApp defined before slice 2; that version called
// cookies.EnsureKey to seed personCtxKey, which is gone. The
// picker is now query-param-driven so no extra setup is needed.
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
	app.setupRoutes()
	// Touch httptest to keep the import in use even when callers
	// don't import it themselves (Go test file imports are scoped
	// to the file; this keeps the helper self-contained).
	_ = httptest.NewRecorder()
	return app
}
