package archive

import (
	"fmt"
	"image"
	"image/jpeg"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
)

const (
	qualityJPEG               = 85
	compressedTrashRetention  = 30 * 24 * time.Hour
)

// CompressService re-encodes stored images to JPEG quality 85. Originals
// are moved to temp_trash/compressed/<UTC-stamp>/ for 30-day retention.
// The service is nil-tolerant in the appshell wiring path so the auto
// hooks can be a no-op when compression is unwired (e.g. in tests).
type CompressService struct {
	db *db.DB
}

// CompressedResult is the per-image outcome of a successful Compress call.
// Callers pass these to RecordCompression to persist the metadata.
type CompressedResult struct {
	RelativePath    string
	OriginalBytes   int64
	CompressedBytes int64
	CompressedAt    time.Time
}

// CompressibleImage describes a single on-disk image row that has not yet
// been compressed (compressed_at IS NULL). Returned by DiscoverUncompressed
// and consumed by the batch UI + CLI.
type CompressibleImage struct {
	RelativePath string
	Size         int64
	ModifiedAt   string
}

// CompressionReport aggregates the per-image outcomes of a parallel batch
// run. The Errors slice is the only failure surface; CompressParallel
// itself never returns an error.
type CompressionReport struct {
	Scanned       int
	Compressed    int
	Skipped       int
	OriginalBytes int64
	FinalBytes    int64
	TrashRoot     string
	Errors        []string
}

// NewCompressService constructs a CompressService bound to the given db.
func NewCompressService(database *db.DB) *CompressService {
	return &CompressService{db: database}
}

// EncodeAndWrite re-encodes the decoded image to JPEG q=85 and writes it
// to the target path atomically. The flow is:
//  1. Capture the original file's byte count.
//  2. Write the encoded image to <targetPath>.tmp.
//  3. Remove the original at targetPath (caller may have already moved it
//     aside — in that case the Remove is a no-op error we tolerate).
//  4. Rename <targetPath>.tmp to targetPath.
//
// Returns the original and final byte counts. The caller is expected to
// have decoded the image already (so unsupported formats surface earlier
// with a clearer error).
func (s *CompressService) EncodeAndWrite(img image.Image, targetPath string) (int64, int64, error) {
	originalInfo, err := os.Stat(targetPath)
	originalBytes := int64(0)
	if err == nil {
		originalBytes = originalInfo.Size()
	} else if !os.IsNotExist(err) {
		return 0, 0, fmt.Errorf("stat target image file: %w", err)
	}
	tmpPath := targetPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return 0, 0, fmt.Errorf("create temp image file: %w", err)
	}
	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: qualityJPEG}); err != nil {
		out.Close()
		_ = os.Remove(tmpPath)
		return 0, 0, fmt.Errorf("encode jpeg: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, 0, fmt.Errorf("close temp image file: %w", err)
	}
	compressedInfo, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return 0, 0, fmt.Errorf("stat temp image file: %w", err)
	}
	compressedBytes := compressedInfo.Size()
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return 0, 0, fmt.Errorf("remove original: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, 0, fmt.Errorf("rename temp image file: %w", err)
	}
	return originalBytes, compressedBytes, nil
}

// Compress reads the file at the relative path under dataDir, decodes it,
// re-encodes via EncodeAndWrite, and moves the original to trash. Returns
// the CompressedResult for caller-side DB persistence. If the encode
// step fails after the original has been moved, Compress restores the
// original from trash and returns a wrapped error so callers (and the
// Phase 4 hook sites that ignore Compress errors) can locate the orphan
// in trash for manual recovery.
func (s *CompressService) Compress(dataDir, relPath string) (CompressedResult, error) {
	abs, err := safeJoinWithinRoot(dataDir, relPath)
	if err != nil {
		return CompressedResult{}, err
	}
	originalInfo, err := os.Stat(abs)
	if err != nil {
		return CompressedResult{}, fmt.Errorf("stat image file: %w", err)
	}
	originalBytes := originalInfo.Size()
	src, err := os.Open(abs)
	if err != nil {
		return CompressedResult{}, fmt.Errorf("open image file: %w", err)
	}
	img, _, err := image.Decode(src)
	src.Close()
	if err != nil {
		return CompressedResult{}, fmt.Errorf("decode image file: %w", err)
	}
	trashRoot := filepath.Join(dataDir, "temp_trash", "compressed", time.Now().UTC().Format("20060102-150405"))
	trashTarget := filepath.Join(trashRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(trashTarget), 0o755); err != nil {
		return CompressedResult{}, fmt.Errorf("create trash dir: %w", err)
	}
	if err := moveFile(abs, trashTarget); err != nil {
		return CompressedResult{}, fmt.Errorf("move original to trash: %w", err)
	}
	_, compressedBytes, err := s.EncodeAndWrite(img, abs)
	if err != nil {
		// Restore the original from trash on encode failure. If the
		// restore itself fails, wrap the error so callers (and the
		// Phase 4 hook sites that ignore Compress errors) can locate
		// the orphan in trash for manual recovery.
		if restoreErr := moveFile(trashTarget, abs); restoreErr != nil {
			return CompressedResult{}, fmt.Errorf("encode failed (%w) and restore from %s failed: %v", err, trashTarget, restoreErr)
		}
		return CompressedResult{}, err
	}
	return CompressedResult{
		RelativePath:    relPath,
		OriginalBytes:   originalBytes,
		CompressedBytes: compressedBytes,
		CompressedAt:    time.Now().UTC(),
	}, nil
}

// DiscoverUncompressed returns images that have no compressed_at timestamp.
// A row whose compressed_at is NULL is treated as uncompressed regardless
// of when it was inserted; the snapshot-backup lazy-backfill path relies
// on this so restored files are picked up by the first scan after restore.
func (s *CompressService) DiscoverUncompressed(dataDir string) ([]CompressibleImage, error) {
	rows, err := s.db.Conn().Query(`SELECT id, COALESCE(file_path, '') FROM images WHERE compressed_at IS NULL AND COALESCE(file_path, '') != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	referenced := map[string]struct{}{}
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
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
	candidates := []CompressibleImage{}
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
		if _, ok := referenced[normalized]; !ok {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		candidates = append(candidates, CompressibleImage{
			RelativePath: normalized,
			Size:         info.Size(),
			ModifiedAt:   info.ModTime().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []CompressibleImage{}, nil
		}
		return nil, err
	}
	return candidates, nil
}

// MoveCompressiblesToTrash is the explicit-trash path used by a future
// "Compress these N images" UI button. The Phase 6 Settings card calls
// Compress directly via CompressParallel; this exists so a future
// explicit-cleanup action can show "moved to trash" without re-encoding.
func (s *CompressService) MoveCompressiblesToTrash(dataDir string, relPaths []string) (int, string, error) {
	if len(relPaths) == 0 {
		return 0, "", nil
	}
	trashRoot := filepath.Join(dataDir, "temp_trash", "compressed", time.Now().UTC().Format("20060102-150405"))
	moved := 0
	for _, relPath := range relPaths {
		abs, err := safeJoinWithinRoot(dataDir, relPath)
		if err != nil {
			continue
		}
		target := filepath.Join(trashRoot, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return moved, trashRoot, err
		}
		if err := moveFile(abs, target); err != nil {
			return moved, trashRoot, err
		}
		moved++
	}
	return moved, trashRoot, nil
}

// PurgeExpiredCompressedTrash removes compressed-trash subdirs older
// than compressedTrashRetention. Mirrors ImageService.PurgeExpiredTrash
// but only walks the "compressed" subdir of temp_trash.
func (s *CompressService) PurgeExpiredCompressedTrash(dataDir string) error {
	trashRoot := filepath.Join(dataDir, "temp_trash", "compressed")
	entries, err := os.ReadDir(trashRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-compressedTrashRetention)
	for _, entry := range entries {
		fullPath := filepath.Join(trashRoot, entry.Name())
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

// RecordCompression writes the post-compress metadata to the images row.
// normalized is computed from relPath internally; callers can pass the raw
// relPath from CompressedResult.RelativePath.
func (s *CompressService) RecordCompression(relPath string, originalBytes, compressedBytes int64, when time.Time) error {
	normalized := filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if normalized == "" {
		return fmt.Errorf("invalid file path")
	}
	_, err := s.db.Conn().Exec(
		`UPDATE images SET compressed_at = ?, original_bytes = ?, compressed_bytes = ? WHERE file_path = ?`,
		when, originalBytes, compressedBytes, normalized,
	)
	return err
}

// CompressParallel runs Compress across the given relative paths with
// up to maxWorkers goroutines (clamped to [1, 8]), calling onResult for
// each completion. Per-image failures are reported via CompressionReport.Errors
// (this function itself never returns an error).
func (s *CompressService) CompressParallel(dataDir string, relPaths []string, maxWorkers int, onResult func(CompressedResult, error)) (CompressionReport, error) {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	if maxWorkers > 8 {
		maxWorkers = 8
	}
	report := CompressionReport{Scanned: len(relPaths)}
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for relPath := range jobs {
				result, err := s.Compress(dataDir, relPath)
				mu.Lock()
				if err != nil {
					report.Skipped++
					report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", relPath, err))
				} else {
					report.Compressed++
					report.OriginalBytes += result.OriginalBytes
					report.FinalBytes += result.CompressedBytes
					if perr := s.RecordCompression(result.RelativePath, result.OriginalBytes, result.CompressedBytes, result.CompressedAt); perr != nil {
						report.Errors = append(report.Errors, fmt.Sprintf("%s: record: %v", relPath, perr))
					}
				}
				mu.Unlock()
				if onResult != nil {
					onResult(result, err)
				}
			}
		}()
	}
	for _, relPath := range relPaths {
		jobs <- relPath
	}
	close(jobs)
	wg.Wait()
	return report, nil
}

