package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the canonical SQLite file name within a Local Archive (dixiedata.db).
const FileName = "dixiedata.db"

// Path returns the absolute SQLite file path for a given dataDir.
func Path(dataDir string) string {
	return filepath.Join(dataDir, FileName)
}

// SnapshotTo writes a VACUUM-INTO snapshot of the live database to outputPath. Used by the backup + restore-point pipeline.
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
