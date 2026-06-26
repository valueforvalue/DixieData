package archive

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
)

const diagnosticsFormatName = "dixiedata-diagnostic-bundle"
const diagnosticsBundleVersion = 2

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

type DiagnosticsService struct {
	db      *db.DB
	soldier *SoldierService
}

func NewDiagnosticsService(database *db.DB, soldier *SoldierService) *DiagnosticsService {
	return &DiagnosticsService{db: database, soldier: soldier}
}

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
		if err := addBackupImages(zipWriter, filepath.Join(dataDir, "images")); err != nil {
			return err
		}
		if err := addBackupImages(zipWriter, filepath.Join(dataDir, "scratchpads")); err != nil {
			return err
		}
		return addBackupImages(zipWriter, filepath.Join(dataDir, "logs"))
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
	logFiles, err := countFilesUnder(filepath.Join(dataDir, "logs"))
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
			"DIXIEDATA_DATA_DIR":     os.Getenv("DIXIEDATA_DATA_DIR"),
			"DIXIEDATA_DEBUG_UI_IDS": os.Getenv("DIXIEDATA_DEBUG_UI_IDS"),
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
// bundle stays bounded in size.
func addBackupLogFiles(zipWriter *zip.Writer, logsDir string) error {
	feedbackPath := filepath.Join(logsDir, "feedback-log.jsonl")
	if err := addBackupImages(zipWriter, feedbackPath); err != nil {
		return err
	}
	appLogPath := filepath.Join(logsDir, "app.log.jsonl")
	return addTruncatedLogFile(zipWriter, appLogPath, "logs/app.log.jsonl", 1000)
}

// addTruncatedLogFile reads up to the last 4 MB of the source log
// and writes at most maxLines lines to the zip entry. Returns nil
// silently if the source file does not exist (no log yet).
func addTruncatedLogFile(zipWriter *zip.Writer, srcPath, entryName string, maxLines int) error {
	f, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

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
	if len(lines) > maxLines {
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

// (no fmt usage here; package-level fmt imported only if needed by future edits)

func DiagnosticsBundleName(now time.Time) string {
	return "dixiedata-bug-report-" + now.Format("2006-01-02") + ".zip"
}
