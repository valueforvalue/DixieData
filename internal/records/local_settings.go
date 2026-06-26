package records

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// LocalSettings holds per-machine, per-user settings that are not part
// of the archive schema and must not be exported/shared. Lives at
// <dataDir>/local_settings.json.
type LocalSettings struct {
	DebugMode bool `json:"debug_mode"`
}

// LocalSettingsPath returns the absolute path to local_settings.json.
func LocalSettingsPath(dataDir string) string {
	return filepath.Join(dataDir, "local_settings.json")
}

var localSettingsMu sync.Mutex

// LoadLocalSettings reads local_settings.json. Returns zero-value
// LocalSettings (DebugMode=false) if the file does not exist. Returns
// the underlying error for other I/O or parse failures.
func LoadLocalSettings(dataDir string) (LocalSettings, error) {
	localSettingsMu.Lock()
	defer localSettingsMu.Unlock()

	var s LocalSettings
	path := LocalSettingsPath(dataDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

// SaveLocalSettings writes local_settings.json atomically (write
// temp + rename) so a crash mid-write doesn't corrupt the file.
func SaveLocalSettings(dataDir string, s LocalSettings) error {
	localSettingsMu.Lock()
	defer localSettingsMu.Unlock()

	path := LocalSettingsPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}