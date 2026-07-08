// Package buildinfo exposes app version, schema version, and export-format version constants.
package buildinfo

import (
	"strings"

	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

const (
	// AppName is the human-readable name emitted in every UI
	// surface, CLI banner, and exported PDF header. Stable since
	// 2024; not versioned.
	AppName = "DixieData"
	// CalendarTimeZone is the IANA tz the calendar + anniversaries
	// surface assumes for "today" + "this month" semantics. All
	// anniversaries are stored as month/day (no year) so the tz
	// affects only the today-vs-occurred split, not the dates
	// themselves.
	CalendarTimeZone = "America/Chicago"
	// JSONExportVersion is bumped every time the JSON export shape
	// (internal/models.Soldier + Records + Images) changes. Read
	// back at JSON import time to gate compatibility.
	JSONExportVersion = 3
	// CSVExportVersion is the version of the per-soldier CSV
	// export shape; one row per soldier, no records/images.
	CSVExportVersion = 4
	// XLSXExportVersion is the version of the Excel workbook
	// export (one sheet per kind: soldiers, records, images).
	XLSXExportVersion = 1
	// ICalendarExportVersion is the version of the iCal feed
	// exported as a Static Archive; one VEVENT per anniversary.
	ICalendarExportVersion = 2
	// SoldierPDFExportVersion is the version of the per-soldier
	// PDF rendering (Typst-backed since the slice 7 cutover).
	SoldierPDFExportVersion = 6
	// MonthlyPDFExportVersion is the version of the per-month
	// anniversary-grid PDF; distinct from SoldierPDFExportVersion
	// because the layout pipeline is different.
	MonthlyPDFExportVersion = 1
	// DatabasePDFExportVersion is the version of the full-database
	// PDF (every soldier in one document). Bumped when the
	// multi-soldier layout changes.
	DatabasePDFExportVersion = 4
	// AnalyticsPDFExportVersion is the version of the insights +
	// analytics PDF (cemeteries, homes, pensions, duplicates).
	AnalyticsPDFExportVersion = 1
	// BackupFormatVersion is the version of the .ddbak backup
	// archive shape. Read back at restore time to decide whether
	// to apply a forward-migration or refuse the restore.
	BackupFormatVersion = 3
	// MemorialArchiveFormatVersion is the version string the
	// FindAGraveScraper browser-side script (externals/
	// FindaGraveScraper.user.js) stamps into every export.
	// Independent of DixieData semver — bumped when the SCRIPT
	// changes shape (entry fields renamed, fields added,
	// envelope structure changed), NOT when DixieData itself
	// ships. Per-surface namespace pattern (Decision 1 in #383).
	// Read back at ImportMemorialArchive time:
	//   - missing → treat as pre-v1, warn in summary, import
	//   - same → silent
	//   - minor bump (memorial_v1.0 → memorial_v1.1) → warn in summary, import
	//   - major bump (memorial_v1 → memorial_v2) → refuse (typed error)
	MemorialArchiveFormatVersion = "memorial_v1"
	// CSVFormatVersion is the discoverable stamp the DixieData
	// CSV exporter (internal/archive.ExportCSV) writes into
	// the per-row metadata block as a `format_version` column.
	// Bumped on DixieData-side CSV shape changes (column
	// additions, encoding changes, header rewrites). Distinct
	// from the legacy integer CSVExportVersion (which is the
	// row-shape counter); the per-surface namespace pattern
	// (Decision 1 in #383) decouples from DixieData semver.
	CSVFormatVersion = "csv_v1"
	// ICalendarFormatVersion is the discoverable stamp the
	// DixieData iCal exporter writes as the
	// `X-DIXIEDATA-FORMAT-VERSION` extension property. Sibling
	// to ICalendarExportVersion (integer row-shape counter).
	ICalendarFormatVersion = "ical_v1"
	// JPGFormatVersion is the discoverable stamp the DixieData
	// JPG exporter writes into a sidecar `.meta.json` next to
	// each rendered page. JPGs don't have an obvious header
	// field for stamps (no envelope, EXIF is limited); the
	// sidecar is the practical hook. Bumped on DixieData-side
	// JPG layout changes (page count changes, filename
	// conventions change, embedded PDF metadata changes).
	JPGFormatVersion = "jpg_v1"
	// PDFFormatVersion is the discoverable stamp the DixieData
	// Typst render path threads into every PDF via the
	// `dixiedata_format_version` --input flag (templates read
	// via `sys.inputs.dixiedata_format_version`). Per-template
	// version (events_v7, soldier_landscape_v6) is separate
	// and lives in the .typ files themselves; this constant
	// is the DixieData-side envelope stamp that travels with
	// every PDF regardless of which template rendered it.
	PDFFormatVersion = "pdf_v1"
	// DDBakFormatVersion is the discoverable stamp the
	// DixieData user-export .ddbak writer writes into the
	// root-level manifest.json as `format_version`. Distinct
	// from the legacy integer BackupFormatVersion (the
	// row-shape counter); the per-surface namespace pattern
	// (Decision 1 in #383) decouples from DixieData semver.
	DDBakFormatVersion = "ddbak_v1"
	// JSONFormatVersion is the discoverable stamp the
	// DixieData JSON exporter writes into the metadata
	// envelope as `format_version`. Sibling to
	// JSONExportVersion (integer row-shape counter).
	JSONFormatVersion = "json_v1"
	// XLSXFormatVersion is the discoverable stamp the
	// DixieData Excel exporter writes into the per-row
	// metadata block + the workbook properties. Sibling to
	// XLSXExportVersion (integer row-shape counter).
	XLSXFormatVersion = "xlsx_v1"
)

// AppVersion is the release-line version string in the
// v{MAJOR}.{U}.{N} shape (issue #266). It drives every
// CLI emission (`debug dump`, `migrate status`, `restore
// point create/list`, export/import SourceAppVersion/
// TargetAppVersion), the update UI's Settings panel
// (`update.Settings.CurrentVersion`), the BackupManifest
// `app_version` field, and `cmd/gold-master/main.go`
// portable-output emit sites. Switched from
// `versioninfo.CurrentAppVersion()` (legacy `v1.2.{schema}`)
// on 2026-07-03; legacy callers of `AppVersionForSchema` /
// `CurrentAppVersion` still work for parse/serialise of old
// .ddbak archives and old GitHub release tags (issue #266
// decision 1: legacy strings parse to U=1).
var (
	// AppVersion is the release-line version string in the
	// v{MAJOR}.{U}.{N} shape (issue #266). It drives every
	// CLI emission (`debug dump`, `migrate status`, `restore
	// point create/list`, export/import SourceAppVersion/
	// TargetAppVersion), the update UI's Settings panel
	// (`update.Settings.CurrentVersion`), the BackupManifest
	// `app_version` field, and `cmd/gold-master/main.go`
	// portable-output emit sites. Switched from
	// `versioninfo.CurrentAppVersion()` (legacy `v1.2.{schema}`)
	// on 2026-07-03; legacy callers of `AppVersionForSchema` /
	// `CurrentAppVersion` still work for parse/serialise of old
	// .ddbak archives and old GitHub release tags (issue #266
	// decision 1: legacy strings parse to U=1).
	AppVersion = versioninfo.AppVersion()
	// SchemaVersion is the current schema version the binary
	// expects. The PR-time bump-verify gate (CI: schema-touching
	// bump detector) enforces that every feat(db) commit bumps
	// this in the same PR.
	SchemaVersion = versioninfo.CurrentSchemaVersion
)

// GitCommit is the git SHA the binary was built from. Set by the
// build pipeline (scripts/build-common.ps1 Invoke-DixieDataBuild).
// Empty in dev builds; the user sees "commit dev" in the
// diagnostic bundle in that case.
var GitCommit = "dev"

// BuildTimestamp is the RFC3339 timestamp the binary was built
// at. Set by the build pipeline; empty in dev builds.
var BuildTimestamp = ""

// GitBranch is the git branch the binary was built from. Set by
// the build pipeline (scripts/build-common.ps1) via the same
// -X ldflag that injects GitCommit + BuildTimestamp. Default "dev"
// matches GitCommit so a developer running `go test` (no ldflag)
// sees a coherent "dev" identity in the footer.
var GitBranch = "dev"

// AppLabel returns the human-readable name + version string for
// UI banners and CLI headers: "DixieData v1.1.55". Stable shape;
// the UI's title bar, the CLI's `--version` output, and the
// diagnostics bundle header all use this.
func AppLabel() string {
	return AppName + " v" + AppVersion
}

// BuildIdentity returns a short description of the binary's
// provenance: "commit <sha> · <timestamp>" when both are set,
// "commit dev" otherwise. Surfaced in the diagnostic bundle and
// the support-request form so the support engineer can identify
// exactly which build the user is on.
func BuildIdentity() string {
	parts := []string{}
	if strings.TrimSpace(GitBranch) != "" {
		parts = append(parts, strings.TrimSpace(GitBranch))
	}
	if strings.TrimSpace(GitCommit) != "" {
		parts = append(parts, "commit "+strings.TrimSpace(GitCommit))
	}
	if strings.TrimSpace(BuildTimestamp) != "" {
		parts = append(parts, strings.TrimSpace(BuildTimestamp))
	}
	if len(parts) == 0 {
		return "commit dev"
	}
	return strings.Join(parts, " · ")
}

// ReleaseLabel is the chrome-friendly codename string. Re-exported
// from versioninfo so the footer + window title read from one
// import path (buildinfo is already imported by every chrome site).
// Mirrors versioninfo.ReleaseLabel; intentionally a thin pass-through
// rather than a const so a future maintainer can override the
// release name from buildinfo (e.g. a custom build that brands itself
// differently) without touching versioninfo.
func ReleaseLabel() string {
	return versioninfo.ReleaseLabel()
}
