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

	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const (
	backupFormatName = "dixiedata-backup"
	backupVersion    = 1
)

type BackupManifest struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
	DataFile  string `json:"data_file"`
	ImageRoot string `json:"image_root"`
	Soldiers  int    `json:"soldiers"`
	Records   int    `json:"records"`
	Images    int    `json:"images"`
}

type BackupService struct {
	soldier *SoldierService
}

func NewBackupService(soldier *SoldierService) *BackupService {
	return &BackupService{soldier: soldier}
}

func (b *BackupService) Export(outputPath, dataDir string) (BackupManifest, error) {
	soldiers, manifest, err := b.loadBackupData()
	if err != nil {
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
	if err := writeBackupJSON(zipWriter, manifest.DataFile, soldiers); err != nil {
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

	manifest, soldiers, err := readBackupContents(&reader.Reader)
	if err != nil {
		return BackupManifest{}, err
	}

	tempDir, err := os.MkdirTemp("", "dixiedata-backup-*")
	if err != nil {
		return BackupManifest{}, err
	}
	defer os.RemoveAll(tempDir)

	if err := extractBackupImages(&reader.Reader, tempDir, manifest.ImageRoot); err != nil {
		return BackupManifest{}, err
	}

	if err := resetBackupData(dataDir); err != nil {
		return BackupManifest{}, err
	}

	database, err := db.Open(dataDir)
	if err != nil {
		return BackupManifest{}, err
	}
	defer database.Close()

	soldierSvc := NewSoldierService(database)
	for _, soldier := range soldiers {
		created, err := soldierSvc.Create(models.Soldier{
			DisplayID:   soldier.DisplayID,
			IsGenerated: soldier.IsGenerated,
			FirstName:   soldier.FirstName,
			LastName:    soldier.LastName,
			Rank:        soldier.Rank,
			Unit:        soldier.Unit,
			DeathYear:   soldier.DeathYear,
			DeathMonth:  soldier.DeathMonth,
			DeathDay:    soldier.DeathDay,
			BirthInfo:   soldier.BirthInfo,
			BuriedIn:    soldier.BuriedIn,
			Notes:       soldier.Notes,
			Records:     soldier.Records,
		})
		if err != nil {
			return BackupManifest{}, err
		}

		for _, image := range soldier.Images {
			sourcePath := filepath.Join(tempDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			destinationPath := filepath.Join(dataDir, filepath.FromSlash(normalizeBackupPath(image.FilePath)))
			if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
				return BackupManifest{}, err
			}
			if err := copyBackupFile(sourcePath, destinationPath); err != nil {
				return BackupManifest{}, err
			}
			if err := soldierSvc.AddImage(created.ID, image.FileName, image.FilePath, image.Caption); err != nil {
				return BackupManifest{}, err
			}
		}
	}

	return manifest, nil
}

func (b *BackupService) loadBackupData() ([]models.Soldier, BackupManifest, error) {
	var soldiers []models.Soldier
	manifest := BackupManifest{
		Format:    backupFormatName,
		Version:   backupVersion,
		CreatedAt: time.Now().Format(time.RFC3339),
		DataFile:  "data/soldiers.json",
		ImageRoot: "images/",
	}

	page := 1
	for {
		batch, _, err := b.soldier.List(page, exportBatchSize)
		if err != nil {
			return nil, BackupManifest{}, err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			soldier, err := b.soldier.GetByID(item.ID)
			if err != nil {
				return nil, BackupManifest{}, err
			}
			soldiers = append(soldiers, *soldier)
			manifest.Soldiers++
			manifest.Records += len(soldier.Records)
			manifest.Images += len(soldier.Images)
		}
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	return soldiers, manifest, nil
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

func readBackupContents(reader *zip.Reader) (BackupManifest, []models.Soldier, error) {
	fileMap := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		fileMap[file.Name] = file
	}

	manifestFile, ok := fileMap["manifest.json"]
	if !ok {
		return BackupManifest{}, nil, fmt.Errorf("backup is missing manifest.json")
	}

	var manifest BackupManifest
	if err := readBackupJSON(manifestFile, &manifest); err != nil {
		return BackupManifest{}, nil, err
	}
	if manifest.Format != backupFormatName || manifest.Version != backupVersion {
		return BackupManifest{}, nil, fmt.Errorf("unsupported backup format")
	}
	if manifest.DataFile == "" || manifest.ImageRoot == "" {
		return BackupManifest{}, nil, fmt.Errorf("backup manifest is incomplete")
	}

	dataFile, ok := fileMap[manifest.DataFile]
	if !ok {
		return BackupManifest{}, nil, fmt.Errorf("backup is missing %s", manifest.DataFile)
	}

	var soldiers []models.Soldier
	if err := readBackupJSON(dataFile, &soldiers); err != nil {
		return BackupManifest{}, nil, err
	}

	imageEntries := make(map[string]struct{})
	for name := range fileMap {
		if strings.HasPrefix(name, manifest.ImageRoot) {
			imageEntries[name] = struct{}{}
		}
	}
	for _, soldier := range soldiers {
		for _, image := range soldier.Images {
			if _, ok := imageEntries[normalizeBackupPath(image.FilePath)]; !ok {
				return BackupManifest{}, nil, fmt.Errorf("backup is missing image file %s", image.FilePath)
			}
		}
	}

	return manifest, soldiers, nil
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

		source, err := file.Open()
		if err != nil {
			return err
		}
		target, err := os.Create(destinationPath)
		if err != nil {
			source.Close()
			return err
		}
		if _, err := io.Copy(target, source); err != nil {
			source.Close()
			target.Close()
			return err
		}
		source.Close()
		target.Close()
	}
	return nil
}

func resetBackupData(dataDir string) error {
	dbPath := filepath.Join(dataDir, "dixiedata.db")
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, suffix := range []string{"-shm", "-wal"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.RemoveAll(filepath.Join(dataDir, "images"))
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

func normalizeBackupPath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "/")
}
