package appshell

import (
	"net/http"
)

func (a *App) handleStartupError(w http.ResponseWriter, r *http.Request) {
	if a.startupErr == nil {
		http.Redirect(w, r, "/calendar", http.StatusSeeOther)
		return
	}
	a.respondErrorPage(w, r, KindUnavailable,
		"DixieData could not open this Local Archive. The archive may need a schema migration or recovery.",
		a.startupErr)
}
