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

	"github.com/valueforvalue/DixieData/internal/appdata"
)

//go:embed scratchpad.ps1
var embeddedScript []byte

func (l *Launcher) Open(displayID, seed string) error {
	displayID = strings.TrimSpace(displayID)
	if displayID == "" {
		return errors.New("missing record ID")
	}

	textPath, statePath := appdata.ScratchpadPaths(l.dataDir, displayID)
	scriptPath, err := l.ensureScript()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(textPath), 0o755); err != nil {
		return err
	}
	if seed = strings.TrimSpace(seed); seed != "" {
		if err := seedScratchpadFile(textPath, seed); err != nil {
			return err
		}
	} else if _, err := os.Stat(textPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(textPath, []byte{}, 0o644); err != nil {
			return err
		}
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
	return cmd.Start()
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

func seedScratchpadFile(path, seed string) error {
	existing, err := os.ReadFile(path)
	if err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(seed), 0o644)
}
