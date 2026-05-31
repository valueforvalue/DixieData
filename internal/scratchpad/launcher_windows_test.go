//go:build windows

package scratchpad

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

type launcherStoreStub struct {
	content   string
	updatedAt time.Time
	saveCalls []string
}

func (s *launcherStoreStub) Scratchpad(displayID string) (string, time.Time, error) {
	return s.content, s.updatedAt, nil
}

func (s *launcherStoreStub) SaveScratchpad(displayID, content string) error {
	s.content = content
	s.updatedAt = time.Now().UTC()
	s.saveCalls = append(s.saveCalls, content)
	return nil
}

func TestLauncherPrepareBridgeSeedsEmptyCanonicalScratchpad(t *testing.T) {
	dataDir := t.TempDir()
	store := &launcherStoreStub{}
	launcher := NewLauncher(dataDir, store)
	textPath, _ := appdata.ScratchpadPaths(dataDir, "DXD-00001")

	if err := launcher.prepareBridge("DXD-00001", textPath, "Seeded from legacy localStorage"); err != nil {
		t.Fatalf("prepareBridge: %v", err)
	}

	if store.content != "Seeded from legacy localStorage" {
		t.Fatalf("content=%q", store.content)
	}
}

func TestLauncherPrepareBridgeImportsNewerBridgeFile(t *testing.T) {
	dataDir := t.TempDir()
	store := &launcherStoreStub{
		content:   "older db content",
		updatedAt: time.Now().Add(-2 * time.Hour).UTC(),
	}
	launcher := NewLauncher(dataDir, store)
	textPath, _ := appdata.ScratchpadPaths(dataDir, "DXD-00002")

	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(textPath, []byte("newer bridge content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := launcher.prepareBridge("DXD-00002", textPath, ""); err != nil {
		t.Fatalf("prepareBridge: %v", err)
	}

	if store.content != "newer bridge content" {
		t.Fatalf("content=%q", store.content)
	}
}
