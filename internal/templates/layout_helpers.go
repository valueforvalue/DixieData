package templates

// currentPagePath is the package-level "currently rendering page"
// state. Handlers must call SetCurrentPagePath before invoking a
// page's Render(). Issue #309 design choice: a package-level
// variable avoids threading the path through every page's templ
// signature (there are 30+ pages that call Layout; each would
// need a new parameter otherwise).
//
// Goroutine safety: the appshell serves requests one at a time
// per chi-mux ServeHTTP call stack (Wails single-window, single-
// page routing), and templ renders complete before the handler
// returns. SetCurrentPagePath + Render() + ClearCurrentPagePath
// is the safe pattern.
var currentPagePath string

// SetCurrentPagePath sets the package-level current page path
// before rendering a page templ. Issue #309: the breadcrumb +
// dev badge + (eventually) the JS toolbox's `dixie.page()` all
// read this to know "what page am I on" without requiring every
// page's templ signature to grow a path parameter.
func SetCurrentPagePath(path string) {
	currentPagePath = path
}

// ClearCurrentPagePath resets the package-level state. Call via
// defer right after SetCurrentPagePath + Render so the next
// request doesn't see the previous request's path.
func ClearCurrentPagePath() {
	currentPagePath = ""
}

// layoutCurrentPath is a private helper that Layout calls. Kept
// here rather than in the templ file so the layer that owns the
// state (this file) and the layer that reads it (the templ) are
// separate. Templ can only call Go functions that are in the
// same package, so this is the right shape.
func layoutCurrentPath() string {
	return currentPagePath
}
