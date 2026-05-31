package appshell

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/valueforvalue/DixieData/internal/appdata"
	"github.com/valueforvalue/DixieData/internal/archive"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/update"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) handleRecovery(w http.ResponseWriter, r *http.Request) {
	if a.pendingRecovery == nil {
		http.Redirect(w, r, "/calendar", http.StatusSeeOther)
		return
	}
	switch r.Method {
	case http.MethodGet:
		presentation.UpdateRecoveryPage(*a.pendingRecovery, a.recoveryFailure, false).Render(r.Context(), w)
	case http.MethodPost:
		if err := a.runRecoveryRollback(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			presentation.UpdateRecoveryPage(*a.pendingRecovery, err.Error(), false).Render(r.Context(), w)
			return
		}
		presentation.UpdateRecoveryPage(*a.pendingRecovery, "", true).Render(r.Context(), w)
		go func() {
			time.Sleep(750 * time.Millisecond)
			if a.ctx != nil {
				runtime.Quit(a.ctx)
			}
		}()
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) runRecoveryRollback() error {
	if a.pendingRecovery == nil {
		return fmt.Errorf("no pending recovery is available")
	}
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	archivePath := filepath.Join(a.dataDir, filepath.FromSlash(a.pendingRecovery.LocalArchivePath))
	if _, err := archive.RestoreBackupArchive(archivePath, a.dataDir); err != nil {
		return err
	}
	scriptPath := filepath.Join(appdata.UpdatesDir(a.dataDir), "rollback-"+a.pendingRecovery.ID+".ps1")
	if err := update.WriteRollbackScript(scriptPath, update.RollbackScriptOptions{
		ProcessID:         os.Getpid(),
		InstallDir:        filepath.Dir(executablePath),
		InstalledBuildDir: filepath.Join(a.dataDir, filepath.FromSlash(a.pendingRecovery.InstalledBuildPath)),
		DataDir:           a.dataDir,
		ExecutableName:    filepath.Base(executablePath),
		LaunchStatePath:   appdata.UpdateRestorePointStatePath(a.dataDir),
	}); err != nil {
		return err
	}
	command := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	return command.Start()
}
