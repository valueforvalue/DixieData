package db

import (
	"fmt"
	"strconv"
	"strings"
)

func (d *DB) NextDXDID() (string, error) {
	nodePrefix, err := d.NodePrefix()
	if err != nil {
		return "", err
	}
	rows, err := d.conn.Query(`SELECT display_id FROM soldiers`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	maxID := 0
	for rows.Next() {
		var displayID string
		if err := rows.Scan(&displayID); err != nil {
			return "", err
		}
		sequence, ok := generatedDisplayIDSequence(displayID, nodePrefix)
		if ok && sequence > maxID {
			maxID = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%05d", nodePrefix, maxID+1), nil
}

func generatedDisplayIDSequence(displayID, nodePrefix string) (int, bool) {
	trimmed := strings.TrimSpace(displayID)
	prefix := NormalizeNodePrefix(nodePrefix)
	candidates := []string{
		prefix + "-",
		prefix + "-DXD-",
		"DXD-",
	}
	for _, candidate := range candidates {
		if !strings.HasPrefix(trimmed, candidate) {
			continue
		}
		value := strings.TrimPrefix(trimmed, candidate)
		if len(value) != 5 {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		return parsed, true
	}
	return 0, false
}
