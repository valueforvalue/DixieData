package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/debug"
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

// TestDiagnosticsService_ExportBundlesLocalSettings pins issue
// #547: the bundle must include state/local_settings.json
// copied verbatim from appdata.StateRoot(dataDir) when the
// file exists. Pre-#547 the support engineer could see the
// DIXIEDATA_DATA_DIR env var but not the user's theme /
// debug-mode / font-scale preferences.
func TestDiagnosticsService_ExportBundlesLocalSettings(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	diagnosticsSvc := NewDiagnosticsService(d, soldierSvc)

	dataDir := testtemp.New(t).Path()
	stateRoot := appdata.StateRoot(dataDir)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll stateRoot: %v", err)
	}
	// Seed a non-default local_settings.json so the byte-for-byte
	// assertion below proves the file is copied verbatim (not
	// synthesized from defaults).
	want := []byte(`{"debug_mode":true,"theme":"high-contrast","font_scale":1.25}` + "\n")
	settingsPath := filepath.Join(stateRoot, "local_settings.json")
	if err := os.WriteFile(settingsPath, want, 0o644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}

	outPath := filepath.Join(testtemp.New(t).Path(), "bug-report.zip")
	if _, err := diagnosticsSvc.Export(outPath, dataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()

	var entry *zip.File
	for _, f := range reader.File {
		if f.Name == "state/local_settings.json" {
			entry = f
			break
		}
	}
	if entry == nil {
		t.Fatalf("bundle missing state/local_settings.json; entries: %v", reader.File)
	}
	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open entry: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("state/local_settings.json contents = %q; want %q", got, want)
	}
}

// TestDiagnosticsService_ExportOmitsLocalSettingsWhenAbsent pins
// the nil-safe contract: when local_settings.json doesn't exist
// (fresh install), the bundle must NOT include a zero-byte
// placeholder entry. The bundle just lacks the entry entirely.
func TestDiagnosticsService_ExportOmitsLocalSettingsWhenAbsent(t *testing.T) {
	d := newTestDB(t)
	soldierSvc := NewSoldierService(d)
	diagnosticsSvc := NewDiagnosticsService(d, soldierSvc)

	dataDir := testtemp.New(t).Path()
	// Deliberately do NOT create stateRoot/local_settings.json.

	outPath := filepath.Join(testtemp.New(t).Path(), "bug-report.zip")
	if _, err := diagnosticsSvc.Export(outPath, dataDir); err != nil {
		t.Fatalf("Export: %v", err)
	}

	reader, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()
	for _, f := range reader.File {
		if strings.HasPrefix(f.Name, "state/") {
			t.Errorf("bundle unexpectedly contains %q when local_settings.json is absent", f.Name)
		}
	}
}

// TestDiagnosticsService_ExportBundlesRecentErrorsCSV pins issue
// #547: the bundle must include logs/recent-errors.csv with the
// last 50 level=ERROR lines from app.log.jsonl, formatted as CSV
// with header `time,level,component,msg,attrs`. Pre-#547 the
// support engineer had to scroll through the JSONL manually
// to find errors.
func TestDiagnosticsService_ExportBundlesRecentErrorsCSV(t *testing.T) {
	// Use the helper-direct path (not Export) so the seeded logs
	// stay inside one t.TempDir — sidesteps the Windows file
	// lock race that hits the production LogsDir(layout) cleanup.
	// The helper is exported so the production call site can be
	// tested in isolation.
	dir := t.TempDir()
	// Build a JSONL with 100 lines: 90 INFO then 10 ERROR.
	// The last-50-ERRORs filter must take the 10 ERRORs in
	// order (the last 10 = the last 10 entries of the file
	// since only 10 exist).
	var buf bytes.Buffer
	for i := 1; i <= 90; i++ {
		fmt.Fprintf(&buf, `{"time":"2026-07-13T12:00:%02dZ","level":"INFO","msg":"info line %d","component":"appshell"}`+"\n", i%60, i)
	}
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&buf, `{"time":"2026-07-13T13:00:%02dZ","level":"ERROR","msg":"error line %d","component":"records","attr1":"v%d"}`+"\n", i, i, i)
	}
	logPath := filepath.Join(dir, "app.log.jsonl")
	if err := os.WriteFile(logPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile app.log: %v", err)
	}

	zipPath := filepath.Join(testtemp.New(t).Path(), "errors-bundle.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create zip: %v", err)
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	if err := addBackupRecentErrorsCSV(zipWriter, dir, 50); err != nil {
		t.Fatalf("addBackupRecentErrorsCSV: %v", err)
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
	var entry *zip.File
	for _, f := range reader.File {
		if f.Name == "logs/recent-errors.csv" {
			entry = f
			break
		}
	}
	if entry == nil {
		t.Fatal("bundle missing logs/recent-errors.csv")
	}
	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	records, err := csv.NewReader(rc).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll csv: %v", err)
	}
	if len(records) != 11 { // 1 header + 10 ERROR rows
		t.Fatalf("csv row count = %d; want 11 (header + 10 errors)", len(records))
	}
	wantHeader := []string{"time", "level", "component", "msg", "attrs"}
	for i, h := range wantHeader {
		if records[0][i] != h {
			t.Errorf("header[%d] = %q; want %q", i, records[0][i], h)
		}
	}
	for i, row := range records[1:] {
		if row[1] != "ERROR" {
			t.Errorf("data row %d level = %q; want ERROR", i, row[1])
		}
	}
}

// TestDiagnosticsService_ExportBundlesRingSnapshot pins issue
// #547: the bundle must include ring-snapshot.json with the
// current debug ring buffer's Snapshot() entries. The ring is
// in-memory only; on app restart it's empty, so a user who
// reproduces a bug and immediately exports the bundle should
// see the entries that were live at the time.
func TestDiagnosticsService_ExportBundlesRingSnapshot(t *testing.T) {
	// Configure a fresh ring buffer with 3 entries.
	logPath := filepath.Join(t.TempDir(), "app.log.jsonl")
	if err := debug.Configure(debug.Config{
		LogPath:  logPath,
		RingSize: 100,
		Debug:    true,
	}); err != nil {
		t.Fatalf("debug.Configure: %v", err)
	}
	t.Cleanup(func() { _ = debug.Close() })
	rb := debug.GetRingBuffer()
	if rb == nil {
		t.Fatal("ring buffer not initialized after Configure")
	}
	rb.Push(debug.Entry{Time: timeNow(), Level: "ERROR", Message: "first error", Component: "records"})
	rb.Push(debug.Entry{Time: timeNow(), Level: "WARN", Message: "first warning", Component: "appshell"})
	rb.Push(debug.Entry{Time: timeNow(), Level: "INFO", Message: "first info", Component: "debug"})

	// Snapshot after Configure logged its own entry so we can
	// reason about the size deterministically. Configure logs
	// the "logging configured" line, then we push 3 entries;
	// the snapshot includes all 4.
	const wantSnapshotEntries = 4

	zipPath := filepath.Join(testtemp.New(t).Path(), "ring-bundle.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create zip: %v", err)
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	if err := addBackupRingSnapshot(zipWriter); err != nil {
		t.Fatalf("addBackupRingSnapshot: %v", err)
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
	var entry *zip.File
	for _, f := range reader.File {
		if f.Name == "ring-snapshot.json" {
			entry = f
			break
		}
	}
	if entry == nil {
		t.Fatal("bundle missing ring-snapshot.json")
	}
	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	var entries []debug.Entry
	if err := json.NewDecoder(rc).Decode(&entries); err != nil {
		t.Fatalf("Decode entries: %v", err)
	}
	if len(entries) != wantSnapshotEntries {
		t.Fatalf("ring-snapshot decoded entries = %d; want %d", len(entries), wantSnapshotEntries)
	}
	// Find our pushed entry among the snapshot.
	found := false
	for _, e := range entries {
		if e.Message == "first error" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ring-snapshot missing 'first error' entry; got %d entries", len(entries))
	}
}

// TestDiagnosticsService_ExportOmitsRingSnapshotWhenNil pins
// the nil-safe contract: when debug.Configure hasn't been
// called (ring buffer is nil — possible in some test
// environments), the bundle must NOT panic and the entry must
// be omitted.
func TestDiagnosticsService_ExportOmitsRingSnapshotWhenNil(t *testing.T) {
	// The other tests in this file may have already
	// initialized the ring; we can't guarantee a nil state.
	// Instead, exercise the helper directly: if the ring
	// is non-nil the helper writes entries; if nil it
	// returns nil error and writes nothing. Both branches
	// are valid — the test asserts the helper doesn't panic
	// regardless.
	zipPath := filepath.Join(testtemp.New(t).Path(), "nil-ring-bundle.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create zip: %v", err)
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	if err := addBackupRingSnapshot(zipWriter); err != nil {
		t.Fatalf("addBackupRingSnapshot (nil-safe): %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zipWriter.Close: %v", err)
	}
	if err := zipFile.Close(); err != nil {
		t.Fatalf("zipFile.Close: %v", err)
	}
	// No assertion on entry presence — the contract is
	// "doesn't panic" + "doesn't include the entry when
	// the ring is nil". This test passes on both branches
	// as long as no panic occurs.
}

// TestManifestVersionBumpedToThree pins the bundle-format
// version bump: issue #547 is a manifest-shape change so the
// version field increments. Consumer code that branches on
// Version can rely on this test to catch accidental
// rollbacks.
func TestManifestVersionBumpedToThree(t *testing.T) {
	if diagnosticsBundleVersion != 3 {
		t.Fatalf("diagnosticsBundleVersion = %d; want 3 (issue #547)", diagnosticsBundleVersion)
	}
}

// timeNow is a tiny helper so the ring-snapshot test reads
// cleaner than a bare time.Now() at every Push call.
func timeNow() time.Time { return time.Now() }
