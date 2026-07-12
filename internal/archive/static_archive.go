// static_archive.go holds StaticArchive* data types, the static-archive
// HTML index template, and the helper functions that build static archive
// exports. Extracted from export_service.go as PR2 of the God-class reduction
// (issue #42). No exported surface beyond the StaticArchive* record types
// (which are exposed for tests and external readers). The *ExportService
// methods StaticArchiveFileName and ExportStaticArchive (in
// export_service.go) call into these helpers but the helpers themselves
// stay package-private.
package archive

import (
	"archive/zip"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/debug"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/peopleinfo"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/records"
)


// --- StaticArchiveRecord/Image/Entry types ---
type StaticArchiveRecord struct {
	DisplayID          string                     `json:"displayId"`
	EntryType          string                     `json:"entryType"`
	DisplayType        string                     `json:"displayType"`
	Name               string                     `json:"name"`
	Dates              string                     `json:"dates"`
	Kind               string                     `json:"kind,omitempty"`
	Description        string                     `json:"description,omitempty"`
	// linkedDisplayIds always serializes (no omitempty) so the
	// JSON bundle emits a uniform shape across linked and
	// unlinked events. newStaticArchiveEventRecord guarantees
	// the slice is non-nil so an empty array renders as `[]`.
	LinkedDisplayIDs   []string                   `json:"linkedDisplayIds"`
	Prefix             string                     `json:"prefix,omitempty"`
	FirstName          string                     `json:"firstName,omitempty"`
	MiddleName         string                     `json:"middleName,omitempty"`
	LastName           string                     `json:"lastName,omitempty"`
	Suffix             string                     `json:"suffix,omitempty"`
	Rank               string                     `json:"rank,omitempty"`
	RankIn             string                     `json:"rankIn,omitempty"`
	RankOut            string                     `json:"rankOut,omitempty"`
	Unit               string                     `json:"unit,omitempty"`
	Location           string                     `json:"location,omitempty"`
	BirthDate          string                     `json:"birthDate,omitempty"`
	DeathDate          string                     `json:"deathDate,omitempty"`
	BirthInfo          string                     `json:"birthInfo,omitempty"`
	Biography          string                     `json:"biography,omitempty"`
	Notes              string                     `json:"notes,omitempty"`
	MaidenName         string                     `json:"maidenName,omitempty"`
	RelationshipLabel  string                     `json:"relationshipLabel,omitempty"`
	SpouseName         string                     `json:"spouseName,omitempty"`
	SpouseDisplayID    string                     `json:"spouseDisplayId,omitempty"`
	PensionID          string                     `json:"pensionId,omitempty"`
	AppID              string                     `json:"appId,omitempty"`
	PensionState       string                     `json:"pensionState,omitempty"`
	HomeStatus         string                     `json:"homeStatus,omitempty"`
	HomeName           string                     `json:"homeName,omitempty"`
	Tags               []string                   `json:"tags,omitempty"`
	NeedsReview        bool                       `json:"needsReview,omitempty"`
	ReviewReason       string                     `json:"reviewReason,omitempty"`
	AddedBy            string                     `json:"addedBy,omitempty"`
	LastEditedBy       string                     `json:"lastEditedBy,omitempty"`
	LastEditedAt       string                     `json:"lastEditedAt,omitempty"`
	LastEditedFields   string                     `json:"lastEditedFields,omitempty"`
	ImagePath          string                     `json:"imagePath,omitempty"`
	Images             []StaticArchiveImage       `json:"images,omitempty"`
	Records            []StaticArchiveRecordEntry `json:"records,omitempty"`
	// Article-only fields (issue #321 slice 5.3). Title +
	// Subtitle + BodyHTML are the headline Article shape; the
	// JS index renders an Articles tab using these fields.
	// The other fields are omitempty because Person + Event
	// records don't carry them.
	Title         string                    `json:"title,omitempty"`
	Subtitle      string                    `json:"subtitle,omitempty"`
	BodyHTML      string                    `json:"bodyHtml,omitempty"`
	ResolvedRefs  []StaticArchiveArticleRef `json:"resolvedRefs,omitempty"`
	CreatedAt     string                    `json:"createdAt,omitempty"`
	UpdatedAt     string                    `json:"updatedAt,omitempty"`
}

// StaticArchiveArticleRef is the per-token projection for the
// in-body markdown link parser (issue #321 slice 5.3). Mirrors
// the LinkedDisplayIDs string slice for events; carries the
// resolved flag so the JS index can render the fail-loud
// "Unknown" marker per locked decision #6.
type StaticArchiveArticleRef struct {
	DisplayID string `json:"displayId"`
	Name      string `json:"name"`
	Resolved  bool   `json:"resolved"`
}

// StaticArchiveImage is an archive-layer type.
type StaticArchiveImage struct {
	FileName string `json:"fileName"`
	Caption  string `json:"caption,omitempty"`
	FilePath string `json:"filePath"`
}

// StaticArchiveRecordEntry is an archive-layer type.
type StaticArchiveRecordEntry struct {
	RecordType string `json:"recordType,omitempty"`
	AppID      string `json:"appId,omitempty"`
	Details    string `json:"details,omitempty"`
}


// --- StaticArchiveCalendar types (issue #498 slice 1) ---
//
// Per locked decision 1 the static archive ships all 12 months
// even when empty so the Calendar landing page renders a full
// wall-calendar. Each month is a single StaticArchiveCalendarMonth
// carrying the per-day rollup the live calendar grid uses:
// AnniversaryCount (death-day anniversaries from soldiers),
// EventCount (calendar_items of type=event), HolidayCount
// (calendar_items of type=holiday). The JS index builds the
// month grid client-side from this snapshot.
//
// StaticArchiveCalendarDay keeps the omitempty semantics on
// counts so empty days render compactly in the JSON bundle
// (`{"h":0,"e":0,"a":0}` -> no, we keep the field names
// short and human-debuggable — see struct definition).
type StaticArchiveCalendarDay struct {
	AnniversaryCount int `json:"a"`
	EventCount       int `json:"e"`
	HolidayCount     int `json:"h"`
}

// StaticArchiveCalendarMonth is one month's grid: the month
// number plus a map keyed by day-of-month (1-31) to the
// per-day rollup. The map serializes as a JSON object so the
// JS can index it in O(1) when rendering day cells.
type StaticArchiveCalendarMonth struct {
	Month int                              `json:"month"`
	Days  map[int]StaticArchiveCalendarDay `json:"days"`
}

// StaticArchiveCalendar is the calendar snapshot the bundle
// carries: a slice of 12 StaticArchiveCalendarMonth entries
// (January first). Per locked decision 1 the archive always
// ships all 12 months so the JS grid renders a full year;
// the JS simply hides months with zero markers.
type StaticArchiveCalendar []StaticArchiveCalendarMonth

// StaticArchiveCalendarItem is a single calendar_items row
// exported into the archive bundle (issue #502). Each item
// carries its canonical fields: item_type (holiday/anniversary/
// event), month/day, title, and optional notes.
type StaticArchiveCalendarItem struct {
	ID        int    `json:"id"`
	ItemType  string `json:"itemType"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	Title     string `json:"title"`
	Notes     string `json:"notes,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}


// --- staticArchiveOwner/IndexData types ---
type staticArchiveOwner struct {
	DisplayName string
	FileStem    string
}

type staticArchiveIndexData struct {
	ArchiveTitle  string
	OwnerShort    string
	Version       string
	Build         string
	GeneratedAt   string
	// FileStemJS + GeneratedAtJS are JS-string-literal-safe
	// versions of FileStem and GeneratedAt for the per-archive
	// localStorage key. See issue #475 — the static archive's
	// theme pick is scoped per archive so switching themes in
	// archive X does not bleed into archive Y.
	FileStemJS    string
	GeneratedAtJS string
	// Issue #509: constants exposed to the printable-report
	// renderer so the print chrome (header archive title,
	// footer "Made with DixieData" text, italic codename)
	// matches what the live PDF export emits.
	ArchiveTitleJS string
	FooterTextJS   string
	CodenameJS     string
}


// --- staticArchiveIndexHTML template ---
const staticArchiveIndexHTML = `<!DOCTYPE html>
<html lang="en" data-theme="soft">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .ArchiveTitle }}</title>
  <!-- Issue #494: the static archive now ships with the Soft theme
       hardcoded (matches the desktop app's new default for fresh
       installs). The archive is a snapshot bundle — no theme picker,
       no per-archive localStorage key, no theme switching. -->
  <script defer src="./archive_data.js"></script>
  <style>
    :root {
      color-scheme: light;
      --paper: #f4ecd8;
      --panel: #ede2c5;
      --panel-strong: #f4ecd8;
      --panel-dark: #3b2a1a;
      --border: #b08f45;
      --gold: #8d7440;
      --gold-dark: #5a4220;
      --ink: #3b2a1a;
      --muted: #5a4220;
      /* Issue #511: --accent was referenced by .export-report-button
         and several other controls but was never declared, so it
         resolved to the browser default (transparent / system color)
         and the button rendered with white text on no visible fill.
         Declared here so all --accent usages resolve to the gold
         accent + readable ink-on-gold contrast. */
      --accent: #8d7440;
      --accent-dark: #5a4220;
      --accent-soft: rgba(141, 116, 64, 0.18);
      --shadow: 0 16px 32px rgba(23, 33, 43, 0.16);
    }

    * {
      box-sizing: border-box;
    }

    body {
      margin: 0;
      min-height: 100vh;
      font-family: "Helvetica Neue", Arial, sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at top left, rgba(255,255,255,0.7), transparent 26%),
        radial-gradient(circle at top right, rgba(255,255,255,0.28), transparent 18%),
        repeating-linear-gradient(135deg, rgba(34,45,57,0.025) 0, rgba(34,45,57,0.025) 6px, transparent 6px, transparent 18px),
        linear-gradient(180deg, var(--paper) 0%, #c9c2b5 42%, #b9b1a3 100%);
    }

    .shell {
      max-width: 1280px;
      margin: 0 auto;
      padding: 0 20px 32px;
    }

    .hero {
      margin: 18px 0 16px;
    }

    .hero-shell {
      display: grid;
      gap: 10px;
      border: 1px solid var(--border);
      border-radius: 24px;
      padding: 16px 18px;
      background: var(--panel-dark);
      color: #f4ead0;
      box-shadow: 0 14px 28px rgba(21, 29, 38, 0.16);
    }

    .hero h1 {
      margin: 0;
      color: #cfb77a;
      font-family: Georgia, "Times New Roman", serif;
      font-size: clamp(1.45rem, 2.8vw, 2.2rem);
      line-height: 1.15;
    }

    .hero p {
      margin: 0;
      max-width: 64rem;
      color: rgba(244, 234, 208, 0.82);
      font-size: 0.95rem;
      line-height: 1.45;
    }

    .search-row {
      display: grid;
      gap: 12px;
    }

    .search-row label {
      font-size: 0.75rem;
      font-weight: 700;
      letter-spacing: 0.18em;
      text-transform: uppercase;
      color: #cfb77a;
    }

    .search-row input {
      width: 100%;
      border-radius: 18px;
      border: 1px solid rgba(141, 116, 64, 0.8);
      background: rgba(245, 242, 236, 0.96);
      padding: 14px 16px;
      font-size: 1rem;
      color: var(--ink);
    }

    .search-row input:focus {
      outline: none;
      border-color: var(--gold);
      box-shadow: 0 0 0 3px rgba(168, 138, 70, 0.2);
    }

    .archive-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 12px 20px;
      font-size: 0.9rem;
      color: rgba(244, 234, 208, 0.8);
    }

    /* Issue #494: theme picker CSS removed — archive is Soft-only. */

    /* Issue #490: tab bar for Persons / Events / Articles. */
    .tab-bar {
      display: flex;
      gap: 4px;
      margin-top: 16px;
      padding: 4px;
      background: rgba(0, 0, 0, 0.15);
      border-radius: 12px;
    }
    .tab-button {
      padding: 8px 20px;
      border: none;
      border-radius: 8px;
      background: transparent;
      color: rgba(244, 234, 208, 0.7);
      font-size: 0.9rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.15s, color 0.15s;
    }
    .tab-button:hover {
      background: rgba(255, 255, 255, 0.08);
      color: rgba(244, 234, 208, 0.9);
    }
    .tab-button.active {
      background: var(--accent);
      color: #fff;
    }
    .tab-button.hidden {
      display: none;
    }

    /* Issue #498 slice 2: small fixed nav menu (Calendar / Filter /
       Insights / View All / Events / Articles). Replaces the
       legacy three-tab segmented control. Each link is a hash-route;
       the JS toggles .active based on the current hash. Empty-entity
       links (Events, Articles) are hidden by the JS when the bundle
       has zero rows. */
    .nav-menu {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-top: 16px;
      padding: 6px;
      background: rgba(0, 0, 0, 0.18);
      border-radius: 14px;
    }
    .nav-link {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 16px;
      border-radius: 10px;
      background: transparent;
      color: rgba(244, 234, 208, 0.78);
      font-size: 0.88rem;
      font-weight: 600;
      text-decoration: none;
      cursor: pointer;
      transition: background 0.15s, color 0.15s;
    }
    .nav-link:hover {
      background: rgba(255, 255, 255, 0.08);
      color: rgba(244, 234, 208, 0.95);
    }
    .nav-link.active {
      background: linear-gradient(180deg, #c5ab68 0%, #a5853f 100%);
      color: #1f2b38;
    }
    .nav-link.hidden {
      display: none;
    }

    /* Issue #498 slice 2: Calendar landing page grid. Mirrors the live
       /calendar page's 7-column weekday-header + day-cell layout but
       inlined so the archive stays a single self-contained file. */
    .calendar-grid {
      display: grid;
      grid-template-columns: repeat(7, minmax(0, 1fr));
      border-top: 1px solid rgba(141, 116, 64, 0.28);
    }
    .calendar-weekday {
      padding: 10px 8px;
      text-align: center;
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--muted);
      background: rgba(36, 48, 61, 0.06);
      border-right: 1px solid rgba(141, 116, 64, 0.18);
      border-bottom: 1px solid rgba(141, 116, 64, 0.28);
    }
    .calendar-weekday:last-child {
      border-right: none;
    }
    .calendar-day {
      position: relative;
      min-height: 86px;
      padding: 8px 10px;
      background: rgba(255, 251, 241, 0.78);
      border-right: 1px solid rgba(141, 116, 64, 0.18);
      border-bottom: 1px solid rgba(141, 116, 64, 0.18);
      cursor: pointer;
      text-align: left;
      font: inherit;
      color: inherit;
      transition: background 0.12s;
    }
    .calendar-day:hover {
      background: rgba(255, 247, 231, 0.96);
    }
    .calendar-day.empty {
      cursor: default;
    }
    .calendar-day-number {
      font-size: 0.95rem;
      font-weight: 700;
      color: var(--ink);
    }
    .calendar-day.empty .calendar-day-number {
      color: var(--muted);
    }
    .calendar-day-markers {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      margin-top: 6px;
    }
    .calendar-day-marker {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 2px 7px;
      border-radius: 999px;
      font-size: 0.66rem;
      font-weight: 700;
      letter-spacing: 0.06em;
      text-transform: uppercase;
    }
    .calendar-day-marker.anniversary {
      background: rgba(197, 171, 104, 0.22);
      border: 1px solid rgba(141, 116, 64, 0.55);
      color: var(--ink);
    }
    .calendar-day-marker.event {
      background: rgba(124, 179, 226, 0.28);
      border: 1px solid rgba(80, 130, 180, 0.55);
      color: #1f2b38;
    }
    .calendar-day-marker.holiday {
      background: rgba(217, 137, 137, 0.28);
      border: 1px solid rgba(180, 90, 90, 0.55);
      color: #1f2b38;
    }
    .calendar-month-block {
      border-bottom: 1px solid rgba(141, 116, 64, 0.28);
      padding: 16px 18px 22px;
    }
    .calendar-month-head {
      display: flex;
      align-items: baseline;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 10px;
    }
    .calendar-month-head h3 {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.25rem;
      color: var(--gold-dark);
    }
    .calendar-legend {
      display: flex;
      gap: 12px;
      font-size: 0.72rem;
      color: var(--muted);
      letter-spacing: 0.06em;
    }
    .calendar-legend span {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .calendar-legend i {
      display: inline-block;
      width: 10px;
      height: 10px;
      border-radius: 999px;
      border: 1px solid rgba(141, 116, 64, 0.55);
    }
    .calendar-selector {
      display: flex;
      align-items: center;
      gap: 10px;
      margin: 6px 0 4px;
    }
    .calendar-nav-btn {
      border-radius: 12px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(255, 251, 241, 0.7);
      color: var(--ink);
      font-weight: 700;
      font-size: 1.1rem;
      padding: 6px 14px;
      cursor: pointer;
      transition: background 0.12s;
    }
    .calendar-nav-btn:hover {
      background: rgba(255, 247, 231, 0.96);
    }
    .calendar-month-select {
      border-radius: 12px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(245, 242, 236, 0.96);
      padding: 7px 12px;
      font-size: 0.95rem;
      color: var(--ink);
      min-width: 160px;
    }
    .calendar-items-link {
      margin: 8px 0 4px;
      font-size: 0.9rem;
    }
    .action-link {
      color: var(--accent);
      font-weight: 700;
      text-decoration: underline;
    }
    .ci-table-wrap {
      overflow-x: auto;
    }
    .ci-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.92rem;
    }
    .ci-table th {
      text-align: left;
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--muted);
      padding: 8px 12px;
      border-bottom: 2px solid rgba(141, 116, 64, 0.28);
    }
    .ci-table td {
      padding: 8px 12px;
      border-bottom: 1px solid rgba(141, 116, 64, 0.12);
      color: var(--ink);
    }
    .ci-date { white-space: nowrap; font-weight: 600; }
    .ci-type .pill {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 999px;
      font-size: 0.72rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .ci-holiday { background: rgba(217, 137, 137, 0.22); border: 1px solid rgba(180, 90, 90, 0.55); color: #1f2b38; }
    .ci-anniversary { background: rgba(197, 171, 104, 0.22); border: 1px solid rgba(141, 116, 64, 0.55); color: #1f2b38; }
    .ci-event { background: rgba(124, 179, 226, 0.28); border: 1px solid rgba(80, 130, 180, 0.55); color: #1f2b38; }

    /* Issue #498 slice 3/4: Browse + Insights cards. Defined here so
       the page renderers can use them even before slices 3 + 4 land
       (the JS stub renders them as "coming soon" placeholders). */
    .page-screen {
      padding: 22px;
    }
    .placeholder-card {
      padding: 22px;
      border-radius: 22px;
      border: 1px dashed rgba(141, 116, 64, 0.55);
      background: rgba(255, 251, 241, 0.58);
      color: var(--muted);
      text-align: center;
      font-size: 0.95rem;
    }

    /* Issue #498 slice 3: Browse toolbar + filter chips +
       pagination + prefill banner. The toolbar arranges the
       search input, sort selector, and page-size selector in a
       grid that collapses on narrow screens. */
    .browse-toolbar {
      display: grid;
      grid-template-columns: minmax(0, 1.5fr) minmax(160px, 0.7fr) minmax(120px, 0.5fr);
      gap: 14px;
      margin: 14px 0 16px;
      padding: 14px 16px;
      border-radius: 18px;
      background: rgba(255, 251, 241, 0.62);
      border: 1px solid rgba(141, 116, 64, 0.28);
    }
    .browse-toolbar label {
      display: block;
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: 6px;
    }
    .browse-toolbar input,
    .browse-toolbar select {
      width: 100%;
      border-radius: 12px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(245, 242, 236, 0.96);
      padding: 9px 12px;
      font-size: 0.95rem;
      color: var(--ink);
    }
    .filter-dropdowns {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
      gap: 12px;
      margin-bottom: 14px;
    }
    .filter-dropdown {
      display: flex;
      flex-direction: column;
      gap: 4px;
    }
    .filter-dropdown-label {
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--gold-dark);
    }
    .filter-select {
      width: 100%;
      min-height: 120px;
      border-radius: 12px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(245, 242, 236, 0.96);
      padding: 6px 8px;
      font-size: 0.85rem;
      color: var(--ink);
    }
    .browse-active-filters {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 6px;
      margin-bottom: 10px;
      min-height: 30px;
    }
    .filter-active-label {
      font-size: 0.75rem;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.14em;
      color: var(--muted);
      margin-right: 4px;
    }
    .filter-removable-chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      border-radius: 999px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(197, 171, 104, 0.22);
      color: var(--ink);
      padding: 3px 10px;
      font-size: 0.78rem;
      font-weight: 600;
    }
    .filter-remove-btn {
      background: none;
      border: none;
      cursor: pointer;
      color: var(--muted);
      font-size: 0.85rem;
      padding: 0;
      line-height: 1;
      margin-left: 2px;
    }
    .filter-remove-btn:hover {
      color: var(--accent);
    }
    .browse-clear-row {
      margin-bottom: 8px;
    }
    .browse-prefilter-banner {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      gap: 8px;
      margin-bottom: 12px;
      padding: 10px 14px;
      border-radius: 14px;
      border: 1px solid rgba(141, 116, 64, 0.45);
      background: rgba(197, 171, 104, 0.16);
      font-size: 0.9rem;
      color: var(--ink);
    }
    .browse-prefilter-banner.hidden {
      display: none;
    }
    .browse-pagination {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 14px;
      margin: 16px 0 6px;
      padding: 10px 14px;
      border-radius: 14px;
      background: rgba(255, 251, 241, 0.62);
      border: 1px solid rgba(141, 116, 64, 0.22);
    }
    .browse-pagination .action-button:disabled {
      opacity: 0.45;
      cursor: not-allowed;
    }
    .browse-page-indicator {
      font-size: 0.85rem;
      color: var(--muted);
    }
    .browse-count {
      font-size: 0.85rem;
      color: var(--muted);
      font-weight: 500;
    }
    @media (max-width: 720px) {
      .browse-toolbar { grid-template-columns: 1fr; }
      .filter-dropdowns { grid-template-columns: 1fr; }
    }

    /* Issue #498 slice 4: Insights page cards. The grid lays out
       the 7 spec cards (record_types + 5 count cards + 2 decade
       cards) in a 2-column layout that collapses to single column
       on narrow screens. Each card's per-row entry links to the
       Browse page with a hash-routed filter pre-fill per locked
       decision 3. */
    .insights-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
      gap: 14px;
    }
    .insight-card {
      border-radius: 18px;
      border: 1px solid rgba(141, 116, 64, 0.32);
      background: rgba(255, 251, 241, 0.78);
      padding: 16px 18px;
    }
    .insight-card h3 {
      margin: 0 0 12px;
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.05rem;
      color: var(--gold-dark);
    }
    .insight-card ul {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: 6px;
      font-size: 0.92rem;
    }
    .insight-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.9rem;
    }
    .insight-table td {
      padding: 4px 6px;
      border-bottom: 1px solid rgba(141, 116, 64, 0.14);
    }
    .insight-table td.insight-count {
      text-align: right;
      color: var(--muted);
      font-variant-numeric: tabular-nums;
      width: 50px;
    }
    .insight-table tr:last-child td { border-bottom: none; }
    .insight-empty {
      margin: 0;
      color: var(--muted);
      font-style: italic;
      font-size: 0.9rem;
    }
    .insight-decade-chart {
      display: grid;
      gap: 4px;
      font-size: 0.84rem;
    }
    .insight-decade-bar {
      display: grid;
      grid-template-columns: 70px 1fr 50px;
      align-items: center;
      gap: 8px;
    }
    .insight-decade-label {
      color: var(--muted);
      font-variant-numeric: tabular-nums;
    }
    .insight-decade-track {
      height: 10px;
      border-radius: 5px;
      background: rgba(141, 116, 64, 0.18);
      overflow: hidden;
    }
    .insight-decade-fill {
      display: block;
      height: 100%;
      background: linear-gradient(90deg, #c5ab68 0%, #8d7440 100%);
    }
    .insight-decade-count {
      text-align: right;
      color: var(--ink);
      font-variant-numeric: tabular-nums;
    }

    .article-body {
      line-height: 1.7;
    }
    .article-body a.record-link {
      color: var(--accent);
    }

    .screen {
      border: 1px solid var(--border);
      border-radius: 30px;
      background: var(--panel);
      box-shadow: var(--shadow);
    }

    .screen.hidden {
      display: none;
    }

    .panel-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      padding: 20px 22px 0;
    }

    .panel-head h2 {
      margin: 0;
      color: var(--gold);
      font-family: Georgia, "Times New Roman", serif;
      font-size: 1.45rem;
    }

    .panel-subtext {
      margin: 6px 22px 0;
      color: var(--muted);
      font-size: 0.95rem;
    }

    .list-screen {
      overflow: hidden;
    }

    .results {
      display: grid;
      gap: 12px;
      padding: 18px 20px 20px;
    }

    .record-row {
      display: grid;
      gap: 14px;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: center;
      border: 1px solid rgba(141, 116, 64, 0.38);
      border-radius: 22px;
      padding: 16px 18px;
      background: rgba(255, 251, 241, 0.82);
      transition: transform 120ms ease, box-shadow 120ms ease, border-color 120ms ease;
    }

    .record-row:hover,
    .record-row.active {
      transform: translateY(-1px);
      border-color: rgba(141, 116, 64, 0.72);
      box-shadow: 0 12px 24px rgba(23, 33, 43, 0.12);
    }

    .row-main {
      display: grid;
      gap: 8px;
      min-width: 0;
    }

    .row-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }

    .pill {
      display: inline-flex;
      align-items: center;
      border-radius: 999px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(36, 48, 61, 0.08);
      padding: 6px 10px;
      color: var(--ink);
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    .row-title {
      margin: 0;
      font-size: 1.12rem;
      line-height: 1.35;
      overflow-wrap: anywhere;
    }

    .row-summary {
      display: flex;
      flex-wrap: wrap;
      gap: 12px 18px;
      color: var(--muted);
      font-size: 0.94rem;
    }

    .row-summary span strong {
      color: var(--ink);
    }

    .row-excerpt {
      color: var(--muted);
      font-size: 0.93rem;
      line-height: 1.5;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .action-button,
    .image-button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: 999px;
      padding: 10px 14px;
      font-size: 0.82rem;
      font-weight: 700;
      border: 1px solid var(--gold-dark);
      cursor: pointer;
      text-decoration: none;
    }

    .action-button {
      background: linear-gradient(180deg, #c5ab68 0%, #a5853f 100%);
      color: #1f2b38;
      white-space: nowrap;
    }

    .action-button:hover {
      background: linear-gradient(180deg, #d1b676 0%, #b08f45 100%);
    }

    .image-button {
      background: rgba(246, 241, 228, 0.92);
      color: var(--ink);
    }

    .image-button:hover {
      background: rgba(255, 247, 231, 0.98);
    }

    .empty-state {
      display: none;
      margin: 0 20px 20px;
      padding: 24px;
      border-radius: 22px;
      border: 1px dashed rgba(141, 116, 64, 0.5);
      color: var(--muted);
      text-align: center;
      background: rgba(255, 251, 241, 0.58);
    }

    .detail-screen {
      padding: 22px;
    }

    .detail-toolbar {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;
      margin-bottom: 18px;
    }

    .back-button,
    .image-button,
    .overlay-close {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: 999px;
      padding: 10px 14px;
      font-size: 0.82rem;
      font-weight: 700;
      border: 1px solid var(--gold-dark);
      cursor: pointer;
      text-decoration: none;
      background: rgba(246, 241, 228, 0.92);
      color: var(--ink);
    }

    .back-button:hover,
    .image-button:hover,
    .overlay-close:hover {
      background: rgba(255, 247, 231, 0.98);
    }

    .export-report-button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      border-radius: 999px;
      padding: 10px 18px;
      font-size: 0.82rem;
      font-weight: 700;
      border: 2px solid var(--accent);
      cursor: pointer;
      text-decoration: none;
      /* Issue #511: explicit resting-state fill + border + text so
         the button reads as a button at rest (not white-on-white).
         The filled-gold treatment matches the .back-button /
         .image-button style used by the rest of the toolbar. */
      background: var(--accent);
      color: #ffffff;
      box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.08);
      transition: background 0.12s, border-color 0.12s;
    }
    .export-report-button:hover {
      background: var(--accent-dark);
      border-color: var(--accent-dark);
    }
    .export-report-button:focus-visible {
      outline: 2px solid var(--accent-dark);
      outline-offset: 2px;
    }
    .export-report-tip {
      font-size: 0.72rem;
      color: var(--muted);
      margin-left: 4px;
    }

    .detail-card {
      border: 1px solid rgba(141, 116, 64, 0.4);
      border-radius: 28px;
      background: var(--panel-strong);
      padding: 24px;
      overflow-wrap: anywhere;
    }

    .detail-header {
      display: grid;
      gap: 10px;
      padding-bottom: 16px;
      border-bottom: 1px solid rgba(141, 116, 64, 0.24);
    }

    .detail-header h3 {
      margin: 0;
      font-size: 1.5rem;
      line-height: 1.3;
    }

    .detail-grid {
      display: grid;
      grid-template-columns: auto 1fr;
      gap: 10px 12px;
      margin-top: 18px;
      font-size: 0.94rem;
    }

    .detail-grid dt {
      color: var(--muted);
      font-weight: 600;
    }

    .detail-grid dd {
      margin: 0;
    }

    .detail-section {
      margin-top: 20px;
    }

    .detail-section h4 {
      margin: 0 0 8px;
      font-size: 0.78rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--gold-dark);
    }

    .detail-section p,
    .detail-section li {
      margin: 0;
      color: var(--muted);
      line-height: 1.6;
      white-space: pre-wrap;
    }

    .detail-section ul {
      margin: 0;
      padding-left: 18px;
      display: grid;
      gap: 10px;
    }

    .detail-layout {
      display: grid;
      gap: 18px;
      grid-template-columns: minmax(0, 1.15fr) minmax(280px, 0.85fr);
      align-items: start;
    }

    .image-list {
      display: grid;
      gap: 10px;
    }

    .image-row {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 12px;
      padding: 12px 14px;
      border-radius: 18px;
      background: rgba(255, 251, 241, 0.72);
      border: 1px solid rgba(141, 116, 64, 0.24);
    }

    .image-caption {
      min-width: 0;
      display: grid;
      gap: 4px;
    }

    .image-caption strong,
    .record-link {
      color: var(--ink);
    }

    .record-link {
      word-break: break-word;
      text-decoration: underline;
      text-underline-offset: 2px;
    }

    .related-links {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 10px;
    }

    .related-list {
      display: grid;
      gap: 10px;
      margin-top: 10px;
    }

    .related-card {
      border: 1px solid rgba(141, 116, 64, 0.24);
      border-radius: 18px;
      background: rgba(255, 251, 241, 0.72);
      padding: 12px 14px;
    }

    .related-card strong {
      color: var(--ink);
    }

    .detail-grid.compact {
      margin-top: 10px;
      gap: 8px 10px;
      font-size: 0.9rem;
    }

    .image-overlay {
      position: fixed;
      inset: 0;
      z-index: 40;
      display: none;
      align-items: center;
      justify-content: center;
      padding: 20px;
      background: rgba(23, 33, 43, 0.78);
      backdrop-filter: blur(6px);
    }

    .image-overlay.open {
      display: flex;
    }

    .image-preview-card {
      max-width: min(1100px, 100%);
      max-height: 100%;
      display: grid;
      gap: 12px;
      padding: 18px;
      border-radius: 26px;
      background: rgba(255, 251, 241, 0.98);
      border: 1px solid rgba(141, 116, 64, 0.48);
      box-shadow: 0 24px 48px rgba(23, 33, 43, 0.3);
    }

    .image-preview-stage {
      position: relative;
      display: grid;
      place-items: center;
      min-height: min(72vh, 720px);
      max-height: 72vh;
      overflow: hidden;
      border-radius: 18px;
      background: rgba(34, 48, 61, 0.08);
      cursor: grab;
      touch-action: none;
    }

    .image-preview-stage.dragging {
      cursor: grabbing;
    }

    .image-preview-card img {
      max-width: min(1000px, 100%);
      max-height: 72vh;
      object-fit: contain;
      user-select: none;
      -webkit-user-drag: none;
      transform-origin: center center;
      will-change: transform;
    }

    .overlay-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }

    .overlay-close {
      border: 1px solid var(--gold-dark);
      border-radius: 999px;
      padding: 8px 12px;
      background: rgba(246, 241, 228, 0.92);
      color: var(--ink);
      cursor: pointer;
      font-weight: 700;
    }

    footer {
      margin-top: 28px;
      padding-top: 18px;
      border-top: 1px solid rgba(141, 116, 64, 0.18);
      color: var(--muted);
      font-size: 0.88rem;
      text-align: center;
    }

    @media (max-width: 980px) {
      .detail-layout {
        grid-template-columns: 1fr;
      }
    }

    @media (max-width: 640px) {
      .shell {
        padding: 0 14px 24px;
      }

      .hero {
        margin: 14px 0 16px;
      }

      .hero-shell {
        padding: 14px 16px;
      }

      .record-row {
        grid-template-columns: 1fr;
      }

      .action-button {
        width: 100%;
      }

      .detail-card {
        padding: 18px;
      }

      .image-row {
        flex-direction: column;
        align-items: stretch;
      }
    }

    /* ============================================================
       Issue #509: Printable report styles. The viewer renders a
       per-Person-Record printable view at #/print/{displayId} on
       click of the Export Report button; this stylesheet makes that
       view produce a clean browser printout that mirrors the live
       app's Typst PDF as closely as CSS can.

       Typst chrome that is reproducible:
         - letter page size (8.5in × 11in portrait, 11in × 8.5in
           landscape via .print-root.landscape)
         - 0.4in top/bottom + 0.63in left/right margins
         - top-left 7pt archive title + horizontal rule below
         - bottom-center 6pt footer ("Made with DixieData | ...")
           + horizontal rule above + italic codename
         - body font Arial 9pt (theme.palette.text_primary in typst)
         - title font Times New Roman 14pt bold
         - page-break-before:always on the biography section so
           long biographies get their own page (typst's pagebreak()
           model)
         - @media screen: hide print-only header/footer chrome; the
           print view is the visible page when not printing.
       ============================================================ */

    .print-root {
      max-width: 8.5in;
      margin: 0 auto;
      padding: 0.4in 0.63in;
      background: #ffffff;
      color: #1f2b38;
      font-family: Arial, "Liberation Sans", sans-serif;
      font-size: 9pt;
      line-height: 1.4;
      position: relative;
    }
    .print-root.landscape {
      max-width: 11in;
    }
    .print-header {
      position: running(printHeader);
      border-bottom: 0.6pt solid #8d7440;
      padding-bottom: 0.15in;
      margin-bottom: 0.25in;
      color: #5a4220;
      font-size: 7pt;
      display: flex;
      justify-content: space-between;
      align-items: baseline;
    }
    .print-footer {
      position: running(printFooter);
      border-top: 0.6pt solid #8d7440;
      padding-top: 0.15in;
      margin-top: 0.25in;
      color: #5a4220;
      font-size: 6pt;
      text-align: center;
    }
    .print-footer .print-codename {
      font-style: italic;
    }
    .print-title-block {
      margin: 0 0 0.25in 0;
    }
    .print-title-block .print-name {
      font-family: "Times New Roman", "Liberation Serif", "DejaVu Serif", serif;
      font-size: 14pt;
      font-weight: 700;
      color: #1f2b38;
    }
    .print-title-block .print-subtitle {
      font-family: "Times New Roman", "Liberation Serif", "DejaVu Serif", serif;
      font-size: 9pt;
      color: #5a4220;
      margin-top: 0.05em;
    }
    .print-section {
      margin: 0.2in 0;
    }
    .print-section h3 {
      font-family: Arial, sans-serif;
      font-size: 10pt;
      font-weight: 700;
      color: #1f2b38;
      margin: 0 0 0.1in 0;
      padding-bottom: 0.05in;
      border-bottom: 0.6pt solid rgba(141, 116, 64, 0.4);
    }
    .print-grid {
      display: grid;
      grid-template-columns: 1.4in 1fr;
      column-gap: 0.15in;
      row-gap: 0.05in;
      margin: 0;
    }
    .print-grid dt {
      font-weight: 700;
      color: #5a4220;
    }
    .print-grid dd {
      margin: 0;
      color: #1f2b38;
    }
    .print-grid dd em {
      font-style: italic;
    }
    .print-image-panel {
      width: 100%;
      height: 2.2in;
      overflow: hidden;
      display: flex;
      align-items: center;
      justify-content: center;
      margin: 0.15in 0;
      border: 1px solid rgba(141, 116, 64, 0.4);
    }
    .print-image-panel img {
      max-width: 100%;
      max-height: 100%;
      object-fit: contain;
    }
    .print-biography {
      page-break-before: always;
      padding-top: 0.25in;
    }
    .print-biography h3 {
      font-family: "Times New Roman", "Liberation Serif", "DejaVu Serif", serif;
      font-size: 20pt;
      font-weight: 700;
      color: #1f2b38;
      margin: 0 0 0.15in 0;
      border-bottom: 0;
    }
    .print-biography-body {
      font-size: 9pt;
      color: #1f2b38;
      white-space: pre-wrap;
    }
    .print-records ul {
      margin: 0;
      padding-left: 0.3in;
    }
    .print-records li {
      margin: 0.05in 0;
    }
    .print-record-link {
      color: #1f2b38;
      text-decoration: underline;
    }
    .print-record-link::after {
      content: " (Click to view)";
      color: #5a4220;
      font-style: italic;
    }

    @media screen {
      .print-header,
      .print-footer {
        position: static;
        margin: 0.2in 0;
      }
      body.print-mode .archive-shell {
        display: none;
      }
    }

    @page {
      size: letter;
      margin: 0.4in 0.63in;
    }
    @media print {
      body {
        margin: 0;
        background: #ffffff;
        color: #1f2b38;
      }
      .archive-shell {
        display: none;
      }
      .print-root {
        margin: 0;
        padding: 0;
        max-width: none;
      }
      .print-header {
        position: running(printHeader);
        border-bottom: 0.6pt solid #8d7440;
      }
      .print-footer {
        position: running(printFooter);
        border-top: 0.6pt solid #8d7440;
      }
      .print-section {
        break-inside: avoid;
      }
      .print-biography {
        break-before: page;
      }
    }
  </style>
</head>
<body>
  <div class="shell">
    <header class="hero">
      <div class="hero-shell">
        <h1>{{ .ArchiveTitle }}</h1>
        <p>Browse this standalone DixieData archive as a read-only mirror of the DixieData app. The Calendar landing shows every anniversary and event day in the archive; use the nav to jump to Filter (filterable Person Record list), Insights (analytics snapshot), or the Event / Article tabs.</p>
        <div class="archive-meta">
          <span>Generated {{ .GeneratedAt }}</span>
        </div>
        <!-- Issue #498 slice 2: small fixed nav menu (Calendar / Filter /
             Insights / View All / Events / Articles). Each link is
             a hash-route to its page renderer; the JS marks the active
             link based on the current hash. Empty-entity links are hidden
             (e.g. no Events tab if bundle.events is empty). -->
        <nav class="nav-menu" id="archive-nav-menu">
          <a href="#/calendar"  class="nav-link" data-route="calendar">Calendar</a>
          <a href="#/browse"    class="nav-link" data-route="browse">Filter</a>
          <a href="#/insights"  class="nav-link" data-route="insights">Insights</a>
          <a href="#/persons"   class="nav-link" data-route="persons">View All</a>
          <a href="#/events"    class="nav-link nav-link-events hidden" data-route="events">Events</a>
          <a href="#/articles"  class="nav-link nav-link-articles hidden" data-route="articles">Articles</a>
        </nav>
      </div>
    </header>

    <main>
      <!-- Issue #498 slice 2: the JS router renders one of the page
           templates into this container. Pages: Calendar landing,
           Browse (filterable list), Insights (analytics snapshot),
           Persons/Events/Articles list screens (legacy from #320/#490,
           re-skinned), per-record detail. -->
      <section id="archive-page" class="screen page-screen" aria-live="polite"></section>
      <section id="archive-detail-screen" class="screen detail-screen hidden">
        <div class="detail-toolbar">
          <button type="button" id="detail-back" class="back-button">← Back to Archive List</button>
          <a id="detail-export-report" class="export-report-button" target="_blank" rel="noopener noreferrer" title="Open the printable report in a new tab. Use your browser's Print → PDF (Cmd+P on Mac, Ctrl+P on Windows/Linux) to save it as a PDF." style="display:none;">Export Report</a>
          <span class="export-report-tip">💡 Open the printable report in a new tab, then use your browser's Print → PDF to save it.</span>
          <span id="detail-position" class="pill">Record View</span>
        </div>
        <div id="detail-content" class="detail-card">Select a record to view its details.</div>
      </section>
    </main>

    <footer>
      Made with DixieData | Version: {{ .Version }} | Build: {{ .Build }}
    </footer>
  </div>

  <div id="image-overlay" class="image-overlay" aria-hidden="true">
    <div class="image-preview-card">
      <div class="overlay-head">
        <strong id="image-overlay-title">Image Preview</strong>
        <button type="button" id="image-overlay-close" class="overlay-close">Close</button>
      </div>
      <div id="image-preview-stage" class="image-preview-stage">
        <img id="image-overlay-img" alt="Archive image preview">
      </div>
    </div>
  </div>

  <script>
function escapeHtml(value) {
      return String(value || "")
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
    }

    function detailHash(record) {
      return '#/person/' + encodeURIComponent(record.displayId || record.name || '');
    }

    function detailLink(displayId) {
      return '#/person/' + encodeURIComponent(String(displayId || '').trim());
    }

    function excerpt(value, maxLength) {
      const text = String(value || '').trim();
      if (!text || text.length <= maxLength) {
        return text;
      }
      return text.slice(0, maxLength - 1).trimEnd() + '…';
    }

    function searchTerms(query) {
      return String(query || '')
        .trim()
        .toLowerCase()
        .split(/[^a-z0-9]+/)
        .filter(Boolean);
    }

    function searchText(record) {
      const recordText = Array.isArray(record.records) ? record.records.map(function(item) {
        return [
          item.recordType,
          item.appId,
          item.details
        ].filter(Boolean).join(' ');
      }).join(' ') : '';

      return [
        record.displayId,
        record.name,
        record.rank,
        record.rankIn,
        record.rankOut,
        record.unit,
        record.pensionId,
        record.appId,
        record.pensionState,
        record.homeStatus,
        record.homeName,
        record.location,
        record.name,
        record.prefix,
        record.firstName,
        record.middleName,
        record.lastName,
        record.suffix,
        record.maidenName,
        record.relationshipLabel,
        record.spouseName,
        record.spouseDisplayId,
        record.birthDate,
        record.deathDate,
        record.birthInfo,
        record.reviewReason,
        record.addedBy,
        record.lastEditedBy,
        record.lastEditedAt,
        record.lastEditedFields,
        record.notes,
        recordText
        ].filter(Boolean).join(' ').toLowerCase();
    }

    function matchesSearch(record, query) {
      const terms = searchTerms(query);
      if (!terms.length) {
        return true;
      }
      const haystack = searchText(record);
      return terms.every(function(term) {
        return haystack.includes(term);
      });
    }

    function detailValue(value) {
      const text = String(value || '').trim();
      return text || 'N/A';
    }

    function blankDetailValue(value) {
      return String(value || '').trim();
    }

    function dateDetailValue(value) {
      const text = String(value || '').trim();
      return text || 'Unknown';
    }

    function detailMarkup(label, value) {
      const text = detailValue(value);
      if (label === 'Maiden Name' && text !== 'N/A') {
        return '<em>' + escapeHtml(text) + '</em>';
      }
      return escapeHtml(text);
    }

    function renderLinkedText(text) {
      return escapeHtml(String(text || '')).replace(/(https?:\/\/[^\s<]+)|\[\[([^\[\]\r\n]+)\]\]/g, function(match, externalUrl, displayId) {
        if (externalUrl) {
          var cleanUrl = externalUrl.replace(/[.,;:!?)\]}]+$/, '');
          var suffix = externalUrl.slice(cleanUrl.length);
          return '<a class="record-link" href="' + escapeHtml(cleanUrl) + '" target="_blank" rel="noreferrer noopener">' + escapeHtml(cleanUrl) + '</a>' + escapeHtml(suffix);
        }
        var target = String(displayId || '').trim();
        if (!target) {
          return escapeHtml(match);
        }
        return '<a class="record-link" href="' + detailLink(target) + '">' + escapeHtml(target) + '</a>';
      }).replace(/\n/g, '<br>');
    }

    function relatedFamilyRecords(record, allRecords) {
      return Array.isArray(allRecords) ? allRecords.filter(function(item) {
        return item.displayId !== record.displayId && item.spouseDisplayId && item.spouseDisplayId === record.displayId;
      }) : [];
    }

    function renderRecord(record, index, allRecords) {
      const relatedFamily = relatedFamilyRecords(record, allRecords);
      return '' +
        '<article class="record-row" data-record-index="' + index + '">' +
          '<div class="row-main">' +
            '<div class="row-meta">' +
              '<span class="pill">' + escapeHtml(record.displayType) + '</span>' +
              '<span class="pill">' + escapeHtml(record.displayId) + '</span>' +
              (record.spouseDisplayId || relatedFamily.length ? '<span class="pill">Family Linked</span>' : '') +
              (record.needsReview ? '<span class="pill">Needs Review</span>' : '') +
            '</div>' +
            (record.tags && record.tags.length ? '<div class="row-meta">' + record.tags.map(function(t) { return '<span class="pill pill-tag">' + escapeHtml(t) + '</span>'; }).join('') + '</div>' : '') +
            '<h3 class="row-title">' + escapeHtml(record.name) + '</h3>' +
            '<div class="row-summary">' +
              '<span><strong>Dates:</strong> ' + escapeHtml(record.dates || 'N/A') + '</span>' +
              '<span><strong>Unit:</strong> ' + escapeHtml(record.unit || '') + '</span>' +
              '<span><strong>Location:</strong> ' + escapeHtml(record.location || 'N/A') + '</span>' +
            '</div>' +
            (record.notes ? '<div class="row-excerpt">' + escapeHtml(excerpt(record.notes, 150)) + '</div>' : '') +
          '</div>' +
          '<button type="button" class="action-button" data-view-record="' + index + '">View More</button>' +
        '</article>';
    }

    function renderDetail(record, allRecords, allEvents) {
      const spouseLink = record.spouseDisplayId
        ? '<a class="image-button" href="' + detailLink(record.spouseDisplayId) + '">Open Linked Soldier</a>'
        : '';
      const relatedFamily = relatedFamilyRecords(record, allRecords);
      const details = [
        ['Record Type', detailValue(record.displayType)],
        ['Display ID', detailValue(record.displayId)],
        ['Prefix', blankDetailValue(record.prefix)],
        ['First Name', blankDetailValue(record.firstName)],
        ['Middle Name', blankDetailValue(record.middleName)],
        ['Last Name', blankDetailValue(record.lastName)],
        ['Suffix', detailValue(record.suffix)],
        ['Dates', record.dates || 'N/A'],
        ['Birth Date', dateDetailValue(record.birthDate)],
        ['Death Date', dateDetailValue(record.deathDate)],
        ['Birth Info', detailValue(record.birthInfo)],
        ['Buried In', detailValue(record.location)]
      ];
      if (record.entryType === 'wife' || record.entryType === 'widow') {
        details.push(['Married To', detailValue(record.spouseName)]);
        details.push(['Linked Soldier Record', detailValue(record.spouseDisplayId)]);
        details.push(['Maiden Name', detailValue(record.maidenName)]);
        if (record.entryType === 'widow') {
          details.push(['Pension ID', detailValue(record.pensionId)]);
          details.push(['Application ID', detailValue(record.appId)]);
        }
      } else if (record.entryType === 'linked_person') {
        details.push(['Relationship to Soldier', detailValue(record.relationshipLabel)]);
        details.push(['Linked Soldier Record', detailValue(record.spouseDisplayId)]);
      } else {
        details.push(['Rank', blankDetailValue(record.rankOut || record.rank || record.rankIn)]);
        details.push(['Rank In', blankDetailValue(record.rankIn)]);
        details.push(['Rank Out', blankDetailValue(record.rankOut || record.rank)]);
        details.push(['Unit', blankDetailValue(record.unit)]);
        details.push(['Pension State', detailValue(record.pensionState)]);
        details.push(['Confederate Home Status', detailValue(record.homeStatus)]);
        details.push(['Confederate Home Name', detailValue(record.homeName)]);
        details.push(['Pension ID', detailValue(record.pensionId)]);
        details.push(['Application ID', detailValue(record.appId)]);
      }

      const primarySections = [];
      const sideSections = [];
      if (spouseLink || relatedFamily.length) {
        primarySections.push(
          '<section class="detail-section"><h4>Family Links</h4>' +
            (spouseLink ? '<div class="related-links">' + spouseLink + '</div>' : '') +
            (relatedFamily.length ? '<div class="related-list">' + relatedFamily.map(function(item) {
              return '' +
                '<div class="related-card">' +
                  '<strong>' + escapeHtml(item.name) + '</strong>' +
                  '<p>' + escapeHtml(item.displayType + ' • ' + item.displayId) + '</p>' +
                  '<div class="related-links"><a class="image-button" href="' + detailLink(item.displayId) + '">Open Related Record</a></div>' +
                '</div>';
            }).join('') + '</div>' : '') +
          '</section>'
        );
      }
      if (record.notes) {
        primarySections.push('<section class="detail-section"><h4>Notes</h4><p>' + renderLinkedText(record.notes) + '</p></section>');
      }
      if (record.biography) {
        primarySections.push('<section class="detail-section"><h4>Biography</h4><p>' + renderLinkedText(record.biography) + '</p></section>');
      }
      if (record.tags && record.tags.length) {
        var tagPills = record.tags.map(function(t) { return '<span class="pill pill-tag">' + escapeHtml(t) + '</span>'; }).join(' ');
        primarySections.push('<section class="detail-section"><h4>Tags</h4><div class="tag-pills">' + tagPills + '</div></section>');
      }
      if (record.records && record.records.length) {
        primarySections.push(
          '<section class="detail-section"><h4>Records</h4><ul>' +
            record.records.map(function(item) {
              const app = item.appId ? ' (' + escapeHtml(item.appId) + ')' : '';
              const detailsText = item.details ? '<br>' + renderLinkedText(item.details) : '';
              return '<li><strong>' + escapeHtml(item.recordType || 'Record') + '</strong>' + app + detailsText + '</li>';
            }).join('') +
          '</ul></section>'
        );
      }
      const linkedEvents = linkedEventsForRecord(record, allEvents);
      if (linkedEvents.length) {
        primarySections.push(
          '<section class="detail-section"><h4>Linked Events</h4><div class="related-list">' +
            linkedEvents.map(function(ev) {
              const evTitle = escapeHtml(ev.description || ev.kind || 'Untitled Event');
              const evMeta = escapeHtml((ev.kind || '') + (ev.dateRange ? ' · ' + ev.dateRange : ''));
              return '<div class="related-card"><strong>' + evTitle + '</strong><p>' + evMeta + '</p>' +
                '<div class="related-links"><a class="image-button" href="#/event/' + encodeURIComponent(String(ev.displayId || ev.id || '')) + '">Open Event</a></div></div>';
            }).join('') +
          '</div></section>'
        );
      }
      sideSections.push(
        '<section class="detail-section"><h4>Archive Metadata</h4><dl class="detail-grid compact">' +
          '<dt>Review Status</dt><dd>' + escapeHtml(record.needsReview ? 'Needs Review' : 'Clean') + '</dd>' +
          '<dt>Review Reason</dt><dd>' + escapeHtml(detailValue(record.reviewReason)) + '</dd>' +
          '<dt>Added By</dt><dd>' + escapeHtml(detailValue(record.addedBy)) + '</dd>' +
          '<dt>Last Edited By</dt><dd>' + escapeHtml(detailValue(record.lastEditedBy)) + '</dd>' +
          '<dt>Last Edited At</dt><dd>' + escapeHtml(detailValue(record.lastEditedAt)) + '</dd>' +
          '<dt>Last Edited Fields</dt><dd>' + escapeHtml(detailValue(record.lastEditedFields)) + '</dd>' +
        '</dl></section>'
      );
      if (record.images && record.images.length) {
        sideSections.push(
          '<section class="detail-section"><h4>Images</h4><div class="image-list">' +
            record.images.map(function(image) {
              const label = image.caption || image.fileName || 'Image';
              return '' +
                '<div class="image-row">' +
                  '<div class="image-caption">' +
                    '<strong>' + escapeHtml(label) + '</strong>' +
                    '<a class="record-link" href="' + encodeURI(image.filePath) + '" target="_blank" rel="noreferrer noopener">' + escapeHtml(image.fileName || image.filePath) + '</a>' +
                  '</div>' +
                  '<button type="button" class="image-button" data-preview-image="' + encodeURI(image.filePath) + '" data-preview-title="' + escapeHtml(label) + '">Preview</button>' +
                '</div>';
            }).join('') +
          '</div></section>'
        );
      }

      return '' +
        '<div class="detail-header">' +
          '<div class="row-meta">' +
            '<span class="pill">' + escapeHtml(record.displayType) + '</span>' +
            '<span class="pill">' + escapeHtml(record.displayId) + '</span>' +
            (record.needsReview ? '<span class="pill">Needs Review</span>' : '') +
          '</div>' +
          '<h3>' + escapeHtml(record.name) + '</h3>' +
        '</div>' +
        '<div class="detail-layout">' +
          '<div>' +
            '<dl class="detail-grid">' +
              details.map(function(line) {
                return '<dt>' + escapeHtml(line[0]) + '</dt><dd>' + detailMarkup(line[0], line[1]) + '</dd>';
              }).join('') +
            '</dl>' +
            primarySections.join('') +
          '</div>' +
          '<div>' +
            (sideSections.length ? sideSections.join('') : '<section class="detail-section"><h4>Images</h4><p>No images recorded for this entry.</p></section>') +
          '</div>' +
        '</div>';
    }

    // --- Printable report helpers (issue #509) ---

    // printMonthNames mirrors templates/common/record_card.typ's
    // month-names dictionary. The Typst long-date formatter maps
    // "05" -> "May"; we reproduce that in JS so the printable
    // view matches what the live PDF export shows.
    var PRINT_MONTH_NAMES = {
      '01': 'January', '02': 'February', '03': 'March', '04': 'April',
      '05': 'May', '06': 'June', '07': 'July', '08': 'August',
      '09': 'September', '10': 'October', '11': 'November', '12': 'December',
    };

    // printLongDate renders a date string in the same long form as
    // templates/common/record_card.typ::long-date. Input is the
    // canonical MM/DD/YYYY shape used by the bundle's date fields.
    //   "00/00/0000" -> "Unknown"
    //   "00/00/1835" -> "1835"        (year only)
    //   "05/00/1844" -> "May 1844"    (month + year, no day)
    //   "05/22/1844" -> "May 22, 1844" (full)
    function printLongDate(s) {
      if (!s || s === 'Unknown' || s === '0000-00-00') return 'Unknown';
      var parts = String(s).split('/');
      if (parts.length !== 3) {
        // Non-canonical shape — passthrough, same as typst's
        // fallback branch (returns the raw string).
        return String(s);
      }
      var monthIdx = parts[0];
      var dayRaw = parts[1];
      var yearRaw = parts[2];
      if (monthIdx === '00' && dayRaw === '00' && yearRaw === '00') return 'Unknown';
      if (monthIdx === '00' && dayRaw === '00') return yearRaw;
      if (dayRaw === '00' && monthIdx !== '00' && yearRaw !== '00') {
        return (PRINT_MONTH_NAMES[monthIdx] || monthIdx) + ' ' + yearRaw;
      }
      if (monthIdx === '00' || dayRaw === '00' || yearRaw === '00') return 'Unknown';
      var day = (dayRaw.length > 1 && dayRaw.charAt(0) === '0') ? dayRaw.slice(1) : dayRaw;
      return (PRINT_MONTH_NAMES[monthIdx] || monthIdx) + ' ' + day + ', ' + yearRaw;
    }

    // printEntryTypeLabel mirrors templates/common/record_card.typ::
    // entry-type-label so the printable view's title subtitle uses
    // the same wording as the live PDF export.
    function printEntryTypeLabel(raw) {
      var r = String(raw || '').trim().toLowerCase();
      if (!r) return 'Soldier';
      if (r === 'soldier') return 'Soldier';
      if (r === 'wife') return 'Wife';
      if (r === 'widow') return 'Widow';
      if (r === 'linked_person') return 'Person Record';
      return r.charAt(0).toUpperCase() + r.slice(1);
    }

    // printRenderLink mirrors templates/common/record_card.typ::
    // render-link. Renders a URL as a 'Click to view' anchor (the
    // URL itself is the href; the visible text reads 'Click to
    // view'). Plain text passes through unchanged.
    function printRenderLink(url) {
      var u = String(url || '').trim();
      if (!u) return '';
      if (!/^https?:\/\//i.test(u)) return escapeHtml(u);
      return '<a class="print-record-link" href="' + escapeHtml(u) + '" target="_blank" rel="noreferrer noopener">Click to view</a>';
    }

    // printFieldRow renders a (label, value) pair as a
    // definition-list row. Used to mirror typst's field-row.
    function printFieldRow(label, value) {
      var display = value;
      if (value === undefined || value === null || String(value).trim() === '') {
        display = 'N/A';
      }
      if (label === 'Maiden Name' && display !== 'N/A') {
        return '<dt>' + escapeHtml(label) + '</dt><dd><em>' + escapeHtml(String(display)) + '</em></dd>';
      }
      return '<dt>' + escapeHtml(label) + '</dt><dd>' + escapeHtml(String(display)) + '</dd>';
    }

    // printComposeName mirrors templates/common/record_card.typ's
    // compose-name helper: Prefix First Middle Last, dropping
    // blank parts. The suffix is appended with a leading space
    // when present (matches typst's "[suffix]" suffix concat).
    function printComposeName(record) {
      var parts = [];
      if (record.prefix) parts.push(record.prefix);
      if (record.firstName) parts.push(record.firstName);
      if (record.middleName) parts.push(record.middleName);
      if (record.lastName) parts.push(record.lastName);
      var name = parts.join(' ');
      var suffix = String(record.suffix || '').trim();
      if (suffix) name = name + ' ' + suffix;
      return name || 'Unknown';
    }

    // renderPrintableReport renders a printable DOM for a single
    // Person Record, mirroring the section structure of
    // templates/common/record_card.typ (title block, identity,
    // service, household, records, biography, image panel). The
    // output is consumed by the #/print/{displayId} route, which
    // replaces the document body so a new tab can Cmd+P / Ctrl+P
    // it directly.
    //
    // Design: client-side from bundle, no Typst at export time.
    // The bundle already carries the StaticArchiveRecord superset
    // of what typst's data.at("soldier") sees; this function
    // reproduces the typst visual surface via HTML + CSS (no
    // 50 MB binary, no per-record tempdir, no shell-out, no
    // per-record .html file in the .zip).
    function renderPrintableReport(record, bundle, landscape) {
      var primaryImage = '';
      if (Array.isArray(record.images) && record.images.length) {
        var primary = record.images[0];
        var filePath = primary.filePath || ('images/' + (primary.fileName || ''));
        if (filePath) {
          primaryImage = '<div class="print-image-panel"><img src="' + escapeHtml(filePath) + '" alt="' + escapeHtml(primary.caption || primary.fileName || '') + '"></div>';
        }
      }

      // Identity section mirrors typst::render-identity-section.
      var identityRows = [
        printFieldRow('Prefix', record.prefix),
        printFieldRow('First Name', record.firstName),
        printFieldRow('Middle Name', record.middleName),
        printFieldRow('Last Name', record.lastName),
        printFieldRow('Suffix', record.suffix),
        printFieldRow('Birth Date', printLongDate(record.birthDate)),
        printFieldRow('Death Date', printLongDate(record.deathDate)),
        printFieldRow('Birth Info', record.birthInfo),
        printFieldRow('Buried In', record.location),
      ].join('');

      // Service section is conditional on entry type, mirroring
      // typst::render-service-section. Soldiers get the full
      // service block; widows get Pension + Application IDs;
      // wives + linked_person get nothing here.
      var serviceRows = '';
      if (record.entryType === 'wife') {
        // No service section for wives (matches typst).
      } else if (record.entryType === 'widow') {
        serviceRows = [
          printFieldRow('Pension ID', record.pensionId),
          printFieldRow('Application ID', record.appId),
        ].join('');
      } else if (record.entryType === 'linked_person') {
        serviceRows = [
          printFieldRow('Relationship to Soldier', record.relationshipLabel),
          printFieldRow('Linked Soldier Record', record.spouseDisplayId),
        ].join('');
      } else {
        serviceRows = [
          printFieldRow('Record Type', printEntryTypeLabel(record.entryType)),
          printFieldRow('Rank', record.rankOut || record.rank || record.rankIn),
          printFieldRow('Rank In', record.rankIn),
          printFieldRow('Rank Out', record.rankOut || record.rank),
          printFieldRow('Unit', record.unit),
          printFieldRow('Pension State', record.pensionState),
          printFieldRow('Pension ID', record.pensionId),
          printFieldRow('Application ID', record.appId),
          printFieldRow('Confederate Home Status', record.homeStatus),
          printFieldRow('Confederate Home Name', record.homeName),
        ].join('');
      }

      // Household section mirrors typst::render-household-section.
      // Suppressed when all entries are blank (matches typst
      // suppression at L349 of record_card.typ).
      var householdRows = '';
      if (record.entryType === 'wife' || record.entryType === 'widow') {
        householdRows = [
          printFieldRow('Married To', record.spouseName),
          printFieldRow('Linked Soldier Record', record.spouseDisplayId),
          printFieldRow('Maiden Name', record.maidenName),
        ].join('');
      }
      var hasHousehold = householdRows.replace(/<[^>]+>/g, '').replace(/N\/A/g, '').trim().length > 0;

      // Records section mirrors typst::render-records-section.
      var recordsHtml = '';
      if (Array.isArray(record.records) && record.records.length) {
        recordsHtml = '<ul>' + record.records.map(function(r) {
          var app = r.appId ? ' (' + escapeHtml(r.appId) + ')' : '';
          var details = r.details ? '<br>' + printRenderLink(r.details) : '';
          return '<li><strong>' + escapeHtml(r.recordType || 'Record') + '</strong>' + app + details + '</li>';
        }).join('') + '</ul>';
      }

      // Biography section mirrors typst::render-biography-page.
      // page-break-before:always via .print-biography so long
      // bios get their own page.
      var biographyHtml = '';
      if (record.biography && String(record.biography).trim()) {
        biographyHtml = '<section class="print-biography">' +
          '<h3>' + escapeHtml(printComposeName(record)) + '</h3>' +
          '<div class="print-biography-body">' + renderLinkedText(record.biography) + '</div>' +
        '</section>';
      }

      var archiveTitle = (bundle && bundle.archiveTitle) || 'DixieData Archive';
      var footerText = (bundle && bundle.footerText) || 'Made with DixieData';
      var codename = (bundle && bundle.codename) || '';

      var orientationClass = landscape ? ' landscape' : '';
      return '' +
        '<div class="print-root' + orientationClass + '">' +
          '<div class="print-header">' +
            '<span>' + escapeHtml(archiveTitle) + '</span>' +
            '<span>' + escapeHtml(record.displayId || '') + '</span>' +
          '</div>' +
          '<div class="print-title-block">' +
            '<div class="print-name">' + escapeHtml(printComposeName(record)) + '</div>' +
            '<div class="print-subtitle">' + escapeHtml((record.displayId || '') + ' \u2014 ' + printEntryTypeLabel(record.entryType)) + '</div>' +
          '</div>' +
          primaryImage +
          '<section class="print-section"><h3>Identity &amp; Vital Details</h3><dl class="print-grid">' + identityRows + '</dl></section>' +
          (serviceRows ? '<section class="print-section"><h3>Service &amp; Archive Details</h3><dl class="print-grid">' + serviceRows + '</dl></section>' : '') +
          (hasHousehold ? '<section class="print-section"><h3>Household &amp; Context</h3><dl class="print-grid">' + householdRows + '</dl></section>' : '') +
          (recordsHtml ? '<section class="print-section print-records"><h3>Records</h3>' + recordsHtml + '</section>' : '') +
          '<div class="print-footer">' +
            '<span>' + escapeHtml(footerText) + '</span>' +
            (codename ? ' <span class="print-codename">\u2014 ' + escapeHtml(codename) + '</span>' : '') +
          '</div>' +
          biographyHtml +
        '</div>';
    }

    function linkedEventsForRecord(record, allEvents) {
      if (!Array.isArray(allEvents) || !record || !record.displayId) return [];
      return allEvents.filter(function(ev) {
        return Array.isArray(ev.linkedDisplayIds) && ev.linkedDisplayIds.indexOf(record.displayId) !== -1;
      });
    }

    function renderEventRow(event, index) {
      return '' +
        '<article class="record-row" data-event-index="' + index + '">' +
          '<div class="row-main">' +
            '<div class="row-meta">' +
              '<span class="pill">Event</span>' +
              (event.kind ? '<span class="pill">' + escapeHtml(event.kind) + '</span>' : '') +
            '</div>' +
            '<h3 class="row-title">' + escapeHtml(event.description || event.kind || 'Untitled Event') + '</h3>' +
            '<div class="row-summary">' +
              '<span><strong>Date:</strong> ' + escapeHtml(event.dateRange || 'N/A') + '</span>' +
              (event.location ? '<span><strong>Location:</strong> ' + escapeHtml(event.location) + '</span>' : '') +
            '</div>' +
          '</div>' +
          '<button type="button" class="action-button" data-view-event="' + index + '">View More</button>' +
        '</article>';
    }

    function renderEventDetail(event, allRecords) {
      const details = [
        ['Kind', detailValue(event.kind)],
        ['Date Range', event.dateRange || 'N/A'],
        ['Location', detailValue(event.location)],
        ['Source Citation', detailValue(event.sourceCitation)]
      ];
      const linkedPersons = Array.isArray(event.linkedPersons) ? event.linkedPersons : [];
      let linkedHtml = '';
      if (linkedPersons.length) {
        linkedHtml = '<section class="detail-section"><h4>Linked Persons</h4><div class="related-list">' +
          linkedPersons.map(function(p) {
            const link = p.displayId ? '<div class="related-links"><a class="image-button" href="' + detailLink(p.displayId) + '">Open Person Record</a></div>' : '';
            return '<div class="related-card"><strong>' + escapeHtml(p.name || p.displayId || 'Unknown') + '</strong><p>' + escapeHtml(p.displayId || '') + '</p>' + link + '</div>';
          }).join('') +
        '</div></section>';
      }
      return '' +
        '<div class="detail-header">' +
          '<div class="row-meta"><span class="pill">Event</span>' + (event.kind ? '<span class="pill">' + escapeHtml(event.kind) + '</span>' : '') + '</div>' +
          '<h3>' + escapeHtml(event.description || event.kind || 'Untitled Event') + '</h3>' +
        '</div>' +
        '<div class="detail-layout"><div>' +
          '<dl class="detail-grid">' +
            details.map(function(line) { return '<dt>' + escapeHtml(line[0]) + '</dt><dd>' + detailMarkup(line[0], line[1]) + '</dd>'; }).join('') +
          '</dl>' +
          linkedHtml +
        '</div><div></div></div>';
    }

    function renderArticleRow(article, index) {
      return '' +
        '<article class="record-row" data-article-index="' + index + '">' +
          '<div class="row-main">' +
            '<div class="row-meta"><span class="pill">Article</span></div>' +
            '<h3 class="row-title">' + escapeHtml(article.title || 'Untitled Article') + '</h3>' +
            (article.subtitle ? '<div class="row-summary"><span>' + escapeHtml(article.subtitle) + '</span></div>' : '') +
          '</div>' +
          '<button type="button" class="action-button" data-view-article="' + index + '">View More</button>' +
        '</article>';
    }

    function renderArticleDetail(article) {
      const refs = Array.isArray(article.resolvedRefs) ? article.resolvedRefs : [];
      let refsHtml = '';
      if (refs.length) {
        refsHtml = '<section class="detail-section"><h4>Referenced Person Records</h4><ul>' +
          refs.map(function(ref) {
            const marker = ref.resolved ? '' : ' <em>(Unknown)</em>';
            const link = ref.resolved && ref.displayId ? '<a class="record-link" href="' + detailLink(ref.displayId) + '">' + escapeHtml(ref.name || ref.displayId) + '</a>' : escapeHtml(ref.name || ref.displayId || 'Unknown');
            return '<li>' + link + marker + '</li>';
          }).join('') +
        '</ul></section>';
      }
      return '' +
        '<div class="detail-header">' +
          '<div class="row-meta"><span class="pill">Article</span></div>' +
          '<h3>' + escapeHtml(article.title || 'Untitled Article') + '</h3>' +
        '</div>' +
        '<div class="detail-layout"><div>' +
          (article.subtitle ? '<p class="row-summary">' + escapeHtml(article.subtitle) + '</p>' : '') +
          '<section class="detail-section"><h4>Body</h4><div class="article-body">' + renderLinkedText(article.bodyHtml || article.bodyMd || '') + '</div></section>' +
          refsHtml +
        '</div><div></div></div>';
    }

    // --- Issue #498 slice 2: hash router + nav menu + page renderers ---

    // Calendar month names (Jan=1 ... Dec=12) for the Calendar landing
    // page grid. Mirrors the live /calendar page's monthName helper.
    var ARCHIVE_CALENDAR_MONTHS = [
      '', 'January', 'February', 'March', 'April', 'May', 'June',
      'July', 'August', 'September', 'October', 'November', 'December'
    ];

    // Days-in-month lookup for the Calendar grid (non-leap-year
    // convention; the archive's anniversaries are historical so
    // Feb 29 anniversaries stay on Feb 29 in the live DB but render
    // as Feb 28 in the grid when not a leap year — minor edge case,
    // matches the live /calendar behavior).
    var ARCHIVE_CALENDAR_DAYS = [0, 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

    // Month-abbreviation → number lookup for the death-date drilldown
    // (issue #510). The bundle's deathDate is dates.Display() output
    // ("May 12, 1865"), not a numeric "MM/DD/YYYY" — so the Browse
    // ?date=MM-DD pre-fill has to parse the display string to compare.
    var ARCHIVE_MONTH_ABBREV_TO_NUM = {
      jan: 1, feb: 2, mar: 3, apr: 4, may: 5, jun: 6,
      jul: 7, aug: 8, sep: 9, oct: 10, nov: 11, dec: 12,
    };

    // Issue #509: printable-report constants. These are templated
    // into index.html at export time so the printable view's
    // header / footer / codename match the live PDF export.
    var ARCHIVE_TITLE = {{ .ArchiveTitleJS }};
    var FOOTER_TEXT = {{ .FooterTextJS }};
    var CODENAME = {{ .CodenameJS }};

    // routeFromHash parses the current window.location.hash and
    // returns one of: {kind:'page', name:'calendar'|'browse'|
    // 'insights'|'persons'|'events'|'articles', query:''},
    // {kind:'detail', entity:'person'|'event'|'article', id:'...'},
    // or null. The legacy #record=, #event=, #article= hashes
    // (issues #320, #490) are normalised into the new #/person/{id},
    // #/event/{id}, #/article/{id} shape so the revamp stays
    // backwards-compatible with previously exported archives.
    function routeFromHash(rawHash) {
      var hash = String(rawHash || window.location.hash || '').trim();
      if (!hash || hash === '#') {
        return { kind: 'page', name: 'calendar', query: '' };
      }
      var path = hash.replace(/^#/, '');
      // Legacy aliases — translate the #record=, #event=, #article=
      // shapes from issues #320 / #490 to the new #/... routes so
      // pre-revamp archives still resolve their detail pages.
      var legacyRecord = path.match(/^record=(.+)$/);
      if (legacyRecord) return { kind: 'detail', entity: 'person', id: decodeURIComponent(legacyRecord[1]) };
      var legacyEvent = path.match(/^event=(.+)$/);
      if (legacyEvent) return { kind: 'detail', entity: 'event', id: decodeURIComponent(legacyEvent[1]) };
      var legacyArticle = path.match(/^article=(.+)$/);
      if (legacyArticle) return { kind: 'detail', entity: 'article', id: decodeURIComponent(legacyArticle[1]) };
      // New #/person/{id}, #/event/{id}, #/article/{id} detail routes.
      var detailMatch = path.match(/^\/(person|event|article)\/(.+)$/);
      if (detailMatch) return { kind: 'detail', entity: detailMatch[1], id: decodeURIComponent(detailMatch[2]) };
      // Issue #509: printable-report route #/print/{displayId}.
      // Opens in a new tab via the toolbar Export Report button;
      // renders renderPrintableReport(record) and replaces the
      // document body so Cmd+P / Ctrl+P produces a clean PDF.
      var printMatch = path.match(/^\/print\/(.+)$/);
      if (printMatch) return { kind: 'print', id: decodeURIComponent(printMatch[1]) };
      // Page routes: #/calendar, #/browse?..., #/insights, #/persons, #/events, #/articles.
      var pageMatch = path.match(/^\/([a-z]+)(\?.*)?$/);
      if (pageMatch) {
        var name = pageMatch[1];
        var query = (pageMatch[2] || '').replace(/^\?/, '');
        if (name === 'calendar' || name === 'browse' || name === 'insights' ||
            name === 'persons' || name === 'events' || name === 'articles' || name === 'calendar-items') {
          return { kind: 'page', name: name, query: query };
        }
      }
      return { kind: 'page', name: 'calendar', query: '' };
    }

    // updateNavActive marks the active route in the nav menu.
    // Empty-entity links (Events / Articles) are hidden by the
    // bootstrap when bundle.events / bundle.articles is empty.
    function updateNavActive(routeName) {
      document.querySelectorAll('.nav-link').forEach(function(link) {
        link.classList.toggle('active', link.getAttribute('data-route') === routeName);
      });
    }

    function showPageScreen() {
      var page = document.getElementById('archive-page');
      var detail = document.getElementById('archive-detail-screen');
      if (page) page.classList.remove('hidden');
      if (detail) detail.classList.add('hidden');
    }

    function showDetailScreen() {
      var page = document.getElementById('archive-page');
      var detail = document.getElementById('archive-detail-screen');
      if (page) page.classList.add('hidden');
      if (detail) detail.classList.remove('hidden');
    }

    function setPageHtml(html) {
      var page = document.getElementById('archive-page');
      if (page) page.innerHTML = html;
    }

    // renderCalendarPage renders the Calendar landing page from
    // bundle.calendar (issue #498 slice 1). Renders ONE month at a
    // time per issue #500 locked decision 1; a month selector
    // dropdown + prev/next buttons let the user flip between months.
    // Defaults to the current calendar month on first mount; hash
    // pre-fill (?month=N) jumps to a specific month.
    function renderCalendarPage(bundle, query) {
      var months = Array.isArray(bundle.calendar) ? bundle.calendar : [];
      // Determine the initial month from the query string or today.
      var q = parseCalendarQuery(query);
      var currentMonth = new Date().getMonth() + 1; // 1-12
      var monthNum = q.month ? Number(q.month) : currentMonth;
      if (monthNum < 1 || monthNum > 12 || isNaN(monthNum)) monthNum = currentMonth;

      var totalDaysWithData = 0;
      var blocks = [];
      function renderMonthGrid(m) {
        var monthData = null;
        for (var i = 0; i < months.length; i++) {
          if (months[i].month === m) { monthData = months[i]; break; }
        }
        var days = (monthData && monthData.days) ? monthData.days : {};
        var monthBlocks = (monthData && monthData.blocks) ? monthData.blocks : 0;
        for (var k in days) {
          if (Object.prototype.hasOwnProperty.call(days, k)) {
            var d = days[k];
            if (d.a + d.e + d.h > 0) totalDaysWithData++;
          }
        }
        // Approximate first weekday of the month (UTC). The bundle
        // is a snapshot so we render day-of-month only — the live
        // /calendar page also relies on JS Date; this approximation
        // matches for non-historical months. Edge cases (Feb 29,
        // 1900 vs 2000) fall back to day-of-month only.
        var firstWeekday = new Date(Date.UTC(2025, m - 1, 1)).getUTCDay();
        var daysInMonth = ARCHIVE_CALENDAR_DAYS[m];
        var cells = [];
        for (var pad = 0; pad < firstWeekday; pad++) {
          cells.push('<div class="calendar-day empty"></div>');
        }
        for (var day = 1; day <= daysInMonth; day++) {
          var dayKey = String(day);
          var marker = days[dayKey];
          var hasAnniversary = marker && marker.a > 0;
          var hasEvent = marker && marker.e > 0;
          var hasHoliday = marker && marker.h > 0;
          if (hasAnniversary || hasEvent || hasHoliday) {
            var monthDate = String(m).padStart(2, '0') + '-' + String(day).padStart(2, '0');
            var clickable = hasAnniversary ? ' onclick="window.location.hash=\'#/browse?date=' + monthDate + '\'"' : '';
            var markers = '<div class="calendar-day-markers">';
            if (hasAnniversary) markers += '<span class="calendar-day-marker anniversary" title="Anniversaries">' + marker.a + '</span>';
            if (hasEvent) markers += '<span class="calendar-day-marker event" title="Events">' + marker.e + '</span>';
            if (hasHoliday) markers += '<span class="calendar-day-marker holiday" title="Holidays">' + marker.h + '</span>';
            markers += '</div>';
            cells.push('<button type="button" class="calendar-day"' + clickable + '><span class="calendar-day-number">' + day + '</span>' + markers + '</button>');
          } else {
            cells.push('<div class="calendar-day empty"><span class="calendar-day-number">' + day + '</span></div>');
          }
        }
        blocks.push(
          '<div class="calendar-month-block">' +
            '<div class="calendar-month-head">' +
              '<h3>' + escapeHtml(ARCHIVE_CALENDAR_MONTHS[m]) + '</h3>' +
              '<div class="calendar-legend">' +
                '<span><i style="background:rgba(197,171,104,0.45)"></i>Anniversaries</span>' +
                '<span><i style="background:rgba(124,179,226,0.5)"></i>Events</span>' +
                '<span><i style="background:rgba(217,137,137,0.5)"></i>Holidays</span>' +
              '</div>' +
            '</div>' +
            '<div class="calendar-grid">' +
              '<div class="calendar-weekday">Sun</div>' +
              '<div class="calendar-weekday">Mon</div>' +
              '<div class="calendar-weekday">Tue</div>' +
              '<div class="calendar-weekday">Wed</div>' +
              '<div class="calendar-weekday">Thu</div>' +
              '<div class="calendar-weekday">Fri</div>' +
              '<div class="calendar-weekday">Sat</div>' +
              cells.join('') +
            '</div>' +
          '</div>'
        );
      }

      // Build the full 12-month index once for the empty check
      // and the total-days-with-data counter.
      for (var m = 1; m <= 12; m++) {
        var monthData = null;
        for (var i = 0; i < months.length; i++) {
          if (months[i].month === m) { monthData = months[i]; break; }
        }
        var days = (monthData && monthData.days) ? monthData.days : {};
        for (var k in days) {
          if (Object.prototype.hasOwnProperty.call(days, k)) {
            var d = days[k];
            if (d.a + d.e + d.h > 0) totalDaysWithData++;
          }
        }
      }

      // Month selector — dropdown for Jan-Dec + prev/next buttons.
      var monthOpts = '';
      for (var mi = 1; mi <= 12; mi++) {
        monthOpts += '<option value="' + mi + '"' + (mi === monthNum ? ' selected' : '') + '>' + escapeHtml(ARCHIVE_CALENDAR_MONTHS[mi]) + '</option>';
      }
      var selectorHtml = '<div class="calendar-selector">' +
        '<button type="button" class="calendar-nav-btn" id="calendar-prev-month" aria-label="Previous month">←</button>' +
        '<select id="calendar-month-select" class="calendar-month-select">' + monthOpts + '</select>' +
        '<button type="button" class="calendar-nav-btn" id="calendar-next-month" aria-label="Next month">→</button>' +
      '</div>';

      // Render grid for the current month.
      renderMonthGrid(monthNum);

      // Issue #507: link copy must match the destination. The
      // destination page renders bundle.calendar_items.length
      // (the total count of items), not totalDaysWithData
      // (which is the per-month day-cell count -- different
      // number). User reported "View all 294 calendar items
      // doesn't do anything" because the destination showed
      // a different total.
      var totalItems = Array.isArray(bundle.calendar_items) ? bundle.calendar_items.length : 0;
      return '' +
        '<div class="panel-head"><h2>Calendar</h2></div>' +
        '<p class="panel-subtext">Every anniversary and event day in this archive, by month. Click a day to filter Person Records for that date.</p>' +
        selectorHtml +
        '<div class="calendar-items-link"><a href="#/calendar-items" class="action-link">View all ' + totalItems + ' calendar items &rarr;</a></div>' +
        (totalDaysWithData === 0
          ? '<div class="placeholder-card">No anniversaries, events, or holidays recorded in this archive.</div>'
          : '<div id="calendar-month-grid">' + blocks.join('') + '</div>');
    }

    // renderBrowsePage renders the Browse page (filterable Person
    // Record list, issue #498 slice 3). Hash-routed pre-fill via
    // the route query string (e.g. #/browse?entry_type=widow from
    // an Insights drilldown). Search + 5 filter chips + sort +
    // pagination are all client-side against bundle.records[].
    // Per locked decision 2 review_status is dropped (no review
    // queue in a read-only archive) and scope is hidden (always
    // "all" — the archive IS the full snapshot).
    // parseCalendarQuery extracts month from the Calendar route
    // query string (?month=5). Issue #500: default to current month
    // when no query is present.
    function parseCalendarQuery(queryString) {
      var out = {};
      var s = String(queryString || '').replace(/^\?/, '');
      if (!s) return out;
      var parts = s.split('&');
      for (var i = 0; i < parts.length; i++) {
        var eq = parts[i].indexOf('=');
        if (eq < 0) {
          out[decodeURIComponent(parts[i])] = '';
        } else {
          out[decodeURIComponent(parts[i].slice(0, eq))] = decodeURIComponent(parts[i].slice(eq + 1));
        }
      }
      return out;
    }

    function renderBrowsePage(bundle, query) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      return '' +
        '<div class="panel-head"><h2>Filter</h2>' +
        '<span class="browse-count" id="browse-count">' + records.length + ' Person Record' + (records.length === 1 ? '' : 's') + '</span>' +
        '</div>' +
        '<p class="panel-subtext">Search and filter the Person Records. Click any row for the full detail view. Click an Insights card on the Insights page to pre-filter this list.</p>' +
        '<div class="browse-toolbar">' +
          '<div class="browse-search-row">' +
            '<label for="browse-search">Search</label>' +
            '<input id="browse-search" type="search" placeholder="Search names, units, locations, notes…" autocomplete="off" spellcheck="false">' +
          '</div>' +
          '<div class="browse-sort-row">' +
            '<label>Sort by</label>' +
            '<select id="browse-sort">' +
              '<option value="display_id">Display ID</option>' +
              '<option value="name" selected>Last name</option>' +
              '<option value="tag">Tag (alphabetical)</option>' +
              '<option value="last_edited">Last Edited</option>' +
            '</select>' +
          '</div>' +
          '<div class="browse-page-size-row">' +
            '<label>Per page</label>' +
            '<select id="browse-page-size">' +
              '<option value="25" selected>25</option>' +
              '<option value="50">50</option>' +
              '<option value="100">100</option>' +
            '</select>' +
          '</div>' +
        '</div>' +
        '<div class="filter-dropdowns" id="browse-filter-dropdowns">' +
          renderFilterDropdown('entry_type', 'Entry type', records, function(r) { return r.entryType; }) +
          renderFilterDropdown('pension_state', 'Pension state', records, function(r) { return r.pensionState; }) +
          renderFilterDropdown('unit', 'Unit', records, function(r) { return r.unit; }) +
          renderFilterDropdown('buried_in', 'Buried in', records, function(r) { return r.location; }) +
          renderFilterDropdown('confederate_home_status', 'Confederate Home status', records, function(r) { return r.homeStatus; }) +
          buildTagsDropdown(records) +
        '</div>' +
        '<div class="browse-active-filters" id="browse-active-filters"></div>' +
        '<div class="browse-clear-row"><button type="button" class="image-button" id="browse-clear-filters">Clear filters</button></div>' +
        '<div id="browse-prefilter-banner" class="browse-prefilter-banner hidden"></div>' +
        '<div class="results" id="browse-results"></div>' +
        '<div class="browse-pagination" id="browse-pagination"></div>' +
        '<div class="empty-state" id="browse-empty" style="display:none;">No Person Records matched the current filters.</div>';
    }

    // renderFilterDropdown renders one filter dropdown: a native
    // <select multiple> with a count label, one option per distinct
    // value in the records slice. Issue #499: replaces the
    // filter-chip rows that ran way down the page with dropdowns.
    function renderFilterDropdown(field, label, records, getter) {
      var values = {};
      for (var i = 0; i < records.length; i++) {
        var v = String(getter(records[i]) || '').trim();
        if (!v) continue;
        values[v] = (values[v] || 0) + 1;
      }
      var sorted = Object.keys(values).sort(function(a, b) { return a.toLowerCase().localeCompare(b.toLowerCase()); });
      var options = '';
      for (var j = 0; j < sorted.length; j++) {
        var val = sorted[j];
        options += '<option value="' + escapeHtml(val) + '">' + escapeHtml(val) + ' (' + values[val] + ')</option>';
      }
      return '<div class="filter-dropdown" data-filter-group="' + field + '">' +
        '<label class="filter-dropdown-label" for="filter-select-' + field + '">' + escapeHtml(label) + '</label>' +
        '<select multiple id="filter-select-' + field + '" class="filter-select" data-filter-field="' + field + '">' +
          options +
        '</select>' +
      '</div>';
    }

    // buildTagsDropdown collects all distinct tag values across
    // records and renders a multi-select dropdown (issue #506).
    function buildTagsDropdown(records) {
      var values = {};
      for (var i = 0; i < records.length; i++) {
        var tags = Array.isArray(records[i].tags) ? records[i].tags : [];
        for (var j = 0; j < tags.length; j++) {
          var v = String(tags[j] || '').trim();
          if (!v) continue;
          values[v] = (values[v] || 0) + 1;
        }
      }
      var sorted = Object.keys(values).sort(function(a, b) { return a.toLowerCase().localeCompare(b.toLowerCase()); });
      if (sorted.length === 0) return '';
      var options = '';
      for (var j = 0; j < sorted.length; j++) {
        var val = sorted[j];
        options += '<option value="' + escapeHtml(val) + '">' + escapeHtml(val) + ' (' + values[val] + ')</option>';
      }
      return '<div class="filter-dropdown" data-filter-group="tags">' +
        '<label class="filter-dropdown-label">Tags</label>' +
        '<select multiple id="filter-select-tags" class="filter-select" data-filter-field="tags">' +
          options +
        '</select>' +
      '</div>';
    }

    // parseBrowseQuery parses the Browse route query string into
    // an object: {field: value, ...} where value can be a string
    // OR an array for comma-separated multi-select pre-fill.
    // Single-value form (?entry_type=widow) stays as a string so
    // Insights card drilldowns from #498 slice 4 work unchanged.
    // Multi-value form (?entry_type=widow,wife) produces an array
    // per issue #499 locked decision 2.
    function parseBrowseQuery(queryString) {
      var out = {};
      var s = String(queryString || '').replace(/^\?/, '');
      if (!s) return out;
      var parts = s.split('&');
      for (var i = 0; i < parts.length; i++) {
        var eq = parts[i].indexOf('=');
        var key, val;
        if (eq < 0) {
          key = decodeURIComponent(parts[i]);
          val = '';
        } else {
          key = decodeURIComponent(parts[i].slice(0, eq));
          val = decodeURIComponent(parts[i].slice(eq + 1));
        }
        // Split comma-separated values into an array; single
        // value stays as a plain string for back-compat with
        // single-value drilldowns from earlier ship dates.
        var parts2 = val.split(',');
        out[key] = parts2.length > 1 ? parts2 : val;
      }
      return out;
    }

    // getBrowseFilterValues reads the currently-selected values for
    // the given field from the multi-select dropdown. Returns an
    // array; empty array = no filter applied ("all values").
    // Issue #499: replaces the single-value chip read with multi-
    // select for OR-within-field filtering.
    function getBrowseFilterValues(field) {
      var select = document.getElementById('filter-select-' + field);
      if (!select) return [];
      var out = [];
      for (var i = 0; i < select.options.length; i++) {
        if (select.options[i].selected) {
          out.push(select.options[i].value);
        }
      }
      return out;
    }

    // applyBrowseFilters reads the search input + every dropdown's
    // selected values + the sort + page size + current page, applies
    // them to bundle.records[], and re-renders the list.
    // Multi-select within a field = OR, across fields = AND (issue #499).
    // Also renders the active-filters removable chips above the results.
    function applyBrowseFilters(bundle, prefill) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      var searchInput = document.getElementById('browse-search');
      var sortSelect = document.getElementById('browse-sort');
      var pageSizeSelect = document.getElementById('browse-page-size');
      if (!searchInput || !sortSelect || !pageSizeSelect) return;
      var query = String(searchInput.value || '').trim().toLowerCase();
      var sort = sortSelect.value || 'name';
      var pageSize = Number(pageSizeSelect.value) || 25;

      // Gather active filter values for the removable-chips display.
      var activeFilters = [];

      var filtered = records.filter(function(r) {
        if (query && !matchesSearch(r, query)) return false;
        for (var fi = 0; fi < BROWSE_FILTER_FIELDS.length; fi++) {
          var field = BROWSE_FILTER_FIELDS[fi];
          var wants;
          if (prefill && prefill[field] !== undefined) {
            wants = prefill[field];
            if (typeof wants === 'string') wants = [wants];
            if (!Array.isArray(wants)) wants = [];
          } else {
            wants = getBrowseFilterValues(field);
          }
          if (wants.length === 0) continue; // no filter for this field
          var got = '';
          if (field === 'entry_type') got = r.entryType;
          else if (field === 'pension_state') got = r.pensionState;
          else if (field === 'unit') got = r.unit;
          else if (field === 'buried_in') got = r.location;
          else if (field === 'confederate_home_status') got = r.homeStatus;
          else if (field === 'tags') {
            var recordTags = Array.isArray(r.tags) ? r.tags : [];
            var tagsMatch = false;
            for (var w = 0; w < wants.length; w++) {
              if (recordTags.indexOf(wants[w]) >= 0) { tagsMatch = true; break; }
            }
            if (!tagsMatch) return false;
            continue;
          }
          var gotStr = String(got || '').trim();
          // OR within field: match if any selected value matches.
          var match = false;
          for (var w = 0; w < wants.length; w++) {
            if (gotStr === wants[w]) { match = true; break; }
          }
          if (!match) return false;
        }
        if (prefill && prefill.date) {
          // Issue #510: r.deathDate is dates.Display() output
          // (e.g. "May 12, 1865"), not "MM/DD/YYYY". Parse the
          // leading month abbreviation + day from the display
          // string, build a "MM-DD" token, and compare to the
          // URL's prefill (also "MM-DD"). Partial dates
          // (year-only "1865", or "May 1865" with no day) cannot
          // match a "MM-DD" token and so correctly return false.
          var date = prefill.date;
          var dStr = String(r.deathDate || '').trim();
          var matchMmdd = '';
          if (dStr.length >= 4) {
            var mAbbr = String(dStr.slice(0, 3)).toLowerCase();
            var mNum = ARCHIVE_MONTH_ABBREV_TO_NUM[mAbbr];
            if (mNum) {
              var dayStart = dStr.indexOf(' ') + 1;
              var dayEnd = dStr.indexOf(',', dayStart);
              if (dayEnd < 0) dayEnd = dStr.length;
              var dNum = parseInt(dStr.slice(dayStart, dayEnd), 10);
              if (dNum >= 1 && dNum <= 31) {
                matchMmdd = (mNum < 10 ? '0' : '') + mNum + '-' +
                            (dNum < 10 ? '0' : '') + dNum;
              }
            }
          }
          if (matchMmdd !== date) return false;
        }
        return true;
      });

      filtered.sort(function(a, b) {
        var ak = '', bk = '';
        if (sort === 'display_id') { ak = a.displayId || ''; bk = b.displayId || ''; }
        else if (sort === 'name') { ak = (a.name || '').toLowerCase(); bk = (b.name || '').toLowerCase(); }
        else if (sort === 'tag') { ak = (Array.isArray(a.tags) && a.tags.length ? a.tags[0] : '').toLowerCase(); bk = (Array.isArray(b.tags) && b.tags.length ? b.tags[0] : '').toLowerCase(); }
        else { ak = a.lastEditedAt || ''; bk = b.lastEditedAt || ''; }
        if (ak < bk) return 1;
        if (ak > bk) return -1;
        return 0;
      });

      var totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
      if (browseState.page < 1) browseState.page = 1;
      if (browseState.page > totalPages) browseState.page = totalPages;
      var startIdx = (browseState.page - 1) * pageSize;
      var pageItems = filtered.slice(startIdx, startIdx + pageSize);

      // Render removable active-filter chips above the results.
      var activeEl = document.getElementById('browse-active-filters');
      if (activeEl) {
        if (prefill) {
          // Collect from prefill object for chips display.
          for (var pfk in prefill) {
            if (Object.prototype.hasOwnProperty.call(prefill, pfk) && pfk !== 'date') {
              var pv = prefill[pfk];
              if (Array.isArray(pv)) {
                for (var pi = 0; pi < pv.length; pi++) {
                  activeFilters.push({field: pfk, value: pv[pi]});
                }
              } else if (pv) {
                activeFilters.push({field: pfk, value: pv});
              }
            }
          }
        } else {
          for (var fi2 = 0; fi2 < BROWSE_FILTER_FIELDS.length; fi2++) {
            var f2 = BROWSE_FILTER_FIELDS[fi2];
            var vals = getBrowseFilterValues(f2);
            for (var vi = 0; vi < vals.length; vi++) {
              activeFilters.push({field: f2, value: vals[vi]});
            }
          }
        }
        if (activeFilters.length) {
          var chipHtml = '';
          for (var ai = 0; ai < activeFilters.length; ai++) {
            var af = activeFilters[ai];
            var fLabel = af.field.replace(/_/g, ' ');
            chipHtml += '<span class="filter-removable-chip" data-remove-filter="' + af.field + '" data-remove-value="' + escapeHtml(af.value) + '">' + escapeHtml(af.value) + ' <button type="button" class="filter-remove-btn" aria-label="Remove ' + escapeHtml(af.value) + ' filter">✕</button></span>';
          }
          activeEl.innerHTML = '<span class="filter-active-label">Active filters:</span> ' + chipHtml;
        } else {
          activeEl.innerHTML = '';
        }
      }

      var results = document.getElementById('browse-results');
      if (results) {
        var rowsHtml = '';
        for (var i = 0; i < pageItems.length; i++) {
          rowsHtml += renderRecord(pageItems[i], startIdx + i, records);
        }
        results.innerHTML = rowsHtml;
      }
      var pagination = document.getElementById('browse-pagination');
      if (pagination) {
        var pHtml = '<button type="button" class="action-button" id="browse-prev"' + (browseState.page <= 1 ? ' disabled' : '') + '>← Prev</button>';
        pHtml += '<span class="browse-page-indicator">Page ' + browseState.page + ' of ' + totalPages + '</span>';
        pHtml += '<button type="button" class="action-button" id="browse-next"' + (browseState.page >= totalPages ? ' disabled' : '') + '>Next →</button>';
        pagination.innerHTML = pHtml;
      }
      var count = document.getElementById('browse-count');
      if (count) count.textContent = filtered.length + ' of ' + records.length + ' Person Record' + (records.length === 1 ? '' : 's');
      var empty = document.getElementById('browse-empty');
      if (empty) empty.style.display = filtered.length ? 'none' : 'block';
    }

    // BROWSE_FILTER_FIELDS is the canonical filter list per
    // locked decision 2: 5 chips. Review status dropped (no review
    // queue in read-only archive); scope hidden (always "all").
    var BROWSE_FILTER_FIELDS = ['entry_type', 'pension_state', 'unit', 'buried_in', 'confederate_home_status', 'tags'];

    // browseState holds the current page index for Browse pagination.
    var browseState = { page: 1 };

    // renderInsightsPage renders the Insights page (analytics
    // snapshot, issue #498 slice 4). Reads bundle.insights (the
    // AnalyticsService.Snapshot() payload) and renders 8 cards:
    // record_types + cemetery_density + confederate_home_status +
    // confederate_home_names + pension_distribution +
    // unit_representation + birth_decade_distribution +
    // death_decade_distribution. Each card's per-row entry links
    // to #/browse?{field}={value} per locked decision 3 (hash-routed
    // Browse pre-fill).
    function renderInsightsPage(bundle) {
      var insights = (bundle && bundle.insights) ? bundle.insights : null;
      var html = '' +
        '<div class="panel-head"><h2>Insights</h2></div>' +
        '<p class="panel-subtext">A pre-computed snapshot of the archive\'s analytics — Person Record Types, top cemeteries, Confederate Home status, pension distribution, top units, and birth/death decades. Click any entry to filter Person Records with that filter pre-filled.</p>' +
        '<div class="insights-grid">';
      if (!insights) {
        html += '<div class="placeholder-card">No insights snapshot available in this archive.</div>';
      } else {
        html += renderRecordTypesCard(insights.record_types);
        html += renderInsightsCountCard('Top cemeteries', 'cemetery_density', insights.cemetery_density, 'buried_in', 'No cemetery data');
        html += renderInsightsCountCard('Confederate Home status', 'confederate_home_status', insights.confederate_home_status, 'confederate_home_status', 'No Confederate Home status data');
        html += renderInsightsCountCard('Pension distribution', 'pension_distribution', insights.pension_distribution, 'pension_state', 'No pension state data');
        html += renderInsightsCountCard('Top units', 'unit_representation', insights.unit_representation, 'unit', 'No unit data');
        html += renderDecadeCard('Birth decade distribution', 'birth_decade_distribution', insights.birth_decade_distribution);
        html += renderDecadeCard('Death decade distribution', 'death_decade_distribution', insights.death_decade_distribution);
      }
      // Issue #506: tag distribution computed from bundle.records.
      html += renderTagDistributionCard(bundle);
      html += '</div>';
      return html;
    }

    // renderRecordTypesCard renders the headline Person Record
    // Type snapshot (soldiers / wives+widows / linked people).
    // No drilldown here — the counts are an at-a-glance rollup;
    // the per-type drilldown lives on the Browse page itself.
    function renderRecordTypesCard(rt) {
      if (!rt) return '';
      var items = [
        ['Soldiers', rt.total_soldiers],
        ['Wives + widows', rt.total_wives_widows],
        ['Linked people', rt.total_linked_people]
      ];
      var rows = '';
      for (var i = 0; i < items.length; i++) {
        rows += '<li><strong>' + escapeHtml(items[i][0]) + ':</strong> ' + items[i][1] + '</li>';
      }
      return '<section class="insight-card"><h3>Person Record Types</h3><ul>' + rows + '</ul></section>';
    }

    // renderInsightsCountCard renders one count-table card (label +
    // rows of {label, count} with each row's label linking to
    // #/browse?{browseField}={value}). The browseField maps the
    // analytics dimension key to the Browse page filter key:
    // cemetery_density uses buried_in, etc.
    function renderInsightsCountCard(title, field, rows, browseField, emptyText) {
      if (!Array.isArray(rows) || !rows.length) {
        return '<section class="insight-card"><h3>' + escapeHtml(title) + '</h3><p class="insight-empty">' + escapeHtml(emptyText) + '.</p></section>';
      }
      var html = '<section class="insight-card"><h3>' + escapeHtml(title) + '</h3><table class="insight-table">';
      for (var i = 0; i < rows.length; i++) {
        var label = String(rows[i].label || '');
        var count = Number(rows[i].count || 0);
        if (!label) continue;
        var hash = '#/browse?' + browseField + '=' + encodeURIComponent(label);
        html += '<tr><td><a class="record-link" href="' + hash + '">' + escapeHtml(label) + '</a></td><td class="insight-count">' + count + '</td></tr>';
      }
      html += '</table></section>';
      return html;
    }

    // renderDecadeCard renders a decade-distribution card with a
    // compact horizontal bar chart. Each decade links to a Browse
    // drilldown filtered by birth/death year range (slice 4 ships
    // the card; the year-range drilldown could be a slice-5 follow-up).
    function renderDecadeCard(title, field, rows) {
      if (!Array.isArray(rows) || !rows.length) {
        return '<section class="insight-card"><h3>' + escapeHtml(title) + '</h3><p class="insight-empty">No decade data.</p></section>';
      }
      var maxCount = 0;
      for (var i = 0; i < rows.length; i++) {
        if (Number(rows[i].count) > maxCount) maxCount = Number(rows[i].count);
      }
      var html = '<section class="insight-card"><h3>' + escapeHtml(title) + '</h3><div class="insight-decade-chart">';
      for (var j = 0; j < rows.length; j++) {
        var label = String(rows[j].label || '');
        var count = Number(rows[j].count || 0);
        var pct = maxCount > 0 ? Math.round((count / maxCount) * 100) : 0;
        html += '<div class="insight-decade-bar">' +
          '<span class="insight-decade-label">' + escapeHtml(label) + '</span>' +
          '<span class="insight-decade-track"><span class="insight-decade-fill" style="width:' + pct + '%"></span></span>' +
          '<span class="insight-decade-count">' + count + '</span>' +
        '</div>';
      }
      html += '</div></section>';
      return html;
    }

    // renderTagDistributionCard computes tag distribution from
    // bundle.records and renders an Insights card (issue #506).
    function renderTagDistributionCard(bundle) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      var tagCounts = {};
      for (var i = 0; i < records.length; i++) {
        var tags = Array.isArray(records[i].tags) ? records[i].tags : [];
        for (var j = 0; j < tags.length; j++) {
          var t = String(tags[j] || '').trim();
          if (!t) continue;
          tagCounts[t] = (tagCounts[t] || 0) + 1;
        }
      }
      var sorted = Object.keys(tagCounts).sort(function(a, b) { return tagCounts[b] - tagCounts[a] || a.toLowerCase().localeCompare(b.toLowerCase()); });
      if (!sorted.length) {
        return '<section class="insight-card"><h3>Tag distribution</h3><p class="insight-empty">No tags in this archive.</p></section>';
      }
      var rows = '';
      for (var k = 0; k < sorted.length; k++) {
        var tag = sorted[k];
        rows += '<tr><td><a href="#/browse?tags=' + encodeURIComponent(tag) + '" class="insight-link">' + escapeHtml(tag) + '</a></td><td class="insight-count">' + tagCounts[tag] + '</td></tr>';
      }
      return '<section class="insight-card"><h3>Tag distribution</h3><table class="insight-table"><tbody>' + rows + '</tbody></table></section>';
    }

    // renderCalendarItemsPage renders the Calendar items list
    // (issue #502). Shows every calendar_items row in a table
    // with item_type filter chips, sorted by month+day.
    function renderCalendarItemsPage(bundle) {
      var items = Array.isArray(bundle.calendar_items) ? bundle.calendar_items : [];
      var rows = '';
      for (var i = 0; i < items.length; i++) {
        var it = items[i];
        var monthName = ARCHIVE_CALENDAR_MONTHS[it.month] || String(it.month);
        rows += '<tr>' +
          '<td class="ci-date">' + escapeHtml(monthName) + ' ' + it.day + '</td>' +
          '<td class="ci-type"><span class="pill ci-' + escapeHtml(it.itemType) + '">' + escapeHtml(it.itemType) + '</span></td>' +
          '<td class="ci-title">' + escapeHtml(it.title) + '</td>' +
          '<td class="ci-notes">' + (it.notes ? escapeHtml(it.notes) : '') + '</td>' +
          '</tr>';
      }
      return '' +
        '<div class="panel-head"><h2>Calendar Items</h2></div>' +
        '<p class="panel-subtext">' + items.length + ' calendar item' + (items.length === 1 ? '' : 's') + ' (holidays, anniversaries, events). Sorted by month and day.</p>' +
        (items.length === 0
          ? '<div class="placeholder-card">No calendar items in this archive.</div>'
          : '<div class="ci-table-wrap"><table class="ci-table">' +
            '<thead><tr><th>Date</th><th>Type</th><th>Title</th><th>Notes</th></tr></thead>' +
            '<tbody>' + rows + '</tbody></table></div>');
    }

    // renderPersonsPage / renderEventsPage / renderArticlesPage are
    // the legacy list screens from #320 / #490, re-routed through
    // the new hash router. Each renders its list (no search input
    // in the hero — search lives on Browse, slice 3) plus a detail
    // view triggered by a row click.
    function renderPersonsPage(bundle) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      var html =
        '<div class="panel-head"><h2>View All</h2></div>' +
        '<p class="panel-subtext">' + records.length + ' Person Record' + (records.length === 1 ? '' : 's') + ' in this archive. Click any row for the full detail view.</p>' +
        '<div class="results">';
      for (var i = 0; i < records.length; i++) {
        html += renderRecord(records[i], i, records);
      }
      if (!records.length) {
        html += '<div class="placeholder-card">No Person Records in this archive.</div>';
      }
      html += '</div>';
      return html;
    }

    function renderEventsPage(bundle) {
      var events = Array.isArray(bundle.events) ? bundle.events : [];
      var html =
        '<div class="panel-head"><h2>Events</h2></div>' +
        '<p class="panel-subtext">' + events.length + ' Event Record' + (events.length === 1 ? '' : 's') + ' in this archive. Click any row for the full detail view.</p>' +
        '<div class="results">';
      for (var i = 0; i < events.length; i++) {
        html += renderEventRow(events[i], i);
      }
      if (!events.length) {
        html += '<div class="placeholder-card">No Event Records in this archive.</div>';
      }
      html += '</div>';
      return html;
    }

    function renderArticlesPage(bundle) {
      var articles = Array.isArray(bundle.articles) ? bundle.articles : [];
      var html =
        '<div class="panel-head"><h2>Articles</h2></div>' +
        '<p class="panel-subtext">' + articles.length + ' Article' + (articles.length === 1 ? '' : 's') + ' in this archive. Click any row for the full detail view.</p>' +
        '<div class="results">';
      for (var i = 0; i < articles.length; i++) {
        html += renderArticleRow(articles[i], i);
      }
      if (!articles.length) {
        html += '<div class="placeholder-card">No Articles in this archive.</div>';
      }
      html += '</div>';
      return html;
    }

    function findRecordByDisplayId(records, id) {
      if (!Array.isArray(records) || !id) return -1;
      for (var i = 0; i < records.length; i++) {
        if (records[i].displayId === id) return i;
      }
      return -1;
    }
    function findEventById(events, id) {
      if (!Array.isArray(events) || !id) return -1;
      for (var i = 0; i < events.length; i++) {
        if (String(events[i].displayId || events[i].id || '') === id) return i;
      }
      return -1;
    }
    function findArticleById(articles, id) {
      if (!Array.isArray(articles) || !id) return -1;
      for (var i = 0; i < articles.length; i++) {
        if (String(articles[i].id || '') === id) return i;
      }
      return -1;
    }

    const imagePreviewState = {
      scale: 1,
      x: 0,
      y: 0,
      dragging: false,
      pointerId: null,
      startX: 0,
      startY: 0,
      originX: 0,
      originY: 0
    };

    function clampImagePosition() {
      const stage = document.getElementById('image-preview-stage');
      const image = document.getElementById('image-overlay-img');
      if (!stage || !image) {
        return;
      }
      const maxX = Math.max(0, (image.offsetWidth * imagePreviewState.scale - stage.clientWidth) / 2);
      const maxY = Math.max(0, (image.offsetHeight * imagePreviewState.scale - stage.clientHeight) / 2);
      imagePreviewState.x = Math.min(maxX, Math.max(-maxX, imagePreviewState.x));
      imagePreviewState.y = Math.min(maxY, Math.max(-maxY, imagePreviewState.y));
    }

    function applyImageTransform() {
      const image = document.getElementById('image-overlay-img');
      if (!image) {
        return;
      }
      clampImagePosition();
      image.style.transform = 'translate(' + imagePreviewState.x + 'px, ' + imagePreviewState.y + 'px) scale(' + imagePreviewState.scale + ')';
    }

    function resetImageTransform() {
      imagePreviewState.scale = 1;
      imagePreviewState.x = 0;
      imagePreviewState.y = 0;
      imagePreviewState.dragging = false;
      imagePreviewState.pointerId = null;
      const stage = document.getElementById('image-preview-stage');
      if (stage) {
        stage.classList.remove('dragging');
      }
      applyImageTransform();
    }

    function openImagePreview(path, title) {
      const overlay = document.getElementById('image-overlay');
      const image = document.getElementById('image-overlay-img');
      const heading = document.getElementById('image-overlay-title');
      resetImageTransform();
      image.src = path;
      image.alt = title || 'Archive image preview';
      heading.textContent = title || 'Image Preview';
      overlay.classList.add('open');
      overlay.setAttribute('aria-hidden', 'false');
      image.onload = function() {
        resetImageTransform();
      };
    }

    function closeImagePreview() {
      const overlay = document.getElementById('image-overlay');
      const image = document.getElementById('image-overlay-img');
      overlay.classList.remove('open');
      overlay.setAttribute('aria-hidden', 'true');
      image.removeAttribute('src');
      image.onload = null;
      resetImageTransform();
    }

    document.addEventListener('DOMContentLoaded', function() {
      const bundle = (window.DIXIE_DATA && typeof window.DIXIE_DATA === 'object') ? window.DIXIE_DATA : {};
      const records = Array.isArray(bundle.records) ? bundle.records : [];
      const events = Array.isArray(bundle.events) ? bundle.events : [];
      const articles = Array.isArray(bundle.articles) ? bundle.articles : [];
      const calendar = Array.isArray(bundle.calendar) ? bundle.calendar : [];
      window.__DIXIE_EVENTS__ = events;
      window.__DIXIE_RECORDS__ = records;
      window.__DIXIE_ARTICLES__ = articles;
      window.__DIXIE_BUNDLE__ = bundle;
      const previewStage = document.getElementById('image-preview-stage');

      // Hide empty-entity nav links: no Events tab if bundle.events
      // is empty, no Articles tab if bundle.articles is empty. The
      // Calendar landing always renders (the calendar array is
      // always 12 months per locked decision 1).
      if (!events.length) {
        var evLink = document.querySelector('.nav-link-events');
        if (evLink) evLink.classList.add('hidden');
      }
      if (!articles.length) {
        var artLink = document.querySelector('.nav-link-articles');
        if (artLink) artLink.classList.add('hidden');
      }

      // Route dispatcher. Called on initial load + every hashchange.
      // Reads routeFromHash() and dispatches to the page renderer
      // or the detail screen.
      function syncViewFromHash() {
        const route = routeFromHash(window.location.hash);
        // Issue #509: #/print/{displayId} replaces the document
        // body with a printable report DOM, then auto-fires
        // window.print() so the user can save to PDF without
        // clicking through the print dialog manually.
        if (route.kind === 'print') {
          var printIdx = findRecordByDisplayId(records, route.id);
          if (printIdx >= 0) {
            document.body.classList.add('print-mode');
            document.title = 'Report — ' + records[printIdx].name + ' (' + records[printIdx].displayId + ')';
            var landscape = false;
            try { landscape = window.location.search.indexOf('landscape=1') >= 0; } catch (e) {}
            document.body.innerHTML = renderPrintableReport(records[printIdx], {
              archiveTitle: typeof ARCHIVE_TITLE !== 'undefined' ? ARCHIVE_TITLE : 'DixieData Archive',
              footerText: typeof FOOTER_TEXT !== 'undefined' ? FOOTER_TEXT : 'Made with DixieData',
              codename: typeof CODENAME !== 'undefined' ? CODENAME : '',
            }, landscape);
            // Defer print until after the layout settles.
            setTimeout(function() { try { window.print(); } catch (e) {} }, 200);
            window.scrollTo(0, 0);
            return;
          }
          document.body.classList.add('print-mode');
          document.body.innerHTML = '<div class="print-root"><p>Person Record "' + escapeHtml(route.id) + '" not found in this archive.</p></div>';
          return;
        }
        if (route.kind === 'detail') {
          showDetailScreen();
          updateNavActive(route.entity === 'person' ? 'persons' : (route.entity === 'event' ? 'events' : 'articles'));
          var content = document.getElementById('detail-content');
          var pos = document.getElementById('detail-position');
          if (route.entity === 'person') {
            var idx = findRecordByDisplayId(records, route.id);
            if (idx >= 0) {
              content.innerHTML = renderDetail(records[idx], records, events);
              pos.textContent = 'Person Record';
              var reportBtn = document.getElementById('detail-export-report');
              if (reportBtn) {
                // Issue #509: client-side printable view. The
                // button opens a new tab pointing at the archive
                // root with the #/print/{displayId} hash; the new
                // tab loads the same bundle, the router dispatches
                // to renderPrintableReport, and the document body
                // is replaced with the printable DOM. No static
                // report-{displayId}.html file ships in the .zip.
                reportBtn.href = 'index.html#/print/' + encodeURIComponent(records[idx].displayId);
                reportBtn.style.display = '';
              }
            } else {
              content.innerHTML = '<p>Person Record "' + escapeHtml(route.id) + '" not found in this archive.</p>';
              pos.textContent = 'Not Found';
            }
          } else if (route.entity === 'event') {
            var evIdx = findEventById(events, route.id);
            if (evIdx >= 0) {
              content.innerHTML = renderEventDetail(events[evIdx], records);
              pos.textContent = 'Event';
              var reportBtn = document.getElementById('detail-export-report');
              if (reportBtn) reportBtn.style.display = 'none';
            } else {
              content.innerHTML = '<p>Event "' + escapeHtml(route.id) + '" not found in this archive.</p>';
              pos.textContent = 'Not Found';
            }
          } else if (route.entity === 'article') {
            var artIdx = findArticleById(articles, route.id);
            if (artIdx >= 0) {
              content.innerHTML = renderArticleDetail(articles[artIdx]);
              pos.textContent = 'Article';
            } else {
              content.innerHTML = '<p>Article "' + escapeHtml(route.id) + '" not found in this archive.</p>';
              pos.textContent = 'Not Found';
            }
          }
          window.scrollTo({ top: 0, behavior: 'smooth' });
          return;
        }
        showPageScreen();
        updateNavActive(route.name);
        var html = '';
        switch (route.name) {
          case 'calendar': html = renderCalendarPage(bundle, route.query); break;
          case 'calendar-items': html = renderCalendarItemsPage(bundle); break;
          case 'browse':
            html = renderBrowsePage(bundle, route.query);
            setPageHtml(html);
            // Issue #498 slice 3: pre-fill chips from the hash query
            // (Insights drilldown target), wire chip + search +
            // sort + page-size + pagination listeners, then render.
            var prefill = parseBrowseQuery(route.query);
            applyBrowsePrefillSelects(prefill);
            renderBrowsePrefillBanner(prefill);
            browseState.page = 1;
            applyBrowseFilters(bundle, prefill);
            wireBrowseListeners(bundle);
            window.scrollTo({ top: 0, behavior: 'smooth' });
            return;
          case 'insights': html = renderInsightsPage(bundle); break;
          case 'persons': html = renderPersonsPage(bundle); break;
          case 'events': html = renderEventsPage(bundle); break;
          case 'articles': html = renderArticlesPage(bundle); break;
          default: html = renderCalendarPage(bundle, '');
        }
        setPageHtml(html);
        window.scrollTo({ top: 0, behavior: 'smooth' });

        // Wire Calendar month-selector listeners (issue #500).
        if (route.name === 'calendar') {
          var monthSel = document.getElementById('calendar-month-select');
          if (monthSel) {
            monthSel.addEventListener('change', function() {
              window.location.hash = '#/calendar?month=' + monthSel.value;
            });
          }
          var prevBtn = document.getElementById('calendar-prev-month');
          var nextBtn = document.getElementById('calendar-next-month');
          if (prevBtn) {
            prevBtn.addEventListener('click', function() {
              var sel = document.getElementById('calendar-month-select');
              if (!sel) return;
              var v = Number(sel.value) || 1;
              if (v > 1) window.location.hash = '#/calendar?month=' + (v - 1);
            });
          }
          if (nextBtn) {
            nextBtn.addEventListener('click', function() {
              var sel = document.getElementById('calendar-month-select');
              if (!sel) return;
              var v = Number(sel.value) || 12;
              if (v < 12) window.location.hash = '#/calendar?month=' + (v + 1);
            });
          }
        }
      }

      // applyBrowsePrefillSelects selects the dropdown options whose
      // values match the prefill object from the route query.
      // Handles both single-value (string) and multi-value (array)
      // per issue #499 locked decision 2.
      function applyBrowsePrefillSelects(prefill) {
        if (!prefill) return;
        for (var key in prefill) {
          if (Object.prototype.hasOwnProperty.call(prefill, key)) {
            // date= handled separately (banner, not dropdown)
            if (key === 'date') continue;
            var vals = prefill[key];
            if (typeof vals === 'string') vals = [vals];
            if (!Array.isArray(vals)) continue;
            var select = document.getElementById('filter-select-' + key);
            if (!select) continue;
            for (var vi = 0; vi < select.options.length; vi++) {
              select.options[vi].selected = vals.indexOf(select.options[vi].value) >= 0;
            }
          }
        }
      }

      // cssEscape escapes a string for use as a CSS attribute-selector
      // value. Quotes the string and escapes backslash + quote chars
      // per the CSS Attribute Selectors Level 4 spec.
      function cssEscape(s) {
        return String(s || '').replace(/\\/g, '\\\\').replace(/"/g, '\\"');
      }

      // renderBrowsePrefillBanner shows a small banner above the
      // results when the Browse page was opened via a drilldown
      // (Insights card or Calendar day cell). The banner has a
      // "Clear filter" button that resets to the unfiltered list.
      function renderBrowsePrefillBanner(prefill) {
        var banner = document.getElementById('browse-prefilter-banner');
        if (!banner) return;
        if (!prefill || Object.keys(prefill).length === 0) {
          banner.classList.add('hidden');
          banner.innerHTML = '';
          return;
        }
        var chips = [];
        for (var k in prefill) {
          if (Object.prototype.hasOwnProperty.call(prefill, k)) {
            var label = k.replace(/_/g, ' ');
            chips.push('<span class="pill">' + escapeHtml(label) + ': <strong>' + escapeHtml(prefill[k]) + '</strong></span>');
          }
        }
        banner.innerHTML = '<strong>Filtered by:</strong> ' + chips.join(' ') +
          ' <button type="button" class="image-button" id="browse-clear-prefilter">Clear filter</button>';
        banner.classList.remove('hidden');
      }

      // wireBrowseListeners attaches dropdown change / search /
      // sort / page-size / pagination / clear-filters / removable-
      // chip click handlers. Called once per Browse mount per
      // issue #499.
      function wireBrowseListeners(bundle) {
        // Wire up multi-select dropdown change handlers.
        for (var fi = 0; fi < BROWSE_FILTER_FIELDS.length; fi++) {
          var sel = document.getElementById('filter-select-' + BROWSE_FILTER_FIELDS[fi]);
          if (sel) {
            sel.addEventListener('change', function() {
              browseState.page = 1;
              applyBrowseFilters(bundle, null);
            });
          }
        }
        // Removable chip click — deselected the matching option.
        var activeEl = document.getElementById('browse-active-filters');
        if (activeEl) {
          activeEl.addEventListener('click', function(e) {
            var chip = e.target.closest('[data-remove-filter]');
            if (!chip) return;
            var field = chip.getAttribute('data-remove-filter');
            var value = chip.getAttribute('data-remove-value');
            var select = document.getElementById('filter-select-' + field);
            if (!select) return;
            for (var oi = 0; oi < select.options.length; oi++) {
              if (select.options[oi].value === value) {
                select.options[oi].selected = false;
                break;
              }
            }
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        }
        var searchInput = document.getElementById('browse-search');
        if (searchInput) {
          searchInput.addEventListener('input', function() {
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        }
        var sortSelect = document.getElementById('browse-sort');
        if (sortSelect) {
          sortSelect.addEventListener('change', function() {
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        }
        var pageSizeSelect = document.getElementById('browse-page-size');
        if (pageSizeSelect) {
          pageSizeSelect.addEventListener('change', function() {
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        }
        var prevBtn = document.getElementById('browse-prev');
        var nextBtn = document.getElementById('browse-next');
        if (prevBtn) {
          prevBtn.addEventListener('click', function() {
            if (browseState.page > 1) {
              browseState.page--;
              applyBrowseFilters(bundle, null);
            }
          });
        }
        if (nextBtn) {
          nextBtn.addEventListener('click', function() {
            browseState.page++;
            applyBrowseFilters(bundle, null);
          });
        }
        var clearBtn = document.getElementById('browse-clear-prefilter');
        if (clearBtn) {
          clearBtn.addEventListener('click', function() {
            window.location.hash = '#/browse';
          });
        }
        var clearFiltersBtn = document.getElementById('browse-clear-filters');
        if (clearFiltersBtn) {
          clearFiltersBtn.addEventListener('click', function() {
            for (var fi2 = 0; fi2 < BROWSE_FILTER_FIELDS.length; fi2++) {
              var sel = document.getElementById('filter-select-' + BROWSE_FILTER_FIELDS[fi2]);
              if (sel) {
                for (var oi = 0; oi < sel.options.length; oi++) {
                  sel.options[oi].selected = false;
                }
              }
            }
            var search = document.getElementById('browse-search');
            if (search) search.value = '';
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        }
      }

      syncViewFromHash();

      document.addEventListener('click', function(event) {
        const viewButton = event.target.closest('[data-view-record]');
        if (viewButton) {
          const index = Number(viewButton.getAttribute('data-view-record'));
          if (!Number.isNaN(index) && records[index]) {
            window.location.hash = detailHash(records[index]);
          }
          return;
        }

        const viewEventButton = event.target.closest('[data-view-event]');
        if (viewEventButton) {
          const index = Number(viewEventButton.getAttribute('data-view-event'));
          if (!Number.isNaN(index) && events[index]) {
            window.location.hash = '#/event/' + encodeURIComponent(String(events[index].displayId || events[index].id || ''));
          }
          return;
        }

        const viewArticleButton = event.target.closest('[data-view-article]');
        if (viewArticleButton) {
          const index = Number(viewArticleButton.getAttribute('data-view-article'));
          if (!Number.isNaN(index) && articles[index]) {
            window.location.hash = '#/article/' + encodeURIComponent(String(articles[index].id || ''));
          }
          return;
        }

        if (event.target.id === 'detail-back') {
          window.location.hash = '#/calendar';
          return;
        }

        const previewButton = event.target.closest('[data-preview-image]');
        if (previewButton) {
          openImagePreview(
            previewButton.getAttribute('data-preview-image'),
            previewButton.getAttribute('data-preview-title')
          );
          return;
        }

        if (event.target.id === 'image-overlay' || event.target.id === 'image-overlay-close') {
          closeImagePreview();
        }
      });

      document.addEventListener('keydown', function(event) {
        if (event.key === 'Escape') {
          closeImagePreview();
        }
      });

      if (previewStage) {
        previewStage.addEventListener('wheel', function(event) {
          event.preventDefault();
          const nextScale = imagePreviewState.scale + (event.deltaY < 0 ? 0.15 : -0.15);
          imagePreviewState.scale = Math.min(5, Math.max(1, nextScale));
          if (imagePreviewState.scale === 1) {
            imagePreviewState.x = 0;
            imagePreviewState.y = 0;
          }
          applyImageTransform();
        }, { passive: false });

        previewStage.addEventListener('pointerdown', function(event) {
          if (event.button !== 0) {
            return;
          }
          imagePreviewState.dragging = true;
          imagePreviewState.pointerId = event.pointerId;
          imagePreviewState.startX = event.clientX;
          imagePreviewState.startY = event.clientY;
          imagePreviewState.originX = imagePreviewState.x;
          imagePreviewState.originY = imagePreviewState.y;
          previewStage.classList.add('dragging');
          previewStage.setPointerCapture(event.pointerId);
        });

        previewStage.addEventListener('pointermove', function(event) {
          if (!imagePreviewState.dragging || imagePreviewState.pointerId !== event.pointerId) {
            return;
          }
          imagePreviewState.x = imagePreviewState.originX + (event.clientX - imagePreviewState.startX);
          imagePreviewState.y = imagePreviewState.originY + (event.clientY - imagePreviewState.startY);
          applyImageTransform();
        });

        function stopPreviewDrag(event) {
          if (imagePreviewState.pointerId !== null && event.pointerId === imagePreviewState.pointerId) {
            previewStage.releasePointerCapture(event.pointerId);
          }
          imagePreviewState.dragging = false;
          imagePreviewState.pointerId = null;
          previewStage.classList.remove('dragging');
        }

        previewStage.addEventListener('pointerup', stopPreviewDrag);
        previewStage.addEventListener('pointercancel', stopPreviewDrag);
        previewStage.addEventListener('dblclick', function() {
          resetImageTransform();
        });
      }

      window.addEventListener('hashchange', syncViewFromHash);
      window.addEventListener('resize', applyImageTransform);});  </script>
</body>
</html>
`

// --- (e *ExportService) staticArchiveOwner ---
func (e *ExportService) staticArchiveOwner() (staticArchiveOwner, error) {
	identity, err := e.db.UserIdentity()
	if err != nil {
		return staticArchiveOwner{}, err
	}
	displayName := strings.TrimSpace(identity.BrandingName())
	if displayName == "" {
		return staticArchiveOwner{}, fmt.Errorf("user identity is incomplete")
	}
	fileStem := sanitizeStaticArchiveStem(strings.ReplaceAll(displayName, ". ", ""))
	if fileStem == "" {
		return staticArchiveOwner{}, fmt.Errorf("user identity is incomplete")
	}
	return staticArchiveOwner{
		DisplayName: displayName,
		FileStem:    fileStem,
	}, nil
}


// --- (e *ExportService) staticArchiveRecords ---
// staticArchiveRecords returns the Person Records (soldiers,
// wives, widows, linked_persons) for the static archive JSON
// bundle. Event Record rows are filtered out and returned via
// staticArchiveEvents instead per issue #320 child #335.
func (e *ExportService) staticArchiveRecords() ([]StaticArchiveRecord, error) {
	batch, err := exportSoldiers(e.soldier)
	if err != nil {
		return nil, err
	}
	fullSoldiers := make([]models.Soldier, 0, len(batch))
	idIndex := make(map[int64]models.Soldier, len(batch))
	for _, item := range batch {
		soldier, err := e.soldier.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		fullSoldier := *soldier
		if fullSoldier.EntryType == models.EntryTypeEvent {
			continue
		}
		fullSoldiers = append(fullSoldiers, fullSoldier)
		idIndex[fullSoldier.ID] = fullSoldier
	}
	records := make([]StaticArchiveRecord, 0, len(fullSoldiers))
	for _, soldier := range fullSoldiers {
		records = append(records, newStaticArchiveRecord(soldier, idIndex))
	}
	sort.Slice(records, func(i, j int) bool {
		left := strings.ToLower(records[i].Name + " " + records[i].DisplayID)
		right := strings.ToLower(records[j].Name + " " + records[j].DisplayID)
		return left < right
	})
	return records, nil
}

// --- (e *ExportService) staticArchiveEvents ---
// staticArchiveEvents returns the Event Records (entry_type = "event")
// for the static archive JSON bundle. Mirrors staticArchiveRecords
// in shape — same per-row payload minus Person-specific columns
// (rank / unit / pensionState / etc.), plus Kind / Date range /
// Description / linkedDisplayIds.
func (e *ExportService) staticArchiveEvents() ([]StaticArchiveRecord, error) {
	batch, err := exportSoldiers(e.soldier)
	if err != nil {
		return nil, err
	}
	fullEvents := make([]models.Soldier, 0)
	idIndex := make(map[int64]models.Soldier)
	for _, item := range batch {
		if item.EntryType != models.EntryTypeEvent {
			continue
		}
		soldier, err := e.soldier.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		fullSoldier := *soldier
		fullEvents = append(fullEvents, fullSoldier)
		idIndex[fullSoldier.ID] = fullSoldier
	}
	// Build a person-link index once so per-event linkedDisplayIds
	// resolution is single-shot. Direct SQL keeps this helper
	// free of an EventService dependency in ExportService (the
	// existing ExportService struct holds *db.DB but no
	// EventService field; adding one is out of scope for the
	// #335 slice).
	linkedIndex := make(map[int64][]string, len(idIndex))
	if len(fullEvents) > 0 {
		eventIDs := make([]int64, len(fullEvents))
		for i, ev := range fullEvents {
			eventIDs[i] = ev.ID
		}
		placeholders := make([]string, len(eventIDs))
		args := make([]any, len(eventIDs))
		for i, id := range eventIDs {
			placeholders[i] = "?"
			args[i] = id
		}
		query := `SELECT epl.event_id, COALESCE(s.display_id, '')
			FROM event_person_links epl
			JOIN soldiers s ON s.id = epl.person_id
			WHERE epl.event_id IN (` + strings.Join(placeholders, ",") + `)
			ORDER BY epl.event_id, s.display_id`
		rows, err := e.db.Conn().Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("query event_person_links: %w", err)
		}
		defer debug.DeferCloseLog(rows, "staticArchiveEvents.rows")
		for rows.Next() {
			var eventID int64
			var displayID string
			if err := rows.Scan(&eventID, &displayID); err != nil {
				return nil, fmt.Errorf("scan linked person: %w", err)
			}
			linkedIndex[eventID] = append(linkedIndex[eventID], displayID)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("event_person_links rows: %w", err)
		}
	}
	records := make([]StaticArchiveRecord, 0, len(fullEvents))
	for _, soldier := range fullEvents {
		records = append(records, newStaticArchiveEventRecord(soldier, linkedIndex[soldier.ID]))
	}
	sort.Slice(records, func(i, j int) bool {
		left := strings.ToLower(records[i].Name + " " + records[i].DisplayID)
		right := strings.ToLower(records[j].Name + " " + records[j].DisplayID)
		return left < right
	})
	return records, nil
}


// --- newStaticArchiveRecord ---
func newStaticArchiveRecord(soldier models.Soldier, idIndex map[int64]models.Soldier) StaticArchiveRecord {
	record := StaticArchiveRecord{
		DisplayID:         strings.TrimSpace(soldier.DisplayID),
		EntryType:         strings.TrimSpace(soldier.EntryType),
		DisplayType:       peopleinfo.DisplayEntryType(soldier),
		Name:              peopleinfo.SoldierDisplayName(soldier),
		Dates:             staticArchiveDateSummary(soldier),
		Prefix:            strings.TrimSpace(soldier.Prefix),
		FirstName:         strings.TrimSpace(soldier.FirstName),
		MiddleName:        strings.TrimSpace(soldier.MiddleName),
		LastName:          strings.TrimSpace(soldier.LastName),
		Suffix:            strings.TrimSpace(soldier.Suffix),
		Rank:              strings.TrimSpace(soldier.Rank),
		RankIn:            strings.TrimSpace(soldier.RankIn),
		RankOut:           strings.TrimSpace(soldier.RankOut),
		Unit:              strings.TrimSpace(soldier.Unit),
		Location:          strings.TrimSpace(soldier.BuriedIn),
		BirthDate:         strings.TrimSpace(dates.Display(soldier.BirthDate)),
		DeathDate:         strings.TrimSpace(dates.Display(soldier.DeathDate)),
		BirthInfo:         strings.TrimSpace(soldier.BirthInfo),
		Biography:         strings.TrimSpace(soldier.Biography),
		Notes:             strings.TrimSpace(soldier.Notes),
		MaidenName:        strings.TrimSpace(soldier.MaidenName),
		RelationshipLabel: strings.TrimSpace(soldier.RelationshipLabel),
		SpouseName:        strings.TrimSpace(soldier.SpouseName),
		PensionID:         strings.TrimSpace(soldier.PensionID),
		AppID:             strings.TrimSpace(soldier.ApplicationID),
		PensionState:      pensionstate.Normalize(soldier.PensionState),
		HomeStatus:        confederatehomestatus.Normalize(soldier.ConfederateHomeStatus),
		HomeName:          strings.TrimSpace(soldier.ConfederateHomeName),
		NeedsReview:       soldier.NeedsReview,
		ReviewReason:      strings.TrimSpace(soldier.ReviewReason),
		AddedBy:           strings.TrimSpace(soldier.AddedBy),
		LastEditedBy:      strings.TrimSpace(soldier.LastEditedBy),
		LastEditedAt:      strings.TrimSpace(soldier.LastEditedAt),
		LastEditedFields:  strings.TrimSpace(soldier.LastEditedFields),
		Images:            make([]StaticArchiveImage, 0, len(soldier.Images)),
		Records:           make([]StaticArchiveRecordEntry, 0, len(soldier.Records)),
		Tags:              soldier.Tags,
	}
	if record.HomeStatus == confederatehomestatus.NotApplicable {
		record.HomeStatus = ""
	}
	if record.PensionState == pensionstate.NotApplicable {
		record.PensionState = ""
	}
	if strings.EqualFold(record.BirthDate, "N/A") {
		record.BirthDate = ""
	}
	if strings.EqualFold(record.DeathDate, "N/A") {
		record.DeathDate = ""
	}
	if soldier.SpouseSoldierID > 0 {
		if linked, ok := idIndex[soldier.SpouseSoldierID]; ok {
			record.SpouseDisplayID = strings.TrimSpace(linked.DisplayID)
			if record.SpouseName == "" {
				record.SpouseName = peopleinfo.SoldierDisplayName(linked)
			}
		}
	}
	for _, image := range soldier.Images {
		filePath := staticArchiveImagePath(image.FilePath)
		record.Images = append(record.Images, StaticArchiveImage{
			FileName: strings.TrimSpace(image.FileName),
			Caption:  strings.TrimSpace(image.Caption),
			FilePath: filePath,
		})
		if record.ImagePath == "" {
			record.ImagePath = filePath
		}
	}
	for _, source := range soldier.Records {
		record.Records = append(record.Records, StaticArchiveRecordEntry{
			RecordType: strings.TrimSpace(source.RecordType),
			AppID:      strings.TrimSpace(source.AppID),
			Details:    strings.TrimSpace(source.Details),
		})
	}
	return record
}


// --- staticArchiveDateSummary ---
func staticArchiveDateSummary(soldier models.Soldier) string {
	birth := strings.TrimSpace(strings.ReplaceAll(dates.Display(soldier.BirthDate), "N/A", ""))
	death := strings.TrimSpace(strings.ReplaceAll(dates.Display(soldier.DeathDate), "N/A", ""))
	switch {
	case birth != "" && death != "":
		return "b. " + birth + " • d. " + death
	case birth != "":
		return "b. " + birth
	case death != "":
		return "d. " + death
	default:
		return "Dates not recorded"
	}
}


// --- newStaticArchiveEventRecord ---
// newStaticArchiveEventRecord builds the StaticArchiveRecord
// payload for an Event Record row (entry_type = event). Mirrors
// newStaticArchiveRecord but omits Person-specific fields
// (rank / unit / pensionState / etc.) since the Event Record
// subtype clears them defensively in EventService.CreateEvent.
// Population: DisplayID, EntryType=event, DisplayType="Event",
// Name "<kind> . <date range>", Dates "<begin> - <end>",
// Kind, Description, LinkedDisplayIDs. Spouse / linked-person
// / image / record fields all stay empty.
func newStaticArchiveEventRecord(event models.Soldier, linkedDisplayIDs []string) StaticArchiveRecord {
	record := StaticArchiveRecord{
		DisplayID:        strings.TrimSpace(event.DisplayID),
		EntryType:        strings.TrimSpace(event.EntryType),
		DisplayType:      peopleinfo.DisplayEntryType(event),
		Name:             staticArchiveEventName(event),
		Dates:            staticArchiveEventDateSummary(event),
		Kind:             strings.TrimSpace(event.Kind),
		Description:      strings.TrimSpace(event.Description),
		// Always non-nil so the JSON bundle emits linkedDisplayIds:[]
		// for unlinked events; consumers don't need a null-check.
		// The omitempty flag is dropped intentionally (see struct
		// definition) for this reason.
		LinkedDisplayIDs: append([]string{}, linkedDisplayIDs...),
		AddedBy:          strings.TrimSpace(event.AddedBy),
		LastEditedBy:     strings.TrimSpace(event.LastEditedBy),
		LastEditedAt:     strings.TrimSpace(event.LastEditedAt),
		LastEditedFields: strings.TrimSpace(event.LastEditedFields),
	}
	if record.EntryType == "" {
		record.EntryType = models.EntryTypeEvent
	}
	return record
}


// --- staticArchiveEventName ---
// staticArchiveEventName returns the user-facing name for an
// Event Record row. Mirrors the on-page render in
// event_detail.templ / events.typ: "<kind> . <date range>" if
// both populated, "<kind>" if kind only, "<date range>" if
// dates only, "Unnamed Event" otherwise.
func staticArchiveEventName(event models.Soldier) string {
	kind := strings.TrimSpace(event.Kind)
	range_ := strings.TrimSpace(event.BeginDate)
	switch {
	case kind != "" && range_ != "":
		return kind + " . " + range_
	case kind != "":
		return kind
	case range_ != "":
		return range_
	}
	return "Unnamed Event"
}


// --- staticArchiveEventDateSummary ---
// staticArchiveEventDateSummary renders a one-line "Begin - End"
// summary. Empty strings on both sides produce "Dates not
// recorded" so the JSON bundle matches the per-Person fallback.
func staticArchiveEventDateSummary(event models.Soldier) string {
	begin := strings.TrimSpace(event.BeginDate)
	end := strings.TrimSpace(event.EndDate)
	switch {
	case begin != "" && end != "":
		return begin + " - " + end
	case begin != "":
		return begin
	case end != "":
		return end
	}
	return "Dates not recorded"
}


// --- staticArchiveImagePath ---
func staticArchiveImagePath(filePath string) string {
	trimmed := filepath.ToSlash(strings.TrimSpace(filePath))
	trimmed = strings.TrimPrefix(trimmed, "./")
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	if index := strings.Index(strings.ToLower(trimmed), "images/"); index >= 0 {
		trimmed = trimmed[index:]
	} else {
		trimmed = path.Join("images", path.Base(trimmed))
	}
	return "./" + trimmed
}


// --- sanitizeStaticArchiveStem ---
func sanitizeStaticArchiveStem(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
		}
	}
	return builder.String()
}


// --- staticArchiveInitial ---
func staticArchiveInitial(value string) string {
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return ""
}


// --- renderStaticArchiveIndex ---
func renderStaticArchiveIndex(data staticArchiveIndexData) (string, error) {
	tpl, err := template.New("static-archive-index").Parse(staticArchiveIndexHTML)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	if err := tpl.Execute(&builder, data); err != nil {
		return "", err
	}
	return builder.String(), nil
}


// --- copyDirectoryContents ---
func copyDirectoryContents(sourceRoot, destRoot string) error {
	info, err := os.Stat(sourceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", sourceRoot)
	}
	return filepath.Walk(sourceRoot, func(current string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(sourceRoot, current)
		if err != nil {
			return err
		}
		target := filepath.Join(destRoot, relative)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(current, target)
	})
}


// --- copyFile ---
func copyFile(sourcePath, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer debug.DeferCloseLog(source, "copyFile.source")
	target, err := os.Create(destPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	return target.Close()
}


// --- zipDirectory ---
func zipDirectory(outputPath, root string) error {
	return writeZipArchive(outputPath, func(zipWriter *zip.Writer) error {
		return filepath.Walk(root, func(current string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, current)
			if err != nil {
				return err
			}
			entry, err := zipWriter.Create(filepath.ToSlash(relative))
			if err != nil {
				return err
			}
			source, err := os.Open(current)
			if err != nil {
				return err
			}
			defer debug.DeferCloseLog(source, "zipDirectory.source")
			_, err = io.Copy(entry, source)
			return err
		})
	})
}


// staticArchiveArticles returns every live-branch Article row
// (issue #321 slice 5.3) projected into the StaticArchiveRecord
// shape. Snapshot rows are excluded (slice-2.5 design). Each
// article carries the body_html (the slice-3.6 sanitized HTML
// render -- already safe to embed) so the JS index can render
// the article without a second pass through the markdown
// renderer. The ResolvedRefs projection mirrors the per-Event
// LinkedDisplayIDs: a list of {display_id, name, resolved}
// dicts the JS index can render inline.
func (e *ExportService) staticArchiveArticles() ([]StaticArchiveRecord, error) {
	soldierSvc := NewSoldierService(e.db)
	articleSvc := records.NewArticleService(soldierSvc)
	batch, err := listAllArticles(e.db)
	if err != nil {
		return nil, err
	}
	records := make([]StaticArchiveRecord, 0, len(batch))
	for _, art := range batch {
		tokens, terr := articleSvc.ResolveRefs(art.ID)
		if terr != nil {
			return nil, fmt.Errorf("ResolveRefs %d: %w", art.ID, terr)
		}
		refList := make([]StaticArchiveArticleRef, 0, len(tokens))
		for _, tok := range tokens {
			displayID := tok.PersonDisplayID
			if displayID == "" {
				displayID = tok.Token
			}
			name := displayID
			if tok.Resolved {
				if s, lookupErr := soldierSvc.GetByID(tok.PersonRecordID); lookupErr == nil && s != nil {
					fullName := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(s.FirstName), strings.TrimSpace(s.LastName)}, " "))
					if fullName != "" {
						name = fullName
					}
				}
			}
			refList = append(refList, StaticArchiveArticleRef{
				DisplayID: displayID,
				Name:      name,
				Resolved:  tok.Resolved,
			})
		}
		records = append(records, newStaticArchiveArticle(art, refList))
	}
	sort.Slice(records, func(i, j int) bool {
		return strings.ToLower(records[i].Title) < strings.ToLower(records[j].Title)
	})
	return records, nil
}

// newStaticArchiveArticle projects a models.Article into the
// StaticArchiveRecord shape (reusing the existing per-Person +
// per-Event struct so the JS index can render an Articles tab
// without a new struct). Title + Subtitle are the headline
// fields; BodyHTML carries the sanitized HTML; ResolvedRefs
// is the per-token projection.
func newStaticArchiveArticle(article models.Article, refs []StaticArchiveArticleRef) StaticArchiveRecord {
	return StaticArchiveRecord{
		DisplayID:  strings.TrimSpace(article.DisplayID),
		EntryType:  "article",
		DisplayType: "Article Record",
		Name:       strings.TrimSpace(article.Title),
		Title:      strings.TrimSpace(article.Title),
		Subtitle:   strings.TrimSpace(article.Subtitle),
		BodyHTML:   article.BodyHTML,
		// Mirror the LinkedDisplayIDs pattern: always non-nil
		// so the JSON bundle emits resolvedRefs:[] for unref'd
		// articles. Consumers don't need a null-check.
		ResolvedRefs:  append([]StaticArchiveArticleRef{}, refs...),
		UpdatedAt:    strings.TrimSpace(article.UpdatedAt),
		CreatedAt:    strings.TrimSpace(article.CreatedAt),
	}
}

// staticArchiveCalendar returns the per-month Calendar snapshot
// the archive bundles for the Calendar landing page (issue #498
// slice 1). Always 12 months in calendar order (Jan=1 ... Dec=12)
// per locked decision 1 — the JS grid renders a full wall-calendar
// and visually hides months with zero markers.
//
// Each month carries a day-keyed map of StaticArchiveCalendarDay
// rollups (AnniversaryCount = soldiers with death on that
// month+day, EventCount = calendar_items.event rows, HolidayCount
// = calendar_items.holiday rows). The day map is empty for
// months with no markers so the JSON bundle stays compact.
//
// The helper constructs a CalendarService on the fly —
// ExportService holds *db.DB but no *CalendarService field,
// mirroring the pattern in staticArchiveArticles that builds an
// ArticleService inline rather than widening the struct.
func (e *ExportService) staticArchiveCalendar() (StaticArchiveCalendar, error) {
	calendarSvc := records.NewCalendarService(e.db)
	months := make(StaticArchiveCalendar, 0, 12)
	for m := 1; m <= 12; m++ {
		summary, err := calendarSvc.GetMonthSummary(m)
		if err != nil {
			return nil, fmt.Errorf("staticArchiveCalendar month %d: %w", m, err)
		}
		days := make(map[int]StaticArchiveCalendarDay, len(summary))
		for day, s := range summary {
			if s.AnniversaryCount == 0 && s.EventCount == 0 && s.HolidayCount == 0 {
				continue
			}
			days[day] = StaticArchiveCalendarDay{
				AnniversaryCount: s.AnniversaryCount,
				EventCount:       s.EventCount,
				HolidayCount:     s.HolidayCount,
			}
		}
		months = append(months, StaticArchiveCalendarMonth{Month: m, Days: days})
	}
	return months, nil
}

// staticArchiveInsights returns the AnalyticsService snapshot for
// the Insights page (issue #498 slice 4). Mirrors the
// staticArchiveCalendar pattern — ExportService holds *db.DB but
// no *AnalyticsService field, so the helper constructs the service
// inline rather than widening the struct. The snapshot includes
// record-type counts + 7 dimension rollups (cemeteries, Confederate
// Home status + names, pension distribution, top units, birth + death
// decade distributions) + the duplicate-audit summary. All counts
// are computed at export time so the JS index renders the Insights
// page without a server round-trip.
func (e *ExportService) staticArchiveInsights() (records.AnalyticsSnapshot, error) {
	return records.NewAnalyticsService(e.db).Snapshot()
}

// staticArchiveCalendarItems returns every row from calendar_items
// projected into StaticArchiveCalendarItem DTOs (issue #502).
// Mirrors the inline-service pattern — constructs CalendarService
// on the fly rather than widening ExportService.
func (e *ExportService) staticArchiveCalendarItems() ([]StaticArchiveCalendarItem, error) {
	rows, err := e.db.Conn().Query(`SELECT id, item_type, month, day, title, COALESCE(notes,'') AS notes, created_at, updated_at
		FROM calendar_items
		ORDER BY month, day, CASE item_type WHEN 'holiday' THEN 0 WHEN 'event' THEN 1 ELSE 2 END, LOWER(title)`)
	if err != nil {
		return nil, fmt.Errorf("staticArchiveCalendarItems: %w", err)
	}
	defer debug.DeferCloseLog(rows, "staticArchiveCalendarItems.rows")
	var items []StaticArchiveCalendarItem
	for rows.Next() {
		var item StaticArchiveCalendarItem
		var notes, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&item.ID, &item.ItemType, &item.Month, &item.Day, &item.Title, &notes, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("staticArchiveCalendarItems scan: %w", err)
		}
		if notes.Valid {
			item.Notes = notes.String
		}
		if createdAt.Valid {
			item.CreatedAt = createdAt.String
		}
		if updatedAt.Valid {
			item.UpdatedAt = updatedAt.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("staticArchiveCalendarItems rows: %w", err)
	}
	return items, nil
}
