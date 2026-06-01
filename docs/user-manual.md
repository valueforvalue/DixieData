# DixieData User Manual

## 1. Introduction

DixieData is a desktop research archive for managing Civil War person records, source records, notes, images, exports, and collaboration files. It is designed for researchers who need a local-first archive with structured person records, printable outputs, and merge workflows.

This manual explains how to use the application day to day.

The current release line is **v1.2.37**.

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

After setup, the app opens the main archive.

## 3. Understanding the main sections

The app is organized around several major pages.

### Calendar

The landing page shows:

- a monthly anniversary calendar
- the total number of **Soldiers**
- the total number of **Wives & Widows**
- a rotating quote panel

Use this page for quick archive awareness and date-based browsing.

### Browse Person Records

This is the main archive browsing area. From here you can:

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

Spouse records are stored in the same archive and can be linked to a soldier record.

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
- spouse memorial findings
- a confidence score

If the scrape confidence is low, the saved record may automatically enter the Review Queue. Always verify scraped data before saving.

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

## 9. Searching the archive

## 9.1 Quick search

Quick search looks across major record text, including:

- names
- IDs
- rank and unit
- burial location
- notes
- scratch pad content

The result cards show the field that matched.

The quick-search index uses SQLite **FTS5** plus the person-record scratch-pad store, so scratch-pad text participates in the same local-archive-wide search experience.

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

### Duplicate comparison

If DixieData detects a suspected duplicate pair, you can open a side-by-side comparison to inspect the triggering fields.

## 11. Duplicate audit

From **Insights**, click **Audit Now** to run the duplicate scan across the local archive.

The audit checks for:

1. exact human duplicates
2. fuzzy name similarities
3. burial/location and maiden-name patterns

When matches are found, DixieData flags the records into the Review Queue.

Resolved pairs stay resolved and are not endlessly re-flagged.

## 12. Insights dashboard

The **Insights** page gives a high-level local archive summary.

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

Creates a structured full-archive JSON export.

### Export Excel

Creates an `.xlsx` workbook for spreadsheet use.

### Export iCalendar

Creates an `.ics` anniversary calendar export.

### Export Static Web Archive

Creates a standalone browser-viewable archive export.

### Full Database Printable PDF

Creates a branded printable PDF for the full archive or for the currently selected record set.

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

Creates a mergeable archive for another DixieData user.

### Export Bug Report Bundle

Creates a support/troubleshooting bundle.

## 13.2 Import options

### Load Backup (`.ddbak`)

This **replaces** the current local archive with the backup.

Use this when you want to restore a full archive state.

### Import Shared Archive (`.ddshare`)

This **merges** incoming records into the current local archive.

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

Use with caution. It removes local:

- soldiers
- records
- images
- backups
- Google sync state

You must type the confirmation word before proceeding.

## 16.2 Image Maintenance

Use this area to:

1. scan for orphaned image files
2. review the orphan list
3. move listed files into temp trash

This cleanup is designed to be safe. Files are staged before permanent removal.

## 17. Static archive output

The static archive export can be opened in a browser without running DixieData.

It includes:

- `viewer.html`
- `archive_data.js`
- copied image files

Use it when you want to share a read-only archive snapshot.

## 18. Backup strategy recommendations

Recommended routine:

1. Export a `.ddbak` backup regularly
2. Keep one or more dated copies outside the app folder
3. Use `.ddshare` only for collaboration/merging, not as your only backup

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

### Search is not finding expected scratch pad text

- confirm the record’s scratch pad window was closed after editing so the latest text saved back to the archive
- retry the search after the updated text is indexed

### A record is wrongly flagged as a duplicate

- open the comparison from the Review Queue
- mark the match resolved if it is not a true duplicate

### I imported a shared archive and got conflicts

- open the Share page
- review the merge-review items
- resolve each conflict using the provided action buttons

### An image file seems missing

- open the record and verify the image is still attached
- run **Image Maintenance** in Settings to scan for orphan problems

## 20. Best practices

- verify Find a Grave autofill before saving
- use the Review Queue regularly
- export backups often
- use shared archives for collaboration
- set a primary image for cleaner exports and display behavior
- use Insights and duplicate audit periodically on large archives

## 21. Quick reference

### Best pages for common tasks

| Task | Best page |
| --- | --- |
| Add a new person | Add Person Record |
| Search archive text | Browse / Quick Search |
| Structured filtering | Advanced Search |
| Review flagged records | Review Queue |
| Merge shared data | Share Archive |
| Run duplicate scan | Insights |
| Clean orphaned files | Settings |
| Export a printable report | Share Archive / Insights |

This manual is the operator guide. For implementation details, see `docs\implementation-and-features.md`.
