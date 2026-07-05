// Package db opens the DixieData SQLite database, applies schema migrations, and snapshots the file.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
	_ "modernc.org/sqlite"
)

// DB is the DixieData SQLite database handle: the connection, the
// dataDir that produced it, and the migration state (PRAGMA
// user_version). Constructed by Open (full path: open file, apply
// schema migrations, run pending blocks) or by NewFromExisting
// (read-only tools that wrap an already-opened *sql.DB). Every
// service facade in internal/archive receives a *DB; the appshell
// owns the only *DB instance per process.
type DB struct {
	conn    *sql.DB
	dataDir string
}

// NewFromExisting wraps an already-opened *sql.DB as a *DB without
// running schema migrations. Used by read-only tools (e.g.
// tools/tune) that need the DB's query methods but must not mutate
// the underlying file. The caller is responsible for closing the
// *sql.DB; the returned *DB's Close closes it.
func NewFromExisting(conn *sql.DB) *DB {
	return &DB{conn: conn}
}

// Open opens the SQLite database at dataDir/dixiedata.db, applies pending schema migrations, and returns a *DB ready for queries. Refuses to open a newer-schema database (the user must downgrade via migrate down first).
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dataDir, FileName)
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	d := &DB{conn: conn, dataDir: dataDir}
	if err := backupBeforeMigrationIfNeeded(d, dbPath); err != nil {
		conn.Close()
		return nil, fmt.Errorf("backup-before-migration: %w", err)
	}
	if err := applySchema(d); err != nil {
		conn.Close()
		return nil, err
	}
	if err := d.ImportLegacyScratchpadFiles(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func backupBeforeMigrationIfNeeded(d *DB, dbPath string) error {
	info, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	currentVersion, err := currentSchemaVersion(d.conn)
	if err != nil {
		return err
	}
	if currentVersion >= CurrentSchemaVersion {
		return nil
	}
	manager := update.NewRetainedBackupManager(d.dataDir)
	_, err = manager.CreatePreSchemaUpgradeBackup(update.CreateRetainedBackupInput{
		SourceAppVersion:    versioninfo.AppVersionForSchema(currentVersion),
		SourceSchemaVersion: currentVersion,
		TargetAppVersion:    buildinfo.AppVersion,
		TargetSchemaVersion: CurrentSchemaVersion,
		BuildIdentity:       buildinfo.BuildIdentity(),
	}, d.SnapshotTo)
	return err
}

func currentSchemaVersion(conn *sql.DB) (int, error) {
	version := 0
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// Close closes the underlying *sql.DB. Safe to call multiple times.
func (d *DB) Close() error {
	return d.conn.Close()
}

// Conn returns the underlying *sql.DB handle for callers that need raw query access (e.g. the per-service SQL files in internal/records).
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// DataDir returns the dataDir this *DB was opened against.
func (d *DB) DataDir() string {
	return d.dataDir
}
