package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
)

func writeZipArchive(outputPath string, build func(*zip.Writer) error) (err error) {
	outputDir := filepath.Dir(outputPath)
	tempFile, err := os.CreateTemp(outputDir, filepath.Base(outputPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	zipWriter := zip.NewWriter(tempFile)
	if err = build(zipWriter); err != nil {
		_ = zipWriter.Close()
		_ = tempFile.Close()
		return err
	}
	if err = zipWriter.Close(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err = tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err = tempFile.Close(); err != nil {
		return err
	}
	if err = os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(tempPath, outputPath); err != nil {
		return err
	}
	return nil
}
