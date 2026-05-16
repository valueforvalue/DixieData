package buildinfo

import "strings"

const (
	AppName                  = "DixieData"
	AppVersion               = "0.1.0"
	SchemaVersion            = 7
	JSONExportVersion        = 1
	CSVExportVersion         = 2
	ICalendarExportVersion   = 1
	SoldierPDFExportVersion  = 3
	MonthlyPDFExportVersion  = 1
	DatabasePDFExportVersion = 1
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
