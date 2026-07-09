// Package defer_close holds the negative fixtures for the
// deferclose analyzer. Every function in this file is expected
// to produce a diagnostic; the magic comments on each line
// tell the analysistest harness which line numbers to expect.
package defer_close

import (
	"database/sql"
	"os"
)

// Bad: bare defer .Close() with no comment. The analyzer must
// report this site.
func badBare(rows *sql.Rows) {
	defer rows.Close() // want `defer \.Close\(\) discards the error`
	_ = rows
}

// Bad: defer on a chain. The receiver expression is `os.File`
// accessed via a variable; the analyzer must still match.
func badChained(f *os.File) {
	defer f.Close() // want `defer \.Close\(\) discards the error`
	_ = f
}

// Good: a defer on a function that does NOT end in .Close().
// Not a Close() method — no diagnostic expected.
func goodNotClose(fn func()) {
	defer fn()
	_ = fn
}

// Good: a non-defer .Close() call. The lint rule only matches
// defer statements.
func goodNotDefer(rows *sql.Rows) error {
	return rows.Close()
}

// Good: the bail-out. The //nolint:dixie/deferclose comment
// suppresses the diagnostic.
func goodBailOut(rows *sql.Rows) {
	defer rows.Close() //nolint:dixie/deferclose
	_ = rows
}
