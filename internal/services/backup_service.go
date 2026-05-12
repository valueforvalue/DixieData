package services

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const backupFormatName = "dixiedata-backup"

type BackupManifest struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	AppVersion    string `json:"app_version,omitempty"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	CreatedAt     string `json:"created_at"`
	DataFormat    string `json:"data_format,omitempty"`
	DataFile      string `json:"data_file,omitempty"`
	DatabaseFile  string `json:"database_file,omitempty"`
	ImageRoot     string `json:"image_root"`
	Soldiers      int    `json:"soldiers"`
	Records       int    `json:"records"`
	Images        int    `json:"images"`
}

type BackupService struct {
	db      *db.DB
	soldier *SoldierService
}

type backupContents struct {
	Manifest BackupManifest
	FileMap  map[string]*zip.File
	Soldiers []models.Soldier
}

func NewBackupService(database *db.DB, soldier *SoldierService) *BackupService {
	return &BackupService{db: database, soldier: soldier}
}

func (b *BackupService) Export(outputPath, dataDir string) (BackupManifest, error) {
	manifest, err := b.loadBackupData()
	if err != nil {
		return BackupManifest{}, err
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-backup-export-*")
	if err != nil {
		return BackupManifest{}, err
	}
	defer os.RemoveAll(tempDir)

	snapshotPath := filepath.Join(tempDir, db.FileName)
	if err := b.db.SnapshotTo(snapshotPath); err != nil {
		return BackupManifest{}, err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	if err := writeBackupJSON(zipWriter, "manifest.json", manifest); err != nil {
		return BackupManifest{}, err
	}
	if err := addBackupFile(zipWriter, manifest.DatabaseFile, snapshotPath); err != nil {
		return BackupManifest{}, err
	}
	if err := addBackupImages(zipWriter, filepath.Join(dataDir, "images")); err != nil {
		return BackupManifest{}, err
	}

	return manifest, nil
}

func (b *BackupService) Import(backupPath, dataDir string) (BackupManifest, error) {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer reader.Close()

	contents, err := readBackupContents(&reader.Reader)
	if err != nil {
		return BackupManifest{}, err
	}

	extractDir, err := os.MkdirTemp("", "dixiedata-backup-*")
	if err != nil {
		return BackupManifest{}, err
	}
	defer os.RemoveAll(extractDir)

	if err := extractBackupImages(&reader.Reader, extractDir, contents.Manifest.ImageRoot); err != nil {
		return BackupManifest{}, err
	}

	stagingDir, err := os.MkdirTemp(filepath.Dir(dataDir), filepath.Base(dataDir)+"-import-*")
	if err != nil {
		return BackupManifest{}, err
	}
	stagingActive := true
	defer func() {
		if stagingActive {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	switch contents.Manifest.DataFormat {
	case "", "json":
		if err := b.restoreLegacyJSONBackup(stagingDir, extractDir, contents.Soldiers); err != nil {
			return BackupManifest{}, err
		}
	case "sqlite":
		if err := restoreSnapshotBackup(stagingDir, extractDir, contents); err != nil {
			return BackupManifest{}, err
		}
	default:
		return BackupManifest{}, fmt.Errorf("unsupported backup data format %q", contents.Manifest.DataFormat)
	}

	if err := validateStagedBackup(stagingDir, contents.Manifest); err != nil {
		return BackupManifest{}, err
	}
	if err := replaceDataDir(dataDir, stagingDir); err != nil {
		return BackupManifest{}, err
	}
	stagingActive = false

	return contents.Manifest, nil
}

func (b *BackupService) loadBackupData() (BackupManifest, error) {
	manifest := BackupManifest{
		Format:        backupFormatName,
		Version:       buildinfo.BackupFormatVersion,
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		CreatedAt:     time.Now().Format(time.RFC3339),
		DataFormat:    "sqlite",
		DatabaseFile:  filepath.ToSlash(filepath.Join("data", db.FileName)),
		ImageRoot:     "images/",
	}

	page := 1
	for {
		batch, _, err := b.soldier.List(page, exportBatchSize)
		if err != nil {
			return BackupManifest{}, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := b.soldier.GetByID(item.ID)
			if err != nil {
				return BackupManifest{}, err
			}
			manifest.Soldiers++
			manifest.Records += len(soldier.Records)
			manifest.Images += len(soldier.Images)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	return manifest, nil
}

func writeBackupJSON(zipWriter *zip.Writer, name string, value interface{}) error {
	writer, err := zipWriter.Create(name)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func addBackupFile(zipWriter *zip.Writer, entryName, sourcePath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	entry, err := zipWriter.Create(entryName)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, source)
	return err
}

func addBackupImages(zipWriter *zip.Writer, imageRoot string) error {
	if _, err := os.Stat(imageRoot); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return filepath.Walk(imageRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relativePath, err := filepath.Rel(filepath.Dir(imageRoot), path)
		if err != nil {
			return err
		}
		entryName := normalizeBackupPath(relativePath)
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()

		entry, err := zipWriter.Create(entryName)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, source)
		return err
	})
}

func readBackupContents(reader *zip.Reader) (backupContents, error) {
	fileMap := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		fileMap[file.Name] = file
	}

	manifestFile, ok := fileMap["manifest.json"]
	if !ok {
		return backupContents{}, fmt.Errorf("backup is missing manifest.json")
	}

	var manifest BackupManifest
	if err := readBackupJSON(manifestFile, &manifest); err != nil {
		return backupContents{}, err
	}
	if manifest.Format != backupFormatName {
		return backupContents{}, fmt.Errorf("unsupported backup format")
	}
	switch manifest.Version {
	case 1:
		manifest.DataFormat = "json"
		if manifest.DataFile == "" {
			manifest.DataFile = "data/soldiers.json"
		}
	case buildinfo.BackupFormatVersion:
		if manifest.SchemaVersion > buildinfo.SchemaVersion {
			return backupContents{}, fmt.Errorf("backup schema version %d is newer than this app supports", manifest.SchemaVersion)
		}
	default:
		return backupContents{}, fmt.Errorf("unsupported backup format version %d", manifest.Version)
	}

	contents := backupContents{
		Manifest: manifest,
		FileMap:  fileMap,
	}
	if manifest.ImageRoot == "" {
		return backupContents{}, fmt.Errorf("backup manifest is incomplete")
	}

	if manifest.DataFormat == "sqlite" {
		if manifest.DatabaseFile == "" {
			return backupContents{}, fmt.Errorf("backup manifest is incomplete")
		}
		if _, ok := fileMap[manifest.DatabaseFile]; !ok {
			return backupContents{}, fmt.Errorf("backup is missing %s", manifest.DatabaseFile)
		}
		return contents, nil
	}

	if manifest.DataFile == "" {
		return backupContents{}, fmt.Errorf("backup manifest is incomplete")
	}
	dataFile, ok := fileMap[manifest.DataFile]
	if !ok {
		return backupContents{}, fmt.Errorf("backup is missing %s", manifest.DataFile)
	}
	if err := readBackupJSON(dataFile, &contents.Soldiers); err != nil {
		return backupContents{}, err
	}

	imageEntries := make(map[string]struct{})
	for name := range fileMap {
		if strings.HasPrefix(name, manifest.ImageRoot) {
			imageEntries[name] = struct{}{}
		}
	}
	for _, soldier := range contents.Soldiers {
		for _, image := range soldier.Images {
			if _, ok := imageEntries[normalizeBackupPath(image.FilePath)]; !ok {
				return backupContents{}, fmt.Errorf("backup is missing image file %s", image.FilePath)
			}
		}
	}

	return contents, nil
}

func readBackupJSON(file *zip.File, target interface{}) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(reader).Decode(target)
}

func extractBackupImages(reader *zip.Reader, destinationRoot, imageRoot string) error {
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, imageRoot) || file.FileInfo().IsDir() {
			continue
		}
		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(file.Name))
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return err
		}
		if err := extractBackupFile(file, destinationPath); err != nil {
			return err
		}
	}
	return nil
}

func extractBackupFile(file *zip.File, destinationPath string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func (b *BackupService) restoreLegacyJSONBackup(dataDir, extractedRoot string, soldiers []models.Soldier) error {
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	defer database.Close()

	soldierSvc := NewSoldierService(database)
	for _, soldier := range soldiers {
		created, err := soldierSvc.Create(models.Soldier{
			DisplayID:     soldier.DisplayID,
			IsGenerated:   soldier.IsGenerated,
			PensionID:     soldier.PensionID,
			ApplicationID: soldier.ApplicationID,
			FirstName:     soldier.FirstName,
			MiddleName:    soldier.MiddleName,
			LastName:      soldier.LastName,
			Rank:          soldier.Rank,
			RankIn:        soldier.RankIn,
			RankOut:       soldier.RankOut,
			Unit:          soldier.Unit,
			PensionState:  soldier.PensionState,
			DeathYear:     soldier.DeathYear,
			DeathMonth:    soldier.DeathMonth,
			DeathDay:      soldier.DeathDay,
			BirthInfo:     soldier.BirthInfo,
			BuriedIn:      soldier.BuriedIn,
			Notes:         soldier.Notes,
			Records:       soldier.Records,
		})
		if err != nil {
			return err
		}

		for _, image := range soldier.Images {
			sourcePath := filepath.Join(extractedRoot, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			destinationPath := filepath.Join(dataDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
				return err
			}
			if err := copyBackupFile(sourcePath, destinationPath); err != nil {
				return err
			}
			if err := soldierSvc.AddImage(created.ID, image.FileName, image.FilePath, image.Caption); err != nil {
				return err
			}
		}
	}
	return nil
}

func restoreSnapshotBackup(dataDir, extractedRoot string, contents backupContents) error {
	databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
	if databaseFile == nil {
		return fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	if err := extractBackupFile(databaseFile, db.Path(dataDir)); err != nil {
		return err
	}
	imageSource := filepath.Join(extractedRoot, "images")
	if _, err := os.Stat(imageSource); err == nil {
		imageTarget := filepath.Join(dataDir, "images")
		if err := copyBackupTree(imageSource, imageTarget); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	return database.Close()
}

func validateStagedBackup(dataDir string, manifest BackupManifest) error {
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	defer database.Close()

	soldierSvc := NewSoldierService(database)
	page := 1
	soldierCount := 0
	recordCount := 0
	imageCount := 0
	for {
		batch, _, err := soldierSvc.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := soldierSvc.GetByID(item.ID)
			if err != nil {
				return err
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
	if soldierCount != manifest.Soldiers || recordCount != manifest.Records || imageCount != manifest.Images {
		return fmt.Errorf("backup validation mismatch: got %d soldiers, %d records, %d images", soldierCount, recordCount, imageCount)
	}
	actualImageFiles, err := countFilesUnder(filepath.Join(dataDir, "images"))
	if err != nil {
		return err
	}
	if actualImageFiles != manifest.Images {
		return fmt.Errorf("backup validation mismatch: found %d image files on disk", actualImageFiles)
	}
	return nil
}

func replaceDataDir(targetDir, stagingDir string) error {
	parent := filepath.Dir(targetDir)
	backupDir, err := os.MkdirTemp(parent, filepath.Base(targetDir)+"-previous-*")
	if err != nil {
		return err
	}
	_ = os.RemoveAll(backupDir)

	targetExists := true
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		targetExists = false
	}
	if targetExists {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return err
		}
	}
	if err := os.Rename(stagingDir, targetDir); err != nil {
		if targetExists {
			_ = os.Rename(backupDir, targetDir)
		}
		return err
	}
	if targetExists {
		return os.RemoveAll(backupDir)
	}
	return os.RemoveAll(backupDir)
}

func copyBackupFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func copyBackupTree(sourceRoot, targetRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetRoot, relativePath)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return copyBackupFile(path, targetPath)
	})
}

func countFilesUnder(root string) (int, error) {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

func normalizeBackupPath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "/")
}
