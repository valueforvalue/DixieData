package appshell

import "net/http"

// blockIfFragment writes 204 + X-DixieData-Redirect and returns true
// when r is an htmx fragment. Callers use it as the first statement
// of any "blocked" branch in App.ServeHTTP; if it returns false, the
// caller proceeds with its existing 303/500/full-doc response.
//
// redirectTo="" skips the X-DixieData-Redirect header — used by the
// pre-mux placeholder where there is no destination page to hint at.
//
// This helper is the single source of truth for the HX-Request
// fragment-204 contract. See #209 (pre-mux), #212 (setup), #214
// (recovery + startupErr) for the bug class history.
func blockIfFragment(w http.ResponseWriter, r *http.Request, redirectTo string) bool {
	if r == nil || r.Header.Get("HX-Request") != "true" {
		return false
	}
	if redirectTo != "" {
		w.Header().Set("X-DixieData-Redirect", redirectTo)
	}
	w.WriteHeader(http.StatusNoContent)
	return true
}