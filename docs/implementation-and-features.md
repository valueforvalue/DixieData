# DixieData Implementation and Feature Reference

## 1. Purpose

DixieData is a Wails-based desktop archive for managing Civil War research records. It combines a Go backend, SQLite database, and Templ-rendered UI into a single desktop application focused on:

- soldier and spouse record management
- long-form research notes
- source record capture
- image storage and viewing
- search and analytics
- import/export and merge workflows
- review queue and duplicate auditing

The current release line is **v1.1.16**.

## 2. Technical stack

| Layer | Implementation |
| --- | --- |
| Desktop shell | Wails v2 |
| Backend | Go |
| DB | SQLite |
| UI rendering | Templ |
| Styling | Tailwind via CDN + app-specific CSS in templates |
| Client interactivity | `frontend\app.js` |
| Search | SQLite FTS5 + fallback `LIKE` |
| PDF generation | `github.com/go-pdf/fpdf` |
| Spreadsheet export | `github.com/xuri/excelize/v2` |

## 3. Runtime architecture

### 3.1 App boot

Startup flow:

1. Wails launches `main.go`
2. `NewApp()` constructs the app
3. `App.startup()`:
   - configures logging
   - resolves `.dixiedata`
   - loads quotes
   - opens SQLite
   - applies schema and migrations
   - reloads services
   - ensures route registration
4. `App.ServeHTTP()` handles every app route

The startup placeholder now retries automatically until routes are ready.

### 3.2 Request model

The app uses a custom HTTP-driven interaction pattern:

- templates render full HTML fragments
- buttons/forms carry `hx-*` attributes
- `frontend\app.js` interprets those attributes and performs fetches
- responses are inserted into the DOM without a separate SPA framework

This keeps server-side rendering as the main UI model while still supporting rich desktop interactivity.

The same client layer also preserves navigation context for the app’s Smart Back behavior.

## 4. Data directory layout

Default local data root: `.dixiedata`

Typical layout:

```text
.dixiedata\
  dixiedata.db
  images\
  scratchpads\
  backups\
  logs\
  merge-review\
  temp_trash\
```

### 4.1 Image storage

Images are sharded using a 2-tier path derived from the sanitized display ID:

```text
images\<first-char>\<second-char>\<display-id>\
```

This prevents an oversized single directory at scale.

### 4.2 Scratch pads

Each record may have:

- a `.txt` scratch pad
- a `.json` scratch pad state file

Scratch pad contents are synced into `scratchpad_cache` for FTS indexing.

## 5. Database design

### 5.1 `soldiers`

Main record table containing both:

- soldiers
- wives/widows

Key fields:

- `display_id`
- `entry_type`
- name fields
- rank/unit fields
- pension fields
- burial location
- notes
- review status
- audit metadata

### 5.2 `records`

Stores attached source/reference items per soldier, such as pension or application records.

### 5.3 `images`

Stores image metadata:

- original filename
- relative file path
- caption
- `is_primary`

### 5.4 Merge tables

- `merge_review_sessions`
- `merge_review_conflicts`

These tables support staged shared-archive conflict review.

### 5.5 Duplicate audit table

`duplicate_audit_findings` stores pair-level duplicate candidates and resolution state.

### 5.6 Search support

- `soldiers_fts`
- `scratchpad_cache`

### 5.7 Version tracking

`schema_version` records applied schema version rows, while `PRAGMA user_version` is also updated.

## 6. Core features

## 6.1 Record creation and editing

Users can:

- create soldier records
- create wife/widow records
- assign spouse links
- add notes
- add multiple source records
- manage review flags

The entry form also supports Find a Grave autofill from pasted HTML.

## 6.2 Image management

Implemented features:

- import multiple images
- download selected images
- delete selected images
- choose a **primary image**
- rotate images
- image viewer modal
- screenshot export from image viewer

## 6.3 Scratch Pad

The floating Scratch Pad button launches a per-record scratch pad using the current record ID on supported pages. Scratch pad text is indexed for search.

## 6.4 Search

### Quick search

Global search uses FTS5 over:

- display metadata
- names
- unit/rank/location fields
- notes
- scratch pad text

The UI also shows the matching field/snippet.
The empty-state browse surface can also show recent records for quick reopening.

### Advanced search

Supports filters such as:

- ID
- names
- maiden name
- rank / rank in / rank out
- unit
- record type
- pension state
- Confederate Home fields
- burial location
- review status
- entry type
- birth/death date components

## 6.5 Calendar and anniversary view

The home page is calendar-driven and includes:

- month navigation
- anniversary record listings
- rotating quote panel
- archive counts for soldiers and wives/widows

## 6.6 Share / export / import

`.ddshare` archives are merge-oriented JSON packages containing record payloads plus referenced images, while `.ddbak` archives remain full replacement SQLite snapshots.

The Share page centralizes portability features.

### Supported exports

- JSON
- Excel workbook
- iCalendar
- static web archive
- full database printable PDF
- backup archive (`.ddbak`)
- mergeable shared archive (`.ddshare`)
- bug-report bundle

### Supported imports

- replacement backup import
- shared archive merge import

## 6.7 Printable PDF exports

The app includes branded PDF outputs with consistent header/footer styling.

### Full database printable PDF

Features:

- landscape printable format
- one record page per entry
- sort options:
  - last name
  - birth year
  - death year
- grouping options:
  - unit
  - pension state
  - Confederate Home status
  - burial location (`buried_in`)

Unknown burial locations are grouped at the end.

### Analytics PDF

Exports the Insights dashboard summary as a branded PDF report.

## 6.8 Static archive export

Static export creates a standalone browser-viewable bundle containing:

- `viewer.html`
- `archive_data.js`
- copied images

The data is exposed as `window.DIXIE_DATA`.

## 6.9 Merge workflow

Shared imports can detect:

- sync collisions
- display ID collisions
- human duplicates

### Resolution actions

- **Keep Local**
- **Keep Shared**
- **Keep Both**

`Keep Shared` updates the local record content while preserving the local ID for relational stability.

`Keep Both` assigns a new clean local ID rather than wrapping the imported ID.

## 6.10 Review Queue

Records flagged with `needs_review = true` appear in a dedicated queue.

Capabilities:

- see review reason
- open record
- compare duplicate candidates
- resolve individual entries
- bulk ignore
- bulk delete

## 6.11 Duplicate audit

The Insights page can run an archive-wide duplicate audit.

The engine performs:

1. exact human matching
2. fuzzy first-name matching with Levenshtein
3. location/maiden-name matching

Resolved pairs are remembered so the same pair is not repeatedly re-flagged.

## 6.12 Insights dashboard

Analytics currently include:

- record type counts
- top cemeteries
- Confederate Home status + name counts
- pension distribution
- top units
- birth/death decades
- duplicate audit summary

## 6.13 Find a Grave integration

The app supports HTML-paste autofill from Find a Grave memorial pages.

Current implementation:

- JS object extraction first
- label-near-text fallback parsing second
- warning output
- confidence scoring
- automatic review flagging for low-confidence results

## 6.14 Google integration

The Share page exposes:

- Google account connect/disconnect
- Drive backup upload
- CSV-to-Sheets export
- calendar sync and unsync

## 6.15 Diagnostics

The app can export a bug-report bundle containing local support artifacts for troubleshooting.

## 7. Frontend implementation details

`frontend\app.js` implements:

- fetch-based request handling for `hx-*`
- redirect state persistence
- toast persistence
- smooth merge-review scrolling
- progress UI for long-running actions
- form draft persistence
- modal handling
- image viewer
- text context menu
- bulk-selection helpers

The code is intentionally framework-light and centralizes browser behavior in one file.

## 8. Migration and safety model

### 8.1 Schema upgrades

Before applying a newer schema version, the DB layer now writes a timestamped backup snapshot into:

```text
.dixiedata\backups\schema-migrations\
```

### 8.2 Image path upgrades

At startup, existing image rows are scanned and migrated to the sharded filesystem layout if needed.

### 8.3 Orphan cleanup

The orphan cleanup flow is deliberately non-destructive:

1. scan for files not referenced in the DB
2. present list in Settings
3. move selected files into `temp_trash`
4. purge after retention window

## 9. Testing and validation

Repository validation commands:

- `templ generate`
- `go test ./...`
- `go build ./...`
- `.\build-release.ps1`

There are tests for:

- templates
- services
- DB helpers
- stress workflow
- exports
- duplicate audit
- merge behavior

## 10. Important implementation conventions

- Use Windows-style paths.
- Use `apply_patch` for code edits.
- Regenerate templ output after `.templ` changes.
- Run a full build after code changes.
- Do not assume image storage is flat; it is now sharded.
- Do not assume search is `LIKE`-only; FTS is the primary search path.
- Do not assume shared imports can wrap IDs; namespace recursion is intentionally blocked.

## 11. Feature summary

At a high level, DixieData now provides:

- archive record management
- spouse/widow support
- image import and primary-image selection
- scratch pad support
- fast global search
- advanced structured search
- review queue management
- fuzzy duplicate auditing
- mergeable shared archives
- branded exports and reports
- analytics dashboard
- static archive publishing
- diagnostics and Google integrations

This document is the technical “what exists and how it is built” reference. For operator guidance, use `docs\user-manual.md`.
