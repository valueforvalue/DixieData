package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

const scratchpadLegacyImportConfigKey = "scratchpad_legacy_import_complete"

func (d *DB) Scratchpad(displayID string) (string, time.Time, error) {
	displayID = strings.TrimSpace(displayID)
	if displayID == "" {
		return "", time.Time{}, errors.New("missing record ID")
	}

	var (
		content     string
		updatedUnix int64
	)
	err := d.conn.QueryRow(`
		SELECT COALESCE(c.scratch_pad, ''), COALESCE(unixepoch(c.updated_at), 0)
		FROM soldiers s
		LEFT JOIN scratchpad_cache c ON c.soldier_id = s.id
		WHERE s.display_id = ?`,
		displayID,
	).Scan(&content, &updatedUnix)
	if err == sql.ErrNoRows {
		return "", time.Time{}, errors.New("record not found")
	}
	if err != nil {
		return "", time.Time{}, err
	}

	if updatedUnix <= 0 {
		return content, time.Time{}, nil
	}
	return content, time.Unix(updatedUnix, 0).UTC(), nil
}

func (d *DB) SaveScratchpad(displayID, content string) error {
	soldierID, err := d.soldierIDByDisplayID(displayID)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(`
		INSERT INTO scratchpad_cache (soldier_id, scratch_pad, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(soldier_id) DO UPDATE SET
			scratch_pad = excluded.scratch_pad,
			updated_at = CURRENT_TIMESTAMP`,
		soldierID, content,
	)
	return err
}

func (d *DB) ScratchpadCount() (int, error) {
	var count int
	err := d.conn.QueryRow(`
		SELECT COUNT(*)
		FROM scratchpad_cache
		WHERE TRIM(COALESCE(scratch_pad, '')) <> ''`,
	).Scan(&count)
	return count, err
}

func (d *DB) ImportLegacyScratchpadFiles() error {
	configValue, err := d.SystemConfig(scratchpadLegacyImportConfigKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(configValue) == "1" {
		return nil
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	entries, err := os.ReadDir(filepath.Join(d.dataDir, "scratchpads"))
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		entries = nil
	}

	soldierByStem, err := scratchpadSoldierIDsByStem(tx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".txt") {
			continue
		}
		soldierID, ok := soldierByStem[strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))]
		if !ok {
			continue
		}
		content, err := os.ReadFile(filepath.Join(d.dataDir, "scratchpads", entry.Name()))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT INTO scratchpad_cache (soldier_id, scratch_pad, updated_at)
			VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(soldier_id) DO UPDATE SET
				scratch_pad = excluded.scratch_pad,
				updated_at = CURRENT_TIMESTAMP`,
			soldierID, string(content),
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
		INSERT INTO system_config(key, value)
		VALUES (?, '1')
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP`,
		scratchpadLegacyImportConfigKey,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) soldierIDByDisplayID(displayID string) (int64, error) {
	displayID = strings.TrimSpace(displayID)
	if displayID == "" {
		return 0, errors.New("missing record ID")
	}
	var soldierID int64
	if err := d.conn.QueryRow(`SELECT id FROM soldiers WHERE display_id = ?`, displayID).Scan(&soldierID); err != nil {
		if err == sql.ErrNoRows {
			return 0, errors.New("record not found")
		}
		return 0, err
	}
	return soldierID, nil
}

func scratchpadSoldierIDsByStem(tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.Query(`SELECT id, display_id FROM soldiers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string]int64{}
	for rows.Next() {
		var (
			soldierID int64
			displayID string
		)
		if err := rows.Scan(&soldierID, &displayID); err != nil {
			return nil, err
		}
		stem := scratchpadStemForDisplayID(displayID)
		if stem != "" {
			result[stem] = soldierID
		}
	}
	return result, rows.Err()
}

func scratchpadStemForDisplayID(displayID string) string {
	textPath, _ := appdata.ScratchpadPaths("", displayID)
	return strings.TrimSuffix(filepath.Base(textPath), filepath.Ext(textPath))
}
