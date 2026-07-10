package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestGenerateCreatesDatabaseRecordsAndImages(t *testing.T) {
	dataDir := testtemp.New(t).Path()

	summary, err := Generate(Options{
		DataDir:  dataDir,
		Soldiers: 12,
		Seed:     42,
		Reset:    true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if summary.Soldiers != 12 {
		t.Fatalf("soldiers=%d want 12", summary.Soldiers)
	}
	if summary.Records < 12 {
		t.Fatalf("records=%d want at least 12", summary.Records)
	}
	if summary.Images < 12 {
		t.Fatalf("images=%d want at least 12", summary.Images)
	}

	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	// Issue #447: events are also soldiers rows (entry_type='event').
	// Use WHERE entry_type = 'soldier' to count only soldier-type rows.
	assertCountWhere(t, database, "soldiers", "entry_type = 'soldier'", 12)
	assertCount(t, database, "records", summary.Records)
	assertCount(t, database, "images", summary.Images)

	// Issue #447: v58-v65 surface — assert non-zero counts on
	// the new entity tables when schema >= 58.
	if summary.Events > 0 {
		assertCountWhere(t, database, "soldiers", "entry_type = 'event'", summary.Events)
		assertCount(t, database, "event_person_links", summary.EventLinks)
		assertCount(t, database, "event_sources", summary.EventSources)
		assertCount(t, database, "articles", summary.Articles)
		assertCount(t, database, "article_refs", summary.ArticleRefs)
		assertCount(t, database, "tags", summary.Tags)
		assertCount(t, database, "person_record_tags", summary.PersonRecordTags)
		if summary.EventLinks == 0 {
			t.Fatalf("event_person_links should be > 0 on v58+ schema")
		}
		if summary.Articles == 0 {
			t.Fatalf("articles should be > 0 on v58+ schema")
		}
		if summary.Tags == 0 {
			t.Fatalf("tags should be > 0 on v58+ schema")
		}
	}

	var displayID, pensionID, applicationID, middleName, rankIn, rankOut, pensionState string
	if err := database.Conn().QueryRow("SELECT display_id, pension_id, application_id, middle_name, rank_in, rank_out, pension_state FROM soldiers ORDER BY id LIMIT 1").Scan(&displayID, &pensionID, &applicationID, &middleName, &rankIn, &rankOut, &pensionState); err != nil {
		t.Fatalf("select generated identifiers: %v", err)
	}
	if displayID == "" || pensionID == "" || applicationID == "" {
		t.Fatalf("expected generated identifiers, got display=%q pension=%q application=%q", displayID, pensionID, applicationID)
	}
	if middleName == "" || rankIn == "" || rankOut == "" || pensionState == "" {
		t.Fatalf("expected new identity fields, got middle=%q rankIn=%q rankOut=%q pensionState=%q", middleName, rankIn, rankOut, pensionState)
	}

	imageFiles := 0
	if err := filepath.WalkDir(filepath.Join(dataDir, "images"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			imageFiles++
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if imageFiles != summary.Images {
		t.Fatalf("image files=%d want %d", imageFiles, summary.Images)
	}

	var storedPath string
	if err := database.Conn().QueryRow("SELECT file_path FROM images LIMIT 1").Scan(&storedPath); err != nil {
		t.Fatalf("select image path: %v", err)
	}
	if filepath.IsAbs(storedPath) {
		t.Fatalf("image path should be stored relative, got %q", storedPath)
	}
}

func assertCount(t *testing.T, database *db.DB, table string, want int) {
	t.Helper()

	var got int
	if err := database.Conn().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count=%d want %d", table, got, want)
	}
}

func assertCountWhere(t *testing.T, database *db.DB, table, where string, want int) {
	t.Helper()

	var got int
	if err := database.Conn().QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + where).Scan(&got); err != nil {
		t.Fatalf("count %s WHERE %s: %v", table, where, err)
	}
	if got != want {
		t.Fatalf("%s WHERE %s count=%d want %d", table, where, got, want)
	}
}
