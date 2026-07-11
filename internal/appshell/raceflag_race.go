//go:build race

package appshell

// raceEnabled reports whether the test binary was built with the
// race detector enabled (`go test -race` or `go build -race`).
// Internal/appshell/browse_response_time_test.go uses this to skip
// the 100ms perf-budget assertion under -race; see issue #466 for
// the runtime.checkptr cost in modernc.org/sqlite that makes the
// budget unreachable under -race.
//
// The build-tag-gated shape (this file + raceflag_norace.go) is
// the only way to detect -race at runtime; Go does not expose a
// public race.Enabled symbol.
func raceEnabled() bool { return true }