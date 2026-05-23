package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	preSchemaUpgradeBackupKind = "pre-schema-upgrade"
	retainedBackupIndexVersion = 1
	defaultMaxRetainedBackups  = 5
)

type SnapshotWriter func(outputPath string) error

type RetainedBackupPolicy struct {
	MaxBackups int `json:"max_backups"`
}

type RetainedBackupRecord struct {
	ID                   string `json:"id"`
	Kind                 string `json:"kind"`
	CreatedAt            string `json:"created_at"`
	SourceAppVersion     string `json:"source_app_version,omitempty"`
	SourceSchemaVersion  int    `json:"source_schema_version,omitempty"`
	TargetAppVersion     string `json:"target_app_version,omitempty"`
	TargetSchemaVersion  int    `json:"target_schema_version,omitempty"`
	BuildIdentity        string `json:"build_identity,omitempty"`
	DatabaseSnapshotPath string `json:"database_snapshot_path"`
	MetadataPath         string `json:"metadata_path,omitempty"`
}

type RetainedBackupIndex struct {
	Version int                    `json:"version"`
	Policy  RetainedBackupPolicy   `json:"policy"`
	Backups []RetainedBackupRecord `json:"backups"`
}

type CreateRetainedBackupInput struct {
	SourceAppVersion    string
	SourceSchemaVersion int
	TargetAppVersion    string
	TargetSchemaVersion int
	BuildIdentity       string
}

type RetainedBackupManager struct {
	dataDir string
	now     func() time.Time
	policy  RetainedBackupPolicy
}

func NewRetainedBackupManager(dataDir string) *RetainedBackupManager {
	return &RetainedBackupManager{
		dataDir: dataDir,
		now:     time.Now,
		policy:  RetainedBackupPolicy{MaxBackups: defaultMaxRetainedBackups},
	}
}

func (m *RetainedBackupManager) CreatePreSchemaUpgradeBackup(input CreateRetainedBackupInput, snapshot SnapshotWriter) (RetainedBackupRecord, error) {
	if snapshot == nil {
		return RetainedBackupRecord{}, fmt.Errorf("snapshot writer is required")
	}
	if err := os.MkdirAll(m.backupsRoot(), 0o755); err != nil {
		return RetainedBackupRecord{}, err
	}

	createdAt := m.now().UTC()
	record := RetainedBackupRecord{
		ID:                   m.backupID(createdAt, input.SourceSchemaVersion, input.TargetSchemaVersion),
		Kind:                 preSchemaUpgradeBackupKind,
		CreatedAt:            createdAt.Format(time.RFC3339),
		SourceAppVersion:     strings.TrimSpace(input.SourceAppVersion),
		SourceSchemaVersion:  input.SourceSchemaVersion,
		TargetAppVersion:     strings.TrimSpace(input.TargetAppVersion),
		TargetSchemaVersion:  input.TargetSchemaVersion,
		BuildIdentity:        strings.TrimSpace(input.BuildIdentity),
		DatabaseSnapshotPath: filepath.ToSlash(filepath.Join("updates", "backups", m.backupID(createdAt, input.SourceSchemaVersion, input.TargetSchemaVersion), "dixiedata-pre-upgrade.db")),
		MetadataPath:         filepath.ToSlash(filepath.Join("updates", "backups", m.backupID(createdAt, input.SourceSchemaVersion, input.TargetSchemaVersion), "metadata.json")),
	}

	backupDir := m.backupDir(record)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return RetainedBackupRecord{}, err
	}
	cleanupBackupDir := true
	defer func() {
		if cleanupBackupDir {
			_ = os.RemoveAll(backupDir)
		}
	}()

	snapshotPath := m.absolutePath(record.DatabaseSnapshotPath)
	if err := snapshot(snapshotPath); err != nil {
		return RetainedBackupRecord{}, err
	}
	if err := writeJSONAtomic(m.absolutePath(record.MetadataPath), record); err != nil {
		return RetainedBackupRecord{}, err
	}

	index, err := m.loadIndex()
	if err != nil {
		return RetainedBackupRecord{}, err
	}
	index.Policy = m.normalizePolicy(index.Policy)
	index.Backups = append([]RetainedBackupRecord{record}, filterBackupRecord(index.Backups, record.ID)...)
	sortRetainedBackups(index.Backups)
	index.Backups = m.pruneBackups(index.Backups)
	if err := writeJSONAtomic(m.indexPath(), index); err != nil {
		return RetainedBackupRecord{}, err
	}

	cleanupBackupDir = false
	return record, nil
}

func (m *RetainedBackupManager) List() ([]RetainedBackupRecord, error) {
	index, err := m.loadIndex()
	if err != nil {
		return nil, err
	}
	backups := append([]RetainedBackupRecord(nil), index.Backups...)
	return backups, nil
}

func (m *RetainedBackupManager) RestoreDatabaseSnapshot(id, outputPath string) (RetainedBackupRecord, error) {
	index, err := m.loadIndex()
	if err != nil {
		return RetainedBackupRecord{}, err
	}
	for _, record := range index.Backups {
		if record.ID != id {
			continue
		}
		if err := copyFileAtomic(m.absolutePath(record.DatabaseSnapshotPath), outputPath); err != nil {
			return RetainedBackupRecord{}, err
		}
		return record, nil
	}
	return RetainedBackupRecord{}, fmt.Errorf("retained backup %q not found", id)
}

func (m *RetainedBackupManager) backupsRoot() string {
	return filepath.Join(m.dataDir, "updates", "backups")
}

func (m *RetainedBackupManager) indexPath() string {
	return filepath.Join(m.backupsRoot(), "index.json")
}

func (m *RetainedBackupManager) backupID(createdAt time.Time, sourceSchemaVersion, targetSchemaVersion int) string {
	return fmt.Sprintf("schema-upgrade-%s-v%d-to-v%d", createdAt.Format("20060102-150405"), sourceSchemaVersion, targetSchemaVersion)
}

func (m *RetainedBackupManager) backupDir(record RetainedBackupRecord) string {
	if strings.TrimSpace(record.MetadataPath) != "" {
		return filepath.Dir(m.absolutePath(record.MetadataPath))
	}
	return filepath.Join(m.backupsRoot(), record.ID)
}

func (m *RetainedBackupManager) absolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.dataDir, filepath.FromSlash(path))
}

func (m *RetainedBackupManager) loadIndex() (RetainedBackupIndex, error) {
	content, err := os.ReadFile(m.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return RetainedBackupIndex{
				Version: retainedBackupIndexVersion,
				Policy:  m.normalizePolicy(RetainedBackupPolicy{}),
				Backups: []RetainedBackupRecord{},
			}, nil
		}
		return RetainedBackupIndex{}, err
	}
	var index RetainedBackupIndex
	if err := json.Unmarshal(content, &index); err != nil {
		return RetainedBackupIndex{}, err
	}
	index.Version = retainedBackupIndexVersion
	index.Policy = m.normalizePolicy(index.Policy)
	sortRetainedBackups(index.Backups)
	return index, nil
}

func (m *RetainedBackupManager) normalizePolicy(policy RetainedBackupPolicy) RetainedBackupPolicy {
	if policy.MaxBackups <= 0 {
		policy.MaxBackups = m.policy.MaxBackups
	}
	if policy.MaxBackups <= 0 {
		policy.MaxBackups = defaultMaxRetainedBackups
	}
	return policy
}

func (m *RetainedBackupManager) pruneBackups(backups []RetainedBackupRecord) []RetainedBackupRecord {
	limit := m.normalizePolicy(RetainedBackupPolicy{}).MaxBackups
	if len(backups) <= limit {
		return backups
	}
	kept := append([]RetainedBackupRecord(nil), backups[:limit]...)
	for _, stale := range backups[limit:] {
		if err := os.RemoveAll(m.backupDir(stale)); err != nil {
			kept = append(kept, stale)
		}
	}
	sortRetainedBackups(kept)
	return kept
}

func sortRetainedBackups(backups []RetainedBackupRecord) {
	sort.SliceStable(backups, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, backups[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339, backups[j].CreatedAt)
		if leftErr != nil || rightErr != nil {
			return backups[i].CreatedAt > backups[j].CreatedAt
		}
		return left.After(right)
	})
}

func filterBackupRecord(backups []RetainedBackupRecord, excludeID string) []RetainedBackupRecord {
	filtered := make([]RetainedBackupRecord, 0, len(backups))
	for _, record := range backups {
		if record.ID == excludeID {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

func copyFileAtomic(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(destinationPath), filepath.Base(destinationPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, source); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Remove(destinationPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tempPath, destinationPath); err != nil {
		return err
	}
	cleanupTemp = false
	return nil
}

func MarshalIndex(index RetainedBackupIndex) ([]byte, error) {
	buffer := bytes.Buffer{}
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(index); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
