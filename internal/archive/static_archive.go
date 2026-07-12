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

    /* Issue #498 slice 2: small fixed nav menu (Calendar / Browse /
       Insights / Person Records / Events / Articles). Replaces the
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
      background: rgba(36, 48, 61, 0.04);
      cursor: default;
    }
    .calendar-day-number {
      font-size: 0.95rem;
      font-weight: 700;
      color: var(--ink);
    }
    .calendar-day.empty .calendar-day-number {
      color: var(--muted);
      opacity: 0.4;
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
    .filter-chips {
      display: grid;
      gap: 10px;
      margin-bottom: 14px;
    }
    .filter-chip-group {
      display: grid;
      grid-template-columns: 160px 1fr;
      align-items: start;
      gap: 12px;
    }
    .filter-chip-label {
      font-size: 0.72rem;
      font-weight: 700;
      letter-spacing: 0.16em;
      text-transform: uppercase;
      color: var(--gold-dark);
      padding-top: 8px;
    }
    .filter-chip-row {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
    }
    .filter-chip {
      border-radius: 999px;
      border: 1px solid rgba(141, 116, 64, 0.55);
      background: rgba(255, 251, 241, 0.7);
      color: var(--ink);
      padding: 5px 12px;
      font-size: 0.78rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.12s, border-color 0.12s;
    }
    .filter-chip:hover {
      background: rgba(255, 247, 231, 0.95);
    }
    .filter-chip.active {
      background: linear-gradient(180deg, #c5ab68 0%, #a5853f 100%);
      color: #1f2b38;
      border-color: var(--gold-dark);
    }
    .filter-chip-count {
      font-size: 0.72rem;
      opacity: 0.7;
      margin-left: 2px;
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
      .filter-chip-group { grid-template-columns: 1fr; }
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
  </style>
</head>
<body>
  <div class="shell">
    <header class="hero">
      <div class="hero-shell">
        <h1>{{ .ArchiveTitle }}</h1>
        <p>Browse this standalone DixieData archive as a read-only mirror of the DixieData app. The Calendar landing shows every anniversary and event day in the archive; use the nav to jump to Browse (filterable Person Record list), Insights (analytics snapshot), or the Event / Article tabs.</p>
        <div class="archive-meta">
          <span>Generated {{ .GeneratedAt }}</span>
        </div>
        <!-- Issue #498 slice 2: small fixed nav menu (Calendar / Browse /
             Insights / Person Records / Events / Articles). Each link is
             a hash-route to its page renderer; the JS marks the active
             link based on the current hash. Empty-entity links are hidden
             (e.g. no Events tab if bundle.events is empty). -->
        <nav class="nav-menu" id="archive-nav-menu">
          <a href="#/calendar"  class="nav-link" data-route="calendar">Calendar</a>
          <a href="#/browse"    class="nav-link" data-route="browse">Browse</a>
          <a href="#/insights"  class="nav-link" data-route="insights">Insights</a>
          <a href="#/persons"   class="nav-link" data-route="persons">Person Records</a>
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
      // Page routes: #/calendar, #/browse?..., #/insights, #/persons, #/events, #/articles.
      var pageMatch = path.match(/^\/([a-z]+)(\?.*)?$/);
      if (pageMatch) {
        var name = pageMatch[1];
        var query = (pageMatch[2] || '').replace(/^\?/, '');
        if (name === 'calendar' || name === 'browse' || name === 'insights' ||
            name === 'persons' || name === 'events' || name === 'articles') {
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
    // bundle.calendar (issue #498 slice 1). Always renders 12
    // months in calendar order per locked decision 1; months with
    // zero markers still show the grid skeleton so the user has
    // visual confirmation of "this month had nothing".
    function renderCalendarPage(bundle) {
      var months = Array.isArray(bundle.calendar) ? bundle.calendar : [];
      var totalDaysWithData = 0;
      var blocks = [];
      for (var m = 1; m <= 12; m++) {
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
      return '' +
        '<div class="panel-head"><h2>Calendar</h2></div>' +
        '<p class="panel-subtext">Every anniversary and event day in this archive, by month. Click a day to browse Person Records for that date.</p>' +
        (totalDaysWithData === 0
          ? '<div class="placeholder-card">No anniversaries, events, or holidays recorded in this archive.</div>'
          : blocks.join(''));
    }

    // renderBrowsePage renders the Browse page (filterable Person
    // Record list, issue #498 slice 3). Hash-routed pre-fill via
    // the route query string (e.g. #/browse?entry_type=widow from
    // an Insights drilldown). Search + 5 filter chips + sort +
    // pagination are all client-side against bundle.records[].
    // Per locked decision 2 review_status is dropped (no review
    // queue in a read-only archive) and scope is hidden (always
    // "all" — the archive IS the full snapshot).
    function renderBrowsePage(bundle, query) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      return '' +
        '<div class="panel-head"><h2>Browse</h2>' +
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
              '<option value="name">Name</option>' +
              '<option value="last_edited" selected>Last Edited</option>' +
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
        '<div class="filter-chips" id="browse-filter-chips">' +
          renderFilterChip('entry_type', 'Entry type', records, function(r) { return r.entryType; }) +
          renderFilterChip('pension_state', 'Pension state', records, function(r) { return r.pensionState; }) +
          renderFilterChip('unit', 'Unit', records, function(r) { return r.unit; }) +
          renderFilterChip('buried_in', 'Buried in', records, function(r) { return r.location; }) +
          renderFilterChip('confederate_home_status', 'Confederate Home status', records, function(r) { return r.homeStatus; }) +
        '</div>' +
        '<div id="browse-prefilter-banner" class="browse-prefilter-banner hidden"></div>' +
        '<div class="results" id="browse-results"></div>' +
        '<div class="browse-pagination" id="browse-pagination"></div>' +
        '<div class="empty-state" id="browse-empty" style="display:none;">No Person Records matched the current filters.</div>';
    }

    // renderFilterChip renders one filter-chip group: a label +
    // a horizontal scroll of pill buttons, one per distinct value
    // in the records slice. Each pill carries data-filter="<field>"
    // + data-value="<value>" so the click handler can toggle.
    function renderFilterChip(field, label, records, getter) {
      var values = {};
      for (var i = 0; i < records.length; i++) {
        var v = String(getter(records[i]) || '').trim();
        if (!v) continue;
        values[v] = (values[v] || 0) + 1;
      }
      var sorted = Object.keys(values).sort(function(a, b) { return a.toLowerCase().localeCompare(b.toLowerCase()); });
      var buttons = '<button type="button" class="filter-chip active" data-filter="' + field + '" data-value="">All ' + sorted.length + '</button>';
      for (var j = 0; j < sorted.length; j++) {
        var val = sorted[j];
        buttons += '<button type="button" class="filter-chip" data-filter="' + field + '" data-value="' + escapeHtml(val) + '">' + escapeHtml(val) + ' <span class="filter-chip-count">' + values[val] + '</span></button>';
      }
      return '<div class="filter-chip-group" data-filter-group="' + field + '">' +
        '<span class="filter-chip-label">' + escapeHtml(label) + '</span>' +
        '<div class="filter-chip-row">' + buttons + '</div>' +
      '</div>';
    }

    // parseBrowseQuery parses the Browse route query string into
    // an object: {field: value, ...}. The Calendar day-cell drilldown
    // uses #/browse?date=05-12 (MM-DD); Insights card drilldowns use
    // #/browse?entry_type=widow or #/browse?unit=1st%20Texas%20Infantry.
    function parseBrowseQuery(queryString) {
      var out = {};
      var s = String(queryString || '').replace(/^\?/, '');
      if (!s) return out;
      var parts = s.split('&');
      for (var i = 0; i < parts.length; i++) {
        var eq = parts[i].indexOf('=');
        if (eq < 0) {
          out[decodeURIComponent(parts[i])] = '';
        } else {
          var key = decodeURIComponent(parts[i].slice(0, eq));
          var val = decodeURIComponent(parts[i].slice(eq + 1));
          out[key] = val;
        }
      }
      return out;
    }

    // getBrowseFilterValue reads the currently-selected filter value
    // for the given field from the chip group. Empty string = "All".
    function getBrowseFilterValue(field) {
      var active = document.querySelector('[data-filter-group="' + field + '"] .filter-chip.active');
      return active ? (active.getAttribute('data-value') || '') : '';
    }

    // applyBrowseFilters reads the search input + every chip's active
    // value + the sort + page size + current page, applies them to
    // bundle.records[], and re-renders the list. Called on every
    // input change and every chip click. Hash pre-fill is applied
    // once on initial Browse mount (not on every re-render).
    function applyBrowseFilters(bundle, prefill) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      var searchInput = document.getElementById('browse-search');
      var sortSelect = document.getElementById('browse-sort');
      var pageSizeSelect = document.getElementById('browse-page-size');
      if (!searchInput || !sortSelect || !pageSizeSelect) return;
      var query = String(searchInput.value || '').trim().toLowerCase();
      var sort = sortSelect.value || 'last_edited';
      var pageSize = Number(pageSizeSelect.value) || 25;

      var filtered = records.filter(function(r) {
        if (query && !matchesSearch(r, query)) return false;
        for (var fi = 0; fi < BROWSE_FILTER_FIELDS.length; fi++) {
          var field = BROWSE_FILTER_FIELDS[fi];
          var want = prefill && prefill[field] !== undefined ? prefill[field] : getBrowseFilterValue(field);
          if (!want) continue;
          var got = '';
          if (field === 'entry_type') got = r.entryType;
          else if (field === 'pension_state') got = r.pensionState;
          else if (field === 'unit') got = r.unit;
          else if (field === 'buried_in') got = r.location;
          else if (field === 'confederate_home_status') got = r.homeStatus;
          if (String(got || '').trim() !== want) return false;
        }
        if (prefill && prefill.date) {
          var date = prefill.date;
          var dStr = String(r.deathDate || '').trim();
          if (dStr.length >= 5) {
            var mmdd = dStr.slice(0, 2) + '-' + dStr.slice(3, 5);
            if (mmdd !== date) return false;
          } else {
            return false;
          }
        }
        return true;
      });

      filtered.sort(function(a, b) {
        var ak = '', bk = '';
        if (sort === 'display_id') { ak = a.displayId || ''; bk = b.displayId || ''; }
        else if (sort === 'name') { ak = (a.name || '').toLowerCase(); bk = (b.name || '').toLowerCase(); }
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
    var BROWSE_FILTER_FIELDS = ['entry_type', 'pension_state', 'unit', 'buried_in', 'confederate_home_status'];

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
        '<p class="panel-subtext">A pre-computed snapshot of the archive\'s analytics — Person Record Types, top cemeteries, Confederate Home status, pension distribution, top units, and birth/death decades. Click any entry to browse Person Records with that filter pre-filled.</p>' +
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

    // renderPersonsPage / renderEventsPage / renderArticlesPage are
    // the legacy list screens from #320 / #490, re-routed through
    // the new hash router. Each renders its list (no search input
    // in the hero — search lives on Browse, slice 3) plus a detail
    // view triggered by a row click.
    function renderPersonsPage(bundle) {
      var records = Array.isArray(bundle.records) ? bundle.records : [];
      var html =
        '<div class="panel-head"><h2>Person Records</h2></div>' +
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
            } else {
              content.innerHTML = '<p>Person Record "' + escapeHtml(route.id) + '" not found in this archive.</p>';
              pos.textContent = 'Not Found';
            }
          } else if (route.entity === 'event') {
            var evIdx = findEventById(events, route.id);
            if (evIdx >= 0) {
              content.innerHTML = renderEventDetail(events[evIdx], records);
              pos.textContent = 'Event';
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
          case 'calendar': html = renderCalendarPage(bundle); break;
          case 'browse':
            html = renderBrowsePage(bundle, route.query);
            setPageHtml(html);
            // Issue #498 slice 3: pre-fill chips from the hash query
            // (Insights drilldown target), wire chip + search +
            // sort + page-size + pagination listeners, then render.
            var prefill = parseBrowseQuery(route.query);
            applyBrowsePrefillChips(prefill);
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
          default: html = renderCalendarPage(bundle);
        }
        setPageHtml(html);
        window.scrollTo({ top: 0, behavior: 'smooth' });
      }

      // applyBrowsePrefillChips activates the chip(s) whose value
      // matches the prefill object from the route query. The 'All'
      // chip in each group is deactivated when a specific value
      // matches; the matched chip is activated.
      function applyBrowsePrefillChips(prefill) {
        if (!prefill) return;
        for (var key in prefill) {
          if (Object.prototype.hasOwnProperty.call(prefill, key)) {
            var val = prefill[key];
            // date= handled separately (banner, not chip)
            if (key === 'date') continue;
            var chip = document.querySelector('[data-filter-group="' + key + '"] [data-value="' + cssEscape(val) + '"]');
            if (chip) {
              var group = chip.closest('[data-filter-group]');
              if (group) {
                group.querySelectorAll('.filter-chip').forEach(function(c) { c.classList.remove('active'); });
              }
              chip.classList.add('active');
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

      // wireBrowseListeners attaches the chip / search / sort /
      // page-size / pagination click + change handlers. Called
      // once per Browse mount; the handlers re-read inputs and
      // call applyBrowseFilters(bundle, null) — no prefill, the
      // user has taken control from this point.
      function wireBrowseListeners(bundle) {
        document.querySelectorAll('[data-filter-group] .filter-chip').forEach(function(chip) {
          chip.addEventListener('click', function() {
            var group = chip.closest('[data-filter-group]');
            if (!group) return;
            group.querySelectorAll('.filter-chip').forEach(function(c) { c.classList.remove('active'); });
            chip.classList.add('active');
            browseState.page = 1;
            applyBrowseFilters(bundle, null);
          });
        });
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
