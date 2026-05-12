package db

import "fmt"

func (d *DB) NextDXDID() (string, error) {
	var count int
	err := d.conn.QueryRow("SELECT COUNT(*) FROM soldiers WHERE is_generated = 1").Scan(&count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DXD-%05d", count+1), nil
}
