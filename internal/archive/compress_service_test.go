package archive

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/db"
)

func writeTestJPEG(t *testing.T, path string, w, h int, quality int) int64 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test jpeg: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("encode test jpeg: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat test jpeg: %v", err)
	}
	return info.Size()
}

func writeTestPNG(t *testing.T, path string, w, h int) int64 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Smooth radial gradient: realistic photo-like content
			// where JPEG q=85 wins over PNG's lossless filter overhead.
			dx := float64(x-w/2) / float64(w/2)
			dy := float64(y-h/2) / float64(h/2)
			d := dx*dx + dy*dy
			if d > 1 {
				d = 1
			}
			v := uint8(255 - int(d*200))
			img.Set(x, y, color.RGBA{R: v, G: uint8(int(v) / 2), B: uint8(255 - int(v)), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat test png: %v", err)
	}
	return info.Size()
}

func newTestCompressDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func decodeFile(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	return img, format, err
}

func TestCompressService_EncodeAndWrite_ShrinksJPEG(t *testing.T) {
	dataDir := t.TempDir()
	s := &CompressService{}
	target := filepath.Join(dataDir, "photo.jpg")
	writeTestJPEG(t, target, 200, 200, 100)
	img, _, err := decodeFile(target)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	originalBytes, compressedBytes, err := s.EncodeAndWrite(img, target)
	if err != nil {
		t.Fatalf("encode and write: %v", err)
	}
	if compressedBytes >= originalBytes {
		t.Errorf("expected q=85 to shrink: original=%d compressed=%d", originalBytes, compressedBytes)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be removed, got err=%v", err)
	}
}

func TestCompressService_EncodeAndWrite_CleansUpTmpOnFailure(t *testing.T) {
	dataDir := t.TempDir()
	s := &CompressService{}
	target := filepath.Join(dataDir, "photo.jpg")
	writeTestJPEG(t, target, 100, 100, 80)
	// Block the encode path by pre-creating a directory at the .tmp
	// path. This makes os.Create fail regardless of platform and is the
	// most portable way to force a failure mid-encode.
	if err := os.Mkdir(target+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}
	img, _, err := decodeFile(target)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	_, _, err = s.EncodeAndWrite(img, target)
	if err == nil {
		t.Fatalf("expected error when .tmp path is a directory")
	}
	// The blocker directory must still be present (we did not delete it),
	// and the target must be untouched.
	if _, err := os.Stat(target+".tmp"); err != nil {
		t.Errorf("expected .tmp blocker still present, got err=%v", err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if targetInfo.Size() == 0 {
		t.Errorf("expected target untouched, got zero size")
	}
}

func TestCompressService_Compress_PNGtoJPEG(t *testing.T) {
	dataDir := t.TempDir()
	target := filepath.Join(dataDir, "photo.png")
	// Use a large noisy PNG so JPEG q=85 beats PNG's lossless filter
	// overhead. Synthetic images with low entropy don't shrink — real
	// photo-like content does.
	writeTestPNG(t, target, 1200, 1200)
	s := NewCompressService(newTestCompressDB(t))
	result, err := s.Compress(dataDir, "photo.png")
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if result.CompressedBytes >= result.OriginalBytes {
		t.Errorf("expected png->jpeg to shrink: original=%d compressed=%d", result.OriginalBytes, result.CompressedBytes)
	}
	if result.OriginalBytes <= 0 || result.CompressedBytes <= 0 {
		t.Errorf("expected positive byte counts, got original=%d compressed=%d", result.OriginalBytes, result.CompressedBytes)
	}
	if !strings.HasSuffix(result.RelativePath, ".png") {
		t.Errorf("expected relative path preserved, got %q", result.RelativePath)
	}
	// Verify the on-disk file is now a valid JPEG (we re-encoded).
	img, format, err := decodeFile(filepath.Join(dataDir, "photo.png"))
	if err != nil {
		t.Fatalf("decode re-encoded file: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("expected re-encoded format=jpeg, got %q", format)
	}
	if img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 1200 {
		t.Errorf("expected 1200x1200, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func insertTestSoldier(t *testing.T, d *db.DB) int64 {
	t.Helper()
	res, err := d.Conn().Exec(`INSERT INTO soldiers (display_id, sync_id, first_name, last_name) VALUES (?, ?, ?, ?)`,
		"DXD-00001", "synctest", "Test", "Soldier")
	if err != nil {
		t.Fatalf("insert soldier: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last id: %v", err)
	}
	return id
}

func TestCompressService_DiscoverUncompressed_OnlyNullRows(t *testing.T) {
	dataDir := t.TempDir()
	imagesDir := filepath.Join(dataDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two files on disk: one referenced by a row with compressed_at NULL,
	// one referenced by a row with compressed_at set.
	uncPath := filepath.Join(imagesDir, "unc.jpg")
	if err := os.WriteFile(uncPath, []byte("a"), 0o644); err != nil {
		t.Fatalf("write unc: %v", err)
	}
	donePath := filepath.Join(imagesDir, "done.jpg")
	if err := os.WriteFile(donePath, []byte("b"), 0o644); err != nil {
		t.Fatalf("write done: %v", err)
	}
	d := newTestCompressDB(t)
	soldierID := insertTestSoldier(t, d)
	if _, err := d.Conn().Exec(`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary, compressed_at) VALUES ('s1', ?, 'ss1', 'unc.jpg', 'images/unc.jpg', '', 0, NULL)`, soldierID); err != nil {
		t.Fatalf("insert unc: %v", err)
	}
	if _, err := d.Conn().Exec(`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary, compressed_at) VALUES ('s2', ?, 'ss1', 'done.jpg', 'images/done.jpg', '', 0, ?)`,
		soldierID, time.Now().UTC()); err != nil {
		t.Fatalf("insert done: %v", err)
	}
	s := NewCompressService(d)
	candidates, err := s.DiscoverUncompressed(dataDir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].RelativePath != "images/unc.jpg" {
		t.Errorf("expected images/unc.jpg, got %q", candidates[0].RelativePath)
	}
}

func TestCompressService_RecordCompression_PersistsMetadata(t *testing.T) {
	d := newTestCompressDB(t)
	soldierID := insertTestSoldier(t, d)
	if _, err := d.Conn().Exec(`INSERT INTO images (sync_id, soldier_id, soldier_sync_id, file_name, file_path, caption, is_primary) VALUES ('s1', ?, 'ss1', 'a.jpg', 'images/a.jpg', '', 0)`, soldierID); err != nil {
		t.Fatalf("insert: %v", err)
	}
	s := NewCompressService(d)
	when := time.Now().UTC().Truncate(time.Second)
	if err := s.RecordCompression("images/a.jpg", 1000, 250, when); err != nil {
		t.Fatalf("record: %v", err)
	}
	var (
		gotOriginal  int64
		gotCompressed int64
		gotAt        time.Time
	)
	if err := d.Conn().QueryRow(`SELECT original_bytes, compressed_bytes, compressed_at FROM images WHERE file_path = ?`, "images/a.jpg").
		Scan(&gotOriginal, &gotCompressed, &gotAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotOriginal != 1000 || gotCompressed != 250 {
		t.Errorf("bytes: orig=%d comp=%d, want 1000/250", gotOriginal, gotCompressed)
	}
	if !gotAt.Equal(when) {
		t.Errorf("compressed_at: got %v, want %v", gotAt, when)
	}
}

func TestCompressService_CompressParallel_AggregatesReport(t *testing.T) {
	dataDir := t.TempDir()
	imagesDir := filepath.Join(dataDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	s := NewCompressService(newTestCompressDB(t))
	relPaths := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		name := filepath.Join("sub-"+strings.Repeat("s", i+1), "img.jpg")
		path := filepath.Join(imagesDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir sub: %v", err)
		}
		writeTestJPEG(t, path, 80, 80, 95)
		rel, _ := filepath.Rel(dataDir, path)
		relPaths = append(relPaths, filepath.ToSlash(filepath.Clean(rel)))
	}
	report, err := s.CompressParallel(dataDir, relPaths, 4, nil)
	if err != nil {
		t.Fatalf("parallel: %v", err)
	}
	if report.Scanned != 8 || report.Compressed != 8 || report.Skipped != 0 {
		t.Errorf("counts: scanned=%d compressed=%d skipped=%d, want 8/8/0", report.Scanned, report.Compressed, report.Skipped)
	}
	if report.OriginalBytes <= report.FinalBytes {
		t.Errorf("expected savings: original=%d final=%d", report.OriginalBytes, report.FinalBytes)
	}
	if len(report.Errors) > 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}
}

func TestCompressService_PurgeExpiredCompressedTrash_RemovesOld(t *testing.T) {
	dataDir := t.TempDir()
	old := filepath.Join(dataDir, "temp_trash", "compressed", "20250101-000000")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	past := time.Now().Add(-31 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	s := &CompressService{}
	if err := s.PurgeExpiredCompressedTrash(dataDir); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old trash removed, got err=%v", err)
	}
}
