//go:build !race

package appshell

// raceEnabled reports whether the test binary was built with the
// race detector enabled. See raceflag_race.go for the -race
// counterpart.
func raceEnabled() bool { return false }