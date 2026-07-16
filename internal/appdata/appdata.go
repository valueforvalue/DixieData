// Package appdata resolves DixieData's on-disk directories and path conventions.
package appdata

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const folderName = ".dixiedata"

// DefaultDir returns the canonical DixieData Local Archive root
// directory the binary should use. Resolution order:
//  1. DIXIEDATA_DATA_DIR env var (if set and non-empty)
//  2. The .dixiedata/ folder under the project root (dev builds)
//  3. The .dixiedata/ folder next to the executable (installed builds)
//  4. The .dixiedata/ folder under the current working directory
//  5. Just ".dixiedata" as a relative path (last-resort fallback)
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

// ProjectRoot returns the dev-build project root by walking up
// from the executable, the cwd, and a few standard candidates
// looking for the go.mod marker. Used by the dev launcher and by
// the diagnostic bundle's "where was this built" report. Returns
// an error if no project root is found (i.e. the binary is not
// running from a dev build).
func ProjectRoot() (string, error) {
	for _, start := range candidateRoots() {
		if root, ok := projectRootFrom(start); ok {
			return root, nil
		}
	}
	return "", errors.New("project root not found")
}

// ProjectRootFromPath is the exported form of the internal
// projectRootFrom helper. Walks up from start looking for the
// go.mod marker; returns (root, true) on hit, ("", false) on miss.
// Used by tests and by tools/tune's CLI to resolve the dev build
// path without re-implementing the walk.
func ProjectRootFromPath(start string) (string, bool) {
	return projectRootFrom(start)
}

// IsDevelopmentBuild returns true when the supplied executable
// path is a dev build (lives under a path the project-root walker
// recognizes), false when it's an installed release build. Drives
// the dev-only menu items (audit, in-place-safety walker) that
// should not appear in shipped binaries.
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

// RecordImageDir returns the per-soldier image directory under dataDir (dataDir/images/<display-id>).
func RecordImageDir(dataDir, displayID string) (string, string) {
	safeDisplayID := sanitizePathComponent(displayID)
	shards := imageShardSegments(safeDisplayID)
	relative := filepath.Join(append([]string{"images"}, append(shards, safeDisplayID)...)...)
	return filepath.Join(dataDir, relative), relative
}

// ArticleImageDir returns the per-article image directory under dataDir (dataDir/images/articles/<display-id>).
//
// Issue #612: Articles have a separate image bucket from Person Records
// so a chapter illustration and a soldier portrait never share a file path
// even when their display IDs collide (article "ART-00001" + soldier
// "ART-00001" would otherwise share images/AR/T/00001/). The sibling
// `images/articles/<display-id>/` directory keeps the on-disk layout
// flat (still under dataDir/images) so the existing /media/* serving
// route and the orphan detector continue to work without changes.
func ArticleImageDir(dataDir, displayID string) (string, string) {
	safeDisplayID := sanitizePathComponent(displayID)
	shards := imageShardSegments(safeDisplayID)
	relative := filepath.Join(append([]string{"images", "articles"}, append(shards, safeDisplayID)...)...)
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

// ScratchpadPaths returns the per-soldier scratchpad file paths (current + archive).
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

// StateRoot returns the root directory DixieData uses for per-user
// preference state that is not part of the archive schema and must
// not be wiped on .ddbak restore. The directory is a sibling of the
// data directory (not a child), mirroring the LogsRoot precedent
// for the same reason: restore renames the entire .dixiedata
// directory and any file held open inside it would block the rename
// on Windows with "Access is denied". User state (theme choice,
// debug mode toggle, future settings) lives here so it survives
// restore and so an in-place app update can read the user's choice,
// ship a corrected palette, and save the corrected value back.
//
// For dataDir = ".../DixieData/.dixiedata" it returns
// ".../DixieData/.dixiedata-state". The folder starts with a dot
// so it sorts with the data folder in directory listings and is
// hidden by default in file explorers.
func StateRoot(dataDir string) string {
	return filepath.Join(filepath.Dir(dataDir), folderName+"-state")
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

// FeedbackLogPath returns the JSONL feedback-log path under dataDir (the user-submitted feedback surface).
func FeedbackLogPath(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "feedback-log.jsonl")
}

// AppLogPath is the JSONL log written by the internal/debug package.
// One line per slog entry, schema_version field present on every line.
func AppLogPath(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "app.log.jsonl")
}

// FeedbackLogArchiveDir returns the per-version archive directory for old feedback logs.
func FeedbackLogArchiveDir(dataDir string) string {
	return filepath.Join(LogsDir(dataDir), "feedback-history")
}

// FeedbackLogArchiveVersionDir returns the version-specific feedback-log archive subdirectory.
func FeedbackLogArchiveVersionDir(dataDir, version string) string {
	return filepath.Join(FeedbackLogArchiveDir(dataDir), sanitizePathComponent(version))
}

// UpdatesDir returns the directory in-place update artifacts are written to.
func UpdatesDir(dataDir string) string {
	return filepath.Join(dataDir, "updates")
}

// UpdateDownloadsDir returns the directory downloaded update archives land in.
func UpdateDownloadsDir(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "downloads")
}

// UpdateRestorePointsDir returns the directory restore-point metadata is stored in.
func UpdateRestorePointsDir(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "restore-points")
}

// UpdateRestorePointStatePath returns the path of the restore-point state file (the .state file that records what the restore-point points at).
func UpdateRestorePointStatePath(dataDir string) string {
	return filepath.Join(UpdatesDir(dataDir), "restore-point-state.json")
}

// UpdateApplyResultPath returns the path of the post-apply result file (the JSON summary the appshell reads to confirm an in-place update succeeded).
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
