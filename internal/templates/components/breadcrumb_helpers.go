package components

import "strings"

// BreadcrumbCrumb is one segment in the "where am I" breadcrumb
// the layout renders below the top-nav header (issue #309).
// Each crumb carries a label for display and a href for the link
// target; the final crumb has IsCurrent = true and the layout
// renders it as plain text (no link, current-page styling).
type BreadcrumbCrumb struct {
	Label     string
	Href      string
	IsCurrent bool
}

// BreadcrumbCrumbs maps the current request path to a slice of
// breadcrumb crumbs for the layout to render. Order: home crumb
// (Home) first when the current path is not "/" itself; then
// top-level section; then any nested sections; then the leaf
// page. Used by the templ Breadcrumb component.
//
// The mapping is hand-curated from internal/appshell/routes.go
// (the single source of truth for URL -> handler). New routes
// need a new case here.
//
// This is the same algorithm the debug toolbox's
// `dixie.page()` JS function ports to JS (issue #309 PR 1 of
// #309); if you change the algorithm, sync the JS port in
// frontend/debug-toolbox.js.
func BreadcrumbCrumbs(currentPath string) []BreadcrumbCrumb {
	if currentPath == "" || currentPath == "/" {
		// Root page is the calendar landing per routes.go
		// (`r.Get("/", a.handleCalendar)`); render as
		// "Home > Calendar" so users see the section
		// context even on the root landing.
		return []BreadcrumbCrumb{
			{Label: "Home", Href: "/calendar", IsCurrent: false},
			{Label: "Calendar", Href: "/calendar", IsCurrent: true},
		}
	}

	// Strip query strings for the segment-match but preserve
	// them so the leaf crumb's href can re-attach if needed.
	// For now we drop them -- breadcrumb only ever shows the
	// logical page, not the filter state.
	path := currentPath
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}

	switch {
	// Calendar family
	case path == "/calendar":
		return joinCrumbs("Calendar", "/calendar", true)
	case strings.HasPrefix(path, "/calendar/"):
		return handleCalendarCrumbs(path)

	// Anniversary family
	case strings.HasPrefix(path, "/anniversary/"):
		return handleAnniversaryCrumbs(path)

	// Search / Quick View + soldier detail
	case path == "/soldiers":
		return joinCrumbs("Search/Quick View", "/soldiers", true)
	case path == "/soldiers/new":
		return joinCrumbs("Search/Quick View", "/soldiers", false, "Add Person", "/soldiers/new", true)
	case path == "/soldiers/search":
		return joinCrumbs("Search/Quick View", "/soldiers", false, "Search", "/soldiers/search", true)
	case strings.HasPrefix(path, "/soldiers/search/"):
		return joinCrumbs("Search/Quick View", "/soldiers", false, "Advanced Search", path, true)
	case strings.HasPrefix(path, "/soldiers/display/"):
		return joinCrumbs("Search/Quick View", "/soldiers", false, shortID(path), path, true)
	case strings.HasPrefix(path, "/soldiers/") && path != "/soldiers/new" && !strings.HasPrefix(path, "/soldiers/search"):
		// /soldiers/{id} or /soldiers/{id}/tags
		return handleSoldierDetailCrumbs(path)

	// Browse family
	case path == "/browse":
		return joinCrumbs("Browse", "/browse", true)
	case path == "/browse/results":
		return joinCrumbs("Browse", "/browse", false, "Results", "/browse/results", true)

	// Review queue
	case path == "/review-queue":
		return joinCrumbs("Review Queue", "/review-queue", true)
	case strings.HasPrefix(path, "/review-queue/compare/"):
		return joinCrumbs("Review Queue", "/review-queue", false, "Compare", path, true)
	case path == "/compare":
		return joinCrumbs("Compare", "/compare", true)

	// Insights
	case path == "/insights":
		return joinCrumbs("Insights", "/insights", true)

	// Share family
	case path == "/share":
		return joinCrumbs("Share", "/share", true)
	case path == "/share/exports":
		return joinCrumbs("Share", "/share", false, "Exports", "/share/exports", true)
	case path == "/share/imports":
		return joinCrumbs("Share", "/share", false, "Imports", "/share/imports", true)
	case path == "/share/sync":
		return joinCrumbs("Share", "/share", false, "Sync", "/share/sync", true)
	case path == "/share/queue":
		return joinCrumbs("Share", "/share", false, "Queue", "/share/queue", true)

	// Tags
	case path == "/tags":
		return joinCrumbs("Tags", "/tags", true)
	case strings.HasPrefix(path, "/tags/"):
		return joinCrumbs("Tags", "/tags", false, "Tag", path, true)

	// Settings
	case strings.HasPrefix(path, "/settings"):
		return joinCrumbs("Settings", "/settings", true)

	// Jobs
	case path == "/jobs/active":
		return joinCrumbs("Jobs", "/jobs/active", true)
	case strings.HasPrefix(path, "/jobs/"):
		return joinCrumbs("Jobs", "/jobs/active", false, jobIDLabel(path), path, true)

	// Scratchpad
	case path == "/scratchpad":
		return joinCrumbs("Scratchpad", "/scratchpad", true)

	// Feedback
	case strings.HasPrefix(path, "/feedback"):
		return joinCrumbs("Feedback", "/feedback", true)

	// Recovery
	case path == "/recovery":
		return joinCrumbs("Recovery", "/recovery", true)

	// Setup
	case path == "/setup":
		return joinCrumbs("Setup", "/setup", true)

	// Fallback: Home + raw path segment.
	default:
		return joinCrumbs("Home", "/calendar", false, labelFromPath(path), path, true)
	}
}

// joinCrumbs is a tiny helper that builds the standard
// [Home, ..., Current] crumb list. Pass leaves as the final
// (Label, Href, true) triplet; intermediate sections as
// (Label, Href, false) triplets.
func joinCrumbs(items ...any) []BreadcrumbCrumb {
	crumbs := make([]BreadcrumbCrumb, 0, len(items)/3+1)
	// Always start with Home (linking to /calendar, the root).
	crumbs = append(crumbs, BreadcrumbCrumb{Label: "Home", Href: "/calendar", IsCurrent: false})
	// items are triplets (label, href, isCurrent)
	for i := 0; i+2 < len(items); i += 3 {
		label, _ := items[i].(string)
		href, _ := items[i+1].(string)
		isCurrent, _ := items[i+2].(bool)
		if label == "" {
			continue
		}
		crumbs = append(crumbs, BreadcrumbCrumb{Label: label, Href: href, IsCurrent: isCurrent})
	}
	return crumbs
}

// handleCalendarCrumbs handles /calendar/{month} and
// /calendar/{month}/anniversary/{month}/{day} (the only
// multi-segment calendar paths registered in routes.go).
func handleCalendarCrumbs(path string) []BreadcrumbCrumb {
	rest := strings.TrimPrefix(path, "/calendar/")
	parts := strings.Split(rest, "/")
	// parts[0] = month (e.g. "january")
	if len(parts) == 1 {
		return joinCrumbs(
			"Calendar", "/calendar", false,
			titleCaseMonth(parts[0]), path, true,
		)
	}
	// parts: ["month", "anniversary", "month", "day"] (issue #161 layout)
	if len(parts) >= 4 && parts[1] == "anniversary" {
		return joinCrumbs(
			"Calendar", "/calendar", false,
			"Anniversaries", "/anniversary", false,
			titleCaseMonth(parts[0]), path, true,
		)
	}
	// Unknown calendar subpath: render as Calendar > <segment>
	return joinCrumbs(
		"Calendar", "/calendar", false,
		labelFromPath(path), path, true,
	)
}

// handleAnniversaryCrumbs handles /anniversary/{month}/{day}.
// Multi-segment: [month, day] -- renders as
// Home > Anniversaries > {Month} > {Day}.
func handleAnniversaryCrumbs(path string) []BreadcrumbCrumb {
	rest := strings.TrimPrefix(path, "/anniversary/")
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 {
		return joinCrumbs(
			"Anniversaries", "/anniversary", false,
			titleCaseMonth(parts[0]), path, false,
			parts[1], path, true,
		)
	}
	if len(parts) == 1 {
		return joinCrumbs(
			"Anniversaries", "/anniversary", false,
			titleCaseMonth(parts[0]), path, true,
		)
	}
	return joinCrumbs(
		"Anniversaries", "/anniversary", true,
	)
}

// handleSoldierDetailCrumbs handles /soldiers/{id}[/tags...].
// The soldier id segment is preserved as a short reference like
// "#42"; the soldier's Display ID isn't available without a DB
// lookup so we keep the raw id in the leaf crumb.
func handleSoldierDetailCrumbs(path string) []BreadcrumbCrumb {
	rest := strings.TrimPrefix(path, "/soldiers/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	leaf := "#" + id
	if len(parts) >= 2 && parts[1] == "tags" {
		return joinCrumbs(
			"Search/Quick View", "/soldiers", false,
			leaf, "/soldiers/"+id, false,
			"Tags", "/soldiers/"+id+"/tags", true,
		)
	}
	return joinCrumbs(
		"Search/Quick View", "/soldiers", false,
		leaf, path, true,
	)
}

// titleCaseMonth renders lowercase month slug as Title Case for
// display ("january" -> "January"). Falls back to the input if
// it isn't a recognizable month.
func titleCaseMonth(slug string) string {
	if slug == "" {
		return slug
	}
	return strings.ToUpper(slug[:1]) + slug[1:]
}

// shortID returns the last path segment for use as a crumb
// label (e.g. "/soldiers/42/tags" -> "42").
func shortID(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// labelFromPath returns a human-readable label derived from the
// last path segment ("share/exports" -> "Exports", "review-queue"
// -> "Review Queue"). A best-effort fallback for unmapped paths.
func labelFromPath(path string) string {
	last := shortID(path)
	last = strings.ReplaceAll(last, "-", " ")
	last = strings.ReplaceAll(last, "_", " ")
	if last == "" {
		return path
	}
	// Title case: capitalize first letter of each word.
	parts := strings.Fields(last)
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// jobIDLabel strips the /jobs/ prefix and returns the job id
// (or short hash) for display in the leaf crumb.
func jobIDLabel(path string) string {
	id := strings.TrimPrefix(path, "/jobs/")
	if id == "" || id == "active" {
		return "Job"
	}
	return "Job " + id
}
