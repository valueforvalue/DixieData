package versioninfo

import "fmt"

const CurrentSchemaVersion = 59

func AppVersionForSchema(schemaVersion int) string {
	if schemaVersion < 0 {
		schemaVersion = 0
	}
	return fmt.Sprintf("1.2.%d", schemaVersion)
}

func CurrentAppVersion() string {
	return AppVersionForSchema(CurrentSchemaVersion)
}
