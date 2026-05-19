package appshell

import (
	"path/filepath"
	"testing"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

func repoFixturePath(t *testing.T, parts ...string) string {
	t.Helper()

	root, err := appdata.ProjectRoot()
	if err != nil {
		t.Fatalf("ProjectRoot: %v", err)
	}
	return filepath.Join(append([]string{root}, parts...)...)
}
