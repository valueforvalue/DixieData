package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/records"
)

type portabilityAudit struct {
	SchemaParity            map[string]map[string]string `json:"schema_parity"`
	FieldAudit              map[string]checkResult       `json:"field_audit"`
	RoundTripParity         map[string]checkResult       `json:"round_trip_parity"`
	SharedArchiveValidation map[string]checkResult       `json:"shared_archive_validation"`
	AssetIntegrity          map[string]checkResult       `json:"asset_integrity"`
	Lossless                bool                         `json:"lossless"`
}

func runPortabilityAudit(reportDir string) (report, error) {
	if err := os.RemoveAll(reportDir); err != nil {
		return report{}, err
	}
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return report{}, err
	}
	dataDir := filepath.Join(reportDir, "portability-audit-data")
	database, err := db.Open(dataDir)
	if err != nil {
		return report{}, err
	}
	defer database.Close()
	if _, err := database.ConfigureUserIdentity("Portability", "Audit", "Harness", 1890); err != nil {
		return report{}, err
	}

	soldierSvc := records.NewSoldierService(database)
	backupSvc := archive.NewBackupService(database, soldierSvc)
	fixture, err := seedFixture(dataDir, database, soldierSvc)
	if err != nil {
		return report{}, err
	}

	artifactsDir := filepath.Join(reportDir, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return report{}, err
	}
	beforeDB := filepath.Join(artifactsDir, "before.db")
	backupPath := filepath.Join(artifactsDir, "portability.ddbak")
	sharedPath := filepath.Join(artifactsDir, "portability.ddshare")
	if err := database.SnapshotTo(beforeDB); err != nil {
		return report{}, err
	}
	if _, err := backupSvc.Export(backupPath, dataDir); err != nil {
		return report{}, err
	}
	if _, err := backupSvc.ExportShared(sharedPath, dataDir); err != nil {
		return report{}, err
	}

	backupEntries, backupManifest, err := readZipWithManifest(backupPath)
	if err != nil {
		return report{}, err
	}
	sharedEntries, sharedManifest, err := readZipWithManifest(sharedPath)
	if err != nil {
		return report{}, err
	}
	backupDBPath := filepath.Join(artifactsDir, "backup-embedded.db")
	if err := os.WriteFile(backupDBPath, backupEntries["data/dixiedata.db"], 0o644); err != nil {
		return report{}, err
	}

	restoreDir := filepath.Join(reportDir, "restored-data")
	if _, err := backupSvc.Import(backupPath, restoreDir); err != nil {
		return report{}, err
	}
	afterDBPath := db.Path(restoreDir)

	targetDir := filepath.Join(reportDir, "shared-target")
	targetDB, err := db.Open(targetDir)
	if err != nil {
		return report{}, err
	}
	defer targetDB.Close()
	if _, err := targetDB.ConfigureUserIdentity("Receiver", "Local", "Archivist", 1910); err != nil {
		return report{}, err
	}
	targetSoldierSvc := records.NewSoldierService(targetDB)
	targetBackupSvc := archive.NewBackupService(targetDB, targetSoldierSvc)
	sharedSummary, err := targetBackupSvc.ImportSharedBackup(sharedPath, targetDir)
	if err != nil {
		return report{}, err
	}

	schemaParity, err := buildSchemaParity(beforeDB, backupDBPath)
	if err != nil {
		return report{}, err
	}
	fieldAudit, err := buildFieldAudit(beforeDB, backupDBPath, backupEntries, backupManifest, sharedEntries, sharedManifest, fixture)
	if err != nil {
		return report{}, err
	}
	roundTrip, err := buildRoundTripParity(beforeDB, afterDBPath)
	if err != nil {
		return report{}, err
	}
	sharedValidation, err := buildSharedValidation(beforeDB, db.Path(targetDir), sharedSummary, sharedEntries, sharedManifest, len(sharedEntries))
	if err != nil {
		return report{}, err
	}
	assetIntegrity, err := buildAssetIntegrity(beforeDB, backupEntries, restoreDir)
	if err != nil {
		return report{}, err
	}

	lossless := allChecksPass(fieldAudit) && allChecksPass(roundTrip) && allChecksPass(sharedValidation) && allChecksPass(assetIntegrity)
	return report{
		Mode:          "portability-audit",
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		Artifacts: map[string]string{
			"before_db":      beforeDB,
			"backup_archive": backupPath,
			"shared_archive": sharedPath,
			"restored_db":    afterDBPath,
			"shared_target":  db.Path(targetDir),
		},
		Notes: []string{
			"Audit executed against the live schema version, not the older v1.0.14 baseline cited in the brief.",
		},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Metrics: map[string]any{
			"shared_import_summary": sharedSummary,
			"portability": portabilityAudit{
				SchemaParity:            schemaParity,
				FieldAudit:              fieldAudit,
				RoundTripParity:         roundTrip,
				SharedArchiveValidation: sharedValidation,
				AssetIntegrity:          assetIntegrity,
				Lossless:                lossless,
			},
		},
	}, nil
}

func buildSchemaParity(liveDBPath, backupDBPath string) (map[string]map[string]string, error) {
	result := map[string]map[string]string{}
	for _, table := range []string{"soldiers", "records", "images"} {
		liveColumns, _, err := tableColumns(liveDBPath, table)
		if err != nil {
			return nil, err
		}
		_, backupColumns, err := tableColumns(backupDBPath, table)
		if err != nil {
			return nil, err
		}
		result[table] = map[string]string{}
		for _, column := range liveColumns {
			status := "missing"
			if backupColumns[column] {
				status = "confirmed"
			}
			result[table][column] = status
		}
	}
	return result, nil
}

func buildFieldAudit(liveDBPath, backupDBPath string, backupEntries map[string][]byte, backupManifest archive.BackupManifest, sharedEntries map[string][]byte, sharedManifest archive.BackupManifest, fixture sampleFixture) (map[string]checkResult, error) {
	result := map[string]checkResult{}

	systemConfigCount, err := tableCount(backupDBPath, "system_config")
	if err != nil {
		return nil, err
	}
	result["identity_and_versioning"] = check(
		"identity_and_versioning",
		backupManifest.SchemaVersion == buildinfo.SchemaVersion &&
			strings.TrimSpace(backupManifest.NodePrefix) != "" &&
			strings.TrimSpace(backupManifest.OwnerName) != "" &&
			systemConfigCount > 0,
		"Manifest includes schema/node metadata and the embedded database carries system_config.",
	)

	metadataPresent, err := rowPresent(backupDBPath, `SELECT 1 FROM soldiers WHERE id = ? AND entry_type = 'widow' AND needs_review = 0 AND COALESCE(review_reason, '') = ''`, fixture.Spouse.ID)
	if err != nil {
		return nil, err
	}
	result["new_record_metadata"] = check("new_record_metadata", metadataPresent, "Entry type and review metadata are present in the embedded backup database.")

	scratchpadCacheCount, err := tableCount(backupDBPath, "scratchpad_cache")
	if err != nil {
		return nil, err
	}
	result["scratchpads_and_search_cache"] = check(
		"scratchpads_and_search_cache",
		scratchpadCacheCount > 0,
		"Checks that scratch pad content survives inside the embedded archive database.",
	)

	duplicateAuditCount, err := tableCount(backupDBPath, "duplicate_audit_findings")
	if err != nil {
		return nil, err
	}
	result["duplicate_audit_findings"] = check("duplicate_audit_findings", duplicateAuditCount > 0, "Duplicate audit findings survive inside the embedded backup snapshot.")

	imageMetadataPresent, err := rowPresent(backupDBPath, `SELECT 1 FROM images WHERE soldier_id = ? AND is_primary = 1 AND (file_path LIKE 'images/%' OR file_path LIKE 'images\%')`, fixture.Soldier.ID)
	if err != nil {
		return nil, err
	}
	result["image_metadata"] = check("image_metadata", imageMetadataPresent, "Sharded image file_path values and is_primary flags are stored in the backup snapshot.")

	sharedIsRecordArchive := sharedManifest.ArchiveKind == "shared" &&
		sharedManifest.DataFormat == "json" &&
		strings.TrimSpace(sharedManifest.DataFile) != "" &&
		strings.TrimSpace(sharedManifest.DatabaseFile) == "" &&
		hasFile(sharedEntries, sharedManifest.DataFile)
	result["shared_archive_structure"] = check("shared_archive_structure", sharedIsRecordArchive, "Shared archive stores merge-ready record payloads instead of a full SQLite snapshot.")
	return result, nil
}

func buildRoundTripParity(beforeDBPath, afterDBPath string) (map[string]checkResult, error) {
	result := map[string]checkResult{}
	for _, table := range []string{
		"soldiers",
		"records",
		"images",
		"system_config",
		"duplicate_audit_findings",
		"scratchpad_cache",
		"research_tasks",
		"research_collections",
		"research_collection_items",
	} {
		diffCount, err := tableDiffCount(beforeDBPath, afterDBPath, table)
		if err != nil {
			return nil, err
		}
		result[table] = check(table, diffCount == 0, fmt.Sprintf("Table %s diff count = %d.", table, diffCount))
	}
	return result, nil
}

func buildSharedValidation(sourceDBPath, targetDBPath string, summary archive.SharedImportSummary, sharedEntries map[string][]byte, sharedManifest archive.BackupManifest, sharedEntryCount int) (map[string]checkResult, error) {
	result := map[string]checkResult{}

	spouseLinked, err := rowPresent(targetDBPath, `SELECT 1 FROM soldiers WHERE entry_type = 'widow' AND spouse_soldier_id IS NOT NULL AND spouse_soldier_id > 0`)
	if err != nil {
		return nil, err
	}
	result["relationship_continuity"] = check("relationship_continuity", spouseLinked, "Widow/husband links survive shared import.")

	recordArchive := sharedManifest.ArchiveKind == "shared" &&
		sharedManifest.DataFormat == "json" &&
		strings.TrimSpace(sharedManifest.DataFile) != "" &&
		strings.TrimSpace(sharedManifest.DatabaseFile) == "" &&
		hasFile(sharedEntries, sharedManifest.DataFile)
	result["subset_dependency_export"] = check("subset_dependency_export", recordArchive, fmt.Sprintf("Shared export now packages merge records and referenced assets without shipping a full snapshot (%d archive entries).", sharedEntryCount))

	namespaceRegenerated, err := rowPresent(targetDBPath, `SELECT 1 FROM soldiers WHERE display_id LIKE 'RLA10-%'`)
	if err != nil {
		return nil, err
	}
	result["namespace_regeneration"] = check("namespace_regeneration", namespaceRegenerated, "Checks whether imported shared records are regenerated into the receiver namespace.")

	addedByPreserved, err := rowPresent(targetDBPath, `SELECT 1 FROM soldiers WHERE COALESCE(added_by, '') <> ''`)
	if err != nil {
		return nil, err
	}
	result["added_by_preservation"] = check("added_by_preservation", addedByPreserved && summary.SoldiersInserted > 0, "Shared import preserves sender added_by metadata.")
	return result, nil
}

func buildAssetIntegrity(beforeDBPath string, backupEntries map[string][]byte, restoreDir string) (map[string]checkResult, error) {
	result := map[string]checkResult{}

	referencedPaths, err := imagePaths(beforeDBPath)
	if err != nil {
		return nil, err
	}
	allReferencedBundled := true
	for _, path := range referencedPaths {
		if !hasFile(backupEntries, path) {
			allReferencedBundled = false
			break
		}
	}
	result["referenced_images_bundled"] = check("referenced_images_bundled", allReferencedBundled, "All database-referenced images appear in the backup archive.")

	orphanIncluded := hasFile(backupEntries, "images/orphaned/orphan.png")
	result["orphan_filtering"] = check("orphan_filtering", !orphanIncluded, "Backup should exclude orphaned image files that are not referenced by the database.")

	relativePaths := true
	restoredImagesExist := true
	for _, path := range referencedPaths {
		if strings.Contains(path, ":\\") || strings.HasPrefix(path, "/") {
			relativePaths = false
		}
		if _, err := os.Stat(filepath.Join(restoreDir, filepath.FromSlash(strings.ReplaceAll(path, "\\", "/")))); err != nil {
			restoredImagesExist = false
		}
	}
	result["path_sanitization"] = check("path_sanitization", relativePaths && restoredImagesExist, "Image file paths stay relative and unpack correctly under a new root.")
	return result, nil
}

func tableColumns(dbPath, table string) ([]string, map[string]bool, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	rows, err := conn.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	columns := []string{}
	set := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, nil, err
		}
		columns = append(columns, name)
		set[name] = true
	}
	return columns, set, rows.Err()
}

func tableCount(dbPath, table string) (int, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func rowPresent(dbPath, query string, args ...any) (bool, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var marker int
	err = conn.QueryRow(query, args...).Scan(&marker)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func tableDiffCount(beforeDBPath, afterDBPath, table string) (int, error) {
	conn, err := sql.Open("sqlite", beforeDBPath)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.Exec(`ATTACH DATABASE ? AS afterdb`, afterDBPath); err != nil {
		return 0, err
	}
	defer conn.Exec(`DETACH DATABASE afterdb`)

	query := fmt.Sprintf(`
		SELECT
			(SELECT COUNT(*) FROM (SELECT * FROM main.%[1]s EXCEPT SELECT * FROM afterdb.%[1]s)) +
			(SELECT COUNT(*) FROM (SELECT * FROM afterdb.%[1]s EXCEPT SELECT * FROM main.%[1]s))
	`, table)
	var count int
	if err := conn.QueryRow(query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func imagePaths(dbPath string) ([]string, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.Query(`SELECT file_path FROM images ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := []string{}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, strings.ReplaceAll(path, "\\", "/"))
	}
	return paths, rows.Err()
}

func hasPathPrefix(entries map[string][]byte, prefix string) bool {
	for name := range entries {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func allChecksPass(checks map[string]checkResult) bool {
	for _, result := range checks {
		if !result.Passed {
			return false
		}
	}
	return true
}

func init() {
	_ = json.Valid
	_ = zip.Store
}
