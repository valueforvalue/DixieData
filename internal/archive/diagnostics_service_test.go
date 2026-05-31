package archive

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
)

func TestDiagnosticsService_ExportCreatesBundle(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	diagnosticsSvc := NewDiagnosticsService(d, soldierSvc)

	dataDir := t.TempDir()
	created, err := soldierSvc.Create(models.Soldier{
		DisplayID: "PENSION-77",
		FirstName: "Robert",
		LastName:  "Lee",
		Records:   []models.Record{{RecordType: "Roster", AppID: "APP-77", Details: "Roster details"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	imagePath := filepath.Join(dataDir, "images", "pension-77", "portrait.png")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(imagePath, pngFixture(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := soldierSvc.AddImage(created.ID, "portrait.png", `images\pension-77\portrait.png`, "Portrait"); err != nil {
		t.Fatalf("AddImage: %v", err)
	}
	if err := d.SaveScratchpad(created.DisplayID, "Canonical scratch pad notes"); err != nil {
		t.Fatalf("SaveScratchpad: %v", err)
	}

	scratchpadPath := filepath.Join(dataDir, "scratchpads", "PENSION-77.txt")
	if err := os.MkdirAll(filepath.Dir(scratchpadPath), 0o755); err != nil {
		t.Fatalf("MkdirAll scratchpads: %v", err)
	}
	if err := os.WriteFile(scratchpadPath, []byte("temporary notes"), 0o644); err != nil {
		t.Fatalf("WriteFile scratchpad: %v", err)
	}
	logPath := filepath.Join(dataDir, "logs", "shared-merge-latest.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll logs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("merge log"), 0o644); err != nil {
		t.Fatalf("WriteFile log: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "bug-report.zip")
	manifest, err := diagnosticsSvc.Export(outPath, dataDir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if manifest.Soldiers != 1 || manifest.Records != 1 || manifest.Images != 1 || manifest.Scratchpads != 1 || manifest.ScratchpadBridgeFiles != 1 || manifest.LogFiles != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	joined := strings.Join(names, "\n")
	for _, expected := range []string{"manifest.json", "data/dixiedata.db", "images/pension-77/portrait.png", "scratchpads/PENSION-77.txt", "logs/shared-merge-latest.log"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bundle missing %s", expected)
		}
	}

	var manifestFile *zip.File
	for _, file := range reader.File {
		if file.Name == "manifest.json" {
			manifestFile = file
			break
		}
	}
	if manifestFile == nil {
		t.Fatal("manifest.json missing")
	}
	rc, err := manifestFile.Open()
	if err != nil {
		t.Fatalf("Open manifest: %v", err)
	}
	defer rc.Close()
	var storedManifest DiagnosticsManifest
	if err := json.NewDecoder(rc).Decode(&storedManifest); err != nil {
		t.Fatalf("Decode manifest: %v", err)
	}
	if storedManifest.AppVersion != buildinfo.AppVersion || storedManifest.SchemaVersion != buildinfo.SchemaVersion || storedManifest.Version != diagnosticsBundleVersion {
		t.Fatalf("unexpected stored manifest: %#v", storedManifest)
	}
}
