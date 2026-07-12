package records

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

// Theme names. Stored verbatim in local_settings.json and resolved
// against the empty-string fallback at read time. The "default"
// value identifies the Classic palette (the historically-Default
// gold/sepia-on-warm-navy theme, renamed to "Classic" in the UI
// in issue #494). Fresh installs see Soft on first launch via the
// empty-string fallback in ResolvedTheme — the persisted value
// "default" continues to bind to the Classic palette for users
// who picked it before the rename. The CSS attribute is
// data-theme="default" for Classic users (preserved for stability);
// data-theme="soft" for Soft users; data-theme="high-contrast"
// for High Contrast users.
const (
	ThemeClassic      = "default"
	ThemeHighContrast = "high-contrast"
	ThemeSoft         = "soft"
)

// LocalSettings holds per-machine, per-user settings that are not part
// of the archive schema and must not be exported/shared. Lives at
// <parent-of-dataDir>/.dixiedata-state/local_settings.json (a sibling
// of the archive dir, NOT inside it, so a .ddbak restore does not
// wipe the user's choice and so an in-place app update can ship
// theme corrections that overwrite the file atomically).
type LocalSettings struct {
	DebugMode bool   `json:"debug_mode"`
	Theme     string `json:"theme,omitempty"` // "" == ThemeSoft (issue #494)
}

// ResolvedTheme returns the theme name with the empty-string fallback
// resolved to ThemeSoft (so fresh installs see Soft on first launch).
// Use this everywhere instead of reading the raw field so callers
// don't have to repeat the fallback. The empty-string fallback
// flipping from ThemeClassic to ThemeSoft is the half of issue #494
// that makes Soft the new default for new users; users whose
// local_settings.json has Theme:"default" continue to resolve to
// ThemeClassic (the constant rename only swapped the Go identifier,
// not the persisted string value).
func (s LocalSettings) ResolvedTheme() string {
	if s.Theme == "" {
		return ThemeSoft
	}
	return s.Theme
}

// LocalSettingsPath returns the absolute path to local_settings.json
// at the sibling-of-archive state root.
func LocalSettingsPath(dataDir string) string {
	return filepath.Join(appdata.StateRoot(dataDir), "local_settings.json")
}

// legacyLocalSettingsPath is the pre-#474 location, kept so
// migrateLocalSettingsToStateRoot can copy any user file that was
// written before the move. Once a build with the new path runs and
// migrates the file, this constant is only used by tests.
func legacyLocalSettingsPath(dataDir string) string {
	return filepath.Join(dataDir, "local_settings.json")
}

var localSettingsMu sync.Mutex

// LoadLocalSettings reads local_settings.json. Returns zero-value
// LocalSettings (DebugMode=false, Theme="") if the file does not
// exist. Returns the underlying error for other I/O or parse
// failures.
//
// On the first call after the path move to <dataDir-parent>/.dixiedata-state/,
// if the new file does not exist AND the legacy file at
// <dataDir>/local_settings.json exists, the legacy file is copied
// to the new path (so existing debug_mode settings are preserved
// across the move). The legacy file is left in place as a tombstone;
// the user may delete it manually. The migration runs at most once
// per machine because subsequent calls see the new file and skip
// the migration.
func LoadLocalSettings(dataDir string) (LocalSettings, error) {
	localSettingsMu.Lock()
	defer localSettingsMu.Unlock()

	var s LocalSettings
	path := LocalSettingsPath(dataDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// First read on the new path. Try the legacy location so
		// existing users keep their debug_mode setting across the
		// move. Errors here are non-fatal — fall through to the
		// zero-value return.
		legacy := legacyLocalSettingsPath(dataDir)
		if legacyData, lerr := os.ReadFile(legacy); lerr == nil {
			if uerr := json.Unmarshal(legacyData, &s); uerr != nil {
				return s, fmt.Errorf("migrate legacy local_settings.json: %w", uerr)
			}
			// Best-effort copy. Failure is non-fatal; the in-memory
			// load succeeded so the app still works this session.
			// A future restart will retry the migration.
			if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
				_ = os.WriteFile(path, legacyData, 0o644)
			}
			return s, nil
		}
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