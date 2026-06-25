package appshell

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/update"
)

func (a *App) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}
	settings, err := a.updater.SaveSource(r.FormValue("source_url"))
	if err != nil {
		current, settingsErr := a.updater.Settings()
		if settingsErr != nil {
			respondValidation(w, r, "Could not save the update source.", err)
			return
		}
		current.NoticeKind = "error"
		current.NoticeMessage = err.Error()
		presentation.SettingsUpdatePanel(current).Render(r.Context(), w)
		return
	}
	settings.NoticeKind = "success"
	if settings.UsingDefaultSource {
		settings.NoticeMessage = "Using the default GitHub latest release feed."
	} else {
		settings.NoticeMessage = "Saved custom update source."
	}
	setToastHeader(w, settings.NoticeMessage)
	presentation.SettingsUpdatePanel(settings).Render(r.Context(), w)
}

func (a *App) handleCheckForUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.updater.Check()
	if err != nil {
		setToastHeaderWithType(w, "Update check failed.", "error")
		presentation.SettingsUpdateStatusMessage("error", err.Error()).Render(r.Context(), w)
		return
	}
	presentation.SettingsUpdateStatus(result).Render(r.Context(), w)
}

func (a *App) handleApplyLatestUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prepared, err := a.updater.PrepareLatest()
	if err != nil {
		setToastHeaderWithType(w, "Update apply failed.", "error")
		presentation.SettingsUpdateStatusMessage("error", err.Error()).Render(r.Context(), w)
		return
	}
	command := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", prepared.ScriptPath)
	if err := command.Start(); err != nil {
		setToastHeaderWithType(w, "Unable to start the update installer.", "error")
		presentation.SettingsUpdateStatusMessage("error", err.Error()).Render(r.Context(), w)
		return
	}
	setToastHeader(w, fmt.Sprintf("Applying DixieData v%s. The app will restart shortly.", prepared.Version))
	presentation.SettingsUpdateApplyStarted(prepared.Version).Render(r.Context(), w)
	go func() {
		time.Sleep(750 * time.Millisecond)
		if a.ctx != nil {
			a.Quit()
		}
	}()
}

var _ updaterFacade = (*update.Service)(nil)
