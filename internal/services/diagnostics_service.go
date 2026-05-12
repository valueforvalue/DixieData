package services

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
)

const diagnosticsFormatName = "dixiedata-diagnostic-bundle"
const diagnosticsBundleVersion = 1

type DiagnosticsManifest struct {
	Format          string            `json:"format"`
	Version         int               `json:"version"`
	AppVersion      string            `json:"app_version"`
	SchemaVersion   int               `json:"schema_version"`
	CreatedAt       string            `json:"created_at"`
	DatabaseFile    string            `json:"database_file"`
	ImageRoot       string            `json:"image_root"`
	ScratchpadRoot  string            `json:"scratchpad_root"`
	Soldiers        int               `json:"soldiers"`
	Records         int               `json:"records"`
	Images          int               `json:"images"`
	ScratchpadFiles int               `json:"scratchpad_files"`
	GOOS            string            `json:"goos"`
	GOARCH          string            `json:"goarch"`
	Executable      string            `json:"executable"`
	DataDir         string            `json:"data_dir"`
	Environment     map[string]string `json:"environment"`
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

	file, err := os.Create(outputPath)
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	if err := writeDiagnosticsJSON(zipWriter, "manifest.json", manifest); err != nil {
		return DiagnosticsManifest{}, err
	}
	if err := addBackupFile(zipWriter, manifest.DatabaseFile, snapshotPath); err != nil {
		return DiagnosticsManifest{}, err
	}
	if err := addBackupImages(zipWriter, filepath.Join(dataDir, "images")); err != nil {
		return DiagnosticsManifest{}, err
	}
	if err := addBackupImages(zipWriter, filepath.Join(dataDir, "scratchpads")); err != nil {
		return DiagnosticsManifest{}, err
	}

	return manifest, nil
}

func (d *DiagnosticsService) buildManifest(dataDir string) (DiagnosticsManifest, error) {
	soldiers, records, images, err := countArchiveData(d.soldier)
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	scratchpadFiles, err := countFilesUnder(filepath.Join(dataDir, "scratchpads"))
	if err != nil {
		return DiagnosticsManifest{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	return DiagnosticsManifest{
		Format:          diagnosticsFormatName,
		Version:         diagnosticsBundleVersion,
		AppVersion:      buildinfo.AppVersion,
		SchemaVersion:   buildinfo.SchemaVersion,
		CreatedAt:       time.Now().Format(time.RFC3339),
		DatabaseFile:    filepath.ToSlash(filepath.Join("data", db.FileName)),
		ImageRoot:       "images/",
		ScratchpadRoot:  "scratchpads/",
		Soldiers:        soldiers,
		Records:         records,
		Images:          images,
		ScratchpadFiles: scratchpadFiles,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		Executable:      executable,
		DataDir:         dataDir,
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

func DiagnosticsBundleName(now time.Time) string {
	return "dixiedata-bug-report-" + now.Format("2006-01-02") + ".zip"
}
