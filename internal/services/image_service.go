package services

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/db"
)

const orphanTrashRetention = 30 * 24 * time.Hour

type ImageService struct {
	db *db.DB
}

type OrphanedImage struct {
	RelativePath string
	Size         int64
	ModifiedAt   string
}

func NewImageService(database *db.DB) *ImageService {
	return &ImageService{db: database}
}

func (s *ImageService) EnsureShardedStorage(dataDir string) error {
	rows, err := s.db.Conn().Query(`
		SELECT images.id, COALESCE(images.file_path, ''), COALESCE(images.file_name, ''), COALESCE(soldiers.display_id, '')
		FROM images
		JOIN soldiers ON soldiers.id = images.soldier_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type imageRow struct {
		id        int64
		filePath  string
		fileName  string
		displayID string
	}
	var images []imageRow
	for rows.Next() {
		var item imageRow
		if err := rows.Scan(&item.id, &item.filePath, &item.fileName, &item.displayID); err != nil {
			return err
		}
		images = append(images, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.db.Conn().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range images {
		recordDir, relativeDir := appdata.RecordImageDir(dataDir, item.displayID)
		targetRelative := filepath.ToSlash(filepath.Join(relativeDir, item.fileName))
		currentRelative := filepath.ToSlash(filepath.Clean(strings.TrimSpace(item.filePath)))
		if currentRelative == targetRelative {
			continue
		}

		currentAbsolute := filepath.Join(dataDir, filepath.FromSlash(currentRelative))
		targetAbsolute := filepath.Join(dataDir, filepath.FromSlash(targetRelative))
		if err := os.MkdirAll(filepath.Dir(targetAbsolute), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(currentAbsolute); err == nil {
			if err := moveFile(currentAbsolute, targetAbsolute); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE images SET file_path = ? WHERE id = ?`, targetRelative, item.id); err != nil {
			return err
		}
		if err := os.MkdirAll(recordDir, 0o755); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *ImageService) DiscoverOrphans(dataDir string) ([]OrphanedImage, error) {
	rows, err := s.db.Conn().Query(`SELECT COALESCE(file_path, '') FROM images`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	referenced := map[string]struct{}{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if normalized != "" {
			referenced[normalized] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	imageRoot := filepath.Join(dataDir, "images")
	orphans := []OrphanedImage{}
	err = filepath.WalkDir(imageRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(filepath.Clean(relative))
		if _, ok := referenced[normalized]; ok {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		orphans = append(orphans, OrphanedImage{
			RelativePath: normalized,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []OrphanedImage{}, nil
		}
		return nil, err
	}
	return orphans, nil
}

func (s *ImageService) MoveOrphansToTrash(dataDir string, relativePaths []string) (int, string, error) {
	if len(relativePaths) == 0 {
		return 0, "", nil
	}
	trashRoot := filepath.Join(dataDir, "temp_trash", "images", time.Now().UTC().Format("20060102-150405"))
	moved := 0
	for _, relativePath := range relativePaths {
		cleanRelative := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relativePath)))
		if cleanRelative == "" {
			continue
		}
		source := filepath.Join(dataDir, filepath.FromSlash(cleanRelative))
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return moved, trashRoot, err
		}
		target := filepath.Join(trashRoot, filepath.FromSlash(cleanRelative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return moved, trashRoot, err
		}
		if err := moveFile(source, target); err != nil {
			return moved, trashRoot, err
		}
		moved++
	}
	return moved, trashRoot, nil
}

func (s *ImageService) PurgeExpiredTrash(dataDir string) error {
	trashRoot := filepath.Join(dataDir, "temp_trash")
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-orphanTrashRetention)
	for _, entry := range entries {
		fullPath := filepath.Join(trashRoot, entry.Name())
		if entry.IsDir() && entry.Name() == "images" {
			imageEntries, err := os.ReadDir(fullPath)
			if err != nil {
				return err
			}
			for _, imageEntry := range imageEntries {
				imagePath := filepath.Join(fullPath, imageEntry.Name())
				info, err := imageEntry.Info()
				if err != nil {
					return err
				}
				if info.ModTime().Before(cutoff) {
					if err := os.RemoveAll(imagePath); err != nil {
						return err
					}
				}
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(fullPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func moveFile(source, target string) error {
	if source == target {
		return nil
	}
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		return err
	}
	return nil
}
