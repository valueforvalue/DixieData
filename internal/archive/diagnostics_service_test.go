package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/testtemp"
)

func TestDiagnosticsService_ExportCreatesBundle(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	diagnosticsSvc := NewDiagnosticsService(d, soldierSvc)

	dataDir := testtemp.New(t).Path()
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
	// The truncation policy (issue #546) is owned by
	// TestDiagnosticsService_ExportTruncatesAppLog below. This
	// test focuses on bundle structure (images, scratchpads,
	// database snapshot) and only asserts the manifest shape;
	// writing a log file under appdata.LogsDir here would
	// race with t.TempDir() cleanup on Windows.
	_ = appdata.LogsDir(dataDir)

	outPath := filepath.Join(testtemp.New(t).Path(), "bug-report.zip")
	manifest, err := diagnosticsSvc.Export(outPath, dataDir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if manifest.Soldiers != 1 || manifest.Records != 1 || manifest.Images != 1 || manifest.Scratchpads != 1 || manifest.ScratchpadBridgeFiles != 1 || manifest.LogFiles != 0 {
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
	for _, expected := range []string{"manifest.json", "data/dixiedata.db", "images/pension-77/portrait.png", "scratchpads/PENSION-77.txt"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("bundle missing %s", expected)
		}
	}
	// No app.log.jsonl / feedback-log.jsonl seeded in this test,
	// so neither logs/* entry should appear in the bundle.
	for _, unexpected := range []string{"logs/app.log.jsonl", "logs/feedback-log.jsonl"} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("bundle unexpectedly contains %s: %s", unexpected, joined)
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

// TestAddBackupLogFiles_TruncatesAppLog pins the bug-report
// bundle's log-truncation policy (issue #546): the full
// app.log.jsonl is replaced by the last 1000 lines so the
// zip stays bounded in size for long-running debug sessions;
// feedback-log.jsonl is bundled in full under logs/. The
// helper is exercised directly (rather than through
// DiagnosticsService.Export) so the seeded logs dir lives
// inside one testtemp.New — sidestepping the Windows file-
// handle race that hits t.TempDir()'s umbrella cleanup when
// the production sibling layout puts logsDir at
// filepath.Dir(dataDir)/.dixiedata-logs/.
func TestAddBackupLogFiles_TruncatesAppLog(t *testing.T) {
	logsDir := testtemp.New(t).Path()
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll logs: %v", err)
	}

	const totalAppLines = 5000
	var appBuf bytes.Buffer
	for i := 1; i <= totalAppLines; i++ {
		appBuf.WriteString(strings.Repeat("x", i%97+1) + "\n")
	}
	if err := os.WriteFile(filepath.Join(logsDir, "app.log.jsonl"), appBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile app.log.jsonl: %v", err)
	}

	const totalFeedbackLines = 10
	var feedbackLines []string
	var feedbackBuf bytes.Buffer
	for i := 1; i <= totalFeedbackLines; i++ {
		line := strings.Repeat("y", i%5+1)
		feedbackLines = append(feedbackLines, line)
		feedbackBuf.WriteString(line + "\n")
	}
	if err := os.WriteFile(filepath.Join(logsDir, "feedback-log.jsonl"), feedbackBuf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile feedback-log.jsonl: %v", err)
	}

	zipPath := filepath.Join(testtemp.New(t).Path(), "logs-bundle.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create zip: %v", err)
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	if err := addBackupLogFiles(zipWriter, logsDir); err != nil {
		t.Fatalf("addBackupLogFiles: %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zipWriter.Close: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("zipFile.Close: %v", err)
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	var appEntry, feedbackEntry *zip.File
	for _, file := range reader.File {
		switch file.Name {
		case "logs/app.log.jsonl":
			appEntry = file
		case "logs/feedback-log.jsonl":
			feedbackEntry = file
		}
	}
	if appEntry == nil {
		t.Fatal("bundle missing logs/app.log.jsonl")
	}
	if feedbackEntry == nil {
		t.Fatal("bundle missing logs/feedback-log.jsonl")
	}

	// app.log.jsonl must be capped at the most recent 1000 lines.
	appLines := readZipLines(t, appEntry)
	if len(appLines) != 1000 {
		t.Fatalf("app.log.jsonl line count = %d; want 1000 (truncation policy)", len(appLines))
	}

	// First 5 + last 5 lines of the bundled file must match the
	// expected slice of the seeded source (lines 4001..5000).
	srcFile, err := os.Open(filepath.Join(logsDir, "app.log.jsonl"))
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	defer srcFile.Close()
	srcScanner := bufio.NewScanner(srcFile)
	srcScanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var srcLines []string
	for srcScanner.Scan() {
		srcLines = append(srcLines, srcScanner.Text())
	}
	if err := srcScanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	want := srcLines[len(srcLines)-1000:]
	for i, line := range want[:5] {
		if appLines[i] != line {
			t.Fatalf("app.log.jsonl line %d = %q; want %q", i, appLines[i], line)
		}
	}
	for i, line := range want[len(want)-5:] {
		j := len(want) - 5 + i
		if appLines[j] != line {
			t.Fatalf("app.log.jsonl line %d = %q; want %q", j, appLines[j], line)
		}
	}

	// feedback-log.jsonl must be bundled in full.
	zippedFeedback := readZipLines(t, feedbackEntry)
	if len(zippedFeedback) != totalFeedbackLines {
		t.Fatalf("feedback-log.jsonl line count = %d; want %d", len(zippedFeedback), totalFeedbackLines)
	}
	for i, line := range zippedFeedback {
		if line != feedbackLines[i] {
			t.Fatalf("feedback-log.jsonl line %d = %q; want %q", i, line, feedbackLines[i])
		}
	}
}

func readZipLines(t *testing.T, file *zip.File) []string {
	t.Helper()
	rc, err := file.Open()
	if err != nil {
		t.Fatalf("Open zip entry %s: %v", file.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll zip entry %s: %v", file.Name, err)
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan zip entry %s: %v", file.Name, err)
	}
	return lines
}
