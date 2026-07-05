# Plan: Event edit inline Linked Persons + Tags (#361)

## Goal

Close two parallel gaps on the Event Record surfaces: (1) the
Event detail page's Linked Persons + Tags panels expose data
but offer no add/edit affordance — add an "Edit Event" CTA on
both panel headers, mirroring the post-#360 Sources panel
pattern, and (2) the Event edit page (`/events/{id}/edit`) is
missing inline link/unlink + tag attach/detach sections,
forcing users to bounce through the Person Record's Events
tab to manage Event↔Person links. After this slice the user
journey is: detail page (read-only) → Edit Event (everything
in one surface).

Sibling slices in flight:
- #357 (Source Records inline on edit page — landed)
- #360 (Edit Event CTA on Sources panel — landed)
- #325 (Person-side Events tab attach/detach — landed)

## Decisions (locked, user-confirmed)

1. **Display ID string input** for "Add Linked Person" form.
   User types a Display ID (e.g. `SOL-01234`), form posts to
   `/events/{id}/links`, handler resolves via
   `a.soldiers.GetByDisplayID(displayID).ID` then calls existing
   `EventService.AttachEventToPerson(eventID, personID)`. Matches
   `/soldiers/{id}/events/attach-by-display-id` pattern at
   `events_handlers.go:348-380`.

2. **Full-page nav** (no in-place fragment swap on the edit
   page). Linked Persons + Tags sections post to dedicated
   sub-routes which return `X-DixieData-Redirect: /events/{id}/edit`.
   Matches `/soldiers/{id}/events` Person-side attach pattern
   in `person_events_tab.templ`.

3. **Add count next to title** on Tags panel header, mirroring
   Linked Persons `' {N} linked '` and Sources `' {N} attached '`
   shapes.

4. **3 slices end-to-end** (not 1 mega-commit, not 2). Each
   slice independently reviewable per AGENTS.md 3-tier rule.
   Slice 1 is the tracer bullet.

5. **Add `LookupPersonIDByDisplayID` to `internal/records/event_service.go`.**
   Single helper, used by 1 caller today but the Articles
   feature (#321) will need it. Avoids the import-direction
   question (EventService → SoldierService) by keeping the
   helper on EventService, which composes via the existing
   `s.soldiers` field if present, or via a new
   `internal/records.PersonResolver` if not. Implementer
   decides the seam at slice 2's session start.

6. **Tags CRUD already works** — `/events/{id}/tags` (POST) +
   `/events/{id}/tags/{tagId}/detach` (POST) routes exist at
   `events_handlers.go:869` (`handleEventTagsRoute`). The issue
   body suggests they "need to be added" but they don't. Slice 3
   reuses them verbatim, just adds the edit-page UI.

## Slice 1 — Detail page CTAs + empty-state copy + Tags count (TRACER BULLET)

**Files touched** (no backend):
- `internal/templates/event_detail.templ` — add Edit Event CTA
  to Linked Persons header (line 101-117) + Tags header
  (line 134-139); mirror Sources panel post-#360 shape at
  line 119-132.
- `internal/templates/event_panels.templ` — update Tags empty
  state copy (line 71) to reference the event editor.
- `internal/appshell/events_handlers_test.go` — add
  `TestEventDetailLinkedPersonsPanelEditCTA` +
  `TestEventDetailTagsPanelEditCTA` (Go regression net; pins
  the markup).
- `audit/smoke_events.mjs` — add `step-04c linked-persons-cta`
  + `step-04d tags-cta` inside the try block at line 266.

**Locked design**:
- Edit Event CTA: `<button type="button" data-action={ templ.SafeURL(fmt.Sprintf("/events/%d/edit", event.ID)) } data-dixie-submit="true" class="ghost-link">Edit Event</button>` (literal button, not `components.Button` — avoids #365 open bug; `type="button"` matches §1.1 bug-pattern guidance).
- Linked Persons panel header — same flex-row shape as Sources panel: title + count + Edit Event button.
- Tags panel header — gets count span added; rest of header (just `<p>Tags</p>`) wrapped in flex row + count + Edit Event button.
- Empty state on Linked Persons (currently at `event_detail.templ:106`): copy changes from "Attach this event to a Person Record from the Person Record detail page's Events tab." to "Manage linked Person Records from the event editor."
- Tags empty state copy (`event_panels.templ:71`) unchanged — already says "Add tags to group this Event with related Person Records." which is fine.

**Success criteria**:
- `/events/{id}` GET response body for a seeded event with no links + no tags contains the strings `Edit Event` twice (once per panel header).
- `/events/{id}` GET response for a seeded event with 1 link + 2 tags contains `1 linked` + `2 tag` substring matches.
- `/events/{id}` GET response for a seeded event with no links contains "Manage linked Person Records from the event editor." substring.

**Regression net**:
- `go test ./internal/appshell/ -run 'TestEventDetail.*PanelEditCTA' -count=1` green.
- `node audit/smoke_events.mjs` runs to completion; new step names appear in the report.
- `make test` green (full suite).

**Out of scope for this slice**:
- Any change to `/events/{id}/edit` page.
- Any backend route changes.
- Changes to the Tags empty-state copy at `event_panels.templ:71` (already adequate).

## Slice 2 — Event editor Linked Persons section (stub)

One-line shape: new `EventLinksListFragment` template +
`/events/{id}/links` POST + `/events/{id}/links/{personId}/detach`
POST routes + `LookupPersonIDByDisplayID` helper +
inline Linked Persons section in `event_form.templ`. UI shape
mirrors `person_events_tab.templ` Linked Events section
(port-of-the-port). Detail at slice 2's session start.

## Slice 3 — Event editor Tags section (stub)

One-line shape: inline Tags section in `event_form.templ`,
reusing existing `/events/{id}/tags` + `/events/{id}/tags/{tagId}/detach`
routes + existing `EventTagsListFragment` swap pattern. Detail
at slice 3's session start.

## Surface inventory (for the orphan-handler probe)

After all 3 slices land, the following NEW routebuilder
helpers + routes must exist with UI callers (otherwise
`audit/discover_orphan_handlers.mjs --strict` fails):

- `routebuilder.EventLinksAttach(eventID int64)` — POST `/events/{id}/links`
- `routebuilder.EventLinksDetach(eventID, personID int64)` — POST `/events/{id}/links/{personId}/detach`

(No new helpers for Tags — the existing
`routebuilder.EventTagAttach` / `EventTagDetach` are already
implicit via the inline URL pattern in `event_panels.templ:79`.
If slice 3 decides to add proper routebuilder wrappers, they
land in this slice too.)

## Cross-cutting risks (flagged for the implementing session)

- **#365 is open bug** — `components.Button` silently drops
  `data-action` when value is `templ.SafeURL`. Slice 1 uses
  literal `<button>` to sidestep. Slice 2/3 must continue
  that pattern for any button carrying `data-action`.
- **UI IDs not yet in registry** — `#data-event-links-list`
  would be a new transient panel ID. Per `conventions.md:62-67`
  ad-hoc selectors are allowed for transient panels, but
  `uiids.go` should grow an `OverlayEventLinksList` constant
  if slice 2's fragment component becomes long-lived.
- **Display ID resolution failure** — `/events/{id}/links` POST
  with a bad Display ID must return a clear 400 + toast, not
  a 500 from `GetByDisplayID`. Handler should pre-validate
  Display ID format and call `GetByDisplayID` in a way that
  returns `ErrNotFound` cleanly. Look at how
  `handleAttachEventByDisplayIDRoute` (`events_handlers.go:348-380`)
  handles this — copy its shape.

## Issue housekeeping

- Relabel #361 from `needs-triage` to `ready-for-agent` (body
  matches the `ready-for-agent` criteria per
  `docs/agents/issue-tracker.md:127`).
- Slice 1 commits can reference `Resolves #361` if all 3 slices
  land in a sequence and the user signs off on merge; otherwise
  slice 1 closes no part of #361 and leaves the issue open for
  the follow-up slices.