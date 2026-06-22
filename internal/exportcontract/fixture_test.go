// Package exportcontract owns the byte-identical PDF snapshot
// contract between internal/archive.ExportService and tools/tune.
// Both code paths render the same (template, record) tuple through
// the same export service. A snapshot test pins the output bytes
// so any drift between the two surfaces is caught by `go test`.
//
// Issue #69 step 4.
package exportcontract

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
)

// FixturePath returns the path to the fixture data directory. The
// fixture is built once per test process by BuildFixture.
func FixturePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := buildFixture(dir); err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return dir
}

// buildFixture creates a deterministic 5-record SQLite in dataDir.
// Records: 1 soldier, 1 widow, 1 wife, 1 linked_person, 1 soldier
// with primary image. All fields populated so the typst renderer
// exercises every render path (image staging, group divider,
// spouse linkage, etc).
func buildFixture(dataDir string) error {
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	defer database.Close()
	svc := records.NewSoldierService(database)

	// 1. Soldier with primary image.
	s1, err := svc.Create(models.Soldier{
		DisplayID:               "FIX-00001",
		Prefix:                  "Capt.",
		FirstName:               "John",
		MiddleName:              "Bell",
		LastName:                "Hood",
		Suffix:                  "Jr.",
		Unit:                    "Texas Brigade",
		RankIn:                  "Captain",
		RankOut:                 "General",
		PensionState:            "Texas",
		PensionID:               "P-001",
		ApplicationID:           "A-001",
		ConfederateHomeStatus:   "Trustee",
		ConfederateHomeName:     "Texas Confederate Home",
		BirthDate:               "06/01/1831",
		DeathDate:               "08/26/1879",
		BuriedIn:                "Marshall Cemetery",
		Biography:               strings.Repeat("Fixture biography for record one. ", 20),
		EntryType:               "soldier",
	})
	if err != nil {
		return err
	}
	if err := writeFixtureImage(dataDir, "FIX-00001.png"); err != nil {
		return err
	}
	if err := svc.AddImage(s1.ID, "FIX-00001.png",
		filepath.ToSlash(filepath.Join("images", "FIX-00001", "FIX-00001.png")),
		""); err != nil {
		return err
	}

	// 2. Widow.
	if _, err := svc.Create(models.Soldier{
		DisplayID:       "FIX-00002",
		FirstName:       "Mary",
		LastName:        "Hood",
		MaidenName:      "Jones",
		SpouseSoldierID: s1.ID,
		PensionID:       "WP-002",
		ApplicationID:   "WA-002",
		PensionState:    "Texas",
		EntryType:       "widow",
	}); err != nil {
		return err
	}

	// 3. Wife.
	if _, err := svc.Create(models.Soldier{
		DisplayID:       "FIX-00003",
		FirstName:       "Sarah",
		LastName:        "Lee",
		MaidenName:      "Carter",
		SpouseSoldierID: s1.ID,
		PensionID:       "WP-003",
		ApplicationID:   "WA-003",
		PensionState:    "Virginia",
		EntryType:       "wife",
	}); err != nil {
		return err
	}

	// 4. Linked person (brother).
	if _, err := svc.Create(models.Soldier{
		DisplayID:         "FIX-00004",
		FirstName:         "James",
		LastName:          "Hood",
		SpouseSoldierID:   s1.ID,
		RelationshipLabel: "Brother",
		EntryType:         "linked_person",
	}); err != nil {
		return err
	}

	// 5. Soldier with a different unit so grouping tests have 2+ groups.
	if _, err := svc.Create(models.Soldier{
		DisplayID:     "FIX-00005",
		FirstName:     "Robert",
		LastName:      "Lee",
		Unit:          "Virginia Cavalry",
		PensionState:  "Virginia",
		BirthDate:     "01/19/1807",
		DeathDate:     "10/12/1870",
		EntryType:     "soldier",
	}); err != nil {
		return err
	}

	// Pin created_at / updated_at / last_edited_at to a fixed
	// timestamp so the fixture is byte-deterministic across test
	// runs. Without this, AddImage's touchAuditFields writes the
	// current time and the rendered PDF (which embeds the timestamp
	// via the "Made with DixieData | Build: ..." footer) differs
	// run-to-run. With these pins, the same fixture renders to the
	// same bytes indefinitely.
	return pinFixtureTimestamps(database)
}

// pinFixtureTimestamps normalises every timestamp column in the
// fixture to a fixed value. SQLite CURRENT_TIMESTAMP is replaced
// with this literal so every test run renders the same bytes.
func pinFixtureTimestamps(database *db.DB) error {
	fixed := "2020-01-01 00:00:00"
	stmts := []string{
		`UPDATE soldiers SET created_at = ?, updated_at = ?, last_edited_at = ?`,
		`UPDATE research_tasks SET created_at = ?, updated_at = ?, resolved_at = ?`,
		`UPDATE research_collections SET created_at = ?, updated_at = ?`,
	}
	for _, stmt := range stmts {
		if _, err := database.Conn().Exec(stmt, fixed, fixed, fixed); err != nil {
			return fmt.Errorf("pin timestamps (%s): %w", stmt[:30], err)
		}
	}
	return nil
}

// writeFixtureImage writes a 64x64 PNG portrait to disk at the
// standard image directory location. Used so the fixture's image
// rendering exercises the staging path.
func writeFixtureImage(dataDir, name string) error {
	relDir := filepath.Join("images", "FIX-00001")
	absDir := filepath.Join(dataDir, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return err
	}
	const w, h = 64, 64
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: 120, B: 70, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(absDir, name), buf.Bytes(), 0o644)
}

// pngFixture returns PNG bytes for tests that need a buffer (not
// a file). Used by tests that build image payloads without touching
// the filesystem.
func pngFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 180, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}