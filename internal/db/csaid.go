package db

import (
	"fmt"
	"strings"
)

// NextDXDID returns the next sequential DixieData ID (DXDID) for a newly-imported archive row. Monotonically increasing across the lifetime of the Local Archive.
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

// EventDisplayIDNamespace is the canonical namespace for Event Record
// Display IDs (issue #320). Mirrors the LegacyDisplayIDNamespace
// constant for the DXD- prefix; Event Records use EVT- so a Service
// Timeline or browse list can disambiguate at a glance.
const EventDisplayIDNamespace = "EVT"

// NextEventID returns the next sequential Event Record Display ID in
// the EVT-NNNNN namespace (issue #320). Mirrors NextDXDID but
// namespace-scoped to EVT-; the counter is independent of the
// per-user node prefix and the per-Person DXD- counter, so the
// two sequences never collide.
//
// On a fresh archive, returns EVT-00001. On an archive with an
// existing EVT-NNNNN row, returns EVT-(MAX+1).
func (d *DB) NextEventID() (string, error) {
	rows, err := d.conn.Query(`SELECT display_id FROM soldiers WHERE display_id LIKE ?`, EventDisplayIDNamespace+"-%")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	maxSeq := 0
	for rows.Next() {
		var displayID string
		if err := rows.Scan(&displayID); err != nil {
			return "", err
		}
		_, seq, ok := CanonicalDisplayID(SanitizeID(displayID, ""))
		if !ok {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%05d", EventDisplayIDNamespace, maxSeq+1), nil
}
