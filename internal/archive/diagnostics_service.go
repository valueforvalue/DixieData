package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/records"
)

const diagnosticsFormatName = "dixiedata-diagnostic-bundle"

// diagnosticsBundleVersion tracks the on-disk shape of the bug-report
// bundle. Bumped to 3 in issue #547: the bundle gains three new
// entries (state/local_settings.json, logs/recent-errors.csv,
// ring-snapshot.json) + the manifest gains three new fields
// (LocalSettingsPath, RecentErrorsCount, RingBufferEntries). Consumer
// code that branches on the manifest's Version field can rely on
// this constant as the single source of truth.
const diagnosticsBundleVersion = 3

// DiagnosticsManifest is the metadata envelope at the top of the bug-report bundle: the source schema version, the snapshot path inside the zip, and the per-record metadata the support engineer needs to reproduce the user's issue without seeing the live database.
type DiagnosticsManifest struct {
	Format                string            `json:"format"`
	Version               int               `json:"version"`
	AppVersion            string            `json:"app_version"`
	SchemaVersion         int               `json:"schema_version"`
	CreatedAt             string            `json:"created_at"`
	DatabaseFile          string            `json:"database_file"`
	ImageRoot             string            `json:"image_root"`
	ScratchpadBridgeRoot  string            `json:"scratchpad_bridge_root"`
	LogRoot               string            `json:"log_root"`
	// Issue #547: the bundle ships three new artifacts. Each
	// has a corresponding manifest field for the support
	// engineer's at-a-glance view. LocalSettingsPath is the
	// zip entry name (constant: "state/local_settings.json")
	// or empty when the user has no local settings file.
	LocalSettingsPath  string `json:"local_settings_path,omitempty"`
	RecentErrorsPath   string `json:"recent_errors_path,omitempty"`
	RingSnapshotPath   string `json:"ring_snapshot_path,omitempty"`
	RecentErrorsCount  int    `json:"recent_errors_count"`
	RingBufferEntries  int    `json:"ring_buffer_entries"`
	Soldiers              int               `json:"soldiers"`
	Records               int               `json:"records"`
	Images                int               `json:"images"`
	Scratchpads           int               `json:"scratchpads"`
	ScratchpadBridgeFiles int               `json:"scratchpad_bridge_files"`
	LogFiles              int               `json:"log_files"`
	GOOS                  string            `json:"goos"`
	GOARCH                string            `json:"goarch"`
	Executable            string            `json:"executable"`
	DataDir               string            `json:"data_dir"`
	Environment           map[string]string `json:"environment"`
}

// DiagnosticsService produces the bug-report bundle the in-place
// update flow + the Support & Diagnostics screen surface: a zip of
// the latest retained snapshot, the active schema version, recent
// crash-log lines, and enough state for support to reproduce a
// user-reported issue without seeing the live database.
type DiagnosticsService struct {
	db      *db.DB
	soldier *SoldierService
}

// NewDiagnosticsService constructs a DiagnosticsService bound to the
// given database and soldier service. The soldier service is used to
// attach display-ID context to the bundle's per-record metadata.
func NewDiagnosticsService(database *db.DB, soldier *SoldierService) *DiagnosticsService {
	return &DiagnosticsService{db: database, soldier: soldier}
}

// Export produces the bug-report bundle zip at outputPath. Returns the per-file metadata so the UI can show the user what was included before the bundle is uploaded to support.
func (d *DiagnosticsService) Export(outputPath, dataDir string) (DiagnosticsManifest, error) {
	manifest, err := d.buildManifest(dataDir)
	if err != nil {
		return DiagnosticsManifest{}, err
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-diagnostics-*")
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	defer os.RemoveAll(tempDir)

	snapshotPath := filepath.Join(tempDir, db.FileName)
	if err := d.db.SnapshotTo(snapshotPath); err != nil {
		return DiagnosticsManifest{}, err
	}

	if err := writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		if err := writeDiagnosticsJSON(zipWriter, "manifest.json", manifest); err != nil {
			return err
		}
		if err := addBackupFile(zipWriter, manifest.DatabaseFile, snapshotPath); err != nil {
			return err
		}
		if err := addBackupImages(zipWriter, filepath.Join(dataDir, "images"), true); err != nil {
			return err
		}
		if err := addBackupImages(zipWriter, filepath.Join(dataDir, "scratchpads"), true); err != nil {
			return err
		}
		// Merge logs are app-level diagnostics, not archive data;
		// they live under .dixiedata-logs/ alongside the data dir.
		// addBackupLogFiles applies the 1000-line truncation policy
		// to app.log.jsonl so the bundle stays bounded in size
		// (issue #546); feedback-log.jsonl is bundled in full.
		if err := addBackupLogFiles(zipWriter, appdata.LogsDir(dataDir)); err != nil {
			return err
		}
		// Issue #547: bundle three new diagnostic artifacts
		// for support engineers. All three are nil-safe: a
		// missing local_settings file, an absent ring buffer,
		// or a missing app.log.jsonl are all no-ops.
		if err := addBackupLocalSettings(zipWriter, dataDir); err != nil {
			return err
		}
		if err := addBackupRecentErrorsCSV(zipWriter, appdata.LogsDir(dataDir), 50); err != nil {
			return err
		}
		return addBackupRingSnapshot(zipWriter)
	}); err != nil {
		return DiagnosticsManifest{}, err
	}

	return manifest, nil
}

func (d *DiagnosticsService) buildManifest(dataDir string) (DiagnosticsManifest, error) {
	soldiers, records, images, err := countArchiveData(d.soldier)
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	scratchpads, err := d.db.ScratchpadCount()
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	scratchpadBridgeFiles, err := countFilesUnder(filepath.Join(dataDir, "scratchpads"))
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	logFiles, err := countFilesUnder(appdata.LogsDir(dataDir))
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	return DiagnosticsManifest{
		Format:                diagnosticsFormatName,
		Version:               diagnosticsBundleVersion,
		AppVersion:            buildinfo.AppVersion,
		SchemaVersion:         buildinfo.SchemaVersion,
		CreatedAt:             time.Now().Format(time.RFC3339),
		DatabaseFile:          filepath.ToSlash(filepath.Join("data", db.FileName)),
		ImageRoot:             "images/",
		ScratchpadBridgeRoot:  "scratchpads/",
		LogRoot:               "logs/",
		LocalSettingsPath:     localSettingsEntryName(dataDir),
		RecentErrorsPath:      "logs/recent-errors.csv",
		RingSnapshotPath:      "ring-snapshot.json",
		RecentErrorsCount:     countRecentErrorsIn(appdata.LogsDir(dataDir), 50),
		RingBufferEntries:     ringBufferSnapshotSize(),
		Soldiers:              soldiers,
		Records:               records,
		Images:                images,
		Scratchpads:           scratchpads,
		ScratchpadBridgeFiles: scratchpadBridgeFiles,
		LogFiles:              logFiles,
		GOOS:                  runtime.GOOS,
		GOARCH:                runtime.GOARCH,
		Executable:            executable,
		DataDir:               dataDir,
		Environment: map[string]string{
			"DIXIEDATA_DATA_DIR": os.Getenv("DIXIEDATA_DATA_DIR"),
		},
	}, nil
}

func countArchiveData(soldierSvc *SoldierService) (int, int, int, error) {
	page := 1
	soldierCount := 0
	recordCount := 0
	imageCount := 0
	for {
		batch, _, err := soldierSvc.List(page, exportBatchSize)
		if err != nil {
			return 0, 0, 0, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := soldierSvc.GetByID(item.ID)
			if err != nil {
				return 0, 0, 0, err
			}
			soldierCount++
			recordCount += len(soldier.Records)
			imageCount += len(soldier.Images)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return soldierCount, recordCount, imageCount, nil
}

func writeDiagnosticsJSON(zipWriter *zip.Writer, name string, value interface{}) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// addBackupLogFiles writes the logs/ directory into the zip with a
// truncation policy: feedback-log.jsonl is included in full, but
// app.log.jsonl is capped at the most recent 1000 lines so the
// bundle stays bounded in size. Both entries land under the
// logs/ prefix in the zip, matching the manifest's LogRoot.
func addBackupLogFiles(zipWriter *zip.Writer, logsDir string) error {
	feedbackPath := filepath.Join(logsDir, "feedback-log.jsonl")
	if err := addTruncatedLogFile(zipWriter, feedbackPath, "logs/feedback-log.jsonl", 0); err != nil {
		return err
	}
	appLogPath := filepath.Join(logsDir, "app.log.jsonl")
	return addTruncatedLogFile(zipWriter, appLogPath, "logs/app.log.jsonl", 1000)
}

// addTruncatedLogFile reads up to the last 4 MB of the source log
// and writes at most maxLines lines to the zip entry. A maxLines
// value of 0 (or negative) means "no cap" — the file is copied in
// full. Returns nil silently if the source file does not exist
// (no log yet).
func addTruncatedLogFile(zipWriter *zip.Writer, srcPath, entryName string, maxLines int) error {
	f, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer debug.DeferCloseLog(f, "addTruncatedLogFile.f")

	const maxRead = 4 * 1024 * 1024
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()
	offset := int64(0)
	if size > maxRead {
		offset = size - maxRead
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	w, err := zipWriter.Create(entryName)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			return err
		}
	}
	return nil
}

// DiagnosticsBundleName returns the filename used for the bug-report
// zip the diagnostics walker produces, dated to the day the bundle
// was generated. The format matches what the in-place update flow
// expects for "user-generated report" attachments.
func DiagnosticsBundleName(now time.Time) string {
	return "dixiedata-bug-report-" + now.Format("2006-01-02") + ".zip"
}

// localSettingsEntryName returns the zip entry name the
// local_settings.json will land at (when the file exists) or
// empty string (when the file is absent). Used by buildManifest
// to populate DiagnosticsManifest.LocalSettingsPath.
func localSettingsEntryName(dataDir string) string {
	src := records.LocalSettingsPath(dataDir)
	if _, err := os.Stat(src); err != nil {
		return ""
	}
	return "state/local_settings.json"
}

// countRecentErrorsIn returns the number of level=ERROR lines
// the addBackupRecentErrorsCSV helper would emit. Used by
// buildManifest to populate DiagnosticsManifest.RecentErrorsCount
// without a second pass over the log file. Walks the last 5000
// lines of app.log.jsonl (the same window addBackupRecentErrorsCSV
// reads), filters level=ERROR, returns min(N, max) where N is
// the actual ERROR count.
func countRecentErrorsIn(logsDir string, max int) int {
	logPath := filepath.Join(logsDir, "app.log.jsonl")
	lines := readLastNLogLines(logPath, 5000)
	count := 0
	for _, line := range lines {
		if isErrorJSONL(line) {
			count++
		}
	}
	if count > max {
		count = max
	}
	return count
}

// ringBufferSnapshotSize returns the size of the current ring
// buffer's Snapshot(), or 0 when the ring is nil. Used by
// buildManifest to populate DiagnosticsManifest.RingBufferEntries.
func ringBufferSnapshotSize() int {
	rb := debug.GetRingBuffer()
	if rb == nil {
		return 0
	}
	return len(rb.Snapshot())
}

// addBackupLocalSettings copies state/local_settings.json into
// the zip when the file exists at records.LocalSettingsPath(dataDir).
// Nil-safe: a missing file is a no-op (the support engineer
// sees the absence in the manifest's LocalSettingsPath = "" field).
//
// The file is copied verbatim — not synthesized from defaults —
// so the support engineer sees the user's actual preferences
// (theme, debug-mode, font-scale).
func addBackupLocalSettings(zipWriter *zip.Writer, dataDir string) error {
	src := records.LocalSettingsPath(dataDir)
	if _, err := os.Stat(src); err != nil {
		// File absent (fresh install) — silently skip. The
		// manifest's LocalSettingsPath = "" already signals
		// the absence; a zero-byte placeholder entry would
		// be misleading.
		return nil
	}
	return addBackupFile(zipWriter, "state/local_settings.json", src)
}

// addBackupRecentErrorsCSV reads the last `maxLines` of
// logs/app.log.jsonl, filters level=ERROR lines, takes the
// most recent `max` of them, and writes them as a CSV file
// to the zip. The CSV header is `time,level,component,msg,attrs`
// (the attrs column is a semicolon-joined key=value summary
// of the JSONL line's top-level keys other than time/level/msg).
//
// Nil-safe: a missing app.log.jsonl is a no-op.
//
// We do NOT walk every line of the log file (which can be
// hundreds of MB); we read the last 4 MB (matching
// addBackupTruncatedLogFile's window) and parse only the
// JSONL lines that look like errors. Keeps the CSV build
// O(recent ERROR count) not O(log size).
func addBackupRecentErrorsCSV(zipWriter *zip.Writer, logsDir string, max int) error {
	logPath := filepath.Join(logsDir, "app.log.jsonl")
	if _, err := os.Stat(logPath); err != nil {
		return nil // no app log — silent no-op
	}
	lines := readLastNLogLines(logPath, 5000)

	type errorEntry struct {
		time      string
		level     string
		component string
		msg       string
		attrs     string
	}
	var errs []errorEntry
	for _, line := range lines {
		if !isErrorJSONL(line) {
			continue
		}
		// Parse just the fields we need; ignore parse errors
		// (malformed lines still count if they look like errors
		// by the level=ERROR substring check).
		var obj map[string]any
		if json.Unmarshal([]byte(line), &obj) == nil {
			errs = append(errs, errorEntry{
				time:      stringOf(obj["time"]),
				level:     stringOf(obj["level"]),
				component: stringOf(obj["component"]),
				msg:       stringOf(obj["msg"]),
				attrs:     attrsSummary(obj),
			})
		} else {
			errs = append(errs, errorEntry{msg: line})
		}
	}
	// Take the most recent `max` (errs is in file order = time
	// order for the JSONL append-only log).
	if len(errs) > max {
		errs = errs[len(errs)-max:]
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"time", "level", "component", "msg", "attrs"})
	for _, e := range errs {
		_ = w.Write([]string{e.time, e.level, e.component, e.msg, e.attrs})
	}
	w.Flush()
	entry, err := zipWriter.Create("logs/recent-errors.csv")
	if err != nil {
		return err
	}
	_, err = entry.Write(buf.Bytes())
	return err
}

// addBackupRingSnapshot writes the current debug ring buffer's
// Snapshot() to ring-snapshot.json in the zip. Nil-safe: a
// nil ring buffer (debug.Configure not called yet) is a
// no-op. The snapshot is captured at export time — if the
// user restarts the app before exporting, the ring is empty
// and the entry is omitted entirely.
func addBackupRingSnapshot(zipWriter *zip.Writer) error {
	rb := debug.GetRingBuffer()
	if rb == nil {
		return nil
	}
	entries := rb.Snapshot()
	if len(entries) == 0 {
		return nil
	}
	return writeDiagnosticsJSON(zipWriter, "ring-snapshot.json", entries)
}

// readLastNLogLines reads the last up to `maxLines` lines
// of a JSONL log file. The implementation mirrors
// addTruncatedLogFile's "last 4 MB" window so the count of
// recent errors in the CSV matches the count in the
// truncated app.log.jsonl the user can also browse.
func readLastNLogLines(logPath string, maxLines int) []string {
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer debug.DeferCloseLog(f, "readLastNLogLines.f")()

	const maxRead = 4 * 1024 * 1024
	stat, err := f.Stat()
	if err != nil {
		return nil
	}
	size := stat.Size()
	offset := int64(0)
	if size > maxRead {
		offset = size - maxRead
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

// isErrorJSONL returns true if the JSONL line's `level` field
// is "ERROR" (case-sensitive, matching the slog default
// level string).
func isErrorJSONL(line string) bool {
	// Quick substring check: a JSONL line that contains
	// `"level":"ERROR"` is a level=ERROR entry. More
	// robust than a full json.Unmarshal for a 100K-line log.
	return bytes.Contains([]byte(line), []byte(`"level":"ERROR"`))
}

// attrsSummary flattens the top-level keys of a parsed
// JSONL object (other than time/level/msg/component) into
// a semicolon-joined key=value string for the CSV's
// attrs column. A nil or empty object returns "".
func attrsSummary(obj map[string]any) string {
	if len(obj) == 0 {
		return ""
	}
	skip := map[string]struct{}{
		"time": {}, "level": {}, "msg": {}, "component": {},
		"schema_version": {}, "app": {}, "version": {}, "build": {},
	}
	var b strings.Builder
	first := true
	for k, v := range obj {
		if _, ok := skip[k]; ok {
			continue
		}
		if !first {
			b.WriteByte(';')
		}
		first = false
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(stringOf(v))
	}
	return b.String()
}

// stringOf converts a generic map value to its string form
// for CSV emission. Non-string values are JSON-encoded so
// the CSV cell remains parseable by downstream tools.
func stringOf(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
