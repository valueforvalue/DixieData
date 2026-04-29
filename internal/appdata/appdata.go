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
	relative := filepath.Join("images", safeDisplayID)
	return filepath.Join(dataDir, relative), relative
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
