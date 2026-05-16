package buildinfo

import "strings"

const (
	AppName                   = "DixieData"
	AppVersion                = "1.0.1"
	SchemaVersion             = 10
	JSONExportVersion         = 3
	CSVExportVersion          = 4
	XLSXExportVersion         = 1
	ICalendarExportVersion    = 2
	SoldierPDFExportVersion   = 5
	MonthlyPDFExportVersion   = 1
	DatabasePDFExportVersion  = 3
	AnalyticsPDFExportVersion = 1
	BackupFormatVersion       = 2
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
