// export_test.go — issue #629 test-package split.
//
// Exports white-box test helpers for external test packages
// (package appshell_test) so handler-level integration tests
// can boot the App and seed data without accessing unexported
// fields. The dual-package layout lets `go test` compile
// each package as a separate binary with its own timeout,
// which is the durable fix for the -race timeout (#479).
//
// Each exported function wraps the corresponding unexported
// helper. The unexported helpers stay in their original files
// so white-box unit tests that need direct field access
// (app.dataDir, app.database, app.reloadServices) continue
// to work unchanged.

package appshell

import (
	"net/http"
	"testing"

	"github.com/valueforvalue/DixieData/internal/models"
)

// NewStressApp boots a fully-wired App backed by a temp database
// with frontend assets and routes registered. Equivalent to the
// unexported newStressApp(t) used by white-box tests.
func NewStressApp(t *testing.T) *App {
	t.Helper()
	return newStressApp(t)
}

// ConfigureTestIdentity seeds the user identity row so the
// handler boot sequence doesn't 500 on the identity guard.
func ConfigureTestIdentity(t *testing.T, app *App) {
	t.Helper()
	configureTestIdentity(t, app)
}

// CreateSoldier seeds a Person Record via the soldiers facade
// and returns the persisted row. DisplayID is auto-generated;
// the label is used as LastName.
func CreateSoldier(t *testing.T, app *App, label string) models.Soldier {
	t.Helper()
	return createSoldier(t, app, label)
}

// CreateEvent seeds an Event Record via the events facade
// and returns the persisted row.
func CreateEvent(t *testing.T, app *App, kind, begin, end, description string) models.Soldier {
	t.Helper()
	return createEvent(t, app, kind, begin, end, description)
}

// ReadAll reads the full response body into a string.
func ReadAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	return readAll(t, resp)
}

// BodyExtract returns a substring of body centered on marker
// for diagnostic test-failure output.
func BodyExtract(body, marker string, radius int) string {
	return bodyExtract(body, marker, radius)
}

// IntStr formats an int64 as a string.
func IntStr(n int64) string {
	return intStr(n)
}

// RepoFixturePath resolves a path relative to the repository root.
func RepoFixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	return repoFixturePath(t, parts...)
}
