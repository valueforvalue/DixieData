package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

const (
	restorePointIndexVersion = 1
	restorePointStateVersion = 1
	defaultMaxRestorePoints  = 2

	RestorePointLaunchPrepared = "prepared"
	RestorePointLaunchStarting = "starting"
)

type RestorePointArchiveWriter func(outputPath string) error
type InstalledBuildSnapshotWriter func(outputDir string) error

type RestorePointPolicy struct {
	MaxRestorePoints int `json:"max_restore_points"`
}

type RestorePointRecord struct {
	ID                  string `json:"id"`
	CreatedAt           string `json:"created_at"`
	SourceAppVersion    string `json:"source_app_version,omitempty"`
	TargetAppVersion    string `json:"target_app_version,omitempty"`
	SourceBuildIdentity string `json:"source_build_identity,omitempty"`
	TargetBuildIdentity string `json:"target_build_identity,omitempty"`
	LocalArchivePath    string `json:"local_archive_path"`
	InstalledBuildPath  string `json:"installed_build_path"`
	MetadataPath        string `json:"metadata_path,omitempty"`
}

type RestorePointIndex struct {
	Version       int                  `json:"version"`
	Policy        RestorePointPolicy   `json:"policy"`
	RestorePoints []RestorePointRecord `json:"restore_points"`
}

type CreateRestorePointInput struct {
	SourceAppVersion    string
	TargetAppVersion    string
	SourceBuildIdentity string
	TargetBuildIdentity string
}

type RestorePointLaunchState struct {
	Version             int    `json:"version"`
	RestorePointID      string `json:"restore_point_id"`
	TargetAppVersion    string `json:"target_app_version"`
	TargetBuildIdentity string `json:"target_build_identity,omitempty"`
	Status              string `json:"status"`
	PreparedAt          string `json:"prepared_at"`
	StartedAt           string `json:"started_at,omitempty"`
}

func (s RestorePointLaunchState) MatchesCurrentBuild(currentVersion, currentBuildIdentity string) bool {
	currentVersion = strings.TrimSpace(currentVersion)
	currentBuildIdentity = strings.TrimSpace(currentBuildIdentity)
	if currentVersion == "" || strings.TrimSpace(s.TargetAppVersion) == "" {
		return false
	}
	if !strings.EqualFold(currentVersion, strings.TrimSpace(s.TargetAppVersion)) {
		return false
	}
	if strings.TrimSpace(s.TargetBuildIdentity) == "" || currentBuildIdentity == "" {
		return true
	}
	return strings.EqualFold(currentBuildIdentity, strings.TrimSpace(s.TargetBuildIdentity))
}

type RestorePointManager struct {
	dataDir string
	now     func() time.Time
	policy  RestorePointPolicy
}

func NewRestorePointManager(dataDir string) *RestorePointManager {
	return &RestorePointManager{
		dataDir: dataDir,
		now:     time.Now,
		policy:  RestorePointPolicy{MaxRestorePoints: defaultMaxRestorePoints},
	}
}

func (m *RestorePointManager) Create(input CreateRestorePointInput, archiveWriter RestorePointArchiveWriter, buildWriter InstalledBuildSnapshotWriter) (RestorePointRecord, error) {
	if archiveWriter == nil {
		return RestorePointRecord{}, fmt.Errorf("restore point archive writer is required")
	}
	if buildWriter == nil {
		return RestorePointRecord{}, fmt.Errorf("installed build snapshot writer is required")
	}
	if err := os.MkdirAll(m.restorePointsRoot(), 0o755); err != nil {
		return RestorePointRecord{}, err
	}

	createdAt := m.now().UTC()
	recordID := m.restorePointID(createdAt)
	record := RestorePointRecord{
		ID:                  recordID,
		CreatedAt:           createdAt.Format(time.RFC3339),
		SourceAppVersion:    strings.TrimSpace(input.SourceAppVersion),
		TargetAppVersion:    strings.TrimSpace(input.TargetAppVersion),
		SourceBuildIdentity: strings.TrimSpace(input.SourceBuildIdentity),
		TargetBuildIdentity: strings.TrimSpace(input.TargetBuildIdentity),
		LocalArchivePath:    filepath.ToSlash(filepath.Join("updates", "restore-points", recordID, "local-archive.ddbak")),
		InstalledBuildPath:  filepath.ToSlash(filepath.Join("updates", "restore-points", recordID, "installed-build")),
		MetadataPath:        filepath.ToSlash(filepath.Join("updates", "restore-points", recordID, "metadata.json")),
	}

	restorePointDir := m.restorePointDir(record)
	if err := os.MkdirAll(restorePointDir, 0o755); err != nil {
		return RestorePointRecord{}, err
	}
	cleanupRestorePointDir := true
	defer func() {
		if cleanupRestorePointDir {
			_ = os.RemoveAll(restorePointDir)
		}
	}()

	if err := archiveWriter(m.absolutePath(record.LocalArchivePath)); err != nil {
		return RestorePointRecord{}, err
	}
	if err := buildWriter(m.absolutePath(record.InstalledBuildPath)); err != nil {
		return RestorePointRecord{}, err
	}
	if err := writeJSONAtomic(m.absolutePath(record.MetadataPath), record); err != nil {
		return RestorePointRecord{}, err
	}

	index, err := m.loadIndex()
	if err != nil {
		return RestorePointRecord{}, err
	}
	index.Policy = m.normalizePolicy(index.Policy)
	index.RestorePoints = append([]RestorePointRecord{record}, filterRestorePointRecord(index.RestorePoints, record.ID)...)
	sortRestorePoints(index.RestorePoints)
	index.RestorePoints = m.pruneRestorePoints(index.RestorePoints)
	if err := writeJSONAtomic(m.indexPath(), index); err != nil {
		return RestorePointRecord{}, err
	}

	cleanupRestorePointDir = false
	return record, nil
}

func (m *RestorePointManager) List() ([]RestorePointRecord, error) {
	index, err := m.loadIndex()
	if err != nil {
		return nil, err
	}
	restorePoints := append([]RestorePointRecord(nil), index.RestorePoints...)
	return restorePoints, nil
}

func (m *RestorePointManager) Get(id string) (RestorePointRecord, error) {
	index, err := m.loadIndex()
	if err != nil {
		return RestorePointRecord{}, err
	}
	for _, record := range index.RestorePoints {
		if record.ID == id {
			return record, nil
		}
	}
	return RestorePointRecord{}, fmt.Errorf("restore point %q not found", id)
}

func (m *RestorePointManager) Housekeeping() error {
	index, err := m.loadIndex()
	if err != nil {
		return err
	}
	index.Policy = m.normalizePolicy(index.Policy)
	index.RestorePoints = m.pruneRestorePoints(index.RestorePoints)
	return writeJSONAtomic(m.indexPath(), index)
}

func (m *RestorePointManager) SaveLaunchState(record RestorePointRecord) error {
	state := RestorePointLaunchState{
		Version:             restorePointStateVersion,
		RestorePointID:      record.ID,
		TargetAppVersion:    strings.TrimSpace(record.TargetAppVersion),
		TargetBuildIdentity: strings.TrimSpace(record.TargetBuildIdentity),
		Status:              RestorePointLaunchPrepared,
		PreparedAt:          m.now().UTC().Format(time.RFC3339),
	}
	return writeJSONAtomic(appdata.UpdateRestorePointStatePath(m.dataDir), state)
}

func (m *RestorePointManager) LoadLaunchState() (*RestorePointLaunchState, error) {
	content, err := os.ReadFile(appdata.UpdateRestorePointStatePath(m.dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state RestorePointLaunchState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, err
	}
	if state.Version == 0 {
		state.Version = restorePointStateVersion
	}
	return &state, nil
}

func (m *RestorePointManager) MarkLaunchStarting() (*RestorePointLaunchState, error) {
	state, err := m.LoadLaunchState()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	state.Status = RestorePointLaunchStarting
	state.StartedAt = m.now().UTC().Format(time.RFC3339)
	if err := writeJSONAtomic(appdata.UpdateRestorePointStatePath(m.dataDir), state); err != nil {
		return nil, err
	}
	return state, nil
}

func (m *RestorePointManager) ClearLaunchState() error {
	path := appdata.UpdateRestorePointStatePath(m.dataDir)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *RestorePointManager) restorePointsRoot() string {
	return appdata.UpdateRestorePointsDir(m.dataDir)
}

func (m *RestorePointManager) indexPath() string {
	return filepath.Join(m.restorePointsRoot(), "index.json")
}

func (m *RestorePointManager) restorePointID(createdAt time.Time) string {
	return fmt.Sprintf("restore-point-%s", createdAt.Format("20060102-150405"))
}

func (m *RestorePointManager) restorePointDir(record RestorePointRecord) string {
	if strings.TrimSpace(record.MetadataPath) != "" {
		return filepath.Dir(m.absolutePath(record.MetadataPath))
	}
	return filepath.Join(m.restorePointsRoot(), record.ID)
}

func (m *RestorePointManager) absolutePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(m.dataDir, filepath.FromSlash(path))
}

func (m *RestorePointManager) loadIndex() (RestorePointIndex, error) {
	content, err := os.ReadFile(m.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return RestorePointIndex{
				Version:       restorePointIndexVersion,
				Policy:        m.normalizePolicy(RestorePointPolicy{}),
				RestorePoints: []RestorePointRecord{},
			}, nil
		}
		return RestorePointIndex{}, err
	}
	var index RestorePointIndex
	if err := json.Unmarshal(content, &index); err != nil {
		return RestorePointIndex{}, err
	}
	index.Version = restorePointIndexVersion
	index.Policy = m.normalizePolicy(index.Policy)
	sortRestorePoints(index.RestorePoints)
	return index, nil
}

func (m *RestorePointManager) normalizePolicy(policy RestorePointPolicy) RestorePointPolicy {
	if policy.MaxRestorePoints <= 0 {
		policy.MaxRestorePoints = m.policy.MaxRestorePoints
	}
	if policy.MaxRestorePoints <= 0 {
		policy.MaxRestorePoints = defaultMaxRestorePoints
	}
	return policy
}

func (m *RestorePointManager) pruneRestorePoints(restorePoints []RestorePointRecord) []RestorePointRecord {
	limit := m.normalizePolicy(RestorePointPolicy{}).MaxRestorePoints
	if len(restorePoints) <= limit {
		return restorePoints
	}
	kept := append([]RestorePointRecord(nil), restorePoints[:limit]...)
	for _, stale := range restorePoints[limit:] {
		if err := os.RemoveAll(m.restorePointDir(stale)); err != nil {
			kept = append(kept, stale)
		}
	}
	sortRestorePoints(kept)
	return kept
}

func sortRestorePoints(restorePoints []RestorePointRecord) {
	sort.SliceStable(restorePoints, func(i, j int) bool {
		left, leftErr := time.Parse(time.RFC3339, restorePoints[i].CreatedAt)
		right, rightErr := time.Parse(time.RFC3339, restorePoints[j].CreatedAt)
		if leftErr != nil || rightErr != nil {
			return restorePoints[i].CreatedAt > restorePoints[j].CreatedAt
		}
		return left.After(right)
	})
}

func filterRestorePointRecord(restorePoints []RestorePointRecord, excludeID string) []RestorePointRecord {
	filtered := make([]RestorePointRecord, 0, len(restorePoints))
	for _, record := range restorePoints {
		if record.ID == excludeID {
			continue
		}
		filtered = append(filtered, record)
	}
	return filtered
}
