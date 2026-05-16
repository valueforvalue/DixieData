package buildinfo

const (
	AppName                 = "DixieData"
	AppVersion              = "0.1.0"
	SchemaVersion           = 4
	JSONExportVersion       = 1
	CSVExportVersion        = 1
	ICalendarExportVersion  = 1
	SoldierPDFExportVersion = 2
	MonthlyPDFExportVersion = 1
	BackupFormatVersion     = 2
)

func AppLabel() string {
	return AppName + " v" + AppVersion
}
