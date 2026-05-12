package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = "dixiedata.db"

func Path(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

func (d *DB) SnapshotTo(outputPath string) error {
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := d.conn.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		return err
	}
	escapedPath := strings.ReplaceAll(outputPath, `'`, `''`)
	_, err := d.conn.Exec(fmt.Sprintf(`VACUUM INTO '%s'`, escapedPath))
	return err
}
