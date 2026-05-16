package services

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const backupFormatName = "dixiedata-backup"
const (
	archiveKindBackup = "backup"
	archiveKindShared = "shared"
)

type BackupManifest struct {
	Format        string `json:"format"`
	Version       int    `json:"version"`
	ArchiveKind   string `json:"archive_kind,omitempty"`
	AppVersion    string `json:"app_version,omitempty"`
	SchemaVersion int    `json:"schema_version,omitempty"`
	NodePrefix    string `json:"node_prefix,omitempty"`
	OwnerName     string `json:"owner_name,omitempty"`
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

type SharedImportSummary struct {
	SoldiersInserted int
	SoldiersUpdated  int
	RecordsInserted  int
	RecordsUpdated   int
	ImagesInserted   int
	ImagesUpdated    int
	PendingConflicts int
	LogPath          string
}

type mergeLogger struct {
	path  string
	lines []string
}

type mergeReviewSnapshot struct {
	Soldier      models.Soldier `json:"soldier"`
	SpouseSyncID string         `json:"spouse_sync_id,omitempty"`
}

func NewBackupService(database *db.DB, soldier *SoldierService) *BackupService {
	return &BackupService{db: database, soldier: soldier}
}

func (b *BackupService) Export(outputPath, dataDir string) (BackupManifest, error) {
	return b.exportArchive(outputPath, dataDir, archiveKindBackup)
}

func (b *BackupService) ExportShared(outputPath, dataDir string) (BackupManifest, error) {
	return b.exportArchive(outputPath, dataDir, archiveKindShared)
}

func (b *BackupService) exportArchive(outputPath, dataDir, archiveKind string) (BackupManifest, error) {
	manifest, err := b.loadBackupData(archiveKind)
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
	localIdentity, preserveLocalIdentity, err := b.currentImportIdentity()
	if err != nil {
		return BackupManifest{}, err
	}
	return b.ImportWithLocalIdentity(backupPath, dataDir, localIdentity, preserveLocalIdentity)
}

func (b *BackupService) ImportWithLocalIdentity(backupPath, dataDir string, localIdentity models.UserIdentity, preserveLocalIdentity bool) (BackupManifest, error) {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return BackupManifest{}, err
	}
	defer reader.Close()

	contents, err := readBackupContents(&reader.Reader)
	if err != nil {
		return BackupManifest{}, err
	}
	if contents.Manifest.ArchiveKind != archiveKindBackup {
		return BackupManifest{}, fmt.Errorf("archive is not a backup archive")
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
		if err := preserveSnapshotImportIdentity(stagingDir, localIdentity, preserveLocalIdentity); err != nil {
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

func (b *BackupService) currentImportIdentity() (models.UserIdentity, bool, error) {
	complete, err := b.db.SystemConfig("user_identity_complete")
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	if strings.TrimSpace(complete) != "1" {
		return models.UserIdentity{}, false, nil
	}
	identity, err := b.db.UserIdentity()
	if err != nil {
		return models.UserIdentity{}, false, err
	}
	return identity, true, nil
}

func preserveSnapshotImportIdentity(dataDir string, identity models.UserIdentity, preserve bool) error {
	if !preserve {
		return nil
	}
	database, err := db.Open(dataDir)
	if err != nil {
		return err
	}
	defer database.Close()

	_, err = database.ConfigureUserIdentity(identity.FirstName, identity.MiddleName, identity.LastName, identity.BirthYear)
	return err
}

func (b *BackupService) ImportSharedBackup(backupPath, dataDir string) (summary SharedImportSummary, err error) {
	logger, logErr := newMergeLogger(dataDir)
	if logErr == nil {
		defer func() {
			status := "success"
			if err != nil {
				status = "failure"
				logger.Printf("result=failure error=%v", err)
			}
			logger.Printf("finished status=%s", status)
			if finalizeErr := logger.Close(); finalizeErr == nil {
				summary.LogPath = logger.path
				if err != nil && !strings.Contains(err.Error(), logger.path) {
					err = fmt.Errorf("%w (merge log: %s)", err, logger.path)
				}
			} else if err == nil {
				err = finalizeErr
			}
		}()
		logger.Printf("started archive=%s", strings.TrimSpace(backupPath))
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return SharedImportSummary{}, err
	}
	defer reader.Close()

	contents, err := readBackupContents(&reader.Reader)
	if err != nil {
		return SharedImportSummary{}, err
	}
	if contents.Manifest.ArchiveKind != archiveKindShared {
		return SharedImportSummary{}, fmt.Errorf("archive is not a shared archive")
	}
	if logger != nil {
		logger.Printf("manifest format=%s version=%d archive_kind=%s data_format=%s schema_version=%d node_prefix=%s owner_name=%q soldiers=%d records=%d images=%d",
			contents.Manifest.Format, contents.Manifest.Version, contents.Manifest.ArchiveKind, contents.Manifest.DataFormat, contents.Manifest.SchemaVersion, contents.Manifest.NodePrefix, contents.Manifest.OwnerName, contents.Manifest.Soldiers, contents.Manifest.Records, contents.Manifest.Images)
	}

	sessionID, err := db.NewSyncID()
	if err != nil {
		return SharedImportSummary{}, err
	}
	sessionRoot := filepath.Join(dataDir, "merge-review", sessionID)
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return SharedImportSummary{}, err
	}
	sessionActive := true
	defer func() {
		if sessionActive {
			_ = os.RemoveAll(sessionRoot)
		}
	}()

	if err := extractBackupImages(&reader.Reader, sessionRoot, contents.Manifest.ImageRoot); err != nil {
		return SharedImportSummary{}, err
	}

	switch contents.Manifest.DataFormat {
	case "", "json":
		summary, err = b.mergeSharedSoldiers(sessionID, backupPath, contents.Soldiers, sessionRoot, dataDir, logger)
	case "sqlite":
		sourceDir, err := os.MkdirTemp("", "dixiedata-shared-backup-db-*")
		if err != nil {
			return SharedImportSummary{}, err
		}
		defer os.RemoveAll(sourceDir)

		databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
		if databaseFile == nil {
			return SharedImportSummary{}, fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
		}
		if err := extractBackupFile(databaseFile, db.Path(sourceDir)); err != nil {
			return SharedImportSummary{}, fmt.Errorf("stage shared backup database: %w", err)
		}
		sourceDB, err := db.Open(sourceDir)
		if err != nil {
			return SharedImportSummary{}, fmt.Errorf("open shared backup database: %w", err)
		}
		defer sourceDB.Close()

		sourceSvc := NewSoldierService(sourceDB)
		soldiers, err := listAllSoldiers(sourceSvc)
		if err != nil {
			return SharedImportSummary{}, fmt.Errorf("read shared backup database: %w", err)
		}
		summary, err = b.mergeSharedSoldiers(sessionID, backupPath, soldiers, sessionRoot, dataDir, logger)
	default:
		return SharedImportSummary{}, fmt.Errorf("unsupported backup data format %q", contents.Manifest.DataFormat)
	}
	if err != nil {
		return SharedImportSummary{}, fmt.Errorf("merge shared backup: %w", err)
	}
	if summary.PendingConflicts == 0 {
		sessionActive = false
		_ = os.RemoveAll(sessionRoot)
	} else {
		sessionActive = false
	}
	return summary, nil
}

func (b *BackupService) loadBackupData(archiveKind string) (BackupManifest, error) {
	manifest := BackupManifest{
		Format:        backupFormatName,
		Version:       buildinfo.BackupFormatVersion,
		ArchiveKind:   archiveKind,
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		CreatedAt:     time.Now().Format(time.RFC3339),
		DataFormat:    "sqlite",
		DatabaseFile:  filepath.ToSlash(filepath.Join("data", db.FileName)),
		ImageRoot:     "images/",
	}
	nodePrefix, err := b.db.NodePrefix()
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.NodePrefix = nodePrefix
	identity, err := b.db.UserIdentity()
	if err != nil {
		return BackupManifest{}, err
	}
	manifest.OwnerName = strings.TrimSpace(strings.Join([]string{identity.FirstName, identity.MiddleName, identity.LastName}, " "))

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
		manifest.ArchiveKind = archiveKindBackup
		if manifest.DataFile == "" {
			manifest.DataFile = "data/soldiers.json"
		}
	case buildinfo.BackupFormatVersion:
		if strings.TrimSpace(manifest.ArchiveKind) == "" {
			manifest.ArchiveKind = archiveKindBackup
		}
		if manifest.ArchiveKind != archiveKindBackup && manifest.ArchiveKind != archiveKindShared {
			return backupContents{}, fmt.Errorf("unsupported archive kind %q", manifest.ArchiveKind)
		}
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
		if err := validateSQLiteBackupImageEntries(contents); err != nil {
			return backupContents{}, err
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

func validateSQLiteBackupImageEntries(contents backupContents) error {
	stageDir, err := os.MkdirTemp("", "dixiedata-backup-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageDir)

	databaseFile := contents.FileMap[contents.Manifest.DatabaseFile]
	if databaseFile == nil {
		return fmt.Errorf("backup is missing %s", contents.Manifest.DatabaseFile)
	}
	if err := extractBackupFile(databaseFile, db.Path(stageDir)); err != nil {
		return fmt.Errorf("stage backup database: %w", err)
	}
	stagedDB, err := db.Open(stageDir)
	if err != nil {
		return fmt.Errorf("open staged backup database: %w", err)
	}
	defer stagedDB.Close()

	soldierSvc := NewSoldierService(stagedDB)
	soldiers, err := listAllSoldiers(soldierSvc)
	if err != nil {
		return fmt.Errorf("read staged backup database: %w", err)
	}
	imageEntries := make(map[string]struct{})
	for name := range contents.FileMap {
		if strings.HasPrefix(name, contents.Manifest.ImageRoot) {
			imageEntries[name] = struct{}{}
		}
	}
	for _, soldier := range soldiers {
		for _, image := range soldier.Images {
			normalized := normalizeBackupPath(image.FilePath)
			if _, ok := imageEntries[normalized]; !ok {
				return fmt.Errorf("backup is missing image file %s", image.FilePath)
			}
		}
	}
	return nil
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
			DisplayID:             soldier.DisplayID,
			EntryType:             soldier.EntryType,
			MaidenName:            soldier.MaidenName,
			IsGenerated:           soldier.IsGenerated,
			SyncID:                soldier.SyncID,
			PensionID:             soldier.PensionID,
			ApplicationID:         soldier.ApplicationID,
			Prefix:                soldier.Prefix,
			FirstName:             soldier.FirstName,
			MiddleName:            soldier.MiddleName,
			LastName:              soldier.LastName,
			Suffix:                soldier.Suffix,
			Rank:                  soldier.Rank,
			RankIn:                soldier.RankIn,
			RankOut:               soldier.RankOut,
			Unit:                  soldier.Unit,
			PensionState:          soldier.PensionState,
			ConfederateHomeStatus: soldier.ConfederateHomeStatus,
			ConfederateHomeName:   soldier.ConfederateHomeName,
			BirthDate:             soldier.BirthDate,
			DeathDate:             soldier.DeathDate,
			DeathYear:             soldier.DeathYear,
			DeathMonth:            soldier.DeathMonth,
			DeathDay:              soldier.DeathDay,
			BirthInfo:             soldier.BirthInfo,
			BuriedIn:              soldier.BuriedIn,
			Notes:                 soldier.Notes,
			AddedBy:               soldier.AddedBy,
			LastEditedBy:          soldier.LastEditedBy,
			LastEditedFields:      soldier.LastEditedFields,
			LastEditedAt:          soldier.LastEditedAt,
			CreatedAt:             soldier.CreatedAt,
			UpdatedAt:             soldier.UpdatedAt,
			Records:               soldier.Records,
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
		if _, err := database.Conn().Exec(`UPDATE soldiers SET added_by = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, created_at = ?, updated_at = ? WHERE id = ?`,
			soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, created.ID); err != nil {
			return err
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
			for _, image := range soldier.Images {
				imagePath := filepath.Join(dataDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
				if _, err := os.Stat(imagePath); err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("backup validation missing image file %s", image.FilePath)
					}
					return err
				}
			}
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	if soldierCount != manifest.Soldiers || recordCount != manifest.Records || imageCount != manifest.Images {
		return fmt.Errorf("backup validation mismatch: got %d soldiers, %d records, %d images", soldierCount, recordCount, imageCount)
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

func listAllSoldiers(svc *SoldierService) ([]models.Soldier, error) {
	page := 1
	all := []models.Soldier{}
	for {
		batch, _, err := svc.List(page, exportBatchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			full, err := svc.GetByID(item.ID)
			if err != nil {
				return nil, err
			}
			all = append(all, *full)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return all, nil
}

func (b *BackupService) mergeSharedSoldiers(sessionID, archivePath string, sourceSoldiers []models.Soldier, sourceDataDir, targetDataDir string, logger *mergeLogger) (SharedImportSummary, error) {
	tx, err := b.db.Conn().Begin()
	if err != nil {
		return SharedImportSummary{}, err
	}
	defer tx.Rollback()

	summary := SharedImportSummary{}
	targetIDsBySync := make(map[string]int64, len(sourceSoldiers))
	sourceSyncByID := make(map[int64]string, len(sourceSoldiers))
	conflictedSyncs := make(map[string]struct{})

	for _, soldier := range sourceSoldiers {
		sourceSyncByID[soldier.ID] = strings.TrimSpace(soldier.SyncID)
	}
	if err := ensureMergeReviewSession(tx, sessionID, archivePath, sourceDataDir); err != nil {
		return SharedImportSummary{}, err
	}

	for _, soldier := range sourceSoldiers {
		snapshot := mergeReviewSnapshot{
			Soldier:      normalizeSharedSoldierSnapshot(soldier),
			SpouseSyncID: strings.TrimSpace(sourceSyncByID[soldier.SpouseSoldierID]),
		}
		localSnapshot, conflictType, reason, err := detectSharedConflict(tx, snapshot)
		if err != nil {
			return SharedImportSummary{}, err
		}
		if conflictType != "" {
			if err := insertMergeReviewConflict(tx, sessionID, conflictType, reason, localSnapshot, snapshot); err != nil {
				return SharedImportSummary{}, err
			}
			conflictedSyncs[snapshot.Soldier.SyncID] = struct{}{}
			summary.PendingConflicts++
			if logger != nil {
				logger.Printf("soldier action=stage-review conflict_type=%s sync_id=%s source_display_id=%s reason=%q",
					conflictType, snapshot.Soldier.SyncID, snapshot.Soldier.DisplayID, reason)
			}
			continue
		}

		targetID, existed, resolvedDisplayID, err := upsertSharedSoldier(tx, snapshot.Soldier)
		if err != nil {
			return SharedImportSummary{}, err
		}
		targetIDsBySync[snapshot.Soldier.SyncID] = targetID
		if existed {
			summary.SoldiersUpdated++
			if logger != nil {
				logger.Printf("soldier action=update sync_id=%s display_id=%s target_id=%d", snapshot.Soldier.SyncID, resolvedDisplayID, targetID)
			}
		} else {
			summary.SoldiersInserted++
			if logger != nil {
				logger.Printf("soldier action=insert sync_id=%s display_id=%s target_id=%d", snapshot.Soldier.SyncID, resolvedDisplayID, targetID)
			}
		}
	}

	for _, soldier := range sourceSoldiers {
		syncID := strings.TrimSpace(soldier.SyncID)
		if _, conflicted := conflictedSyncs[syncID]; conflicted {
			continue
		}
		targetID := targetIDsBySync[syncID]
		if targetID < 1 {
			return SharedImportSummary{}, fmt.Errorf("missing merged soldier target for sync_id %s", syncID)
		}
		spouseTargetID, err := resolveSharedSpouseTargetID(sourceSyncByID, targetIDsBySync, soldier)
		if err != nil {
			return SharedImportSummary{}, err
		}
		if _, err := tx.Exec(`UPDATE soldiers SET spouse_soldier_id = ? WHERE id = ?`, nullableInt64(spouseTargetID), targetID); err != nil {
			return SharedImportSummary{}, err
		}

		for _, record := range soldier.Records {
			existed, err := upsertSharedRecord(tx, targetID, syncID, record)
			if err != nil {
				return SharedImportSummary{}, err
			}
			if existed {
				summary.RecordsUpdated++
			} else {
				summary.RecordsInserted++
			}
		}

		for _, image := range soldier.Images {
			if err := copySharedImageFile(sourceDataDir, targetDataDir, image.FilePath); err != nil {
				return SharedImportSummary{}, err
			}
			existed, err := upsertSharedImage(tx, targetID, syncID, image)
			if err != nil {
				return SharedImportSummary{}, err
			}
			if existed {
				summary.ImagesUpdated++
			} else {
				summary.ImagesInserted++
			}
		}
	}

	if summary.PendingConflicts == 0 {
		if _, err := tx.Exec(`DELETE FROM merge_review_sessions WHERE id = ?`, sessionID); err != nil {
			return SharedImportSummary{}, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE merge_review_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
			return SharedImportSummary{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SharedImportSummary{}, err
	}
	if logger != nil {
		logger.Printf("summary soldiers_inserted=%d soldiers_updated=%d records_inserted=%d records_updated=%d images_inserted=%d images_updated=%d conflicts_pending=%d",
			summary.SoldiersInserted, summary.SoldiersUpdated, summary.RecordsInserted, summary.RecordsUpdated, summary.ImagesInserted, summary.ImagesUpdated, summary.PendingConflicts)
	}
	return summary, nil
}

func (b *BackupService) PendingMergeConflicts() ([]models.MergeReviewConflict, error) {
	rows, err := b.db.Conn().Query(`SELECT id, session_id, conflict_type, reason, COALESCE(local_soldier_id, 0), COALESCE(local_display_id, ''), source_display_id, COALESCE(resolution, ''), created_at, local_data, source_data
		FROM merge_review_conflicts
		WHERE COALESCE(resolution, '') = ''
		ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conflicts := []models.MergeReviewConflict{}
	for rows.Next() {
		var (
			conflict   models.MergeReviewConflict
			localJSON  sql.NullString
			sourceJSON string
		)
		if err := rows.Scan(&conflict.ID, &conflict.SessionID, &conflict.ConflictType, &conflict.Reason, &conflict.LocalSoldierID, &conflict.LocalDisplayID, &conflict.SourceDisplayID, &conflict.Resolution, &conflict.CreatedAt, &localJSON, &sourceJSON); err != nil {
			return nil, err
		}
		if strings.TrimSpace(localJSON.String) != "" {
			localSnapshot, err := unmarshalMergeReviewSnapshot(localJSON.String)
			if err != nil {
				return nil, err
			}
			conflict.LocalSoldier = &localSnapshot.Soldier
		}
		sourceSnapshot, err := unmarshalMergeReviewSnapshot(sourceJSON)
		if err != nil {
			return nil, err
		}
		conflict.SourceSoldier = sourceSnapshot.Soldier
		conflicts = append(conflicts, conflict)
	}
	return conflicts, rows.Err()
}

func (b *BackupService) ResolveMergeConflict(conflictID int64, decision, dataDir string) error {
	tx, err := b.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	conflict, sessionRoot, err := loadMergeReviewConflict(tx, conflictID)
	if err != nil {
		return err
	}
	switch decision {
	case "keep-local":
	case "keep-both":
		if conflict.ConflictType != "display-id-collision" {
			return fmt.Errorf("keep both is only supported for display ID collisions")
		}
		if err := applySharedConflictResolution(tx, conflict, decision, sessionRoot, dataDir); err != nil {
			return err
		}
	case "use-shared", "keep-shared":
		if err := applySharedConflictResolution(tx, conflict, decision, sessionRoot, dataDir); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported merge review decision %q", decision)
	}

	if _, err := tx.Exec(`UPDATE merge_review_conflicts SET resolution = ?, resolved_at = CURRENT_TIMESTAMP WHERE id = ?`, decision, conflictID); err != nil {
		return err
	}
	if err := finalizeMergeReviewSession(tx, conflict.SessionID, sessionRoot); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertSharedSoldier(tx *sql.Tx, soldier models.Soldier) (int64, bool, string, error) {
	syncID := strings.TrimSpace(soldier.SyncID)
	if syncID == "" {
		return 0, false, "", fmt.Errorf("shared database soldier missing sync_id")
	}

	displayID, err := resolveSharedDisplayID(tx, soldier.DisplayID, syncID)
	if err != nil {
		return 0, false, "", err
	}

	var existingID int64
	err = tx.QueryRow(`SELECT id FROM soldiers WHERE sync_id = ?`, syncID).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(`UPDATE soldiers
			SET display_id = ?, entry_type = ?, maiden_name = ?, is_generated = ?, pension_id = ?, application_id = ?, prefix = ?, first_name = ?, middle_name = ?, last_name = ?, suffix = ?, rank = ?, rank_in = ?, rank_out = ?, unit = ?, pension_state = ?, confederate_home_status = ?, confederate_home_name = ?, death_year = ?, death_month = ?, death_day = ?, birth_date = ?, death_date = ?, birth_info = ?, buried_in = ?, notes = ?, needs_review = ?, review_reason = ?, added_by = ?, last_edited_by = ?, last_edited_fields = ?, last_edited_at = ?, created_at = ?, updated_at = ?
			WHERE id = ?`,
			displayID, soldier.EntryType, soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.ConfederateHomeStatus, soldier.ConfederateHomeName, soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt, existingID)
		if err != nil {
			return 0, false, "", err
		}
		if err := refreshSoldierFTS(tx, existingID, soldier); err != nil {
			return 0, false, "", err
		}
		return existingID, true, displayID, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, "", err
	}

	res, err := tx.Exec(`INSERT INTO soldiers
		(display_id, sync_id, entry_type, spouse_soldier_id, maiden_name, is_generated, pension_id, application_id, prefix, first_name, middle_name, last_name, suffix, rank, rank_in, rank_out, unit, pension_state, confederate_home_status, confederate_home_name, death_year, death_month, death_day, birth_date, death_date, birth_info, buried_in, notes, needs_review, review_reason, added_by, last_edited_by, last_edited_fields, last_edited_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		displayID, syncID, soldier.EntryType, nil, soldier.MaidenName, soldier.IsGenerated, soldier.PensionID, soldier.ApplicationID, soldier.Prefix, soldier.FirstName, soldier.MiddleName, soldier.LastName, soldier.Suffix, soldier.Rank, soldier.RankIn, soldier.RankOut, soldier.Unit, soldier.PensionState, soldier.ConfederateHomeStatus, soldier.ConfederateHomeName, soldier.DeathYear, soldier.DeathMonth, soldier.DeathDay, soldier.BirthDate, soldier.DeathDate, soldier.BirthInfo, soldier.BuriedIn, soldier.Notes, soldier.NeedsReview, soldier.ReviewReason, soldier.AddedBy, soldier.LastEditedBy, soldier.LastEditedFields, soldier.LastEditedAt, soldier.CreatedAt, soldier.UpdatedAt)
	if err != nil {
		return 0, false, "", err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, "", err
	}
	if err := insertSoldierFTS(tx, id, soldier); err != nil {
		return 0, false, "", err
	}
	return id, false, displayID, nil
}

func refreshSoldierFTS(tx *sql.Tx, soldierID int64, soldier models.Soldier) error {
	if _, err := tx.Exec(`INSERT INTO soldiers_fts(soldiers_fts, rowid) VALUES('delete', ?)`, soldierID); err != nil {
		return err
	}
	return insertSoldierFTS(tx, soldierID, soldier)
}

func insertSoldierFTS(tx *sql.Tx, soldierID int64, soldier models.Soldier) error {
	_, err := tx.Exec(`INSERT INTO soldiers_fts(rowid, first_name, last_name, unit, soldier_rank) VALUES (?,?,?,?,?)`,
		soldierID, searchableFirstName(soldier), searchableLastName(soldier), searchableUnit(soldier), searchableRank(soldier))
	return err
}

func upsertSharedRecord(tx *sql.Tx, targetSoldierID int64, soldierSyncID string, record models.Record) (bool, error) {
	syncID := strings.TrimSpace(record.SyncID)
	if syncID == "" {
		return false, fmt.Errorf("shared database record missing sync_id")
	}
	var existingID int64
	err := tx.QueryRow(`SELECT id FROM records WHERE sync_id = ?`, syncID).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(`UPDATE records SET soldier_id = ?, soldier_sync_id = ?, record_type = ?, app_id = ?, details = ? WHERE id = ?`,
			targetSoldierID, soldierSyncID, record.RecordType, record.AppID, record.Details, existingID)
		return true, err
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = tx.Exec(`INSERT INTO records (sync_id, soldier_id, soldier_sync_id, record_type, app_id, details) VALUES (?, ?, ?, ?, ?, ?)`,
		syncID, targetSoldierID, soldierSyncID, record.RecordType, record.AppID, record.Details)
	return false, err
}

func upsertSharedImage(tx *sql.Tx, targetSoldierID int64, soldierSyncID string, image models.Image) (bool, error) {
	syncID := strings.TrimSpace(image.SyncID)
	if syncID == "" {
		return false, fmt.Errorf("shared database image missing sync_id")
	}
	var existingID int64
	err := tx.QueryRow(`SELECT id FROM images WHERE sync_id = ?`, syncID).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(`UPDATE images SET soldier_id = ?, soldier_sync_id = ?, file_name = ?, file_path = ?, caption = ?, is_primary = ? WHERE id = ?`,
			targetSoldierID, soldierSyncID, image.FileName, image.FilePath, image.Caption, image.IsPrimary, existingID)
		return true, err
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	_, err = tx.Exec(`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		syncID, targetSoldierID, soldierSyncID, image.FileName, image.FilePath, image.Caption, image.IsPrimary)
	return false, err
}

func copySharedImageFile(sourceDataDir, targetDataDir, relativePath string) error {
	normalized := normalizeBackupPath(relativePath)
	if normalized == "" {
		return nil
	}
	sourcePath := filepath.Join(sourceDataDir, filepath.FromSlash(normalized))
	if _, err := os.Stat(sourcePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("shared database is missing image file %s", relativePath)
		}
		return err
	}
	targetPath := filepath.Join(targetDataDir, filepath.FromSlash(normalized))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return copyBackupFile(sourcePath, targetPath)
}

func resolveSharedDisplayID(tx *sql.Tx, desiredDisplayID, syncID string) (string, error) {
	nodePrefix, err := nodePrefixFromTx(tx)
	if err != nil {
		return "", err
	}
	candidate := db.SanitizeID(desiredDisplayID, nodePrefix)
	if candidate == "" {
		return nextLocalGeneratedDisplayID(tx)
	}
	return ensureUniqueDisplayID(tx, candidate, syncID)
}

func ensureUniqueDisplayID(tx *sql.Tx, candidate, syncID string) (string, error) {
	var existingSync sql.NullString
	err := tx.QueryRow(`SELECT sync_id FROM soldiers WHERE display_id = ?`, candidate).Scan(&existingSync)
	if err == sql.ErrNoRows {
		return candidate, nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(existingSync.String) == strings.TrimSpace(syncID) {
		return candidate, nil
	}
	return nextLocalGeneratedDisplayID(tx)
}

func nextLocalGeneratedDisplayID(tx *sql.Tx) (string, error) {
	nodePrefix, err := nodePrefixFromTx(tx)
	if err != nil {
		return "", err
	}
	rows, err := tx.Query(`SELECT display_id, is_generated FROM soldiers`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	maxID := 0
	for rows.Next() {
		var (
			displayID   string
			isGenerated bool
		)
		if err := rows.Scan(&displayID, &isGenerated); err != nil {
			return "", err
		}
		sequence, ok := mergeGeneratedDisplayIDSequence(displayID, nodePrefix, isGenerated)
		if ok && sequence > maxID {
			maxID = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return db.NextGeneratedDisplayID(nodePrefix, maxID+1), nil
}

func nodePrefixFromTx(tx *sql.Tx) (string, error) {
	var prefix sql.NullString
	if err := tx.QueryRow(`SELECT value FROM system_config WHERE key = 'node_prefix'`).Scan(&prefix); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	return db.NormalizeNodePrefix(prefix.String), nil
}

func mergeGeneratedDisplayIDSequence(displayID, nodePrefix string, isGenerated bool) (int, bool) {
	namespace, sequence, ok := db.CanonicalDisplayID(db.SanitizeID(displayID, nodePrefix))
	if !ok {
		return 0, false
	}
	if isGenerated || strings.EqualFold(namespace, db.LegacyDisplayIDNamespace) || strings.EqualFold(namespace, db.NormalizeNodePrefix(nodePrefix)) {
		return sequence, true
	}
	return 0, false
}

func detectSharedConflict(tx *sql.Tx, source mergeReviewSnapshot) (*mergeReviewSnapshot, string, string, error) {
	localBySync, err := loadSoldierSnapshotBySync(tx, source.Soldier.SyncID)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil {
		if equivalentMergeReviewSnapshots(*localBySync, source) {
			return nil, "", "", nil
		}
		return localBySync, "soldier-update", describeSoldierConflict(*localBySync, source), nil
	}

	localByDisplay, err := loadSoldierSnapshotByDisplayID(tx, source.Soldier.DisplayID)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil && strings.TrimSpace(localByDisplay.Soldier.SyncID) != strings.TrimSpace(source.Soldier.SyncID) {
		return localByDisplay, "display-id-collision", fmt.Sprintf("Shared record %s collides with existing local record %s.", source.Soldier.DisplayID, localByDisplay.Soldier.DisplayID), nil
	}

	localByHuman, err := loadSoldierSnapshotByHumanMatch(tx, source.Soldier)
	if err != nil && err != sql.ErrNoRows {
		return nil, "", "", err
	}
	if err == nil && strings.TrimSpace(localByHuman.Soldier.SyncID) != strings.TrimSpace(source.Soldier.SyncID) {
		return localByHuman, "human-duplicate", describeHumanDuplicateConflict(*localByHuman, source), nil
	}
	return nil, "", "", nil
}

func ensureMergeReviewSession(tx *sql.Tx, sessionID, archivePath, sourceRoot string) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO merge_review_sessions (id, archive_path, source_root, status, created_at, updated_at)
		VALUES (?, ?, ?, 'open', COALESCE((SELECT created_at FROM merge_review_sessions WHERE id = ?), CURRENT_TIMESTAMP), CURRENT_TIMESTAMP)`,
		sessionID, archivePath, sourceRoot, sessionID)
	return err
}

func insertMergeReviewConflict(tx *sql.Tx, sessionID, conflictType, reason string, localSnapshot *mergeReviewSnapshot, sourceSnapshot mergeReviewSnapshot) error {
	sourceJSON, err := marshalMergeReviewSnapshot(sourceSnapshot)
	if err != nil {
		return err
	}
	localJSON := ""
	localSoldierID := int64(0)
	localDisplayID := ""
	if localSnapshot != nil {
		localJSON, err = marshalMergeReviewSnapshot(*localSnapshot)
		if err != nil {
			return err
		}
		localSoldierID = localSnapshot.Soldier.ID
		localDisplayID = localSnapshot.Soldier.DisplayID
	}
	_, err = tx.Exec(`INSERT INTO merge_review_conflicts
		(session_id, conflict_type, reason, soldier_sync_id, local_soldier_id, local_display_id, source_display_id, local_data, source_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, conflictType, reason, sourceSnapshot.Soldier.SyncID, nullableInt64(localSoldierID), localDisplayID, sourceSnapshot.Soldier.DisplayID, nullableString(localJSON), sourceJSON)
	return err
}

func loadMergeReviewConflict(tx *sql.Tx, conflictID int64) (models.MergeReviewConflict, string, error) {
	var (
		conflict    models.MergeReviewConflict
		sessionRoot string
		localJSON   sql.NullString
		sourceJSON  string
	)
	err := tx.QueryRow(`SELECT c.id, c.session_id, c.conflict_type, c.reason, COALESCE(c.local_soldier_id, 0), COALESCE(c.local_display_id, ''), c.source_display_id,
		COALESCE(c.resolution, ''), c.created_at, c.local_data, c.source_data, s.source_root
		FROM merge_review_conflicts c
		JOIN merge_review_sessions s ON s.id = c.session_id
		WHERE c.id = ?`, conflictID).
		Scan(&conflict.ID, &conflict.SessionID, &conflict.ConflictType, &conflict.Reason, &conflict.LocalSoldierID, &conflict.LocalDisplayID, &conflict.SourceDisplayID,
			&conflict.Resolution, &conflict.CreatedAt, &localJSON, &sourceJSON, &sessionRoot)
	if err != nil {
		return models.MergeReviewConflict{}, "", err
	}
	if strings.TrimSpace(conflict.Resolution) != "" {
		return models.MergeReviewConflict{}, "", fmt.Errorf("merge review item %d is already resolved", conflictID)
	}
	if strings.TrimSpace(localJSON.String) != "" {
		localSnapshot, err := unmarshalMergeReviewSnapshot(localJSON.String)
		if err != nil {
			return models.MergeReviewConflict{}, "", err
		}
		conflict.LocalSoldier = &localSnapshot.Soldier
	}
	sourceSnapshot, err := unmarshalMergeReviewSnapshot(sourceJSON)
	if err != nil {
		return models.MergeReviewConflict{}, "", err
	}
	conflict.SourceSoldier = sourceSnapshot.Soldier
	return conflict, sessionRoot, nil
}

func applySharedConflictResolution(tx *sql.Tx, conflict models.MergeReviewConflict, decision, sourceDataDir, targetDataDir string) error {
	sourceSnapshot, err := loadSourceSnapshotForConflict(tx, conflict.ID)
	if err != nil {
		return err
	}
	preserveLocalIdentifiers := decision != "keep-both" && conflict.LocalSoldier != nil
	if preserveLocalIdentifiers {
		sourceSnapshot.Soldier.ID = conflict.LocalSoldierID
		sourceSnapshot.Soldier.DisplayID = conflict.LocalSoldier.DisplayID
		sourceSnapshot.Soldier.SyncID = conflict.LocalSoldier.SyncID
		sourceSnapshot.Soldier.AddedBy = conflict.LocalSoldier.AddedBy
		sourceSnapshot.Soldier.CreatedAt = conflict.LocalSoldier.CreatedAt
	}
	targetID, _, _, err := upsertSharedSoldier(tx, sourceSnapshot.Soldier)
	if err != nil {
		return err
	}
	if decision == "keep-both" && conflict.LocalSoldier != nil {
		reviewReason := fmt.Sprintf("Potential duplicate preserved during shared merge against %s.", strings.TrimSpace(sourceSnapshot.Soldier.DisplayID))
		if err := setReviewStatusTx(tx, conflict.LocalSoldierID, true, reviewReason); err != nil {
			return err
		}
		if err := setReviewStatusTx(tx, targetID, true, fmt.Sprintf("Potential duplicate imported from shared record %s.", strings.TrimSpace(conflict.LocalSoldier.DisplayID))); err != nil {
			return err
		}
	}
	spouseTargetID := int64(0)
	if strings.TrimSpace(sourceSnapshot.SpouseSyncID) != "" {
		spouseTargetID, err = loadTargetSoldierIDBySync(tx, sourceSnapshot.SpouseSyncID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("resolve the linked spouse record before applying shared changes")
			}
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE soldiers SET spouse_soldier_id = ? WHERE id = ?`, nullableInt64(spouseTargetID), targetID); err != nil {
		return err
	}
	for _, record := range sourceSnapshot.Soldier.Records {
		if _, err := upsertSharedRecord(tx, targetID, sourceSnapshot.Soldier.SyncID, record); err != nil {
			return err
		}
	}
	for _, image := range sourceSnapshot.Soldier.Images {
		if err := copySharedImageFile(sourceDataDir, targetDataDir, image.FilePath); err != nil {
			return err
		}
		if _, err := upsertSharedImage(tx, targetID, sourceSnapshot.Soldier.SyncID, image); err != nil {
			return err
		}
	}
	return nil
}

func setReviewStatusTx(tx *sql.Tx, soldierID int64, needsReview bool, reason string) error {
	reason = strings.TrimSpace(reason)
	if !needsReview {
		reason = ""
	}
	_, err := tx.Exec(`UPDATE soldiers SET needs_review = ?, review_reason = ? WHERE id = ?`, needsReview, reason, soldierID)
	return err
}

func loadSoldierSnapshotByHumanMatch(tx *sql.Tx, source models.Soldier) (*mergeReviewSnapshot, error) {
	birthYear, ok := humanDuplicateBirthYear(source)
	if !ok {
		return nil, sql.ErrNoRows
	}
	firstName := strings.TrimSpace(source.FirstName)
	lastName := strings.TrimSpace(source.LastName)
	unit := strings.TrimSpace(source.Unit)
	if firstName == "" || lastName == "" || unit == "" {
		return nil, sql.ErrNoRows
	}

	var soldierID int64
	err := tx.QueryRow(`SELECT id
		FROM soldiers
		WHERE TRIM(COALESCE(first_name, '')) = ?
		  AND TRIM(COALESCE(last_name, '')) = ?
		  AND TRIM(COALESCE(unit, '')) = ?
		  AND CAST(SUBSTR(TRIM(COALESCE(birth_date, '')), 7, 4) AS INTEGER) = ?
		  AND TRIM(COALESCE(sync_id, '')) <> ?
		ORDER BY id
		LIMIT 1`,
		firstName, lastName, unit, birthYear, strings.TrimSpace(source.SyncID),
	).Scan(&soldierID)
	if err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, soldierID)
}

func describeHumanDuplicateConflict(local mergeReviewSnapshot, source mergeReviewSnapshot) string {
	birthYear, _ := humanDuplicateBirthYear(source.Soldier)
	return fmt.Sprintf("Shared record %s matches %s on name, birth year %d, and unit %s.", source.Soldier.DisplayID, local.Soldier.DisplayID, birthYear, strings.TrimSpace(source.Soldier.Unit))
}

func humanDuplicateBirthYear(soldier models.Soldier) (int, bool) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(soldier.BirthDate))
	if err == nil && partial.Year >= 1000 {
		return partial.Year, true
	}
	if parsedBirth := dates.ParseBirthInfo(strings.TrimSpace(soldier.BirthInfo)); parsedBirth != "" {
		partial, err := dates.ParseCanonical(parsedBirth)
		if err == nil && partial.Year >= 1000 {
			return partial.Year, true
		}
	}
	return 0, false
}

func finalizeMergeReviewSession(tx *sql.Tx, sessionID, sessionRoot string) error {
	var unresolved int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM merge_review_conflicts WHERE session_id = ? AND COALESCE(resolution, '') = ''`, sessionID).Scan(&unresolved); err != nil {
		return err
	}
	if unresolved > 0 {
		_, err := tx.Exec(`UPDATE merge_review_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID)
		return err
	}
	if _, err := tx.Exec(`UPDATE merge_review_sessions SET status = 'resolved', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, sessionID); err != nil {
		return err
	}
	if strings.TrimSpace(sessionRoot) != "" {
		if err := os.RemoveAll(sessionRoot); err != nil {
			return err
		}
	}
	return nil
}

func loadSourceSnapshotForConflict(tx *sql.Tx, conflictID int64) (mergeReviewSnapshot, error) {
	var sourceJSON string
	if err := tx.QueryRow(`SELECT source_data FROM merge_review_conflicts WHERE id = ?`, conflictID).Scan(&sourceJSON); err != nil {
		return mergeReviewSnapshot{}, err
	}
	return unmarshalMergeReviewSnapshot(sourceJSON)
}

func loadTargetSoldierIDBySync(tx *sql.Tx, syncID string) (int64, error) {
	var targetID int64
	err := tx.QueryRow(`SELECT id FROM soldiers WHERE sync_id = ?`, strings.TrimSpace(syncID)).Scan(&targetID)
	return targetID, err
}

func loadSoldierSnapshotBySync(tx *sql.Tx, syncID string) (*mergeReviewSnapshot, error) {
	var id int64
	if err := tx.QueryRow(`SELECT id FROM soldiers WHERE sync_id = ?`, strings.TrimSpace(syncID)).Scan(&id); err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, id)
}

func loadSoldierSnapshotByDisplayID(tx *sql.Tx, displayID string) (*mergeReviewSnapshot, error) {
	var id int64
	if err := tx.QueryRow(`SELECT id FROM soldiers WHERE display_id = ?`, strings.TrimSpace(displayID)).Scan(&id); err != nil {
		return nil, err
	}
	return loadSoldierSnapshotByID(tx, id)
}

func loadSoldierSnapshotByID(tx *sql.Tx, soldierID int64) (*mergeReviewSnapshot, error) {
	row := tx.QueryRow(`SELECT `+soldierSelectColumns+` FROM soldiers WHERE id = ?`, soldierID)
	soldier, err := scanSoldier(row)
	if err != nil {
		return nil, err
	}
	records, err := loadRecordsForSoldierTx(tx, soldierID)
	if err != nil {
		return nil, err
	}
	images, err := loadImagesForSoldierTx(tx, soldierID)
	if err != nil {
		return nil, err
	}
	soldier.Records = records
	soldier.Images = images
	spouseSyncID := ""
	if soldier.SpouseSoldierID > 0 {
		_ = tx.QueryRow(`SELECT COALESCE(sync_id, '') FROM soldiers WHERE id = ?`, soldier.SpouseSoldierID).Scan(&spouseSyncID)
	}
	normalized := normalizeSharedSoldierSnapshot(*soldier)
	normalized.ID = soldierID
	return &mergeReviewSnapshot{
		Soldier:      normalized,
		SpouseSyncID: strings.TrimSpace(spouseSyncID),
	}, nil
}

func loadRecordsForSoldierTx(tx *sql.Tx, soldierID int64) ([]models.Record, error) {
	rows, err := tx.Query(`SELECT `+recordSelectColumns+` FROM records WHERE soldier_id = ? ORDER BY id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []models.Record{}
	for rows.Next() {
		var record models.Record
		if err := rows.Scan(&record.ID, &record.SyncID, &record.SoldierID, &record.SoldierSyncID, &record.RecordType, &record.AppID, &record.Details); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func loadImagesForSoldierTx(tx *sql.Tx, soldierID int64) ([]models.Image, error) {
	rows, err := tx.Query(`SELECT `+imageSelectColumns+` FROM images WHERE soldier_id = ? ORDER BY is_primary DESC, id`, soldierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	images := []models.Image{}
	for rows.Next() {
		var image models.Image
		if err := rows.Scan(&image.ID, &image.SyncID, &image.SoldierID, &image.SoldierSyncID, &image.FileName, &image.FilePath, &image.Caption, &image.IsPrimary); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func resolveSharedSpouseTargetID(sourceSyncByID map[int64]string, targetIDsBySync map[string]int64, soldier models.Soldier) (int64, error) {
	if soldier.SpouseSoldierID < 1 {
		return 0, nil
	}
	spouseSyncID := strings.TrimSpace(sourceSyncByID[soldier.SpouseSoldierID])
	if spouseSyncID == "" {
		return 0, fmt.Errorf("shared database spouse link missing sync id for soldier %s", soldier.DisplayID)
	}
	spouseTargetID := targetIDsBySync[spouseSyncID]
	if spouseTargetID < 1 {
		return 0, fmt.Errorf("shared database spouse link missing target for soldier %s", soldier.DisplayID)
	}
	return spouseTargetID, nil
}

func equivalentMergeReviewSnapshots(local, source mergeReviewSnapshot) bool {
	return describeSoldierConflict(local, source) == ""
}

func describeSoldierConflict(local, source mergeReviewSnapshot) string {
	differences := make([]string, 0, 8)
	appendDiff := func(label, left, right string) {
		left = strings.TrimSpace(left)
		right = strings.TrimSpace(right)
		if left != right {
			differences = append(differences, label)
		}
	}
	appendDiff("display ID", local.Soldier.DisplayID, source.Soldier.DisplayID)
	appendDiff("entry type", local.Soldier.EntryType, source.Soldier.EntryType)
	appendDiff("first name", local.Soldier.FirstName, source.Soldier.FirstName)
	appendDiff("middle name", local.Soldier.MiddleName, source.Soldier.MiddleName)
	appendDiff("last name", local.Soldier.LastName, source.Soldier.LastName)
	appendDiff("maiden name", local.Soldier.MaidenName, source.Soldier.MaidenName)
	appendDiff("rank", local.Soldier.Rank, source.Soldier.Rank)
	appendDiff("rank in", local.Soldier.RankIn, source.Soldier.RankIn)
	appendDiff("rank out", local.Soldier.RankOut, source.Soldier.RankOut)
	appendDiff("unit", local.Soldier.Unit, source.Soldier.Unit)
	appendDiff("pension state", local.Soldier.PensionState, source.Soldier.PensionState)
	appendDiff("pension ID", local.Soldier.PensionID, source.Soldier.PensionID)
	appendDiff("application ID", local.Soldier.ApplicationID, source.Soldier.ApplicationID)
	appendDiff("birth date", local.Soldier.BirthDate, source.Soldier.BirthDate)
	appendDiff("death date", local.Soldier.DeathDate, source.Soldier.DeathDate)
	appendDiff("birth info", local.Soldier.BirthInfo, source.Soldier.BirthInfo)
	appendDiff("buried in", local.Soldier.BuriedIn, source.Soldier.BuriedIn)
	appendDiff("notes", local.Soldier.Notes, source.Soldier.Notes)
	appendDiff("spouse link", local.SpouseSyncID, source.SpouseSyncID)
	if len(differences) == 0 {
		return ""
	}
	return "Shared archive changed " + strings.Join(differences, ", ") + "."
}

func normalizeSharedSoldierSnapshot(soldier models.Soldier) models.Soldier {
	soldier.Records = append([]models.Record(nil), soldier.Records...)
	soldier.Images = append([]models.Image(nil), soldier.Images...)
	for index := range soldier.Records {
		soldier.Records[index].ID = 0
		soldier.Records[index].SoldierID = 0
	}
	for index := range soldier.Images {
		soldier.Images[index].ID = 0
		soldier.Images[index].SoldierID = 0
	}
	soldier.ID = 0
	soldier.SpouseSoldierID = 0
	soldier.SpouseName = ""
	soldier.IsGenerated = soldier.IsGenerated || isGeneratedDisplayID(soldier.DisplayID)
	return soldier
}

func marshalMergeReviewSnapshot(snapshot mergeReviewSnapshot) (string, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func unmarshalMergeReviewSnapshot(raw string) (mergeReviewSnapshot, error) {
	var snapshot mergeReviewSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return mergeReviewSnapshot{}, err
	}
	snapshot.Soldier = normalizeSharedSoldierSnapshot(snapshot.Soldier)
	return snapshot, nil
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func newMergeLogger(dataDir string) (*mergeLogger, error) {
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	fileName := "shared-merge-" + time.Now().Format("20060102-150405") + ".log"
	return &mergeLogger{
		path:  filepath.Join(logDir, fileName),
		lines: []string{"DixieData shared archive merge log", "created_at=" + time.Now().Format(time.RFC3339)},
	}, nil
}

func (m *mergeLogger) Printf(format string, args ...interface{}) {
	if m == nil {
		return
	}
	m.lines = append(m.lines, time.Now().Format(time.RFC3339)+" "+fmt.Sprintf(format, args...))
}

func (m *mergeLogger) Close() error {
	if m == nil {
		return nil
	}
	body := strings.Join(m.lines, "\n") + "\n"
	if err := os.WriteFile(m.path, []byte(body), 0o644); err != nil {
		return err
	}
	latestPath := filepath.Join(filepath.Dir(m.path), "shared-merge-latest.log")
	if err := os.WriteFile(latestPath, []byte(body), fs.FileMode(0o644)); err != nil {
		return err
	}
	return nil
}
