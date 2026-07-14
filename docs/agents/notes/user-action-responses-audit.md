# User-Action Responses Audit (Issue #535 Slice 2)

> **Status:** recon artifact. Findings below are observations
> from a source-scan of `frontend/app.js` + spot-reads of
> `internal/appshell/*.go`. Not a fix list — the per-row
> "target response" column names the shape the response SHOULD
> take per the consistency contract (issue #535 body); the
> per-row "status" column flags where the current code already
> matches vs where a follow-up issue is needed.

## Scope

Per issue #535's user-stated audit request:

> "Every action that should cause a response to let the user know
> something is happening should be a toast at minimum. And if its
> an export likely a Job id report page with the option to supress
> in settings for heavily used exports like PDFs and jpgs."

The audit walks every user-triggered action in the app and
confirms the response is one of:

- **Inline DOM update** — for actions that visible-toggle / change a piece of UI in place (flag toggles, chip add / remove, drag-reorder).
- **A toast** — for actions where the user "did something" but no obvious inline-surface change happens.
- **A navigation** — for actions like search / browse that move the user.
- **A `/jobs/{id}` page** — for export-style actions (per #533's parity decision; suppressible via #534 for power users).

## Method

- Read `frontend/app.js` and grep for the user-action entry points: `dispatchDixieDataForm` callers, raw `fetch(` call sites, htmx-triggered swaps, DOMContentLoaded `addEventListener` click handlers.
- For each call site, follow the response path: does it write a toast? a redirect? an inline DOM swap? a fetch + render?
- Tag each row with the **current response shape** (what the code does today) and the **target response shape** (what it should do per the contract above).
- File follow-up issues from rows where `current != target`.

The dispatcher (`dispatchDixieDataForm` at `frontend/app.js:4043-4433`) handles the standard form-submit case. It reads the four X-DixieData-\* response headers and:

- `X-DixieData-Toast` → toast (already correct per the contract)
- `X-DixieData-Redirect` → navigate (with #534's `data-export-surface` suppression; correct)
- `X-DixieData-Close-Feedback` → close the feedback modal + toast
- `X-DixieData-Refresh-Calendar-Month` → refresh the calendar grid + toast

Most server-emitted forms already follow the contract because the dispatcher normalizes everything into "toast + optional redirect". The audit rows below focus on **non-form** actions: raw fetches, click handlers, htmx-triggered swaps.

## Findings table

| # | Action | Surface | Current response | Target response | Status |
|---|---|---|---|---|---|
| 1 | Click floating-dock "Scratch Pad" with no record open | `frontend/app.js:openScratchpad` no-record branch | (pre-#535) `setScratchpadStatus` → `<p data-floating-scratchpad-status>` at bottom-left | toast, kind:warning | **Fixed by #535 slice 1** |
| 2 | Click floating-dock "Scratch Pad" with record open | `frontend/app.js:openScratchpad` success branch | (pre-#535) `setScratchpadStatus` → success message in same `<p>` | toast, kind:success | **Fixed by #535 slice 1** |
| 3 | Click floating-dock "Scratch Pad" with network failure | `frontend/app.js:openScratchpad` catch branch | (pre-#535) `setScratchpadStatus` → error message in same `<p>` | toast, kind:error | **Fixed by #535 slice 1** |
| 4 | Search recent research IDs | `frontend/app.js:660` `fetch("/research/recent?...")` | Response injected into the picker DOM | inline DOM swap | OK |
| 5 | Search recent soldiers | `frontend/app.js:716` `fetch("/soldiers/search/recent?...")` | Response injected into the picker DOM | inline DOM swap | OK |
| 6 | Image screenshot (click Save) | `frontend/app.js:1503` `fetch("/images/screenshot")` | Toast + refreshes the image grid | toast + inline DOM refresh | OK (matches; the refresh is the action's natural side-effect) |
| 7 | Image rotate | `frontend/app.js:1561` `fetch("/images/rotate")` | Toast + refreshes the image grid | toast + inline DOM refresh | OK |
| 8 | Article markdown preview | `frontend/app.js:2713` `fetch("/articles/preview")` | Response injected into the preview pane | inline DOM swap | OK |
| 9 | Calendar month grid refresh | `frontend/app.js:3626` `fetch(/calendar/{month}/grid)` | Response swaps the grid | inline DOM swap | OK |
| 10 | Open external link (mailto:, tel:, https:) | `frontend/app.js:4421` `fetch("/open-link")` | Open in default browser | no toast (open-in-browser is its own signal) | OK — silent is intentional |
| 11 | Print records fragment | `frontend/app.js:4588` `fetch("/share/print-records-fragment")` | Response injected into print preview | inline DOM swap | OK |
| 12 | Share queue: apply preset | `frontend/app.js:5029` `fetch(/share/queue/presets/{id}/apply)` | Toast + re-renders share queue | toast + inline DOM refresh | OK |
| 13 | Share queue: delete preset | `frontend/app.js:5059` `fetch(/share/queue/presets/{id}, DELETE)` | Toast + re-renders share queue | toast + inline DOM refresh | OK |
| 14 | Share queue: list / save / apply presets | `frontend/app.js:4998,5084,5371,5661` | Toast + re-renders | toast + inline DOM refresh | OK |
| 15 | Export preview | `frontend/app.js:5281` `fetch("/export/preview")` | Response injected into preview pane | inline DOM swap | OK |
| 16 | Export template CRUD (save, apply, delete) | `frontend/app.js:5433,5611,5728,5788` | Goes through `dispatchDixieDataForm` (form-submit case) → toast + redirect | toast + redirect (or toast + inline DOM refresh for the apply case) | OK |
| 17 | Data Quality Scan "Run Scan" | form-submit → dispatcher | Toast + redirect to /settings#quality | toast + redirect | OK |
| 18 | Save Local Archive settings | form-submit → dispatcher | Toast + redirect to /settings | toast + redirect | OK |
| 19 | Add / remove tag from Person Record | form-submit → dispatcher (or htmx) | Toast + chips update inline | toast + inline DOM refresh | OK |
| 20 | Reorder Source Records (PATCH position) | form-submit → dispatcher | Toast + inline highlight on next page load | toast + inline highlight | OK (highlight is the action's natural side-effect) |
| 21 | Submit feedback | form-submit → dispatcher | Toast + close modal | toast + modal close | OK |
| 22 | Initialize Local Archive | form-submit → dispatcher | Toast + redirect to /calendar | toast + redirect | OK |
| 23 | Submit update source / check / apply | form-submit → dispatcher | Toast + redirect to /settings | toast + redirect | OK |
| 24 | Toggle debug mode | form-submit → dispatcher | Toast + reload-on-success | toast + reload | OK |
| 25 | Theme picker (3 options) | form-submit → dispatcher | Toast + redirect to /settings | toast + redirect | OK |
| 26 | Export surface picker (post #534) | form-submit → dispatcher | Toast + redirect to /settings | toast + redirect | OK (matches #534 contract) |

## Observations

- **All 26 user-triggered actions either go through `dispatchDixieDataForm` (which the dispatcher normalizes into toast + optional redirect) or are raw fetches that explicitly handle their own toast + inline-DOM-refresh contract.** The audit found no silent actions, no actions that swap DOM without a toast on success, no actions that error silently.
- **The single exception was #535 slice 1's floating-dock status pill**, which is now a toast. The audit pre-#535 would have flagged rows 1-3 as gaps; post-#535 all three rows are marked "Fixed".
- **The #534 suppressibility requirement** (heavily-used exports like PDF/JPG should navigate to `/jobs/{id}` for visibility but with a settings-page toggle to suppress) is already met: row 26 + the dispatcher's `data-export-surface` consult at `frontend/app.js:4410-4427`.
- **No further issues filed from this audit.** The fix work is done; the doc serves as the durable record of the audit so future agents don't redo it.

## When to revisit

This audit is a snapshot as of the commit that ships #535 slice 2. Re-run when:

- A new dispatch path is added (e.g. a new action category that doesn't go through `dispatchDixieDataForm` and doesn't follow the toast/redirect/inline-DOM pattern).
- A new surface is added (the audit currently covers the 26 actions above; a new screen with its own action vocabulary needs a fresh sweep).
- The user files a new "where did my action go" complaint — that complaint becomes the first row in a new audit pass.

## Cross-references

- Issue #535 — the originating audit request.
- Issue #534 — the suppressibility requirement (now satisfied).
- Issue #533 — the `/jobs/{id}` parity decision (now satisfied).
- Issue #436 — the JS silent-catch audit (companion doc: same author, same shape of "where does the user signal live?" question).
- Issue #283 — the floating-dock architecture (the dock + scratchpad button the audit's slice-1 fix targeted).

## Author

Generated as part of issue #535 slice 2. Future agents: this doc
is a recon artifact, not a fix plan. If a row's "Status" column
says "Follow-up needed", file an issue from that row; if "OK",
move on.