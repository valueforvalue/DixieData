// Test files are exempt from the deferclose rule (issue #438).
// This fixture exercises that exemption: bare defer .Close() in
// a _test.go file is NOT flagged.
package defer_close

import "database/sql"

func goodTest(rows *sql.Rows) {
	defer rows.Close() // exempt because of _test.go suffix
	_ = rows
}
