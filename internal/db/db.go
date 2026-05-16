package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn                 *sql.DB
	dataDir              string
	scratchpadIndexMu    sync.Mutex
	scratchpadIndexState map[string]scratchpadFileState
	scratchpadIndexReady bool
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
	d := &DB{
		conn:                 conn,
		dataDir:              dataDir,
		scratchpadIndexState: map[string]scratchpadFileState{},
	}
	if err := backupBeforeMigrationIfNeeded(d, dbPath); err != nil {
		conn.Close()
		return nil, err
	}
	if err := applySchema(d); err != nil {
		conn.Close()
		return nil, err
	}
	if err := d.SyncScratchpadSearchIndex(); err != nil {
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
	backupDir := filepath.Join(d.dataDir, "backups", "schema-migrations")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, "dixiedata-pre-migration-"+time.Now().UTC().Format("20060102-150405")+".db")
	return d.SnapshotTo(backupPath)
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

type scratchpadFileState struct {
	ModTime   time.Time
	Size      int64
	SoldierID int64
}

func (d *DB) SyncScratchpadSearchIndex() error {
	d.scratchpadIndexMu.Lock()
	defer d.scratchpadIndexMu.Unlock()

	soldierRows, err := d.conn.Query(`SELECT id, display_id FROM soldiers`)
	if err != nil {
		return err
	}
	defer soldierRows.Close()

	soldierByStem := map[string]int64{}
	for soldierRows.Next() {
		var (
			soldierID int64
			displayID string
		)
		if err := soldierRows.Scan(&soldierID, &displayID); err != nil {
			return err
		}
		stem := scratchpadStemForDisplayID(displayID)
		if stem != "" {
			soldierByStem[stem] = soldierID
		}
	}
	if err := soldierRows.Err(); err != nil {
		return err
	}

	scratchpadDir := filepath.Join(d.dataDir, "scratchpads")
	entries, err := os.ReadDir(scratchpadDir)
	if err != nil {
		if os.IsNotExist(err) {
			if d.scratchpadIndexReady && len(d.scratchpadIndexState) == 0 {
				return nil
			}
			if _, clearErr := d.conn.Exec(`DELETE FROM scratchpad_cache`); clearErr != nil {
				return clearErr
			}
			d.scratchpadIndexState = map[string]scratchpadFileState{}
			d.scratchpadIndexReady = true
			return nil
		}
		return err
	}

	current := map[string]scratchpadFileState{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			continue
		}
		soldierID, ok := soldierByStem[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))]
		if !ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		current[entry.Name()] = scratchpadFileState{
			ModTime:   info.ModTime().UTC(),
			Size:      info.Size(),
			SoldierID: soldierID,
		}
		previous, seen := d.scratchpadIndexState[entry.Name()]
		if seen && previous.ModTime.Equal(current[entry.Name()].ModTime) && previous.Size == current[entry.Name()].Size && previous.SoldierID == soldierID {
			continue
		}
		content, err := os.ReadFile(filepath.Join(scratchpadDir, entry.Name()))
		if err != nil {
			return err
		}
		if _, err := d.conn.Exec(`
			INSERT INTO scratchpad_cache (soldier_id, scratch_pad, updated_at)
			VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(soldier_id) DO UPDATE SET scratch_pad = excluded.scratch_pad, updated_at = CURRENT_TIMESTAMP`,
			soldierID, string(content)); err != nil {
			return err
		}
	}

	if err := d.clearRemovedScratchpadCache(current); err != nil {
		return err
	}
	d.scratchpadIndexState = current
	d.scratchpadIndexReady = true
	return nil
}

func (d *DB) clearRemovedScratchpadCache(current map[string]scratchpadFileState) error {
	for name, previous := range d.scratchpadIndexState {
		if current != nil {
			if replacement, ok := current[name]; ok && replacement.SoldierID == previous.SoldierID {
				continue
			}
		}
		if _, err := d.conn.Exec(`DELETE FROM scratchpad_cache WHERE soldier_id = ?`, previous.SoldierID); err != nil {
			return err
		}
	}
	if current == nil {
		d.scratchpadIndexState = map[string]scratchpadFileState{}
	}
	return nil
}

func scratchpadStemForDisplayID(displayID string) string {
	textPath, _ := appdata.ScratchpadPaths("", displayID)
	return strings.TrimSuffix(filepath.Base(textPath), filepath.Ext(textPath))
}
