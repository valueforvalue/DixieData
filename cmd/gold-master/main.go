package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/services"
)

type checkResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type report struct {
	Mode          string            `json:"mode"`
	AppVersion    string            `json:"app_version"`
	SchemaVersion int               `json:"schema_version"`
	Checks        []checkResult     `json:"checks,omitempty"`
	Metrics       map[string]any    `json:"metrics,omitempty"`
	Artifacts     map[string]string `json:"artifacts,omitempty"`
	Notes         []string          `json:"notes,omitempty"`
	GeneratedAt   string            `json:"generated_at"`
}

type sampleFixture struct {
	DataDir      string
	Soldier      *models.Soldier
	Spouse       *models.Soldier
	PrimaryImage string
	Secondary    string
}

func main() {
	mode := flag.String("mode", "output-audit", "output-audit or benchmark")
	reportDir := flag.String("report-dir", "", "Directory where reports and artifacts are written")
	dataDir := flag.String("data-dir", "", "Existing data directory for benchmark mode")
	flag.Parse()

	targetReportDir := strings.TrimSpace(*reportDir)
	if targetReportDir == "" {
		targetReportDir = filepath.Join("tests", "goldmaster", "artifacts", strings.TrimSpace(*mode))
	}
	if err := os.MkdirAll(targetReportDir, 0o755); err != nil {
		fail(err)
	}

	var out report
	var err error
	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "output-audit":
		out, err = runOutputAudit(targetReportDir)
	case "portability-audit":
		out, err = runPortabilityAudit(targetReportDir)
	case "benchmark":
		if strings.TrimSpace(*dataDir) == "" {
			fail(fmt.Errorf("-data-dir is required for benchmark mode"))
		}
		out, err = runBenchmark(targetReportDir, *dataDir)
	default:
		fail(fmt.Errorf("unsupported mode %q", *mode))
	}
	if err != nil {
		fail(err)
	}

	payload, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fail(err)
	}
	reportPath := filepath.Join(targetReportDir, "report.json")
	if err := os.WriteFile(reportPath, payload, 0o644); err != nil {
		fail(err)
	}
	fmt.Println(string(payload))
}

func runOutputAudit(reportDir string) (report, error) {
	dataDir := filepath.Join(reportDir, "output-audit-data")
	if err := os.RemoveAll(dataDir); err != nil {
		return report{}, err
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return report{}, err
	}
	defer database.Close()
	if _, err := database.ConfigureUserIdentity("Gold", "Master", "Harness", 1890); err != nil {
		return report{}, err
	}

	soldierSvc := services.NewSoldierService(database)
	exportSvc := services.NewExportService(database, soldierSvc)
	exportSvc.SetDataDir(dataDir)
	backupSvc := services.NewBackupService(database, soldierSvc)
	analyticsSvc := services.NewAnalyticsService(database)

	fixture, err := seedFixture(dataDir, database, soldierSvc)
	if err != nil {
		return report{}, err
	}

	artifactsDir := filepath.Join(reportDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return report{}, err
	}
	soldierPDF := filepath.Join(artifactsDir, "soldier-detail.pdf")
	analyticsPDF := filepath.Join(artifactsDir, "analytics-summary.pdf")
	registryPDF := filepath.Join(artifactsDir, "full-archive.pdf")
	backupPath := filepath.Join(artifactsDir, "archive.ddbak")
	sharedPath := filepath.Join(artifactsDir, "archive.ddshare")
	staticZipPath := filepath.Join(artifactsDir, "static-archive.zip")
	staticDir := filepath.Join(artifactsDir, "static-archive")

	if err := exportSvc.ExportSoldierPDF(soldierPDF, *fixture.Spouse, archive.PDFOptions{}); err != nil {
		return report{}, err
	}
	snapshot, err := analyticsSvc.Snapshot()
	if err != nil {
		return report{}, err
	}
	if err := exportSvc.ExportAnalyticsSummaryPDF(analyticsPDF, snapshot, archive.PDFOptions{}); err != nil {
		return report{}, err
	}
	if err := exportSvc.ExportFullDatabasePDF(registryPDF, services.PrintSettings{}); err != nil {
		return report{}, err
	}
	if _, err := backupSvc.Export(backupPath, dataDir); err != nil {
		return report{}, err
	}
	if _, err := backupSvc.ExportShared(sharedPath, dataDir); err != nil {
		return report{}, err
	}
	if err := exportSvc.ExportStaticArchive(staticZipPath, dataDir); err != nil {
		return report{}, err
	}
	if err := extractZip(staticZipPath, staticDir); err != nil {
		return report{}, err
	}

	soldierPDFBytes, err := os.ReadFile(soldierPDF)
	if err != nil {
		return report{}, err
	}
	analyticsPDFBytes, err := os.ReadFile(analyticsPDF)
	if err != nil {
		return report{}, err
	}
	registryPDFBytes, err := os.ReadFile(registryPDF)
	if err != nil {
		return report{}, err
	}

	backupEntries, backupManifest, err := readZipWithManifest(backupPath)
	if err != nil {
		return report{}, err
	}
	_, sharedManifest, err := readZipWithManifest(sharedPath)
	if err != nil {
		return report{}, err
	}

	extractedDB := filepath.Join(reportDir, "extracted-backup.db")
	if err := os.WriteFile(extractedDB, backupEntries["data/dixiedata.db"], 0o644); err != nil {
		return report{}, err
	}
	userVersion, err := sqliteUserVersion(extractedDB)
	if err != nil {
		return report{}, err
	}

	restoreDir := filepath.Join(reportDir, "restored-data")
	if err := os.RemoveAll(restoreDir); err != nil {
		return report{}, err
	}
	if _, err := backupSvc.Import(backupPath, restoreDir); err != nil {
		return report{}, err
	}
	restoredDB, err := db.Open(restoreDir)
	if err != nil {
		return report{}, err
	}
	defer restoredDB.Close()
	restoredSvc := services.NewSoldierService(restoredDB)
	restoredSoldier, err := restoredSvc.GetByID(fixture.Soldier.ID)
	if err != nil {
		return report{}, err
	}

	targetDir := filepath.Join(reportDir, "shared-target")
	if err := os.RemoveAll(targetDir); err != nil {
		return report{}, err
	}
	targetDB, err := db.Open(targetDir)
	if err != nil {
		return report{}, err
	}
	defer targetDB.Close()
	if _, err := targetDB.ConfigureUserIdentity("Local", "Merge", "Owner", 1911); err != nil {
		return report{}, err
	}
	targetSoldierSvc := services.NewSoldierService(targetDB)
	targetBackupSvc := services.NewBackupService(targetDB, targetSoldierSvc)
	sharedSummary, err := targetBackupSvc.ImportSharedBackup(sharedPath, targetDir)
	if err != nil {
		return report{}, err
	}

	staticData, err := os.ReadFile(filepath.Join(staticDir, "archive_data.js"))
	if err != nil {
		return report{}, err
	}
	viewerHTML, err := os.ReadFile(filepath.Join(staticDir, "viewer.html"))
	if err != nil {
		return report{}, err
	}

	checks := []checkResult{
		check("detail-pdf-record-labels",
			strings.Contains(string(soldierPDFBytes), "Record Type") &&
				strings.Contains(string(soldierPDFBytes), "Widow") &&
				strings.Contains(string(soldierPDFBytes), "Maiden Name"),
			"Soldier detail PDF includes widow entry-type fields."),
		check("detail-pdf-married-to-id-cleanup",
			!strings.Contains(string(soldierPDFBytes), "Married to ID"),
			"Detail PDF no longer prints the redundant Married to ID label."),
		check("analytics-pdf-sections",
			strings.Contains(string(analyticsPDFBytes), "Archive Summary Report") &&
				strings.Contains(string(analyticsPDFBytes), "Record Types"),
			"Analytics PDF contains the summary sections expected by the current export layout."),
		check("registry-pdf-entry-types",
			strings.Contains(string(registryPDFBytes), "Printable Archive Registry") &&
				strings.Contains(string(registryPDFBytes), "Widow") &&
				strings.Contains(string(registryPDFBytes), "Soldier"),
			"Full archive PDF includes current entry-type labels."),
		check("backup-manifest-schema",
			backupManifest.SchemaVersion == buildinfo.SchemaVersion && userVersion == buildinfo.SchemaVersion,
			fmt.Sprintf("Backup manifest and embedded SQLite both report schema %d.", buildinfo.SchemaVersion)),
		check("backup-sharded-image-path",
			hasFile(backupEntries, filepath.ToSlash(fixture.PrimaryImage)) &&
				hasFile(backupEntries, filepath.ToSlash(fixture.Secondary)),
			"Backup contains the sharded image paths recorded in the database."),
		check("backup-roundtrip-records",
			restoredSoldier.DisplayID == fixture.Soldier.DisplayID &&
				len(restoredSoldier.Records) == len(fixture.Soldier.Records) &&
				len(restoredSoldier.Images) == 2 &&
				strings.Contains(restoredSoldier.Notes, "Gold master"),
			"Backup export/import round-trips record content, notes, and images."),
		check("shared-archive-import",
			sharedManifest.SchemaVersion == buildinfo.SchemaVersion &&
				sharedSummary.SoldiersInserted == 2 &&
				sharedSummary.PendingConflicts == 0,
			"Shared archive imports cleanly into a second archive."),
		check("static-archive-crosslinks",
			strings.Contains(string(staticData), fixture.Spouse.DisplayID) &&
				strings.Contains(string(staticData), `"spouseDisplayId": "`+fixture.Soldier.DisplayID+`"`) &&
				strings.Contains(string(viewerHTML), "Family Links") &&
				strings.Contains(string(viewerHTML), "Archive Metadata"),
			"Static archive viewer exposes family links and metadata for linked records."),
	}

	return report{
		Mode:          "output-audit",
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		Checks:        checks,
		Artifacts: map[string]string{
			"soldier_pdf":      soldierPDF,
			"analytics_pdf":    analyticsPDF,
			"full_archive_pdf": registryPDF,
			"backup_archive":   backupPath,
			"shared_archive":   sharedPath,
			"static_archive":   staticDir,
		},
		Notes: []string{
			"The suite validates against the current runtime schema version rather than the older 1.0.14 baseline referenced in the brief.",
		},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func runBenchmark(reportDir, dataDir string) (report, error) {
	database, err := db.Open(dataDir)
	if err != nil {
		return report{}, err
	}
	defer database.Close()
	if _, err := database.ConfigureUserIdentity("Scale", "Stress", "Harness", 1890); err != nil {
		return report{}, err
	}

	soldierSvc := services.NewSoldierService(database)
	exportSvc := services.NewExportService(database, soldierSvc)
	exportSvc.SetDataDir(dataDir)
	analyticsSvc := services.NewAnalyticsService(database)

	totalSoldiers, err := countRows(database.Conn(), "soldiers")
	if err != nil {
		return report{}, err
	}
	totalImages, err := countRows(database.Conn(), "images")
	if err != nil {
		return report{}, err
	}

	snapshotStart := time.Now()
	snapshot, err := analyticsSvc.Snapshot()
	if err != nil {
		return report{}, err
	}
	snapshotDuration := time.Since(snapshotStart)

	renderStart := time.Now()
	var rendered bytes.Buffer
	if err := presentation.InsightsView(snapshot).Render(context.Background(), &rendered); err != nil {
		return report{}, err
	}
	renderDuration := time.Since(renderStart)

	pdfPath := filepath.Join(reportDir, "benchmark-full-archive.pdf")
	exportStart := time.Now()
	if err := exportSvc.ExportFullDatabasePDF(pdfPath, services.PrintSettings{}); err != nil {
		return report{}, err
	}
	exportDuration := time.Since(exportStart)
	info, err := os.Stat(pdfPath)
	if err != nil {
		return report{}, err
	}

	return report{
		Mode:          "benchmark",
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		Metrics: map[string]any{
			"soldiers":                       totalSoldiers,
			"images":                         totalImages,
			"insights_snapshot_ms":           snapshotDuration.Milliseconds(),
			"insights_render_ms":             renderDuration.Milliseconds(),
			"full_archive_pdf_export_ms":     exportDuration.Milliseconds(),
			"full_archive_pdf_size_bytes":    info.Size(),
			"record_type_totals":             snapshot.RecordTypes,
			"insights_render_contains_title": strings.Contains(rendered.String(), "Insights"),
		},
		Artifacts: map[string]string{
			"full_archive_pdf": pdfPath,
		},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func seedFixture(dataDir string, database *db.DB, soldierSvc *services.SoldierService) (sampleFixture, error) {
	soldier, err := soldierSvc.Create(models.Soldier{
		FirstName:             "Thomas",
		LastName:              "Carter",
		Prefix:                "Capt.",
		Suffix:                "Sr.",
		RankIn:                "Captain",
		RankOut:               "Colonel",
		Unit:                  "5th Virginia Infantry, Company B",
		PensionState:          "Virginia",
		ConfederateHomeStatus: "Trustee",
		ConfederateHomeName:   "Virginia Soldiers Home",
		BirthDate:             "11/09/1831",
		DeathDate:             "02/14/1901",
		BirthInfo:             "Orange County, Virginia",
		BuriedIn:              "Hollywood Cemetery",
		NeedsReview:           true,
		ReviewReason:          "Gold master export audit sample.",
		Notes:                 "Gold master note: linked spouse and sharded images should survive export/import.",
		Records: []models.Record{
			{RecordType: "Service Record", AppID: "SR-001", Details: "Muster roll dated 05/01/1862."},
			{RecordType: "Pension", AppID: "P-001", Details: "Filed in 1897 after relocation."},
		},
	})
	if err != nil {
		return sampleFixture{}, err
	}
	spouse, err := soldierSvc.Create(models.Soldier{
		EntryType:       "widow",
		FirstName:       "Sarah",
		LastName:        "Carter",
		MaidenName:      "Cole",
		SpouseSoldierID: soldier.ID,
		PensionID:       "WP-001",
		ApplicationID:   "WA-001",
		Notes:           "Widow pension file cross-linked to the primary soldier record.",
	})
	if err != nil {
		return sampleFixture{}, err
	}

	imageDir, imageRelDir := appdata.RecordImageDir(dataDir, soldier.DisplayID)
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return sampleFixture{}, err
	}
	primaryPath := filepath.Join(imageDir, "portrait-primary.png")
	secondaryPath := filepath.Join(imageDir, "portrait-secondary.png")
	if err := os.WriteFile(primaryPath, pngFixture(), 0o644); err != nil {
		return sampleFixture{}, err
	}
	if err := os.WriteFile(secondaryPath, pngFixture(), 0o644); err != nil {
		return sampleFixture{}, err
	}
	primaryRel := filepath.Join(imageRelDir, "portrait-primary.png")
	secondaryRel := filepath.Join(imageRelDir, "portrait-secondary.png")
	if err := soldierSvc.AddImage(soldier.ID, "portrait-primary.png", primaryRel, "Primary portrait"); err != nil {
		return sampleFixture{}, err
	}
	if err := soldierSvc.AddImage(soldier.ID, "portrait-secondary.png", secondaryRel, "Alternate portrait"); err != nil {
		return sampleFixture{}, err
	}
	refreshed, err := soldierSvc.GetByID(soldier.ID)
	if err != nil {
		return sampleFixture{}, err
	}
	for _, image := range refreshed.Images {
		if image.FileName == "portrait-secondary.png" {
			if err := soldierSvc.SetPrimaryImage(soldier.ID, image.ID); err != nil {
				return sampleFixture{}, err
			}
		}
	}
	refreshed, err = soldierSvc.GetByID(soldier.ID)
	if err != nil {
		return sampleFixture{}, err
	}

	if err := database.SaveScratchpad(soldier.DisplayID, "Gold master scratch pad note for FTS coverage."); err != nil {
		return sampleFixture{}, err
	}
	if _, err := database.Conn().Exec(`INSERT INTO duplicate_audit_findings (pair_key, left_soldier_id, right_soldier_id, finding_type, reason, highlight_fields, status, last_detected_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		fmt.Sprintf("%d:%d", soldier.ID, spouse.ID), soldier.ID, spouse.ID, "reviewed", "Linked spouse pair reviewed during portability audit.", "spouse_soldier_id,entry_type", "resolved"); err != nil {
		return sampleFixture{}, err
	}
	if _, err := database.Conn().Exec(`INSERT INTO research_tasks (soldier_id, title, notes, evidence_type, status, created_at, updated_at, resolved_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		soldier.ID, "Confirm widow pension packet", "Audit fixture task", "pension", "resolved"); err != nil {
		return sampleFixture{}, err
	}
	result, err := database.Conn().Exec(`INSERT INTO research_collections (name, description, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"Gold Master Collection", "Portability audit collection")
	if err != nil {
		return sampleFixture{}, err
	}
	collectionID, err := result.LastInsertId()
	if err != nil {
		return sampleFixture{}, err
	}
	if _, err := database.Conn().Exec(`INSERT INTO research_collection_items (collection_id, soldier_id, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`, collectionID, soldier.ID); err != nil {
		return sampleFixture{}, err
	}
	orphanDir := filepath.Join(dataDir, "images", "orphaned")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		return sampleFixture{}, err
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "orphan.png"), pngFixture(), 0o644); err != nil {
		return sampleFixture{}, err
	}

	return sampleFixture{
		DataDir:      dataDir,
		Soldier:      refreshed,
		Spouse:       spouse,
		PrimaryImage: primaryRel,
		Secondary:    secondaryRel,
	}, nil
}

func readZipWithManifest(path string) (map[string][]byte, services.BackupManifest, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, services.BackupManifest{}, err
	}
	defer reader.Close()

	files := make(map[string][]byte, len(reader.File))
	var manifest services.BackupManifest
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, services.BackupManifest{}, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, services.BackupManifest{}, err
		}
		files[file.Name] = data
		if file.Name == "manifest.json" {
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, services.BackupManifest{}, err
			}
		}
	}
	return files, manifest, nil
}

func extractZip(path, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}

	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target := filepath.Join(destination, filepath.FromSlash(file.Name))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sqliteUserVersion(path string) (int, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func countRows(conn *sql.DB, table string) (int, error) {
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func hasFile(entries map[string][]byte, path string) bool {
	_, ok := entries[strings.ReplaceAll(path, "\\", "/")]
	return ok
}

func check(name string, passed bool, detail string) checkResult {
	return checkResult{Name: name, Passed: passed, Detail: detail}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func pngFixture() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
		0x54, 0x08, 0x99, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xC9, 0xFE, 0x92,
		0xEF, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}
}
