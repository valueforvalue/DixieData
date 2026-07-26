package appshell

import (
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/valueforvalue/DixieData/internal/debug"
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
		// Issue #384 / Slice 5: wrap Render.
		if err := presentation.SettingsUpdatePanel(current).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the update settings panel.", err)
		}
		return
	}
	settings.NoticeKind = "success"
	if settings.UsingDefaultSource {
		settings.NoticeMessage = "Using the default GitHub latest release feed."
	} else {
		settings.NoticeMessage = "Saved custom update source."
	}
	setToastHeader(w, settings.NoticeMessage)
	// Issue #384 / Slice 5: wrap Render.
	if err := presentation.SettingsUpdatePanel(settings).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the update settings panel.", err)
	}
}

func (a *App) handleCheckForUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	result, err := a.updater.Check()
	if err != nil {
		log := debug.FromContext(r.Context())
		log.Error("update check failed", "err", err.Error())
		setToastHeaderWithType(w, "Update check failed.", "error")
		// Issue #384 / Slice 5: wrap Render.
		if err := presentation.SettingsUpdateStatusMessage("error", err.Error()).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the update check error.", err)
		}
		return
	}
	// Issue #384 / Slice 5: wrap Render.
	if err := presentation.SettingsUpdateStatus(result).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the update check result.", err)
	}
}

func (a *App) handleApplyLatestUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Issue #661: reset progress + spawn the prepare in a
	// goroutine so the apply button returns within ~50ms
	// instead of blocking on a multi-MB download. The JS
	// dispatcher polls /settings/updates/progress every 500ms
	// and writes the live progress fragment into the
	// #settings-update-progress target.
	a.updateProgress.set(update.UpdateProgress{Phase: update.PhaseDownload, Message: "Starting update…"})
	go func() {
		prepared, err := a.updater.PrepareLatestWithProgress(func(p update.UpdateProgress) {
			a.updateProgress.set(p)
		})
		if err != nil {
			log := debug.FromContext(r.Context())
			log.Error("update prepare failed", "err", err.Error())
			return
		}
		command := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", prepared.ScriptPath)
		if err := command.Start(); err != nil {
			log := debug.FromContext(r.Context())
			log.Error("installer start failed", "err", err.Error())
			a.updateProgress.set(update.UpdateProgress{Phase: update.PhaseError, Message: err.Error(), Error: err.Error()})
			return
		}
		// Final-phase update so the polling UI can transition
		// from the "Creating restore point…" label to the
		// "App will restart shortly…" label before the app
		// actually quits. The Quit happens in a separate
		// goroutine below so the progress callback fires first.
		setToastHeader(w, fmt.Sprintf("Applying DixieData v%s. The app will restart shortly.", prepared.Version))
		go func() {
			time.Sleep(750 * time.Millisecond)
			if a.ctx != nil {
				a.Quit()
			}
		}()
	}()
	// Issue #384 / Slice 5: wrap Render. The fragment
	// carries the data-poll-progress marker so the JS
	// dispatcher starts polling for live progress.
	if err := presentation.SettingsUpdateApplyStarting().Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the apply-started status.", err)
	}
}

// handleUpdateProgress serves the live progress fragment
// during an in-place update (issue #661). The JS dispatcher
// polls this endpoint every 500ms while the
// #settings-update-progress target is mounted with
// data-poll-progress. Terminal phases (PhaseApplyStarted,
// PhaseError) signal the polling loop to stop.
func (a *App) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	progress := a.updateProgress.get()
	if err := presentation.SettingsUpdateProgress(progress).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the update progress.", err)
	}
}
