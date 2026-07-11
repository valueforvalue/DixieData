// events_handlers.go holds the Event Record HTTP handlers for
// slice 3 of issue #320. The handlers route through a.events
// (the eventsFacade wired in app.go:reloadServices) and never
// call a.soldiers for Event-only operations.
//
// Per the RPCI spec (events_handlers.go is the slice-3
// apply-site), the handlers cover the v1 user-facing surface:
//
//   GET    /events                              list page
//   GET    /events/new                          new-event form
//   POST   /events/new                          create event
//   GET    /events/{id}                         event detail page
//   PUT    /events/{id}                         update event
//   DELETE /events/{id}                         delete event
//   GET    /events/{id}/edit                    edit-event form
//   POST   /events/{id}/edit                    update event (alias)
//   GET    /soldiers/{id}/events                Person Events tab
//   POST   /soldiers/{id}/events/{eventId}/attach   link event to person
//   POST   /soldiers/{id}/events/{eventId}/detach   unlink event
//   POST   /soldiers/{id}/events/quick-add     create + link in one tx
//   POST   /soldiers/{id}/events/attach-by-display-id  link by EVT-NNNNN
//   GET    /events/{id}/research-log          Event research log (slice #328)
//   POST   /events/{id}/research-log/tasks    add a research task to an Event
//   POST   /events/{id}/research-log/tasks/{entryId}/resolve  resolve a task
//
// Sources, scratchpad, research-log, tags, images, and per-event
// PDF handlers are tracked as follow-up issues per the
// out-of-scope section of the RPCI spec.
package appshell

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/valueforvalue/DixieData/internal/jobs"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/routebuilder"
	"github.com/valueforvalue/DixieData/internal/presentation"
	"github.com/valueforvalue/DixieData/internal/records"
	"github.com/valueforvalue/DixieData/internal/templates"
	"github.com/valueforvalue/DixieData/internal/viewmodel"
	"database/sql"
)

// handleEvents renders the /events list page. Method must be
// GET; POST is rejected with 405. The list excludes the
// linked-Person-Records subquery for efficiency; the detail
// page is where the link set is rendered.
func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page := parsePage(r.URL.Query().Get("page"))
	events, err := a.events.ListEvents(page, 50)
	if err != nil {
		respondInternal(w, r, "Could not list event records.", err)
		return
	}
	// total = len(events) keeps the presentation.EventList
	// signature stable. The "Showing N of M" copy on the
	// list page reads correctly for the v1 landing; once
	// the EventService grows a Count() method the total
	// here can switch to the real value.
	total := len(events)
	// Issue #384 / Slice 1: wrap the Render call so a templ failure
	// surfaces an EmptyStateError fragment instead of an empty body.
	if err := presentation.EventList(events, page, total).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, "Could not render the events list.", err)
	}
}

// newEventDefaults builds the starting values for the
// /events/new form. The Display ID is pre-allocated via
// (*DB).NextEventID so the field is read-only but present on
// first render (mirrors newSoldierDefaults for the Person
// Record form). The entry_type is hard-coded to "event" so
// the form's hidden entry_type field stays consistent with
// the persisted row.
func (a *App) newEventDefaults() (models.Soldier, error) {
	displayID, err := a.database.NextEventID()
	if err != nil {
		return models.Soldier{}, err
	}
	return models.Soldier{
		DisplayID: displayID,
		EntryType: models.EntryTypeEvent,
	}, nil
}

// handleNewEvent renders the GET form for /events/new and
// processes the POST that creates a new Event Record. The
// form body is parsed by parseEventForm; the service layer's
// CreateEvent enforces entry_type=event and clears the
// person-specific fields defensively.
func (a *App) handleNewEvent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		defaults, err := a.newEventDefaults()
		if err != nil {
			respondInternal(w, r, "Could not build the new-event defaults.", err)
			return
		}
		// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
		// EmptyStateError fragment instead of leaving the user with an empty body.
		if err := presentation.EventForm(defaults, false).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, "Could not render the new-event form.", err)
		}
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the event form.", err)
			return
		}
		event, sources, err := parseEventForm(r)
		if err != nil {
			defaults, defaultsErr := a.newEventDefaults()
			if defaultsErr != nil {
				respondInternal(w, r, "Could not build the new-event defaults.", defaultsErr)
				return
			}
			// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
			// EmptyStateError fragment instead of leaving the user with an empty
			// body. The err here is the parse failure the form is meant to surface
			// inline; a Render failure on top of that would leave the user staring
			// at a blank panel.
			if err := presentation.EventFormWithError(defaults, false, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the new-event form with errors.", err)
			}
			return
		}
		event.CreatedByImportPath = "create_event"
		created, err := a.events.CreateEvent(event)
		if err != nil {
			defaults, defaultsErr := a.newEventDefaults()
			if defaultsErr != nil {
				respondInternal(w, r, "Could not build the new-event defaults.", defaultsErr)
				return
			}
			// Issue #384 / Slice 2: see sibling block above.
			if err := presentation.EventFormWithError(defaults, false, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the new-event form with errors.", err)
			}
			return
		}
		// Issue #357: attach inline Source Record rows submitted
		// with the form. Empty rows are skipped by the service.
		if _, attachErr := a.events.AttachSourcesToEvent(created.ID, sources); attachErr != nil {
			respondInternal(w, r, fmt.Sprintf("Could not attach sources to event record %d.", created.ID), attachErr)
			return
		}
		// Option C: dispatchDixieDataForm reads
		// X-DixieData-Redirect and navigates the client to
		// the new event's detail page.
		writeExportRedirect(w, fmt.Sprintf("/events/%d", created.ID))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEventByID dispatches /events/{id} requests:
//   GET    -> render detail page
//   PUT    -> update event
//   DELETE -> delete event
// The /events/{id}/edit and /events/{id}/pdf sub-paths route
// through dedicated handlers and are NOT matched here; the
// chi router routes /events/{id:[0-9]+}/edit and
// /events/{id:[0-9]+}/pdf as their own paths.
func (a *App) handleEventByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/events/", "")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		event, err := a.events.GetEventByID(id)
		if err != nil {
			respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", id), err)
			return
		}
		tags, tagErr := a.events.ListTagsForEvent(id)
		if tagErr != nil {
			respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", id), tagErr)
			return
		}
		// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
		// EmptyStateError fragment instead of leaving the user with an empty body.
		if err := presentation.EventDetail(tags, event).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, fmt.Sprintf("Could not render event record %d.", id), err)
		}
	case http.MethodPut, http.MethodPost:
		// Issue #320 child #323: the smoke probe caught that the
		// edit form in event_form.templ posts to /events/{id} (not
		// /events/{id}/edit) so the JS dispatcher can re-use the
		// same Option C handler for new + edit. Routes.go registers
		// both POST + PUT on /events/{id}; this handler originally
		// only handled PUT and 405'd on POST. The two verbs share
		// the update-by-form path so they live in the same case.
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the event form.", err)
			return
		}
		event, err := a.events.GetEventByID(id)
		if err != nil {
			respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", id), err)
			return
		}
		updated, sources, err := parseEventForm(r)
		if err != nil {
			// Issue #361 slice 2: load links for the error
			// render so the user sees their existing links
			// alongside the validation error.
			linked, lerr := a.events.ListForEvent(event.Event.ID)
			if lerr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load linked Person Records for Event %d.", event.Event.ID), lerr)
				return
			}
			// Issue #361 slice 3: also load tags for the same
			// reason (so the user sees their existing chips
			// alongside the validation error).
			tags, tagErr := a.events.ListTagsForEvent(event.Event.ID)
			if tagErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", event.Event.ID), tagErr)
				return
			}
			// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
			// EmptyStateError fragment instead of leaving the user with an empty body.
			if err := presentation.EventFormWithErrorAndLinksAndTags(event.Event, linked, tags, true, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the edit-event form with errors.", err)
			}
			return
		}
		updated.ID = id
		if err := a.events.UpdateEvent(updated); err != nil {
			// Issue #361 slice 2: load links for the error
			// render so the user sees their existing links
			// alongside the validation error.
			linked, lerr := a.events.ListForEvent(event.Event.ID)
			if lerr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load linked Person Records for Event %d.", event.Event.ID), lerr)
				return
			}
			// Issue #361 slice 3: also load tags for the same
			// reason.
			tags, tagErr := a.events.ListTagsForEvent(event.Event.ID)
			if tagErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", event.Event.ID), tagErr)
				return
			}
			// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
			// EmptyStateError fragment instead of leaving the user with an empty body.
			if err := presentation.EventFormWithErrorAndLinksAndTags(event.Event, linked, tags, true, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the edit-event form with errors.", err)
			}
			return
		}
		// Issue #357: attach inline Source Record rows submitted
		// with the edit form. Empty rows are skipped.
		if _, attachErr := a.events.AttachSourcesToEvent(id, sources); attachErr != nil {
			respondInternal(w, r, fmt.Sprintf("Could not attach sources to event record %d.", id), attachErr)
			return
		}
		writeExportRedirect(w, fmt.Sprintf("/events/%d", id))
	case http.MethodDelete:
		if err := a.events.DeleteEvent(id); err != nil {
			respondInternal(w, r, fmt.Sprintf("Could not delete event record %d.", id), err)
			return
		}
		writeExportRedirect(w, "/events")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEditEvent renders the /events/{id}/edit form on GET
// and processes the POST that updates the Event. Both
// verbs reach the same UpdateEvent service call.
func (a *App) handleEditEvent(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodGet:
		event, err := a.events.GetEventByID(id)
		if err != nil {
			respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", id), err)
			return
		}
		// Issue #361 slice 2: populate LinkedPersons on the
		// viewmodel so the editor can render the inline
		// link/unlink section (mirrors how the detail page
		// already receives `linked []viewmodel.PersonRecord`).
		linked, err := a.events.ListForEvent(event.Event.ID)
		if err != nil {
			respondInternal(w, r, fmt.Sprintf("Could not load linked Person Records for Event %d.", event.Event.ID), err)
			return
		}
		// Issue #361 slice 3: also load tags so the inline
		// Tags section on the edit form can render the
		// current chips + a free-text add form.
		tags, tagErr := a.events.ListTagsForEvent(event.Event.ID)
		if tagErr != nil {
			respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", event.Event.ID), tagErr)
			return
		}
		// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
		// EmptyStateError fragment instead of leaving the user with an empty body.
		if err := presentation.EventFormWithLinksAndTags(event.Event, linked, tags, true).Render(r.Context(), w); err != nil {
			respondErrorFragment(w, r, KindInternal, fmt.Sprintf("Could not render the edit-event form for event record %d.", event.Event.ID), err)
		}
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			respondValidation(w, r, "Could not read the event form.", err)
			return
		}
		event, sources, err := parseEventForm(r)
		if err != nil {
			existing, fetchErr := a.events.GetEventByID(id)
			if fetchErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load Event %d for the error form.", id), fetchErr)
				return
			}
			// Issue #361 slice 2: load links for the error
			// render so the user sees their existing links
			// alongside the validation error.
			linked, lerr := a.events.ListForEvent(existing.Event.ID)
			if lerr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load linked Person Records for Event %d.", existing.Event.ID), lerr)
				return
			}
			// Issue #361 slice 3: also load tags.
			tags, tagErr := a.events.ListTagsForEvent(existing.Event.ID)
			if tagErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", existing.Event.ID), tagErr)
				return
			}
			// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
			// EmptyStateError fragment instead of leaving the user with an empty body.
			if err := presentation.EventFormWithErrorAndLinksAndTags(existing.Event, linked, tags, true, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the edit-event form with errors.", err)
			}
			return
		}
		event.ID = id
		if err := a.events.UpdateEvent(event); err != nil {
			existing, fetchErr := a.events.GetEventByID(id)
			if fetchErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load Event %d for the error form.", id), fetchErr)
				return
			}
			// Issue #361 slice 2: load links for the error
			// render so the user sees their existing links
			// alongside the validation error.
			linked, lerr := a.events.ListForEvent(existing.Event.ID)
			if lerr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load linked Person Records for Event %d.", existing.Event.ID), lerr)
				return
			}
			// Issue #361 slice 3: also load tags.
			tags, tagErr := a.events.ListTagsForEvent(existing.Event.ID)
			if tagErr != nil {
				respondInternal(w, r, fmt.Sprintf("Could not load tags for event record %d.", existing.Event.ID), tagErr)
				return
			}
			// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
			// EmptyStateError fragment instead of leaving the user with an empty body.
			if err := presentation.EventFormWithErrorAndLinksAndTags(existing.Event, linked, tags, true, err.Error()).Render(r.Context(), w); err != nil {
				respondErrorFragment(w, r, KindInternal, "Could not render the edit-event form with errors.", err)
			}
			return
		}
		// Issue #357: attach inline Source Record rows submitted
		// with the edit form. Empty rows are skipped.
		if _, attachErr := a.events.AttachSourcesToEvent(id, sources); attachErr != nil {
			respondInternal(w, r, fmt.Sprintf("Could not attach sources to event record %d.", id), attachErr)
			return
		}
		writeExportRedirect(w, fmt.Sprintf("/events/%d", id))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePersonEventsTab renders the Events section on a
// Person Record detail page. The route is
// /soldiers/{id}/events. For the v1 landing the handler
// redirects to the Person Record detail page; the Events
// section is rendered server-side on that page when the
// optimization can lazy-load the section as an htmx
func (a *App) handlePersonEventsTab(w http.ResponseWriter, r *http.Request, personID int64) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := a.soldiers.GetByID(personID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", personID), err)
		return
	}
	// Issue #320 slice #324: render the Events tab fragment
	// instead of redirecting to the Person Record detail page.
	// The section on soldier_card.templ uses hx-get to fetch
	// this route; the response swaps innerHTML.
	linked, err := a.events.ListForPerson(personID)
	if err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not load linked Event Records for Person %d.", personID), err)
		return
	}
	// Issue #384 / Slice 2: wrap Render so a templ failure surfaces an
	// EmptyStateError fragment instead of leaving the user with an empty tab.
	if err := presentation.PersonEventsTab(personID, linked).Render(r.Context(), w); err != nil {
		respondErrorFragment(w, r, KindInternal, fmt.Sprintf("Could not render the Events tab for Person %d.", personID), err)
	}
}
// Record by creating a row in event_person_links. The
// request body is empty (no form fields). On duplicate-link
// errors the handler returns 409 via respondConflict; the
// toast header surfaces the user-friendly message.
func (a *App) handleAttachEvent(w http.ResponseWriter, r *http.Request, personID, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := a.soldiers.GetByID(personID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", personID), err)
		return
	}
	if _, err := a.events.GetEventByID(eventID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", eventID), err)
		return
	}
	if _, err := a.events.AttachEventToPerson(eventID, personID); err != nil {
		if errors.Is(err, records.ErrDuplicateLink) {
			respondConflict(w, r, "This event is already linked to this person record.", err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not link event %d to person record %d.", eventID, personID), err)
		return
	}
	// Issue #345: redirect back to the Events tab (where the
	// user just clicked the form) instead of the Person detail.
	// The handler is invoked from the inline attach form on
	// /soldiers/{id}/events; without the /events suffix the user
	// loses the events context on every link action.
	writeExportRedirect(w, fmt.Sprintf("/soldiers/%d/events", personID))
}

// handleDetachEvent removes the event_person_links row that
// connects an Event to a Person Record. Idempotent: a
// missing link is treated as success (the EventService
// silently no-ops in that case).
func (a *App) handleDetachEvent(w http.ResponseWriter, r *http.Request, personID, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.events.DetachEventFromPerson(eventID, personID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not unlink event %d from person record %d.", eventID, personID), err)
		return
	}
	// Issue #345: redirect back to the Events tab (where the
	// unlink form lives) instead of the Person detail.
	writeExportRedirect(w, fmt.Sprintf("/soldiers/%d/events", personID))
}

// handleAttachEventByDisplayID wires the inline "Add existing
// event" form (issue #320 slice #325) to the linking flow.
// The form has a single `display_id` input; the handler
// resolves the Event via events.GetEventByDisplayID and
// delegates to handleAttachEvent for the duplicate-link and
// not-found paths. The attach-by-ID route is a separate
// endpoint (POST /soldiers/{id}/events/attach-by-display-id)
// so the URL surface for the existing /attach path (which
// takes an eventId path segment) stays unchanged.
func (a *App) handleAttachEventByDisplayID(w http.ResponseWriter, r *http.Request, personID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the attach form.", err)
		return
	}
	if _, err := a.soldiers.GetByID(personID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", personID), err)
		return
	}
	displayID := strings.TrimSpace(r.FormValue("display_id"))
	if displayID == "" {
		respondValidation(w, r, "Provide an Event Display ID like EVT-00001.", nil)
		return
	}
	resolved, err := a.events.GetEventByDisplayID(displayID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Event %q not found.", displayID), err)
		return
	}
	a.handleAttachEvent(w, r, personID, resolved.Event.ID)
}

// handleEventLinksAttach wires the Event editor's Add Linked
// Person form (issue #361 slice 2). The form has a single
// `display_id` input; the handler resolves the Person Record
// via EventService.LookupPersonIDByDisplayID (case-insensitive,
// whitespace-trimmed, returns os.ErrNotExist on missing/empty)
// and delegates to existing AttachEventToPerson. On success,
// redirects back to the editor surface (NOT the detail page)
// via X-DixieData-Redirect so the user keeps their in-progress
// edits. Mirrors /soldiers/{id}/events/attach-by-display-id
// (the Person-side counterpart).
func (a *App) handleEventLinksAttach(w http.ResponseWriter, r *http.Request, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the link form.", err)
		return
	}
	if _, err := a.events.GetEventByID(eventID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Event %d not found.", eventID), err)
		return
	}
	displayID := strings.TrimSpace(r.FormValue("display_id"))
	if displayID == "" {
		respondValidation(w, r, "Provide a Person Record Display ID (DXD-00001) or a name fragment.", nil)
		return
	}
	// Issue #373: the Add Linked Person form accepts a Display
	// ID OR a name fragment. Display ID wins when the input
	// matches one exactly (case-insensitive, whitespace-trimmed
	// via SoldierService.GetByDisplayID); if it does not match
	// we fall back to a substring search on the concatenated
	// name fields (LookupPersonIDByName). First match wins on
	// ties (sorted by display_id). Either path returns the same
	// os.ErrNotExist sentinel on miss, which the existing
	// errors.Is branch maps to HTTP 404.
	personID, lookupErr := a.events.LookupPersonIDByDisplayID(displayID)
	if lookupErr != nil {
		if !errors.Is(lookupErr, os.ErrNotExist) {
			respondError(w, r, KindInternal, "Could not look up the Person Record.", lookupErr)
			return
		}
		personID, lookupErr = a.events.LookupPersonIDByName(displayID)
		if lookupErr != nil {
			if errors.Is(lookupErr, os.ErrNotExist) {
				respondNotFound(w, r, fmt.Sprintf("No Person Record matched %q. Try a Display ID like DXD-00001 or a name fragment.", displayID), lookupErr)
				return
			}
			respondError(w, r, KindInternal, "Could not look up the Person Record.", lookupErr)
			return
		}
	}
	if _, err := a.events.AttachEventToPerson(eventID, personID); err != nil {
		// Duplicate-link is the only AttachEventToPerson error
		// path that's user-actionable (the user posted a
		// Display ID that's already linked). Other errors are
		// infrastructure-level (db down, etc.) and surface via
		// the standard error envelope.
		respondError(w, r, KindConflict, fmt.Sprintf("%s is already linked to this event.", displayID), err)
		return
	}
	writeExportRedirect(w, routebuilder.EventEdit(eventID))
}

// handleEventLinksDetach wires the Event editor's per-row
// Unlink button (issue #361 slice 2). The handler reads the
// personId path segment, calls existing DetachEventFromPerson,
// and redirects back to the editor. Detach is idempotent at
// the service layer, so a stale form post is harmless.
func (a *App) handleEventLinksDetach(w http.ResponseWriter, r *http.Request, eventID, personID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.events.DetachEventFromPerson(eventID, personID); err != nil {
		respondError(w, r, KindInternal, "Could not unlink the Person Record.", err)
		return
	}
	writeExportRedirect(w, routebuilder.EventEdit(eventID))
}

// handleQuickAddEvent creates a new Event and links it to a
// Person Record in one user action. The form fields are the
// same as /events/new plus the implicit link to the central
// Person Record. The handler calls CreateEvent (which mints
// the EVT-NNNNN Display ID) then attaches the new event to
// the Person Record. A failure in the attach step leaves
// the event in place; the response surfaces the error so
// the user can retry the link.
func (a *App) handleQuickAddEvent(w http.ResponseWriter, r *http.Request, personID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the quick-add event form.", err)
		return
	}
	if _, err := a.soldiers.GetByID(personID); err != nil {
		respondNotFound(w, r, fmt.Sprintf("Person record %d not found.", personID), err)
		return
	}
	event, sources, err := parseEventForm(r)
	if err != nil {
		respondValidation(w, r, err.Error(), err)
		return
	}
	// Issue #377 slice 2: stamp the import path so future
	// "where did this row come from?" investigations can
	// attribute the row to the Quick-Add Event form (vs. the
	// dedicated /events/new form, which stamps 'create_event').
	// The service-layer default already covers empty values, but
	// explicit stamping documents the intent at the handler site.
	event.CreatedByImportPath = "quick_add_event"
	created, err := a.events.CreateEvent(event)
	if err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not create the event for person record %d.", personID), err)
		return
	}
	// Issue #357: attach inline Source Record rows submitted with
	// the quick-add form.
	if _, attachErr := a.events.AttachSourcesToEvent(created.ID, sources); attachErr != nil {
		respondInternal(w, r, fmt.Sprintf("Could not attach sources to event record %d.", created.ID), attachErr)
		return
	}
	if _, err := a.events.AttachEventToPerson(created.ID, personID); err != nil {
		if errors.Is(err, records.ErrDuplicateLink) {
			writeExportRedirect(w, fmt.Sprintf("/soldiers/%d/events", personID))
			return
		}
		respondInternal(w, r, fmt.Sprintf("Event %d was created but the link to person record %d could not be saved.", created.ID, personID), err)
		return
	}
	// Issue #345: redirect back to the Person Events tab so the
	// researcher sees the freshly linked event in place.
	writeExportRedirect(w, fmt.Sprintf("/soldiers/%d/events", personID))
}

// parseEventForm reads the form fields for an Event Record
// from r and returns the domain models.Soldier payload the
// EventService.CreateEvent / UpdateEvent calls expect, plus
// any inline Source Record rows the form submitted. It mirrors
// parseSoldierForm's shape for the Event Record subtype: parses
// begin_date / end_date through the same canonical-date helper,
// hard-codes EntryType to "event", and clears the
// person-specific fields the service-layer normalizeSoldierEntry
// event branch will clear anyway.
//
// Issue #357: the Event form exposes a Source Records section
// mirroring the soldier entry form's Records[] rows. The
// record_type / record_app_id / record_details field triples
// arrive as parallel arrays (one entry per submitted row);
// parseRecordInputs (the soldier helper) flattens them into
// []models.Record so the handler can attach them in one call
// after CreateEvent / UpdateEvent succeeds.
func parseEventForm(r *http.Request) (models.Soldier, []models.Record, error) {
	beginDate, err := parseOptionalCanonicalDate(r.FormValue("begin_date"), "begin_date")
	if err != nil {
		return models.Soldier{}, nil, err
	}
	endDate, err := parseOptionalCanonicalDate(r.FormValue("end_date"), "end_date")
	if err != nil {
		return models.Soldier{}, nil, err
	}
	event := models.Soldier{
		DisplayID:          strings.TrimSpace(r.FormValue("display_id")),
		EntryType:          models.EntryTypeEvent,
		Kind:               strings.TrimSpace(r.FormValue("kind")),
		BeginDate:          beginDate,
		EndDate:            endDate,
		Description:        r.FormValue("description"),
		PDFExcerptOverride: r.FormValue("pdf_excerpt_override"),
		Notes:              r.FormValue("notes"),
	}
	sources := parseRecordInputs(r)
	return event, sources, nil
}


// handleEventResearchTaskCreate is the Event-side equivalent of
// the Person-Record handleResearchTaskCreate. Re-uses the
// same service methods (research_tasks is subtype-agnostic
// at the schema level) and only differs on the redirect URL.
func (a *App) handleEventResearchTaskCreate(w http.ResponseWriter, r *http.Request, eventID int64) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the research task form.", err)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	notes := strings.TrimSpace(r.FormValue("notes"))
	evidenceType := strings.TrimSpace(r.FormValue("evidence_type"))
	if err := a.soldiers.AddResearchTask(eventID, title, notes, evidenceType); err != nil {
		setToastHeaderWithType(w, "Research task could not be saved.", "error")
		respondInternal(w, r, fmt.Sprintf("Could not save research task for event record %d.", eventID), err)
		return
	}
	setToastHeader(w, "Success: research task added.")
	w.Header().Set("X-DixieData-Redirect", eventResearchLogRedirect(eventID))
	fmt.Fprint(w, "Research task saved.")
}

// handleEventResearchTaskResolve closes a research task on an
// Event. Service delegation matches the create path; the
// redirect URL is the only meaningful difference.
func (a *App) handleEventResearchTaskResolve(w http.ResponseWriter, r *http.Request, eventID, taskID int64) {
	if err := a.soldiers.ResolveResearchTask(eventID, taskID); err != nil {
		setToastHeaderWithType(w, "Research task could not be resolved.", "error")
		respondInternal(w, r, fmt.Sprintf("Could not resolve research task %d for event record %d.", taskID, eventID), err)
		return
	}
	setToastHeader(w, "Success: research task resolved.")
	w.Header().Set("X-DixieData-Redirect", eventResearchLogRedirect(eventID))
	fmt.Fprint(w, "Research task resolved.")
}

// eventResearchLogRedirect returns the post-action URL for
// Event research log mutations. Kept as a helper so future
// sub-types can branch to their own URL without touching
// the handlers.
func eventResearchLogRedirect(eventID int64) string {
	return fmt.Sprintf("/events/%d/research-log", eventID)
}

// handleEditEventRoute is the chi route shim for
// /events/{id}/edit. Parses the id from the URL path and
// delegates to handleEditEvent.
func (a *App) handleEditEventRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/events/", "/edit")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handleEditEvent(w, r, id)
}

// handlePersonEventsTabRoute is the chi route shim for
// /soldiers/{id}/events. Parses the id from the URL path and
// delegates to handlePersonEventsTab.
func (a *App) handlePersonEventsTabRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/soldiers/", "/events")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handlePersonEventsTab(w, r, id)
}

// handleAttachEventRoute is the chi route shim for
// /soldiers/{personId}/events/{eventId}/attach.
func (a *App) handleAttachEventRoute(w http.ResponseWriter, r *http.Request) {
	personID, eventID, err := parsePersonEventIDs(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid ids", http.StatusBadRequest)
		return
	}
	a.handleAttachEvent(w, r, personID, eventID)
}

// handleDetachEventRoute is the chi route shim for
// /soldiers/{personId}/events/{eventId}/detach.
func (a *App) handleDetachEventRoute(w http.ResponseWriter, r *http.Request) {
	personID, eventID, err := parsePersonEventIDs(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid ids", http.StatusBadRequest)
		return
	}
	a.handleDetachEvent(w, r, personID, eventID)
}

// handleQuickAddEventRoute is the chi route shim for
// /soldiers/{id}/events/quick-add.
func (a *App) handleQuickAddEventRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/soldiers/", "/events/quick-add")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handleQuickAddEvent(w, r, id)
}

// handleAttachEventByDisplayIDRoute is the chi route shim for
// /soldiers/{id}/events/attach-by-display-id.
func (a *App) handleAttachEventByDisplayIDRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/soldiers/", "/events/attach-by-display-id")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handleAttachEventByDisplayID(w, r, id)
}


// parseIntFromPath extracts the integer id embedded between
// two literal path segments. Used by the route shims to
// keep chi's pattern captures and the handler signatures
// in sync.
func parseIntFromPath(path, prefix, suffix string) (int64, error) {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.TrimSuffix(trimmed, suffix)
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", path, err)
	}
	return id, nil
}

// parsePersonEventIDs extracts (personID, eventID) from a
// /soldiers/{personID}/events/{eventID}/{action} URL path.
func parsePersonEventIDs(path string) (int64, int64, error) {
	trimmed := strings.TrimPrefix(path, "/soldiers/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 4 || parts[1] != "events" {
		return 0, 0, fmt.Errorf("unexpected path shape: %q", path)
	}
	personID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	eventID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return personID, eventID, nil
}

// handleEventPDFRoute is the chi route shim for /events/{id}/pdf.
// Parses the id from the URL path and delegates to handleEventPDF.
func (a *App) handleEventPDFRoute(w http.ResponseWriter, r *http.Request) {
	id, err := parseIntFromPath(r.URL.Path, "/events/", "/pdf")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handleEventPDF(w, r, id)
}

// handleEventPDF renders an Event Record (issue #320 v1, issue
// #374 portrait) to a single PDF. Mirrors the article PDF
// handler (handleArticlePDF): pre-render via
// EventService.RenderPDF + guarded SaveFileDialog
// (docs/agents/dialog-guard.md) + synchronous write. The
// orientation form field (portrait|landscape, default
// landscape) selects the matching event_<orientation>.typ
// template via the Registry's templateForRecordType mapping.
//
// The pre-render runs synchronously on the request goroutine
// because Event Record PDFs are small (single card, no images
// in v1); no job-enqueue overhead. The orientation picker
// lives on the Event detail page (components/event_pdf_export.templ)
// and posts the same shape as the article picker.
func (a *App) handleEventPDF(w http.ResponseWriter, r *http.Request, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the Event PDF export form.", err)
		return
	}
	orientation := strings.TrimSpace(r.PostFormValue("orientation"))
	if orientation == "" {
		orientation = "landscape"
	}

	result, err := a.events.RenderPDF(eventID, orientation)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", eventID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not render Event %d PDF.", eventID), err)
		return
	}

	opts := runtime.SaveDialogOptions{
		DefaultFilename: result.Filename,
		Filters: []runtime.FileFilter{
			{DisplayName: "PDF document", Pattern: "*.pdf"},
		},
	}
	dupKey := fmt.Sprintf("event-pdf|%d|%s|%s", eventID, orientation, result.Filename)
	path, outcome := a.guardedSaveFileDialog(dupKey, opts)
	switch outcome {
	case SaveOutcomeDuplicated:
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	case SaveOutcomeDialogAborted:
		respondError(w, r, KindValidation, "Event PDF export cancelled.", nil)
		return
	}
	if err := os.WriteFile(path, result.Bytes, 0o644); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not write PDF to %q.", path), err)
		return
	}
	w.Header().Set("X-DixieData-Toast", fmt.Sprintf("Event PDF saved to %s.", filepath.Base(path)))
	w.Header().Set("X-DixieData-Toast-Type", "success")
	w.WriteHeader(http.StatusOK)
}

// renderEventSourcesListFragment loads the Event's source
// records and writes the per-Event Sources list HTML into w.
// Shared by GET /events/{id}/sources (lazy-load probe) and
// the POST attach / detach handlers (in-place swap target).
// The fragment matches the on-page event_detail.templ render
// via the templ helper EventSourcesListFragment so the
// data-results-target swap is visually identical to the
// initial page render.
func (a *App) renderEventSourcesListFragment(w http.ResponseWriter, r *http.Request, eventID int64) {
	rows, err := a.events.ListSourcesForEvent(eventID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Sources for event record %d not found.", eventID), err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.EventSourcesListFragment(eventID, viewmodel.SourceRecordsFromModels(rows)).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render sources for event record %d.", eventID), err)
	}
}

// handleEventSourcesGet renders the Sources panel fragment for
// the Event. Reachable as a lazy-load probe (orphan-handler
// probe flagged it; intended JS consumer is the post-attach /
// post-detach data-results-target swap on event_detail.templ).
func (a *App) handleEventSourcesGet(w http.ResponseWriter, r *http.Request, eventID int64) {
	a.renderEventSourcesListFragment(w, r, eventID)
}

// handleEventSourceDetach removes a source record row from the
// Event. Service verifies the row belongs to the Event. After
// the write, the handler re-renders the Sources list fragment
// so the JS dispatcher can swap the result into
// #data-event-sources-list in place. Issue #341.
func (a *App) handleEventSourceDetach(w http.ResponseWriter, r *http.Request, eventID, sourceID int64) {
	if err := a.events.DetachSourceFromEvent(eventID, sourceID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not detach source %d from event record %d.", sourceID, eventID), err)
		return
	}
	setToastHeader(w, "Success: source detached.")
	a.renderEventSourcesListFragment(w, r, eventID)
}


// renderEventTagsListFragment loads the Event's tags and writes
// the per-Event Tags chip HTML into w. Shared by GET
// /events/{id}/tags (lazy-load probe) and the POST detach handler
// (in-place swap target). The fragment matches the on-page
// event_detail.templ render via the templ helper
// EventTagsListFragment so the data-results-target swap is
// visually identical to the initial page render.
func (a *App) renderEventTagsListFragment(w http.ResponseWriter, r *http.Request, eventID int64) {
	tags, err := a.events.ListTagsForEvent(eventID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Tags for event record %d not found.", eventID), err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.EventTagsListFragment(eventID, viewmodel.TagsFromModels(tags)).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render tags for event record %d.", eventID), err)
	}
}

// handleEventTagsGet renders the Tags chip fragment for the Event.
// Reachable as a lazy-load probe (orphan-handler probe flagged it;
// intended JS consumer is the post-detach data-results-target swap
// on event_detail.templ).
func (a *App) handleEventTagsGet(w http.ResponseWriter, r *http.Request, eventID int64) {
	a.renderEventTagsListFragment(w, r, eventID)
}

// handleEventTagAdd attaches a tag to the Event. The form may
// post either `tag_id` (numeric; existing behavior, used by
// the picker page that picks from a known tag list) OR
// `tag_name` (free-text; used by the inline edit-form picker
// added in issue #361 slice 3). When `tag_name` is supplied,
// the handler upserts via TagService.UpsertByName (case-
// insensitive dedup matches the soldier-side /soldiers/{id}/
// tags picker UX) and then attaches the resulting tag. After
// the write, the handler re-renders the Tags list fragment
// so the JS dispatcher can swap the result into
// #data-event-tags-list in place. Issue #341 — previously
// this handler set X-DixieData-Redirect, which sent the
// browser to the fragment-returning GET endpoint and
// displayed raw HTML as a page.
//
// The fragment-returning response shape (no
// X-DixieData-Redirect) is what enables in-place swap on the
// edit page: the user can attach/detach tags mid-edit
// without losing their unsaved changes to the kind /
// description / other form fields.
func (a *App) handleEventTagAdd(w http.ResponseWriter, r *http.Request, eventID int64) {
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the tag form.", err)
		return
	}
	var tagID int64
	if rawID := strings.TrimSpace(r.FormValue("tag_id")); rawID != "" {
		parsed, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || parsed < 1 {
			respondValidation(w, r, "Invalid tag id.", err)
			return
		}
		tagID = parsed
	} else if rawName := strings.TrimSpace(r.FormValue("tag_name")); rawName != "" {
		// Free-text path (slice 3): upsert + attach in one
		// round-trip. UpsertByName dedupes case-insensitively
		// (see TagService.UpsertByName at
		// internal/records/tag_service.go:75).
		tag, err := a.tags.UpsertByName(r.Context(), rawName)
		if err != nil {
			respondValidation(w, r, fmt.Sprintf("Could not create tag %q.", rawName), err)
			return
		}
		tagID = tag.ID
	} else {
		respondValidation(w, r, "Provide a tag id (numeric) or a tag name (free-text).", nil)
		return
	}
	if err := a.events.AddTagToEvent(eventID, tagID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not attach tag to event record %d.", eventID), err)
		return
	}
	setToastHeader(w, "Success: tag attached.")
	a.renderEventTagsListFragment(w, r, eventID)
}

// handleEventTagDetach removes the tag binding from the Event.
// After the write, the handler re-renders the Tags list
// fragment so the JS dispatcher can swap the result into
// #data-event-tags-list in place. Issue #341.
func (a *App) handleEventTagDetach(w http.ResponseWriter, r *http.Request, eventID, tagID int64) {
	if err := a.events.DetachTagFromEvent(eventID, tagID); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not detach tag %d from event record %d.", tagID, eventID), err)
		return
	}
	setToastHeader(w, "Success: tag detached.")
	a.renderEventTagsListFragment(w, r, eventID)
}

// renderEventImagesListFragment loads the Event's images and
// writes the per-Event Images panel HTML into w. Shared by GET
// /events/{id}/images (lazy-load probe) and the POST delete
// handler (in-place swap target). The fragment matches the
// on-page event_detail.templ render via the templ helper
// EventImagesListFragment so the data-results-target swap is
// visually identical to the initial page render.
func (a *App) renderEventImagesListFragment(w http.ResponseWriter, r *http.Request, eventID int64) {
	withLinks, err := a.events.GetEventByID(eventID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Images for event record %d not found.", eventID), err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.EventImagesListFragment(eventID, viewmodel.ImagesFromModels(withLinks.Event.Images)).Render(r.Context(), w); err != nil {
		respondInternal(w, r, fmt.Sprintf("Could not render images for event record %d.", eventID), err)
	}
}

// handleEventImagesGet renders the Images panel fragment for
// the Event. Reachable as a lazy-load probe + post-action
// swap target.
func (a *App) handleEventImagesGet(w http.ResponseWriter, r *http.Request, eventID int64) {
	a.renderEventImagesListFragment(w, r, eventID)
}

// handleEventImageImport opens the native file picker for image
// selection and enqueues an image_import background job, then
// redirects to the /jobs/{id} page so the user sees real
// progress during the file copy. Mirrors the Person Record
// /soldiers/{id}/images/import path; the only Event-specific
// change is the redirect target (the Event detail page, since
// events don't have an "edit" landing).
func (a *App) handleEventImageImport(w http.ResponseWriter, r *http.Request, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	withLinks, err := a.events.GetEventByID(eventID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", eventID), err)
		return
	}
	event := withLinks.Event

	// Issue #401: web-mode posts multipart/form-data (the import
	// form has a <input type="file" name="images" multiple>); the
	// native OpenMultipleFilesDialog path only runs when no upload
	// is present. See readUploadedImagePaths in app.go for the
	// upload parser. Web branch is synchronous: the JS dispatcher
	// reads the response body into the gallery wrapper via the
	// form's data-results-target, so the user stays on the event
	// detail page instead of bouncing through /jobs/{id}.
	uploadedPaths := readUploadedImagePaths(w, r)
	if uploadedPaths != nil {
		imported, importErr := a.importImagePaths(event, uploadedPaths)
		if importErr != nil {
			slog.Error("appshell: event image import (web)", "audit", "respond-error", "event_id", eventID, "imported", imported, "err", importErr.Error())
			respondInternal(w, r, "Could not import the uploaded images.", importErr)
			return
		}
		setToastHeader(w, fmt.Sprintf("Imported %d image(s).", imported))
		a.renderEventImagesListFragment(w, r, eventID)
		return
	}

	pathsOpts := runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{
			{DisplayName: "Image files", Pattern: "*.png;*.jpg;*.jpeg;*.gif;*.bmp;*.webp;*.svg"},
		},
	}
	dupKey := guardedOpenMultipleFilesDialogKey("import_images", pathsOpts)
	paths, admitted, ok := a.guardedOpenMultipleFilesDialog(dupKey, pathsOpts)
	if !admitted {
		a.respondDuplicateInFlight(w, r, dupKey)
		return
	}
	if !ok {
		respondError(w, r, KindValidation, "Image import cancelled.", nil)
		return
	}

	a.runEventImageImportJob(w, event, eventID, paths)
}

// runEventImageImportJob enqueues the image_import background job
// for an event with the given already-resolved source paths and
// writes the redirect response. Extracted from handleEventImageImport
// in issue #401 so the multipart upload branch and the native-dialog
// branch can share the same job-enqueue + response sequence.
func (a *App) runEventImageImportJob(w http.ResponseWriter, event models.Soldier, eventID int64, paths []string) {
	jobID := a.jobs.Start("image_import", func(ctx context.Context, p *jobs.Progress) error {
		p.Set(5, fmt.Sprintf("Importing %d image(s)", len(paths)))
		p.Shimmer(ctx, 5, 95, 60*time.Second, "Encoding images…")
		imported, importErr := a.importImagePaths(event, paths)
		if importErr != nil {
			slog.Error("appshell: event image import", "audit", "respond-error", "event_id", eventID, "imported", imported, "err", importErr.Error())
			return importErr
		}
		p.Set(100, fmt.Sprintf("Imported %d image(s).", imported))
		return nil
	})
	setInfoToastHeader(w, fmt.Sprintf("Importing %d image(s)…", len(paths)))
	writeExportRedirect(w, "/jobs/"+jobID)
}

// handleEventImagesDelete removes the image rows + files for
// the given image_ids. After the write, re-renders the
// images fragment in place (no X-DixieData-Redirect, per
// issue #341). Mirrors handleDeleteSoldierImages with the
// Event-specific adjustment.
func (a *App) handleEventImagesDelete(w http.ResponseWriter, r *http.Request, eventID int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the image delete form.", err)
		return
	}
	withLinks, err := a.events.GetEventByID(eventID)
	if err != nil {
		respondNotFound(w, r, fmt.Sprintf("Event record %d not found.", eventID), err)
		return
	}
	event := withLinks.Event
	selected, err := selectedRecordImages(event, r.Form["image_ids"], a.dataDir)
	if err != nil {
		respondValidation(w, r, "Could not parse selected image ids.", err)
		return
	}
	if len(selected) == 0 {
		respondError(w, r, KindValidation, "Select at least one image to delete.", nil)
		return
	}

	for _, image := range selected {
		if err := os.Remove(image.FilePath); err != nil && !os.IsNotExist(err) {
			respondInternal(w, r, fmt.Sprintf("Could not delete image file %s.", image.FilePath), err)
			return
		}
	}

	imageIDs := make([]int64, 0, len(selected))
	for _, image := range selected {
		imageIDs = append(imageIDs, image.ID)
	}
	if err := a.events.RemoveImages(eventID, imageIDs); err != nil {
		respondInternal(w, r, "Could not remove the image records from the database.", err)
		return
	}

	setToastHeader(w, fmt.Sprintf("Deleted %d image(s).", len(selected)))
	a.renderEventImagesListFragment(w, r, eventID)
}

// handleEventLinksAttachRoute is the chi route shim for
// /events/{id}/links (issue #361 slice 2).
func (a *App) handleEventLinksAttachRoute(w http.ResponseWriter, r *http.Request) {
	eventID, err := parseIntFromPath(r.URL.Path, "/events/", "/links")
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a.handleEventLinksAttach(w, r, eventID)
}

// handleEventLinksDetachRoute is the chi route shim for
// /events/{id}/links/{personId}/detach (issue #361 slice 2).
func (a *App) handleEventLinksDetachRoute(w http.ResponseWriter, r *http.Request) {
	// Path shape: /events/{eventID}/links/{personID}/detach
	trimmed := strings.TrimPrefix(r.URL.Path, "/events/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 4 || parts[1] != "links" || parts[3] != "detach" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	eventID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}
	personID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid person id", http.StatusBadRequest)
		return
	}
	a.handleEventLinksDetach(w, r, eventID, personID)
}

// handleMoveEventSource reorders an Event Source within
// its owning Event. Issue #368 slice 2.
//
// Method: PATCH /events/{id}/sources/{sourceId}/position
// Body: form field `position` (1-indexed; clamped server-side
// to [1, N]).
func (a *App) handleMoveEventSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/events/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 || parts[1] != "sources" || parts[3] != "position" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	eventID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}
	sourceID, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid source id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		respondValidation(w, r, "Could not read the position form.", err)
		return
	}
	position, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("position")), 10, 64)
	if err != nil || position < 1 {
		respondValidation(w, r, "Position must be a positive integer.", err)
		return
	}
	if err := a.events.MoveEventSource(eventID, sourceID, position); err != nil {
		if strings.Contains(err.Error(), "not attached to event") {
			respondNotFound(w, r, fmt.Sprintf("Source %d is not attached to event %d.", sourceID, eventID), err)
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, fmt.Sprintf("Source %d not found.", sourceID), err)
			return
		}
		respondInternal(w, r, fmt.Sprintf("Could not reorder source %d for event %d.", sourceID, eventID), err)
		return
	}
	setToastHeader(w, "Event source reordered.")
	w.Header().Set("X-DixieData-Redirect", fmt.Sprintf("/events/%d", eventID))
	w.WriteHeader(http.StatusOK)
}
