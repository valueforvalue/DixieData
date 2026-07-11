// event_panel.go — registry-driven path-suffix dispatcher for the
// per-Event panel routes (issue #343 finding #3). Replaces the
// four near-identical chi-route shims in events_handlers.go
// (handleEventResearchLogRoute, handleEventSourcesRoute,
// handleEventTagsRoute, handleEventImagesRoute) with a single
// handleEventPanelRoute that walks a per-panel route table.
//
// Adding a new Event-side panel used to require ~30 LoC of
// dispatcher copy: parse eventID, parse suffix, switch on
// method, switch on suffix variant, validate sub-id, call
// handler. With the registry, a new panel adds one entry to
// the eventPanels slice below — the route shim, the parse
// logic, and the method dispatch are all shared.
//
// The handler signature is identical across all four panels:
// func(w http.ResponseWriter, r *http.Request, eventID int64,
//        subID int64). The subID is -1 when the route's
// subPath has no {id} placeholder; otherwise it's the parsed
// int from the path position. This keeps every panel handler
// symmetric — no special-cased sub-id extraction per panel.

package appshell

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/valueforvalue/DixieData/internal/presentation"
)

// eventPanelHandler is the per-panel dispatch signature.
// a is the *App so the handler can call back into the
// service layer (a.events, a.soldiers, etc.). eventID is the
// Event's row id (parsed from the URL once by the chi route
// shim). subID is the {id} placeholder from the subPath, or
// -1 if the route has no sub-id. Returning without writing
// the response is treated as a 204 (the handler is expected
// to write its own response — see respondValidation /
// respondInternal / fragment Render calls in the per-panel
// handlers).
type eventPanelHandler func(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64)

// eventPanelRoute is one (method, subPath) → handler entry
// inside an eventPanel.
//
// subPath is matched against the URL suffix (everything after
// `/events/{id}/`):
//
//   ""               — match the bare panel name with no suffix.
//                       e.g. GET /events/42/sources matches
//                       subPath="".
//   "attach"         — match the literal sub-path. e.g. POST
//                       /events/42/sources/attach matches
//                       subPath="attach".
//   "{id}/detach"    — match a parametric sub-path. The first
//                       path segment after the panel name is
//                       parsed as int64 and passed to the
//                       handler as subID. The remaining
//                       segments ("detach") are matched
//                       literally. e.g. POST
//                       /events/42/sources/7/detach parses
//                       subID=7 and matches subPath=
//                       "{id}/detach".
type eventPanelRoute struct {
	method  string
	subPath string
	handler eventPanelHandler
}

// eventPanel is one Event-side panel: a name + the (method,
// subPath) routes it owns. The chi URL prefix is
// `/events/{id}/{panelName}/...`.
type eventPanel struct {
	name   string
	routes []eventPanelRoute
}

// subIDPlaceholder is the literal token that marks a numeric
// sub-id position in a subPath. Keep it short so the per-panel
// route declarations stay scannable.
const subIDPlaceholder = "{id}"

// matchSubPath checks whether the request's suffix matches the
// route's subPath. When the subPath contains the sub-id
// placeholder, the first path segment is parsed as int64 and
// returned via the subID pointer. On literal-only subPath
// values, subID is set to -1. The boolean return is true on
// a match, false otherwise; an unmatched subPath that
// happens to start with the sub-id placeholder is also false
// (so the dispatcher can fall through to a 404 rather than a
// 400 from a malformed numeric).
func matchSubPath(suffix, subPath string, subID *int64) bool {
	if !strings.Contains(subPath, subIDPlaceholder) {
		// Literal match. Empty subPath only matches empty
		// suffix.
		if subPath == "" {
			*subID = -1
			return suffix == ""
		}
		// Trim leading/trailing slash so callers can pass
		// either "attach" or "/attach".
		want := strings.Trim(subPath, "/")
		*subID = -1
		return suffix == want
	}
	// Parametric match: split the suffix on "/" and the
	// subPath on "/". Walk in parallel — the placeholder
	// position gets parsed as int64; the rest must match
	// literally.
	suffixParts := strings.Split(strings.Trim(suffix, "/"), "/")
	pathParts := strings.Split(strings.Trim(subPath, "/"), "/")
	if len(suffixParts) != len(pathParts) {
		return false
	}
	var parsed int64 = -1
	for i, pp := range pathParts {
		if pp == subIDPlaceholder {
			id, err := strconv.ParseInt(suffixParts[i], 10, 64)
			if err != nil {
				return false
			}
			parsed = id
			continue
		}
		if pp != suffixParts[i] {
			return false
		}
	}
	*subID = parsed
	return true
}

// handleEventPanelRoute is the single chi route shim for all
// four Event-side panel routes (research-log, sources, tags,
// images). It parses the eventID once, then walks
// eventPanels to find the matching panel + (method, subPath)
// route. The architecture-review body (#343 finding #3) flagged
// the prior shape — four near-identical dispatcher blocks in
// events_handlers.go — as a deletion-test signal: "delete the
// dispatcher, do the panels keep working? Yes. The handler
// functions below are the real work; the dispatcher is glue."
//
// On a match, the per-panel handler is called with eventID +
// subID. On no match, the dispatcher returns 404 for an
// unknown panel or unknown subPath, 405 for a known panel +
// unknown method. The 405-vs-404 split keeps the per-panel
// REST contract honest: clients can probe the allowed methods
// without having to guess from a generic 404.
func (a *App) handleEventPanelRoute(w http.ResponseWriter, r *http.Request) {
	prefix := "/events/"
	trimmed := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	eventID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	suffix := parts[1]
	// Find the panel whose name is a prefix of the suffix.
	// /events/42/sources/attach → suffix="sources/attach",
	// panelName="sources", remainder="attach". /events/42/
	// sources → suffix="sources", remainder="".
	for _, panel := range eventPanels {
		if !strings.HasPrefix(suffix, panel.name) {
			continue
		}
		// Must be followed by "/" or be the entire suffix.
		rest := strings.TrimPrefix(suffix, panel.name)
		if rest != "" && !strings.HasPrefix(rest, "/") {
			continue
		}
		remainder := strings.TrimPrefix(rest, "/")
		// Walk the panel's routes for a matching (method,
		// subPath). Track whether any route matched on
		// method alone so we can emit 405 vs 404 correctly.
		methodMatched := false
		for _, route := range panel.routes {
			if route.method != r.Method {
				continue
			}
			methodMatched = true
			var subID int64 = -1
			if matchSubPath(remainder, route.subPath, &subID) {
				route.handler(a, w, r, eventID, subID)
				return
			}
		}
		if methodMatched {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

// eventPanels is the registry. Adding a new Event-side panel
// = appending one entry below. The chi route registrations
// in routes.go must match (each entry's name becomes the
// {panelName} URL segment).
//
// All panel handlers below are package-level functions that
// match the eventPanelHandler signature. They wrap the
// existing per-panel helpers (handleEventSourcesGet,
// handleEventTagAdd, handleEventImageImport, etc.) so the
// collapse does NOT change the inner behavior. The handler
// signatures on those inner helpers are unchanged; only the
// dispatch path changes.
var eventPanels = []eventPanel{
	{
		name: "research-log",
		routes: []eventPanelRoute{
			// /events/{id}/research-log              → log page
			{method: http.MethodGet, subPath: "", handler: handleEventResearchLogGet},
			// /events/{id}/research-log/tasks        → create task
			{method: http.MethodPost, subPath: "tasks", handler: handleEventResearchLogPostTasks},
			// /events/{id}/research-log/tasks/{id}/resolve
			{method: http.MethodPost, subPath: "tasks/{id}/resolve", handler: handleEventResearchLogPostResolve},
		},
	},
	{
		name: "sources",
		routes: []eventPanelRoute{
			{method: http.MethodGet, subPath: "", handler: handleEventSourcesGetForDispatch},
			{method: http.MethodPost, subPath: "attach", handler: handleEventSourceAttachForDispatch},
			{method: http.MethodPost, subPath: "{id}/detach", handler: handleEventSourceDetachForDispatch},
		},
	},
	{
		name: "tags",
		routes: []eventPanelRoute{
			{method: http.MethodGet, subPath: "", handler: handleEventTagsGetForDispatch},
			{method: http.MethodPost, subPath: "", handler: handleEventTagAddForDispatch},
			{method: http.MethodPost, subPath: "{id}/detach", handler: handleEventTagDetachForDispatch},
		},
	},
	{
		name: "images",
		routes: []eventPanelRoute{
			{method: http.MethodGet, subPath: "", handler: handleEventImagesGetForDispatch},
			{method: http.MethodPost, subPath: "import", handler: handleEventImageImportForDispatch},
			{method: http.MethodPost, subPath: "delete", handler: handleEventImagesDeleteForDispatch},
		},
	},
}

// The handlers below are thin method-bound adapters that
// match the eventPanelHandler signature (taking both eventID
// and subID). The adapters exist because the inner handlers
// (handleEventSourcesGet, handleEventTagAdd, etc.) have
// existing signatures that take only eventID — keeping those
// signatures unchanged preserves every existing caller's
// contract. The adapter ignores subID for routes that don't
// use it (e.g. handleEventSourcesGet) and forwards it for
// the parametric routes (e.g. handleEventSourceDetach).

// Sources panel adapters — handleEventSourcesGet / Attach /
// Detach already take (w, r, eventID) / (w, r, eventID,
// sourceID). The adapters normalize to (w, r, eventID,
// subID) so the registry can call them with a single
// signature. handleEventSourceDetach validates subID > 0
// before forwarding; the dispatcher will pass -1 when the
// route has no sub-id (which the sources panel never does,
// so subID is always positive here).
// The handlers below are thin method-bound adapters that
// match the eventPanelHandler signature (taking *App, w, r,
// eventID, subID). The adapters exist because the inner
// handlers (handleEventSourcesGet, handleEventTagAdd, etc.)
// have existing signatures that take only (w, r, eventID) —
// keeping those signatures unchanged preserves every existing
// caller's contract. The adapter ignores subID for routes
// that don't use it (e.g. handleEventSourcesGet) and forwards
// it for the parametric routes (e.g. handleEventSourceDetach).

// Sources panel adapters.
func handleEventSourcesGetForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventSourcesGet(w, r, eventID)
}

func handleEventSourceAttachForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventSourceAttach(w, r, eventID)
}

func handleEventSourceDetachForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	if subID <= 0 {
		http.Error(w, "invalid source id", http.StatusBadRequest)
		return
	}
	a.handleEventSourceDetach(w, r, eventID, subID)
}

// Tags panel adapters — same shape as sources.
func handleEventTagsGetForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventTagsGet(w, r, eventID)
}

func handleEventTagAddForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventTagAdd(w, r, eventID)
}

func handleEventTagDetachForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	if subID <= 0 {
		http.Error(w, "invalid tag id", http.StatusBadRequest)
		return
	}
	a.handleEventTagDetach(w, r, eventID, subID)
}

// Images panel adapters.
func handleEventImagesGetForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventImagesGet(w, r, eventID)
}

func handleEventImageImportForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventImageImport(w, r, eventID)
}

func handleEventImagesDeleteForDispatch(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventImagesDelete(w, r, eventID)
}

// Research-log panel adapters. The prior shape parsed the
// suffix inline inside handleEventResearchLog; the
// dispatcher now does the suffix parse, so the inner
// handler can be split into three per-route funcs.
func handleEventResearchLogGet(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	log, err := a.soldiers.ResearchLog(eventID)
	if err != nil {
		respondNotFound(w, r, "Research log for event record not found.", err)
		return
	}
	if err := presentation.ResearchLogView(*log).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the research log for the event record.", err)
	}
}

// handleEventResearchLogPostTasks creates a new research task.
// Pulled out of the prior handleEventResearchLog so the
// dispatcher can route POST /research-log/tasks to it.
func handleEventResearchLogPostTasks(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	a.handleEventResearchTaskCreate(w, r, eventID)
}

// handleEventResearchLogPostResolve marks a task resolved.
// subID is the parsed task id; the dispatcher validates it.
func handleEventResearchLogPostResolve(a *App, w http.ResponseWriter, r *http.Request, eventID int64, subID int64) {
	if subID <= 0 {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}
	a.handleEventResearchTaskResolve(w, r, eventID, subID)
}