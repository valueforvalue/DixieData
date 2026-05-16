package buildinfo

import "strings"

const (
	AppName                  = "DixieData"
	AppVersion               = "1.0.0"
	SchemaVersion            = 9
	JSONExportVersion        = 3
	CSVExportVersion         = 4
	XLSXExportVersion        = 1
	ICalendarExportVersion   = 2
	SoldierPDFExportVersion  = 4
	MonthlyPDFExportVersion  = 1
	DatabasePDFExportVersion = 2
	BackupFormatVersion      = 2
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
