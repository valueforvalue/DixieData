//go:build windows

package scratchpad

import (
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
)

//go:embed scratchpad.ps1
var embeddedScript []byte

func (l *Launcher) Open(displayID, seed string) error {
	displayID = strings.TrimSpace(displayID)
	if displayID == "" {
		return errors.New("missing record ID")
	}
	if l.store == nil {
		return errors.New("scratch pad storage unavailable")
	}

	textPath, statePath := appdata.ScratchpadPaths(l.dataDir, displayID)
	scriptPath, err := l.ensureScript()
	if err != nil {
		return err
	}
	if err := l.prepareBridge(displayID, textPath, seed); err != nil {
		return err
	}

	powerShellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		return err
	}

	cmd := exec.Command(
		powerShellPath,
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-STA",
		"-WindowStyle", "Hidden",
		"-File", scriptPath,
		"-DisplayId", displayID,
		"-TextPath", textPath,
		"-StatePath", statePath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
		_ = l.syncBridgeToStore(displayID, textPath)
	}()
	return nil
}

func (l *Launcher) ensureScript() (string, error) {
	baseDir := filepath.Join(l.dataDir, "scratchpads")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", err
	}
	scriptPath := filepath.Join(baseDir, "scratchpad-window.ps1")
	if err := os.WriteFile(scriptPath, embeddedScript, 0o644); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func (l *Launcher) prepareBridge(displayID, textPath, seed string) error {
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		return err
	}

	dbContent, updatedAt, err := l.store.Scratchpad(displayID)
	if err != nil {
		return err
	}
	if err := l.syncBridgeToStore(displayID, textPath); err != nil {
		return err
	}
	dbContent, updatedAt, err = l.store.Scratchpad(displayID)
	if err != nil {
		return err
	}
	if dbContent == "" && strings.TrimSpace(seed) != "" {
		if err := l.store.SaveScratchpad(displayID, seed); err != nil {
			return err
		}
		dbContent = seed
	}

	bridgeContent, bridgeUpdatedAt, err := readBridgeFile(textPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && bridgeUpdatedAt.After(updatedAt) && bridgeContent != dbContent {
		dbContent = bridgeContent
	}
	return writeScratchpadBridgeFile(textPath, dbContent)
}

func (l *Launcher) syncBridgeToStore(displayID, textPath string) error {
	bridgeContent, bridgeUpdatedAt, err := readBridgeFile(textPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	dbContent, updatedAt, err := l.store.Scratchpad(displayID)
	if err != nil {
		return err
	}
	if !updatedAt.IsZero() && !bridgeUpdatedAt.After(updatedAt) {
		return nil
	}
	if bridgeContent == dbContent {
		return nil
	}
	return l.store.SaveScratchpad(displayID, bridgeContent)
}

func readBridgeFile(path string) (string, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", time.Time{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}
	return string(content), info.ModTime().UTC(), nil
}

func writeScratchpadBridgeFile(path, content string) error {
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == content {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
