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

// currentLayoutHasOpenReview mirrors currentPagePath for the
// per-render flag that drives the red review-state treatment on
// the Research & Review foldout's "Open Review Queue" menuitem
// (issue #460). Default false: only handlers that know the
// pending review count flips it on. The badge poll
// (/layout/review-count) carries its own red background, so this
// is purely visual continuity between the trigger badge + the
// menuitem it counts.
var currentLayoutHasOpenReview bool

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

// SetLayoutHasOpenReview sets the per-render flag that drives the
// red treatment on the Research & Review foldout's "Open Review
// Queue" menuitem (issue #460). Pair with ClearLayoutHasOpenReview
// via defer so the next request starts in the default state. The
// helper itself does not query the audit: callers are responsible
// for the CountNeedsReview() check. Pages that don't have the
// count handy (most of them) skip the Set call and the menuitem
// renders in its neutral pill state.
func SetLayoutHasOpenReview(hasOpenReview bool) {
	currentLayoutHasOpenReview = hasOpenReview
}

// ClearLayoutHasOpenReview resets the per-render flag. Pair with
// SetLayoutHasOpenReview via defer.
func ClearLayoutHasOpenReview() {
	currentLayoutHasOpenReview = false
}

// layoutCurrentPath is a private helper that Layout calls. Kept
// here rather than in the templ file so the layer that owns the
// state (this file) and the layer that reads it (the templ) are
// separate. Templ can only call Go functions that are in the
// same package, so this is the right shape.
func layoutCurrentPath() string {
	return currentPagePath
}

// layoutHasOpenReview mirrors layoutCurrentPath for the red
// treatment flag. Same go-template-callable contract.
func layoutHasOpenReview() bool {
	return currentLayoutHasOpenReview
}
