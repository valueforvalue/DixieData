package db

import "fmt"

func (d *DB) NextDXDID() (string, error) {
	var maxID int
	err := d.conn.QueryRow(`
		SELECT COALESCE(MAX(CAST(SUBSTR(display_id, 5) AS INTEGER)), 0)
		FROM soldiers
		WHERE display_id GLOB 'DXD-[0-9][0-9][0-9][0-9][0-9]'
	`).Scan(&maxID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DXD-%05d", maxID+1), nil
}
