# DixieData AI Handoff

## Project snapshot

- **App:** DixieData
- **Version:** `1.1.19`
- **Schema version:** `19`
- **Platform:** Wails desktop app (Windows-first workflow)
- **Backend:** Go + SQLite
- **Frontend:** server-rendered Templ HTML, Tailwind CDN styling, custom `frontend\app.js`
- **Primary repo entry points:** `main.go`, `app.go`

DixieData is a desktop archive manager for Civil War / Confederate research records. The app stores soldiers plus spouse records, supporting notes, records, images, scratch pads, printable reports, portable exports, mergeable shared archives, analytics, duplicate review, and Google integrations.

Recent v1.1 hardening highlights:

- Smart Back restores the previous browse context after detail/edit navigation
- quick search is FTS5-backed and indexes scratch-pad content
- advanced search includes entry-type filtering for soldiers, wives, and widows
- image storage is sharded on disk for scale
- `.ddshare` exports are merge-ready record packages instead of full SQLite snapshots

## Current architecture

### App shell

- `main.go` starts Wails and binds a single `App` instance.
- `app.go` builds an `http.ServeMux` and serves the whole UI through normal HTTP handlers.
- Wails embeds `frontend\` assets and routes requests through `App.ServeHTTP`.
- If the UI requests content before startup finishes, `ServeHTTP` now returns an **auto-refreshing loading placeholder** instead of a dead-end plain text `503`. This prevents the app from getting stuck on “Application is still starting up.”

### Storage model

The local working directory is `.dixiedata` by default and is resolved from the project root or executable location. It can be overridden with `DIXIEDATA_DATA_DIR`.

Common contents:

- `dixiedata.db` - main SQLite database
- `images\` - sharded image storage
- `scratchpads\` - per-record scratch pad text/json files
- `backups\` - generated backup artifacts and pre-migration DB snapshots
- `merge-review\` - staged merge-review working files
- `logs\` - merge and troubleshooting logs
- `temp_trash\` - delayed-delete staging area for orphan cleanup

### Data model

Core tables in `internal\db\schema.go`:

- `soldiers`
- `records`
- `images`
- `merge_review_sessions`
- `merge_review_conflicts`
- `duplicate_audit_findings`
- `system_config`
- `scratchpad_cache`
- `schema_version`

Important behaviors:

- `soldiers` contains both true soldiers and spouse entries via `entry_type`.
- spouse records reference the soldier table through `spouse_soldier_id`.
- review state is persisted on `soldiers` via `needs_review` and `review_reason`.
- duplicate-audit pair state is persisted separately in `duplicate_audit_findings`.
- FTS search is backed by `soldiers_fts` plus triggers and `scratchpad_cache`.

### Identity and ID rules

Identity is configured during first-run setup and stored in `system_config`.

Relevant rules:

- display IDs follow a namespace pattern such as `STC38-00020`
- legacy `DXD-00001` IDs are treated as canonical legacy IDs
- recursive wrapping is forbidden
- merge keep-both now generates a **fresh local ID** in the local namespace rather than nesting prefixes
- imported legacy or shared rows preserve authorship in `added_by`

Identity helpers live in:

- `internal\db\identity.go`
- `internal\db\displayid.go`
- `internal\db\csaid.go`
- merge logic in `internal\services\backup_service.go`

## Major UI surfaces

Routes are registered in `app.go`.

### Main pages

- `/calendar` - landing page, archive counts, rotating quote, month navigation
- `/soldiers` - browse archive
- `/soldiers/new` - create record
- `/soldiers/{id}` - detail
- `/soldiers/{id}/edit` - edit
- `/review-queue` - flagged duplicate/research queue
- `/share` - exports, imports, merge review, Google tools
- `/insights` - analytics dashboard and duplicate audit trigger
- `/settings` - initialization + image maintenance
- `/setup` - first-run identity configuration

### Templates

- `internal\templates\layout.templ` - shared shell, floating toast region, app chrome
- `internal\templates\calendar.templ` - home page and anniversary calendar
- `internal\templates\entry_form.templ` - create/edit record form, scraper UI, settings view
- `internal\templates\soldier_card.templ` - browse/detail card fragments
- `internal\templates\share.templ` - share/import/export/merge UI
- `internal\templates\insights.templ` - analytics dashboard
- `internal\templates\review_queue.templ` - review queue + duplicate comparison

## Service map

### `internal\services\soldier_service.go`

Primary CRUD and search service.

Responsibilities:

- create/update/delete soldiers
- spouse normalization
- archive counts
- browse and advanced search
- image metadata
- review queue data
- review resolution
- FTS-backed quick search

### `internal\services\backup_service.go`

Handles:

- `.ddbak` replacement backups
- `.ddshare` merge archives
- merge staging
- conflict resolution (`keep-local`, `keep-shared`, `keep-both`)
- human duplicate detection during shared import
- local namespace ID regeneration when needed

### `internal\services\audit_service.go`

Implements the advanced duplicate engine.

Passes:

1. exact human duplicate
2. fuzzy first-name matching within grouped buckets using Levenshtein
3. burial-location / maiden-name heuristics

Persisted findings are stored in `duplicate_audit_findings`, and resolved pairs are suppressed from future re-flagging.

### `internal\services\analytics_service.go`

Builds the Insights dashboard data:

- top cemeteries
- Confederate Home breakdown
- pension distribution
- top units
- birth/death decades
- review/duplicate metrics

### `internal\services\export_service.go`

Handles:

- JSON export
- Excel export
- iCalendar export
- static web archive export
- printable full database PDF
- soldier PDF
- month/calendar PDF
- analytics PDF
- backup/shared archive generation
- bug-report bundle export

### `internal\services\image_service.go`

Recent hardening service.

Responsibilities:

- migrate old image paths into sharded layout
- discover orphan files
- move orphan files into `temp_trash`
- purge expired temp-trash contents

### `internal\services\google_service.go`

Google integration service for:

- account connect/disconnect
- Drive backup upload
- CSV export to Sheets
- calendar sync / unsync

### Supporting services

- `anniversary_service.go`
- `diagnostics_service.go`

## Frontend behavior

`frontend\app.js` is a custom lightweight HTMX-style interaction layer.

It is responsible for:

- handling `hx-*` requests
- preserving scroll position
- redirect+toast behavior
- merge-review jump/restore logic
- floating toast system
- duplicate-audit progress messaging
- tab switching
- image viewer
- scratch pad launcher
- record-row add/remove
- print-config modal
- text context menu
- bulk checkbox helpers

Important recent change:

- the request layer now preserves the actual submitter button when posting forms, which is required for review-queue bulk actions and similar button-driven form behavior.

## Search implementation

Global search no longer relies only on SQL `LIKE`.

Current stack:

- `soldiers_fts` FTS5 virtual table
- trigger-maintained sync from `soldiers`
- `scratchpad_cache` bridges scratch pad text into search
- fallback `LIKE` search remains for some record-detail surfaces
- DB opens with WAL and `synchronous(NORMAL)` for better throughput during heavy indexing/merge work

## Image storage implementation

Current image layout:

- `images\<A>\<B>\<sanitized-display-id>\...`

Examples:

- `images\S\T\STC38-00020\STC38-00020-001.jpg`
- `images\D\X\DXD-00001\portrait.png`

Behavior:

- startup migrates old non-sharded image paths to the new structure
- image DB rows are updated to the new relative path
- orphan scan compares filesystem files to `images.file_path`
- cleanup moves files to `.dixiedata\temp_trash\images\<timestamp>\...`

## Merge and review workflow

### Shared archive import

`/import/shared-archive` stages merge conflicts when:

- the same logical person already exists locally
- a display ID collides
- content matches a human duplicate heuristic

The v1.1 shared-archive format is `manifest.json` + `data\soldiers.json` + referenced images. Newly inserted shared records regenerate into the receiver namespace while preserving sender attribution fields.

### Conflict resolution options

- **Keep Local** - preserve local record
- **Keep Shared** - overwrite local content but keep the local ID
- **Keep Both** - preserve local record and import the shared record under a new local ID

### Review Queue

`/review-queue` shows records where `needs_review = true`.

It supports:

- per-record resolution
- duplicate comparison view
- bulk ignore
- bulk delete

## Duplicate audit

Triggered from `/insights`.

Current logic:

- uses `github.com/agnivade/levenshtein`
- configurable similarity threshold in `system_config`
- flags candidate pairs with descriptive reasons
- avoids rediscovering resolved pairs
- exposes side-by-side compare UI

## Find a Grave scraper

The scraper accepts **raw pasted HTML only**.

Current parser behavior:

- prefers embedded JS memorial values
- falls back to visible label-driven extraction
- computes a confidence score
- surfaces warnings in the entry form
- automatically flags low-confidence scraped records for review

Important note:

- URL scraping was intentionally removed because Find a Grave started returning challenge/403 behavior.

## Static archive export

The static archive now emits:

- `viewer.html`
- `archive_data.js`
- `window.DIXIE_DATA`

For compatibility, `index.html` is also written.

The output is designed to be opened directly in a browser with no app server.

## Build and validation workflow

Known-good commands:

- `templ generate`
- `go test ./...`
- `go build ./...`
- `.\build-release.ps1`
- `.\build-debug.ps1`
- `.\run-debug.ps1`
- `.\run-stress-tests.ps1`

Build scripts preserve `build\bin\google-oauth-defaults.json`.

## Recent work already completed

Recent major additions already in the repository:

- buried-in grouping for printable full archive PDF
- primary image selection for soldier records
- split archive counts for soldiers vs wives/widows
- Insights dashboard and analytics PDF
- merge ID de-recursion and sanitized namespace behavior
- keep-shared merge action
- Review Queue system
- floating toast notifications
- duplicate audit with fuzzy matching
- FTS5 global search
- image sharding and orphan cleanup tools
- schema backup/version tracking
- scraper confidence scoring
- startup auto-refresh loading placeholder

## Known operational cautions

- startup still performs real initialization work such as schema checks, scratchpad sync, and image migration; the difference now is that the UI retries cleanly instead of hanging.
- `entry_form.templ` also contains the Settings view, so changes to settings UI live there rather than in a separate file.
- `templ generate` must be run whenever `.templ` files change.
- the app is Windows-oriented and paths should stay Windows-safe.

## Recommended next places to inspect

If another AI needs to continue work, read in this order:

1. `README.md`
2. `app.go`
3. `internal\db\schema.go`
4. `internal\services\soldier_service.go`
5. `internal\services\backup_service.go`
6. `internal\services\audit_service.go`
7. `internal\services\export_service.go`
8. `internal\services\image_service.go`
9. `internal\templates\share.templ`
10. `internal\templates\entry_form.templ`
11. `internal\templates\review_queue.templ`
12. `frontend\app.js`

## Handoff summary

The repo is in a working state. The recent scale-hardening and UX work has already landed, the startup dead-end was fixed, and the full build/test pipeline passes. A new AI should treat the current storage layout, FTS search stack, duplicate-audit persistence, and merge-review workflow as the canonical behavior.
