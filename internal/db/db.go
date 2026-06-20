package db

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/valueforvalue/DixieData/internal/versioninfo"
	_ "modernc.org/sqlite"
)

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
		return nil, err
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

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) Conn() *sql.DB {
	return d.conn
}

func (d *DB) DataDir() string {
	return d.dataDir
}
