package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
)

const DefaultNodePrefix = "TDM65"

func NormalizeNodePrefix(prefix string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(prefix))
	if trimmed == "" {
		return DefaultNodePrefix
	}
	return trimmed
}

func (d *DB) SystemConfig(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM system_config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (d *DB) SetSystemConfig(key, value string) error {
	_, err := d.conn.Exec(`
		INSERT INTO system_config(key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, key, value)
	return err
}

func (d *DB) NodePrefix() (string, error) {
	value, err := d.SystemConfig("node_prefix")
	if err != nil {
		return "", err
	}
	return NormalizeNodePrefix(value), nil
}

func NewSyncID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x70
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
