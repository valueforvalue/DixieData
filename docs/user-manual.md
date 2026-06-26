# DixieData User Manual

## 1. Introduction

DixieData is a desktop research **Local Archive** for managing Civil War Person Records, Source Records, notes, images, exports, and collaboration files. It is designed for researchers who need a local-first archive with structured Person Records, printable outputs, and merge workflows.

This manual explains how to use the application day to day.

The current release line is **v1.2.55**.

## 1.1 Glossary

DixieData has a strict vocabulary. Throughout this manual:

- **Person Record** — primary archive entry for one person (soldier, wife, widow). Not "record", "soldier record", "archive entry", or "profile".
- **Display ID** — canonical user-facing identifier for a Person Record (e.g. `DXD-00047`). Not "record ID" or "person ID".
- **Local Archive** — live working collection of Person Records, Source Records, notes, images, and review state on one machine.
- **Shared Archive** — merge-oriented archive package exchanged between DixieData users.
- **Backup Archive** — full replacement archive snapshot used for restore and safekeeping.
- **Restore Point** — automatic pre-update recovery bundle that preserves a recoverable Local Archive state and the previously safe app build for rollback.
- **Static Archive** — read-only browser-viewable archive export.
- **Source Record** — attached evidence item that documents or supports a Person Record, such as a pension or application record.
- **Claim** — assertion about a person that is extracted from a Source Record.
- **Finding** — a Claim that has cleared review and is endorsed as a research conclusion.

Full glossary lives in [`CONTEXT.md`](../CONTEXT.md). When the manual says "the archive" without qualification it means the Local Archive.

## 2. First launch

When you open DixieData for the first time, the app asks for your identity details:

- first name
- middle name
- last name
- birth year

These values are used to build your local researcher namespace for generated display IDs.

Example generated IDs:

- `STC38-00001`
- `JCM87-00025`

After setup, the app opens the Local Archive.

## 3. Understanding the main sections

The app is organized around several major pages (each operates on the Local Archive unless otherwise noted):

### Calendar

The landing page shows:

- a monthly anniversary calendar
- the total number of **Soldiers**
- the total number of **Wives & Widows**
- a rotating quote panel

Use this page for quick Local Archive awareness and date-based browsing.

### Browse Person Records

This is the main Local Archive browsing area. From here you can:

- browse person records
- run quick searches
- reopen recently accessed person records before typing a search
- open person records
- access advanced search

### Add Person Record

Use this page to create a new soldier, wife, or widow person record.

### Review Queue

This page shows records flagged for review, including:

- suspected duplicates
- low-confidence scraped records
- other follow-up items

### Share Archive

Use this page for:

- exporting
- importing
- backups
- merge review
- Google integrations

### Insights

This page provides high-level analytics and access to the duplicate audit.

### Settings

Use Settings to:

- initialize/reset local data
- scan for orphaned image files
- move orphaned files into temp trash

## 4. Creating a record

To add a new person record:

1. Open **Add Person Record**
2. Enter the core fields
3. Add notes, source records, and images
4. Save

### Record types

DixieData supports:

- **Soldier**
- **Wife**
- **Widow**

Spouse Person Records are stored in the same Local Archive and can be linked to a Soldier Person Record.

### Common fields

Depending on the record, you may enter:

- display ID
- prefix / first / middle / last / suffix
- maiden name
- rank in / rank out
- unit
- pension ID
- application ID
- pension state
- Confederate Home status and name
- birth date / death date
- birth information
- burial location
- notes

### Source records

You can add multiple source records to one person, such as:

- Pension
- Application
- other source record types

Use the add/remove controls in the form to manage these rows.

## 5. Using Find a Grave autofill

The new-record form includes a **Scrape Find a Grave** section.

### How to use it

1. Open the memorial page in your browser
2. Copy the raw page HTML
3. Paste it into the Find a Grave HTML box
4. Click **Fetch Data**

The form will autofill available fields.

### Important limits

- Use **raw pasted HTML only**
- Direct URL scraping is not supported

### Confidence and warnings

After parsing, DixieData may show:

- warning messages
- spouse memorial Claims (see §1.1 Glossary)
- a confidence score

If the scrape confidence is low, the saved record may automatically enter the Review Queue. Always verify scraped data before saving.

Claims extracted from a Find a Grave scrape become Finding entries only after they clear review. See the Glossary (§1.1) for the Claim → Finding distinction.

## 6. Editing a record

From a record detail page, choose **Edit** to update the record.

Use edit mode to:

- correct names or dates
- update notes
- adjust spouse links
- change review status
- add/remove source records
- manage images

When you use the normal return actions from detail and edit pages, DixieData uses **Smart Back** behavior to bring you back to the right browse or report surface with its earlier filters and scroll position preserved.

## 7. Working with images

Each record can have multiple images.

### Importing images

From a record page:

1. choose image import
2. select one or more files
3. the files are copied into DixieData storage

Stored media now uses a sharded on-disk layout under `.dixiedata\images\<A>\<B>\<display-id>\...` so large archives stay manageable.

### Deleting images

Select one or more stored images and delete them from the record page.

### Choosing a primary image

One image can be marked as the **primary image**. This is the image used where the app needs a single representative record image.

### Rotating images

The image viewer supports clockwise and counterclockwise rotation.

### Downloading images

You can export selected images from a record to a folder outside the app.

## 8. Scratch Pad

The floating Scratch Pad button opens a record-specific scratch pad.

Use it for:

- temporary transcription work
- longer freeform notes
- research staging

Scratch Pad text is searchable through the app’s global search.

## 9. Searching the Local Archive

## 9.1 Quick search

Quick search looks across major record text, including:

- names
- IDs
- rank and unit
- burial location
- notes
- scratch pad content

The result cards show the field that matched.

The Quick Search index uses SQLite **FTS5** plus the Person Record scratch-pad store, so Scratch Pad text participates in Quick Search results. **Advanced Search filters on entry type, review state, and Person Record fields but does not match Scratch Pad content** — use Quick Search when you need to find a scratch note.

## 9.2 Advanced search

Advanced search supports more precise filtering.

Examples:

- search only review-queue records
- filter by unit
- filter by pension state
- filter by burial location
- filter by maiden name
- filter by entry type
- filter by date ranges

### Review status filter

You can limit results to:

- all records
- clean records only
- review queue only

## 10. Review Queue

The Review Queue is where flagged items wait for human review.

### Why a record appears there

Common reasons:

- potential duplicate
- merge conflict follow-up
- low-confidence scraped record

### What you can do

- view the record
- open duplicate comparison
- mark the item resolved
- bulk ignore multiple selected items
- bulk delete multiple selected items

### Bulk delete and Ignore — semantics and recovery

- **Bulk delete** removes the selected Person Records (and any Source Records only they reference) from the Local Archive. **This cannot be undone within the app.** DixieData does not stage deleted records in temp_trash. **Recovery path:** restore the most recent `.ddbak` you exported before the bulk delete (see §18 Backup strategy).
- **Ignore** marks the selected items as reviewed and resolved. The Person Records stay in the Local Archive; they just move out of the Review Queue. **Ignore is not destructive** and does not need a backup.

**Before running bulk delete**, export a `.ddbak` (§13.1) so you can roll back if you selected the wrong rows.

### Duplicate comparison

If DixieData detects a suspected duplicate pair, you can open a side-by-side comparison to inspect the triggering fields.

## 11. Duplicate audit

From **Insights**, click **Audit Now** to run the duplicate scan across the Local Archive.

The audit checks for:

1. exact human duplicates
2. fuzzy name similarities
3. burial/location and maiden-name patterns

When matches are found, DixieData flags the records into the Review Queue.

Resolved pairs stay resolved and are not endlessly re-flagged.

## 12. Insights dashboard

The **Insights** page gives a high-level Local Archive summary.

Available cards include:

- record type snapshot
- top cemeteries
- Confederate Home status breakdown
- most frequent home names
- pension distribution
- top units
- duplicate audit status
- birth and death decade charts

### Export Analytics Report

You can generate a PDF report from the Insights page.

## 13. Share Archive page

The Share page is the archive’s portability and collaboration workspace.

## 13.1 Export options

### Export JSON

Creates a structured full-Local-Archive JSON export (a Backup Archive).

### Export Excel

Creates an `.xlsx` workbook for spreadsheet use.

### Export iCalendar

Creates an `.ics` anniversary calendar export.

### Export Static Web Archive

Creates a standalone browser-viewable archive export (a Static Archive).

### Full Database Printable PDF

Creates a branded printable PDF for the full Local Archive or for the currently selected Person Record set.

Available sort options:

- alphabetical by last name
- chronological by birth year
- chronological by death year

Available grouping options:

- unit
- pension state
- Confederate Home status
- burial location

### Export Backup (`.ddbak`)

Creates a full replacement backup.

### Export Shared Archive (`.ddshare`)

Creates a mergeable archive (a Shared Archive) for another DixieData user.

### Export Bug Report Bundle

Creates a support/troubleshooting bundle.

## 13.2 Import options

### Load Backup (`.ddbak`)

This **replaces** the current Local Archive with the Backup Archive. The current Local Archive is **not** preserved unless you have already exported a fresh `.ddbak`. There is no automatic undo.

**Recovery path:** before loading any backup, export a fresh `.ddbak` from §13.1 (Export options). If the backup you loaded turns out to be wrong, load the fresh `.ddbak` you just exported.

Use this when you want to restore a full Local Archive state.

### Import Shared Archive (`.ddshare`)

This **merges** incoming Person Records from the Shared Archive into the current Local Archive.

Use this for collaboration and record sharing.

## 14. Merge review

If a shared import finds a conflict, DixieData stages the issue in the merge-review area.

You may see actions such as:

- **Keep Local**
- **Keep Incoming**
- **Keep Both**

### What these mean

**Keep Local**

- retains the current local record

**Keep Incoming**

- updates local content using the shared version
- keeps the local display ID

**Keep Both**

- keeps the local record
- imports the shared record as a separate local record with a new local ID

The app scrolls you into the merge-review area and shows a visible “Data Loaded” message when conflicts are ready.

## 15. Google integration

The Share page also includes Google tools.

Depending on your configuration, you can:

- connect a Google account
- upload a backup to Google Drive
- export CSV data to Google Sheets
- sync anniversary events to Google Calendar
- unsync previously created events

## 16. Settings

## 16.1 Initialize Data

This fully rebuilds the local `.dixiedata` workspace.

**This cannot be undone.** Before clicking Initialize:

1. Export a fresh `.ddbak` (§13.1 Export options) and save it somewhere outside `.dixiedata/`.
2. Verify the `.ddbak` file is non-zero in size and you can open it in a file manager.
3. Note your Google sync state (Settings → Google) — it will be cleared and you will need to reconnect afterward.

Use with caution. It removes local:

- Person Records (all subtypes)
- Source Records
- images
- backups
- Google sync state
- scratch pads
- review queue

You must type the confirmation word before proceeding.

**Recovery path:** if you exported a `.ddbak` first, load it via §13.2 Load Backup after Initialize completes. Otherwise the Local Archive is gone.

## 16.2 Image Maintenance

Use this area to:

1. scan for orphaned image files
2. review the orphan list
3. move listed files into temp trash

This cleanup is designed to be safe. Files are staged before permanent removal.

**Retention window:** files moved into temp trash stay in `<data-dir>/temp_trash/images/<timestamp>/` for **30 days**, then are permanently removed by a scheduled cleanup pass. To recover a deleted image within that window, copy the file back to its original location under `<data-dir>/images/` before the 30-day window expires.

**Recovery path:** if a real (non-orphan) image was moved to trash by mistake, copy the file back from `<data-dir>/temp_trash/images/<timestamp>/<file>` to `<data-dir>/images/<record>/<file>` within 30 days. After 30 days the file is permanently deleted.

## 17. Static Archive output

The Static Archive export can be opened in a browser without running DixieData.

It includes:

- `index.html`
- `archive_data.js`
- copied image files

Use it when you want to share a read-only Local Archive snapshot.

## 18. Backup strategy recommendations

Recommended routine:

1. Export a `.ddbak` backup regularly
2. Keep one or more dated copies outside the app folder
3. Use `.ddshare` only for collaboration/merging, not as your only backup

**Before any destructive action** — Initialize Data (§16.1), Load Backup (§13.2), or Bulk Delete on the Review Queue (§10) — export a fresh `.ddbak` first via §13.1 Export options. None of those operations are reversible within the app; only a recent `.ddbak` lets you roll back.

## 19. Troubleshooting

### The app says it is still starting up

The loading screen should now refresh automatically while the app finishes starting. If it takes unusually long:

- close and reopen the app
- verify the local `.dixiedata` folder is accessible
- check for very large migration work or storage issues

### An update failed before the app finished opening

- use the recovery screen to restore the previous build and Local Archive state
- keep the app open until DixieData relaunches itself after the rollback
- if the recovery screen does not appear, restart the app once to re-check the retained restore point

### If an update fails: full recovery walkthrough

When DixieData starts an in-place update, it **first creates a Restore Point**. The Restore Point captures both the previously safe app build and the Local Archive state immediately before the update begins. If the update fails or hangs, the app falls back to the recovery screen on the next launch.

Steps:

1. The recovery screen appears automatically. If it does not, launch DixieData once more to trigger it.
2. Click **Restore previous build and Local Archive**. This rolls back both the app binary and the Local Archive in one step.
3. Keep DixieData open. The app relaunches itself after the rollback completes; do not close it mid-rollback.
4. After the app reopens, verify the version number under **Settings → Version** matches the previously safe release line.

The Restore Point and the previously safe app build are preserved together until the rollback succeeds. If the rollback itself fails, both are still on disk and the next launch will re-prompt the recovery screen. The detailed mechanism is documented in [`docs/implementation-and-features.md` §8.2](../implementation-and-features.md).

### Search is not finding expected scratch pad text

- confirm the record’s scratch pad window was closed after editing so the latest text saved back to the Local Archive
- retry the search after the updated text is indexed

### A record is wrongly flagged as a duplicate

- open the comparison from the Review Queue
- mark the match resolved if it is not a true duplicate

### I imported a Shared Archive and got conflicts

- open the Share page
- review the merge-review items
- resolve each conflict using the provided action buttons

### An image file seems missing

- open the record and verify the image is still attached
- run **Image Maintenance** in Settings to scan for orphan problems

### Debug logging + Debug Console

DixieData emits a single structured log file at `<dataDir>/logs/app.log.jsonl` (one JSON object per line). Every request carries a 16-character `request_id` echoed in the `X-Request-Id` response header so a single operation's log trail can be grepped end-to-end.

**Turn debug mode on**

- Click the 🐞 **Debug** button that appears in the footer when debug mode is off, then **Debug mode** in Settings → the toggle persists in `<dataDir>/local_settings.json` and survives restarts; or
- Launch the app with `DIXIEDATA_DEBUG=1` in the environment (PowerShell: `$env:DIXIEDATA_DEBUG = "1"; make run`; cmd: `set DIXIEDATA_DEBUG=1 && make run`; bash: `DIXIEDATA_DEBUG=1 make run`).

When debug mode is on, the slog level drops to Debug, output also mirrors to stderr, and the 🐞 button opens the **Debug Console** panel showing the most recent 500 in-memory log entries with a level filter, copy, open-folder (reveals `~/.dixiedata/logs/` in the OS file manager), and clear controls. The frontend's own console output, `window.onerror`, and `unhandledrejection` events are batched and posted back to Go every 2 s (or every 50 entries / 32 KB) under `source=frontend`.

**Reading the log**

```bash
# Tail everything at INFO+
tail -f ~/.dixiedata/logs/app.log.jsonl | jq -c 'select(.level != "DEBUG")'

# Trace a single request
grep deadbeef00000001 ~/.dixiedata/logs/app.log.jsonl | jq -c .
```

**Bug reports**

The bug-report zip (Share → Support & Diagnostics → Export bug report) bundles `logs/app.log.jsonl` truncated to the last 1000 lines so operators have recent crash + error context without the file growing unboundedly. `feedback-log.jsonl` is included in full (user-submitted content, separate retention).

## 20. Best practices

- verify Find a Grave autofill before saving
- use the Review Queue regularly
- export backups often
- use shared archives for collaboration
- set a primary image for cleaner exports and display behavior
- use Insights and duplicate audit periodically on large archives

## 20.1 Browse page (`/browse`)

A separate browsing view with paging, sortable columns, and a built-in **compare-selection** mode (check rows, then click **Compare** to open `/compare?id1=...&id2=...`). Useful for medium-to-large Local Archives where the `/soldiers` Quick Search list becomes unwieldy. The mobile breakpoint collapses rows to card view.

## 20.2 Per-Person Record research views

Every Person Record detail page (`/soldiers/{id}`) exposes three research views accessible from the record header:

- **Unit Camaraderie** (`/soldiers/{id}/camaraderie`) — list of soldiers who served in the same unit, with shared service dates. Helps find candidates for cross-references and spouse links.
- **Service Timeline** (`/soldiers/{id}/timeline`) — chronological list of dated events (enlistment, muster, transfer, discharge, pension application). Pulls from structured fields plus any dated notes.
- **Conflict Ledger** (`/soldiers/{id}/conflict-ledger`) — items currently flagged for review against this Person Record (merge conflicts, suspected duplicates, low-confidence scraped data).

## 20.3 Research collections

`/research-collections` lets you group related Person Records into named collections (a regiment, a family, an investigation). Each collection has its own page at `/research-collections/{id}` showing its Person Records and any collection-level notes. Useful for tracking multi-arc investigations.

## 20.4 Research log + research pack

- **Research Log** (`/soldiers/{id}/research-log`) — append-only task list scoped to a Person Record. Create tasks (`Research Task Create`), resolve tasks (`Research Task Resolve`), and the log survives across sessions.
- **Research Pack** (`/soldiers/{id}/research-pack/{state|county}`) — a pre-formatted PDF/HTML bundle (state-level or county-level) you can take into a library or archive. It includes the Person Record fields plus the relevant Source Record excerpts and conflict ledger items.

## 20.5 Recent searches

`/soldiers/search/recent` shows the last 25 Quick Searches you've run this session and across sessions. Click any entry to re-run it.

## 20.6 Per-month anniversary PDF

The Calendar page (`/calendar/`) renders the current month. Each month footer exposes **Export this month as PDF**, which produces a single-page printable of all anniversaries falling in that month (births, deaths, muster dates). Use it for monthly newsletter inserts or just-in-time planning.

## 20.7 Individual Person Record exports

From the Person Record detail page header, you can export:

- **PDF** — single-record printable, same template as the database PDF export but scoped to one Person Record. Useful for archival printouts and sharing one record without exposing the whole Local Archive.
- **JPG** — flattened single-record image suitable for thumbnails, social posts, or quick visual reference.

## 20.8 Memorial JSON import

The Share page exposes a **Memorial JSON Import** flow separate from the Shared Archive path. It accepts a structured JSON document describing one or more Person Records (often exported from a third-party memorial platform) and lets you preview the parsed fields before confirming the import. Routes:

- `POST /import/memorial-json/preview` — parse and show field-by-field preview.
- `POST /import/memorial-json/confirm` — commit the previewed records.

This path does not run the merge-review flow (it is a fresh import, not a merge), so review each preview carefully before confirming.

## 20.9 Data Quality Scan

`/settings/quality/scan` runs a one-pass audit over the Local Archive flagging:

- Person Records with missing required fields (pension state, unit, etc.)
- Source Records pointing to images that no longer exist
- duplicate Display IDs (always a bug; never expected)
- scratch pad notes orphaned by deleted Person Records

After the scan completes, `/settings/quality/apply` lets you apply automated fixes (e.g. normalize pension state values, drop orphan scratch pad entries). The apply step is reversible via the most recent `.ddbak`.

## 20.10 Software Updates panel

`/settings/updates/*` exposes the in-place update flow:

- `POST /settings/updates/source` — switch between release channels (stable, pre-release).
- `POST /settings/updates/check` — fetch the latest available update metadata.
- `POST /settings/updates/apply` — trigger the update. This **creates a Restore Point** (§19 Troubleshooting) before applying.
- `POST /settings/updates/health/bootstrap` — internal heartbeat used by the update handoff. Not user-callable.

If an update fails, the recovery screen (§19) takes over on the next launch.

## 20.11 Global Feedback modal

A floating dock button (bottom-right) opens the **Feedback** modal. You can submit free-form feedback that is appended to a feedback log file in `.dixiedata/feedback.jsonl`. Use **Export Feedback Log** (`/export/feedback-log`) to download the full log for sharing with support or for your own records.

## 20.12 Printable PDF grouping options

The database printable PDF export (Settings → Export → Printable Database PDF) supports three grouping dimensions, selected at export time:

- **Buried-in** — group by cemetery/location. Useful for cemetery-tour planning.
- **Confederate Home** — group by admission state and home name. Useful for home-history research.
- **Pension state** — group by the state that granted the pension. Useful for state-level archival handoffs.

Each grouping produces a separate section in the printed output with a divider page.

## 20.13 Floating dock and server-side routes

The floating dock at the bottom of every page exposes quick links (Calendar, Browse, Review Queue, etc.). Two non-page routes work in the background:

- **`/open-link`** (`handleOpenLink`) — server-side external link launcher. Clicking a `data-open-link` element asks the desktop shell to open the URL in the system browser. Required because the Wails WebView otherwise traps `target="_blank"`.
- **`/scratchpad/open`** (`handleScratchpadOpen`) — server-side scratch pad editor launcher. Pops the record's scratch pad window to the foreground.

These are not pages — they return immediately and trigger desktop-side behavior. They are listed here so future agents do not delete them while cleaning up routes.

## 21. Quick reference

### Best pages for common tasks

| Task | Best page |
| --- | --- |
| Add a new person | Add Person Record (`/soldiers/new`) |
| Search Local Archive text | Browse / Quick Search (`/soldiers`) |
| Structured filtering | Advanced Search (`/soldiers/search/advanced`) |
| Re-run a previous search | Recent Searches (`/soldiers/search/recent`) |
| Page through all records | Browse (`/browse`) |
| Side-by-side record compare | Compare (`/compare`) |
| Review flagged records | Review Queue (`/review-queue`) |
| Bulk ignore / delete queue items | Review Queue Bulk (`/review-queue/bulk`) |
| Merge shared data | Share Archive (`/share`) |
| Preview a memorial JSON import | Share Archive → Memorial JSON Import |
| Run duplicate scan | Insights (`/insights`) |
| Duplicate drilldown | Insights Drilldown (`/insights/drilldown`) |
| Per-Person Record research | `/soldiers/{id}/{camaraderie,timeline,conflict-ledger}` |
| Research collections | `/research-collections` |
| Research log / research pack | `/soldiers/{id}/{research-log,research-pack/{state,county}}` |
| Calendar + anniversaries | Calendar (`/calendar`) |
| Per-month anniversary PDF | Calendar month footer |
| Clean orphaned files | Settings → Image Maintenance |
| Run data quality scan | Settings → Data Quality |
| In-place update | Settings → Updates |
| Initialize local data | Settings → Initialize Data (destructive; see §16.1) |
| Export printable PDF | Share Archive / Insights |
| Export single-record PDF/JPG | Person Record detail header |
| Send feedback | Floating dock → Feedback |
| Export feedback log | `/export/feedback-log` |
| Recovery screen (after failed update) | `/recovery` |

### All registered routes (from `internal/appshell/routes.go`)

Static assets: `/app.js`, `/app.css`, `/htmx.min.js`, `/index.html`, `/media/`

Calendar: `/`, `/calendar`, `/calendar/`, `/anniversary/`

Person Records: `/soldiers`, `/soldiers/{id}`, `/soldiers/display/{display_id}`, `/soldiers/new`, `/soldiers/scrape-findagrave`

Search: `/soldiers/search`, `/soldiers/search/recent`, `/soldiers/search/advanced`

Browse + Compare: `/browse`, `/browse/results`, `/compare`

Review: `/review-queue`, `/review-queue/bulk`, `/review-queue/compare/{id}`, `/merge-review/{id}`

Research: `/soldiers/{id}/camaraderie`, `/soldiers/{id}/timeline`, `/soldiers/{id}/conflict-ledger`, `/soldiers/{id}/research-log`, `/soldiers/{id}/research-pack/{state|county}`, `/research-collections`, `/research-collections/{id}`

Insights: `/insights`, `/insights/drilldown`, `/insights/audit/duplicates`, `/insights/report/pdf`

Share: `/share`, `/import/backup`, `/import/shared-archive`, `/import/memorial-json/preview`, `/import/memorial-json/confirm`

Exports: `/export/json`, `/export/csv`, `/export/ical`, `/export/static-archive`, `/export/database-pdf`, `/export/backup`, `/export/shared-archive`, `/export/bug-report`, `/export/feedback-log`

Settings: `/settings`, `/settings/initialize`, `/settings/updates/source`, `/settings/updates/check`, `/settings/updates/apply`, `/settings/updates/health/bootstrap`, `/settings/images/orphans/scan`, `/settings/images/orphans/cleanup`, `/settings/quality/scan`, `/settings/quality/apply`

Google integrations: `/integrations/google/{connect,disconnect,backup,sheets/export}`, `/integrations/google/calendar/{use-managed,preferences/save,sync-managed,unsync-managed,use-test,sync-test,unsync-test}`

Images: `/images/screenshot`, `/images/rotate`

Background: `/open-link`, `/scratchpad/open`, `/feedback/submit`

Meta: `/version`, `/setup`, `/recovery`

This manual is the operator guide. For implementation details, see `docs\implementation-and-features.md`.
