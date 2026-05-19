package stress

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/records"
	_ "modernc.org/sqlite"
)

type GarbageDatabaseSummary struct {
	SoldiersInserted int
	RecordsInserted  int
	MaxPayloadBytes  int
}

func newStressDB(t *testing.T) (*db.DB, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), ".dixiedata")
	database, err := db.Open(dataDir)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if _, err := database.ConfigureUserIdentity("Stress", "Harness", "User", 1900); err != nil {
		t.Fatalf("ConfigureUserIdentity: %v", err)
	}
	return database, dataDir
}

func openExistingStressDB(dataDir string) (*db.DB, error) {
	return db.Open(dataDir)
}

func newStressServices(t *testing.T) (*db.DB, *records.SoldierService, *archive.BackupService, string) {
	t.Helper()
	database, dataDir := newStressDB(t)
	soldierSvc := records.NewSoldierService(database)
	backupSvc := archive.NewBackupService(database, soldierSvc)
	return database, soldierSvc, backupSvc, dataDir
}

func weirdPayload(index int) string {
	huge := strings.Repeat(fmt.Sprintf("STRESS-%02d-", index), 1600)
	injection := `'; DROP TABLE soldiers; --`
	emoji := "🧨🪖🧪"
	rawBytes := string([]byte{0xff, 0xfe, 0xfd, byte('A' + index%26)})
	return huge + " | " + injection + " | " + emoji + " | " + rawBytes
}

func impossibleDate(index int) string {
	dates := []string{
		"02/31/0000",
		"13/40/9999",
		"00/00/9999",
		"11/31/1200",
	}
	return dates[index%len(dates)]
}

func GenerateGarbageDatabase(dataDir string, records int) (GarbageDatabaseSummary, error) {
	database, err := db.Open(dataDir)
	if err != nil {
		return GarbageDatabaseSummary{}, err
	}
	defer database.Close()
	if _, err := database.ConfigureUserIdentity("Garbage", "Stress", "User", 1900); err != nil {
		return GarbageDatabaseSummary{}, err
	}

	summary := GarbageDatabaseSummary{}
	conn := database.Conn()
	for i := 0; i < records; i++ {
		displayID := fmt.Sprintf("GARBAGE-%05d", i+1)
		syncID, err := db.NewSyncID()
		if err != nil {
			return summary, err
		}
		payload := weirdPayload(i)
		if len(payload) > summary.MaxPayloadBytes {
			summary.MaxPayloadBytes = len(payload)
		}
		entryType := "soldier"
		if i%3 == 1 {
			entryType = "widow"
		}
		res, err := conn.Exec(
			`INSERT INTO soldiers (
				display_id, sync_id, entry_type, maiden_name, pension_id, application_id, prefix, first_name,
				middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state,
				confederate_home_status, confederate_home_name, death_year, death_month, death_day,
				birth_date, death_date, birth_info, buried_in, notes, created_at, updated_at
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
			displayID,
			syncID,
			entryType,
			payload,
			payload,
			payload,
			"Dr.",
			fmt.Sprintf("First%d", i),
			payload,
			fmt.Sprintf("Last%d", i),
			"Esq.",
			payload,
			payload,
			payload,
			payload,
			"None",
			"Trustee",
			payload,
			0,
			99,
			99,
			impossibleDate(i),
			impossibleDate(i+1),
			payload,
			payload,
			payload,
		)
		if err != nil {
			return summary, err
		}
		soldierID, err := res.LastInsertId()
		if err != nil {
			return summary, err
		}
		summary.SoldiersInserted++

		recordSyncID, err := db.NewSyncID()
		if err != nil {
			return summary, err
		}
		if _, err := conn.Exec(
			`INSERT INTO records (sync_id, soldier_id, soldier_sync_id, record_type, app_id, details) VALUES (?,?,?,?,?,?)`,
			recordSyncID,
			soldierID,
			syncID,
			payload,
			payload,
			payload,
		); err != nil {
			return summary, err
		}
		summary.RecordsInserted++
	}

	if _, err := conn.Exec(`UPDATE soldiers SET spouse_soldier_id = 2 WHERE id = 1`); err != nil {
		return summary, err
	}
	if _, err := conn.Exec(`UPDATE soldiers SET spouse_soldier_id = 1 WHERE id = 2`); err != nil {
		return summary, err
	}

	return summary, nil
}

func writePoisonArchive(path string, manifest archive.BackupManifest, files map[string][]byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	manifestWriter, err := zipWriter.Create("manifest.json")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(manifestWriter).Encode(manifest); err != nil {
		return err
	}
	for name, data := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
	}
	return zipWriter.Close()
}

func createLegacySchemaV1DB(t *testing.T, dbPath string) {
	t.Helper()
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer conn.Close()

	schemaV1 := `
CREATE TABLE soldiers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    display_id TEXT UNIQUE NOT NULL,
    is_generated BOOLEAN DEFAULT 0,
    pension_id TEXT,
    application_id TEXT,
    first_name TEXT,
    middle_name TEXT,
    last_name TEXT,
    rank TEXT,
    rank_in TEXT,
    rank_out TEXT,
    unit TEXT,
    pension_state TEXT,
    death_year INTEGER,
    death_month INTEGER,
    death_day INTEGER,
    birth_date TEXT,
    death_date TEXT,
    birth_info TEXT,
    buried_in TEXT,
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    soldier_id INTEGER REFERENCES soldiers(id) ON DELETE CASCADE,
    record_type TEXT,
    app_id TEXT,
    details TEXT
);
PRAGMA user_version = 1;
`
	if _, err := conn.Exec(schemaV1); err != nil {
		t.Fatalf("Exec schemaV1: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO soldiers (display_id, first_name, last_name, birth_date, death_date, notes) VALUES (?, ?, ?, ?, ?, ?)`,
		"TDM65-DXD-00001",
		"Legacy",
		"Bomb",
		"01/13/1842",
		"02/01/1901",
		"legacy import",
	); err != nil {
		t.Fatalf("Insert legacy soldier: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO records (soldier_id, record_type, app_id, details) VALUES (1, 'Roster', 'LEG-1', 'legacy record')`); err != nil {
		t.Fatalf("Insert legacy record: %v", err)
	}
}

func createLegacyBackupZip(t *testing.T, outputPath string) {
	t.Helper()
	legacyDBPath := filepath.Join(t.TempDir(), db.FileName)
	createLegacySchemaV1DB(t, legacyDBPath)
	manifest := archive.BackupManifest{
		Format:        "dixiedata-backup",
		Version:       buildinfo.BackupFormatVersion,
		ArchiveKind:   "backup",
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: 1,
		CreatedAt:     time.Now().Format(time.RFC3339),
		DataFormat:    "sqlite",
		DatabaseFile:  "data/dixiedata.db",
		ImageRoot:     "images/",
		Soldiers:      1,
		Records:       1,
		Images:        0,
	}
	if err := writePoisonArchive(outputPath, manifest, map[string][]byte{
		manifest.DatabaseFile: mustReadFile(t, legacyDBPath),
	}); err != nil {
		t.Fatalf("writePoisonArchive: %v", err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return data
}

func columnNames(t *testing.T, conn *sql.DB, table string) []string {
	t.Helper()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("Scan PRAGMA: %v", err)
		}
		names = append(names, name)
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func createValidCurrentBackup(t *testing.T) (string, string) {
	t.Helper()
	database, soldierSvc, backupSvc, dataDir := newStressServices(t)
	defer database.Close()

	created, err := soldierSvc.Create(models.Soldier{
		DisplayID:             "STRESS-BACKUP-1",
		Prefix:                "Capt.",
		FirstName:             "Current",
		LastName:              "Backup",
		Suffix:                "Sr.",
		PensionState:          "Virginia",
		ConfederateHomeStatus: "Trustee",
		ConfederateHomeName:   "Stress Home",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := soldierSvc.AddImage(created.ID, "tiny.png", filepath.ToSlash(filepath.Join("images", "stress", "tiny.png")), "tiny"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	imagePath := filepath.Join(dataDir, "images", "stress", "tiny.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll image: %v", err)
	}
	if err := os.WriteFile(imagePath, []byte("tiny-image"), 0o644); err != nil {
		t.Fatalf("WriteFile image: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "valid.ddbak")
	if _, err := backupSvc.Export(backupPath, dataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}
	return backupPath, dataDir
}
