package appdata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const folderName = ".dixiedata"

func DefaultDir() string {
	if configured := strings.TrimSpace(os.Getenv("DIXIEDATA_DATA_DIR")); configured != "" {
		return configured
	}

	for _, start := range candidateRoots() {
		if root, ok := projectRootFrom(start); ok {
			return filepath.Join(root, folderName)
		}
	}

	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), folderName)
	}

	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, folderName)
	}

	return folderName
}

func ProjectRoot() (string, error) {
	for _, start := range candidateRoots() {
		if root, ok := projectRootFrom(start); ok {
			return root, nil
		}
	}
	return "", errors.New("project root not found")
}

func ProjectRootFromPath(start string) (string, bool) {
	return projectRootFrom(start)
}

func IsDevelopmentBuild(executablePath string) bool {
	executablePath = strings.TrimSpace(executablePath)
	if executablePath == "" {
		return false
	}
	root, ok := projectRootFrom(filepath.Dir(executablePath))
	if !ok {
		return false
	}
	relative, err := filepath.Rel(root, filepath.Dir(executablePath))
	if err != nil {
		return false
	}
	relative = filepath.Clean(relative)
	return strings.EqualFold(relative, filepath.Join("build", "bin"))
}

func candidateRoots() []string {
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, wd)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(exe))
	}
	return candidates
}

func projectRootFrom(start string) (string, bool) {
	current := start
	for {
		if _, err := os.Stat(filepath.Join(current, "wails.json")); err == nil {
			return current, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func RecordImageDir(dataDir, displayID string) (string, string) {
	safeDisplayID := sanitizePathComponent(displayID)
	shards := imageShardSegments(safeDisplayID)
	relative := filepath.Join(append([]string{"images"}, append(shards, safeDisplayID)...)...)
	return filepath.Join(dataDir, relative), relative
}

func imageShardSegments(safeDisplayID string) []string {
	upper := strings.ToUpper(strings.TrimSpace(safeDisplayID))
	if upper == "" {
		return []string{"U", "N"}
	}
	first := string(upper[0])
	second := "X"
	for _, r := range upper[1:] {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			second = string(r)
			break
		}
	}
	return []string{first, second}
}

func ScratchpadPaths(dataDir, displayID string) (string, string) {
	safeDisplayID := sanitizePathComponent(displayID)
	base := filepath.Join(dataDir, "scratchpads")
	return filepath.Join(base, safeDisplayID+".txt"), filepath.Join(base, safeDisplayID+".json")
}

// LogsRoot returns the root directory DixieData uses for app-level
// logs and feedback archives. It is a sibling of the data directory,
// not a child, so .ddbak restore (which renames the entire data
// directory) never needs the log file to release its Windows file
// handle. For dataDir = ".../DixieData/.dixiedata" it returns
// ".../DixieData/.dixiedata-logs". The folder starts with a dot so
// it sorts with the data folder in directory listings and is
// hidden by default in file explorers.
func LogsRoot(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), folderName+"-logs")
}

// LogsDir returns the directory that holds the JSONL log files. As
// of the layout change that splits app state from archive state,
// this is LogsRoot(dataDir), not dataDir/logs. The data directory
// is renamed atomically during restore; logs must live outside it
// or the rename fails on Windows with "Access is denied" while the
// log file handle is open.
func LogsDir(dataDir string) string {
	return LogsRoot(dataDir)
}

func FeedbackLogPath(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "feedback-log.jsonl")
}

// AppLogPath is the JSONL log written by the internal/debug package.
// One line per slog entry, schema_version field present on every line.
func AppLogPath(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "app.log.jsonl")
}

func FeedbackLogArchiveDir(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "feedback-history")
}

func FeedbackLogArchiveVersionDir(dataDir, version string) string {
	return filepath.Join(FeedbackLogArchiveDir(dataDir), sanitizePathComponent(version))
}

func UpdatesDir(dataDir string) string {
	return filepath.Join(dataDir, "updates")
}

func UpdateDownloadsDir(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "downloads")
}

func UpdateRestorePointsDir(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "restore-points")
}

func UpdateRestorePointStatePath(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "restore-point-state.json")
}

func UpdateApplyResultPath(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "apply-result.json")
}

func sanitizePathComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unfiled"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range trimmed {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}

	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "unfiled"
	}
	return sanitized
}
