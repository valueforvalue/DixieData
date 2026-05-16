package db

import "fmt"

func (d *DB) NextDXDID() (string, error) {
	nodePrefix, err := d.NodePrefix()
	if err != nil {
		return "", err
	}
	startIndex := len(nodePrefix) + len("-DXD-") + 1
	pattern := fmt.Sprintf("%s-DXD-[0-9][0-9][0-9][0-9][0-9]", nodePrefix)

	var maxID int
	err = d.conn.QueryRow(`
		SELECT COALESCE(MAX(CAST(SUBSTR(display_id, ?) AS INTEGER)), 0)
		FROM soldiers
		WHERE display_id GLOB ?
	`, startIndex, pattern).Scan(&maxID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-DXD-%05d", nodePrefix, maxID+1), nil
}
