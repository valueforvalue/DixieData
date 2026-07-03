package buildinfo

import (
	"strings"

	"github.com/valueforvalue/DixieData/internal/versioninfo"
)

const (
	AppName                   = "DixieData"
	CalendarTimeZone          = "America/Chicago"
	JSONExportVersion         = 3
	CSVExportVersion          = 4
	XLSXExportVersion         = 1
	ICalendarExportVersion    = 2
	SoldierPDFExportVersion   = 6
	MonthlyPDFExportVersion   = 1
	DatabasePDFExportVersion  = 4
	AnalyticsPDFExportVersion = 1
	BackupFormatVersion       = 3
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
	AppVersion    = versioninfo.AppVersion()
	SchemaVersion = versioninfo.CurrentSchemaVersion
)

var GitCommit = "dev"
var BuildTimestamp = ""

func AppLabel() string {
	return AppName + " v" + AppVersion
}

func BuildIdentity() string {
	parts := []string{}
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
