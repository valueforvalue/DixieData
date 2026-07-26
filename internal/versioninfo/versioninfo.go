// Package versioninfo defines and formats DixieData's schema, update-flow, and app release versions.
package versioninfo

import "fmt"

// AppName is the human-readable app name emitted alongside the
// codename in chrome surfaces. Mirrors buildinfo.AppName for
// callers that don't want the import cycle.
const AppName = "DixieData"

// CurrentSchemaVersion is the SQLite user_version the data
// plane ships with today. Bumped when migration files in
// internal/db/ land. Independent from the app version string
// (see CurrentUpdateFlowVersion below).
//
// Issue #459: bumped 66 → 67 when ensureSoldierFTS split out of
// block-60-v54-to-v60-jump into its own per-block-commit entry
// (block-67-ensure-soldier-fts). The split unsticks the
// connectionOpener-held SHARED lock collision that surfaced as
// SQLITE_LOCKED (6) at every .ddbak restore path on Windows.
// Fresh v66 archives already in the wild stay on v66 until
// they re-open on a v67 binary; applySchema short-circuits on
// version >= 67, so block-67's Up is no-op for fresh v66+. v54-
// v66 archives land in the v67 state via block-60 (sets columns)
// + block-67 (sets FTS5).
const CurrentSchemaVersion = 68

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

// CurrentReleaseTag is the optional pre-release suffix
// appended to AppVersion in chrome (footer, window title, CLI,
// settings/build panel). Empty for stable releases; "-rc1",
// "-rc2", etc. for the RC cohort's pre-release cuts. The
// updater's versionFromString regex strips the suffix before
// comparing (the regex only captures 3 numeric segments), so
// the numeric comparison is unaffected — a cohort on
// 1.1.4-rc1 sees a manifest advertising 1.1.4-rc2 as
// "different enough" because the version string is different,
// but the comparison happens on 1.1.4 vs 1.1.4 (same) — see
// the cohort's manifest-host workflow (dixiedata-rc-manifest)
// for how the cohort actually receives new RCs.
//
// The default "" matches a stable release. Set at build time
// via -ldflags (scripts/build-common.ps1) when packaging an
// RC zip: `-X github.com/valueforvalue/DixieData/internal/versioninfo.CurrentReleaseTag=rc1`
// yields AppVersionFull() == "1.1.4-rc1". The Chrome surfaces
// read this via buildinfo.AppVersionFull() / AppLabel().
var CurrentReleaseTag = ""

// AppVersionFull returns AppVersion with the optional
// pre-release suffix appended: "1.1.4" when CurrentReleaseTag
// is empty, "1.1.4-rc1" when CurrentReleaseTag is "rc1". This
// is the string chrome surfaces display; the bare AppVersion
// stays canonical for the updater's version comparison + every
// `app_version` field in BackupManifest / .ddbak / gold-master
// portable output.
func AppVersionFull() string {
	if CurrentReleaseTag == "" {
		return AppVersion()
	}
	return AppVersion() + "-" + CurrentReleaseTag
}

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
//
// Issue #578 catch-up (2026-07-14): bumped from 1 to 4 to
// reflect the body of post-#266 release-aimed work on dev
// branch — the #544 + #566 feedback chain (Formspark wire-up
// and the slice-1/2/3 set of commits), the #561 microcopy
// sweep (slices 1-16 across multiple commits), and the #570
// buildinfo consolidation. The slice-1 regression test
// (`internal/versioninfo/app_version_int_test.go::TestCurrentAppVersionIntReflectsPostCutoverWork`)
// pins this value; the slice-2 CI gate at
// `.github/workflows/test.yml` enforces per-release +1 from
// here forward.
//
// RC1 cohort (2026-07-26): bumped from 4 to 5 so the RC
// manifest can advertise 1.1.5-rc1, making the updater's
// compareVersions see Newer=true against the installed 1.1.4
// binary (issue #658).
var CurrentAppVersionInt = 17

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

// CurrentReleaseName is the human-friendly codename the user
// picks per release (issue #370). Single source of truth -
// every chrome surface (footer, window title, CLI --version,
// /settings/build panel, gold-master report) reads from this
// constant. Bumped by scripts/bump-version.ps1 -BumpCodename
// exactly like the other counters (mutually exclusive with
// -BumpSchema / -BumpUpdateFlow / -BumpRelease per the law in
// CONTEXT.md Release counter N != schema version).
//
// Naming rules:
//   - Single English word(s) (no hyphens, no underscores)
//   - Thematic - Southern place names work for DixieData
//   - Stable across the lifetime of one release; deprecation
//     rule for embarrassing names lives in docs/RELEASING.md
//
// First codename: First Manassas (the first battle of
// Bull Run, July 21 1861 - thematically apt for the first
// named release).
var CurrentReleaseName = "First Manassas"

// ReleaseLabel is the chrome-friendly string combining the app
// name and the current codename: "DixieData First Manassas".
// Window title, footer, and CLI banner read from this helper
// so a codename rename updates all chrome surfaces in one
// place. Distinct from AppLabel() (which carries the numeric
// version, "DixieData v1.1.65"); both render in the footer
// together - "DixieData v1.1.65 . First Manassas".
func ReleaseLabel() string {
	return AppName + " " + CurrentReleaseName
}
