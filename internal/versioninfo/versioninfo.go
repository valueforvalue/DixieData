// Package versioninfo defines and formats DixieData's schema, update-flow, and app release versions.
package versioninfo

import "fmt"

// CurrentSchemaVersion is the SQLite user_version the data
// plane ships with today. Bumped when migration files in
// internal/db/ land. Independent from the app version string
// (see CurrentUpdateFlowVersion below).
const CurrentSchemaVersion = 59

// CurrentUpdateFlowVersion is the update-flow-shape gate.
// Bumped when the auto-update mechanism itself changes shape
// (restore-point storage moves, eligibility rules change,
// in-place codepaths removed, etc.). Releases with a higher
// U cannot be auto-applied by a binary with the current U;
// the user must reinstall.
//
// Default U=1 covers every release published before #266
// landed; legacy v1.2.{N} strings parse to U=1 in the
// update-flow's compareVersions.
//
// Doc touchpoints for the planned future surface updates
// (follow-up issues): cli_*.go output shape, bump-version.ps1,
// RELEASING.md, ArchiveManifest field, footer display.
const CurrentUpdateFlowVersion = 1

// AppVersion returns the human-facing release version string.
// Shape: v{MAJOR}.{U}.{N} where N is the release counter
// (every release bumps N; not tied 1:1 to CurrentSchemaVersion
// because bug-fix-only releases bump N without a schema
// change). Reference: issue #266.
func AppVersion() string {
	return fmt.Sprintf("1.%d.%d", CurrentUpdateFlowVersion, AppRelease())
}

// AppRelease returns the current release counter (N). Kept
// as a separate helper so future bump scripts can write a new
// N independent of any schema-bump event.
func AppRelease() int {
	return CurrentAppVersionInt
}

// CurrentAppVersionInt is the current release counter (N).
// Kept as a var (not a const) because future bump scripts may
// want to compute it from a source-of-truth file before the
// binary links. Initial value = 1 (the first release under the
// new model). Historical releases had N == CurrentSchemaVersion;
// going forward those two diverge.
var CurrentAppVersionInt = 1

// AppVersionForSchema composes an AppVersion-like string using
// the historical formula (v1.2.{schema}). Kept for callers
// that need the old shape — do NOT use for new code; use
// AppVersion() instead.
func AppVersionForSchema(schemaVersion int) string {
	if schemaVersion < 0 {
		schemaVersion = 0
	}
	return fmt.Sprintf("1.2.%d", schemaVersion)
}

// CurrentAppVersion returns the historical app version string
// (v1.2.{schema}). Kept for callers that need the old shape.
// New code should use AppVersion() (v1.{U}.{N}).
func CurrentAppVersion() string {
	return AppVersionForSchema(CurrentSchemaVersion)
}
