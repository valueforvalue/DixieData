package db

import "strings"

func (d *DB) NextDXDID() (string, error) {
	nodePrefix, err := d.NodePrefix()
	if err != nil {
		return "", err
	}
	rows, err := d.conn.Query(`SELECT display_id, is_generated FROM soldiers`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	maxID := 0
	for rows.Next() {
		var (
			displayID   string
			isGenerated bool
		)
		if err := rows.Scan(&displayID, &isGenerated); err != nil {
			return "", err
		}
		sequence, ok := generatedDisplayIDSequence(displayID, nodePrefix, isGenerated)
		if ok && sequence > maxID {
			maxID = sequence
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return NextGeneratedDisplayID(nodePrefix, maxID+1), nil
}

func generatedDisplayIDSequence(displayID, nodePrefix string, isGenerated bool) (int, bool) {
	namespace, sequence, ok := CanonicalDisplayID(SanitizeID(displayID, nodePrefix))
	if !ok {
		return 0, false
	}
	if isGenerated || strings.EqualFold(namespace, LegacyDisplayIDNamespace) || strings.EqualFold(namespace, NormalizeNodePrefix(nodePrefix)) {
		return sequence, true
	}
	return 0, false
}
