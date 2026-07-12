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
        <p>Browse this standalone DixieData archive in a list-first layout. Search the archive in real time, then open any record in a full-page detail view with notes, source records, and image previews.</p>
        <div class="search-row">
          <label for="archive-search">Search the archive</label>
          <input id="archive-search" type="search" placeholder="Search by name, unit, or location..." autocomplete="off" spellcheck="false">
        </div>
        <div class="archive-meta">
          <span id="result-count">0 records</span>
          <span>Generated {{ .GeneratedAt }}</span>
        </div>
        <!-- Issue #490: three-tab segmented control. The JS shows/hides
             tabs based on bundle contents — a tab with zero items is
             hidden, not shown empty. -->
        <nav class="tab-bar" id="archive-tabs">
          <button type="button" class="tab-button active" data-tab="persons">Persons</button>
          <button type="button" class="tab-button" data-tab="events">Events</button>
          <button type="button" class="tab-button" data-tab="articles">Articles</button>
        </nav>
        <!-- Issue #494: theme picker removed. The static archive ships with
             Soft hardcoded; no theme switching inside the archive. -->
      </div>
    </header>

    <main>
      <section id="archive-list-screen" class="screen list-screen">
        <div class="panel-head">
          <h2>Archive List</h2>
        </div>
        <p class="panel-subtext">Images stay off the main list for faster browsing. Use <strong>View More</strong> on any entry to open a full-page archive view.</p>
        <section id="archive-results" class="results" aria-live="polite"></section>
        <div id="archive-empty" class="empty-state">No records matched the current search.</div>
        <!-- Issue #490: Events + Articles list containers. The JS
             populates these from bundle.events / bundle.articles and
             toggles visibility via the tab-bar. -->
        <section id="archive-events-results" class="results hidden" aria-live="polite"></section>
        <div id="archive-events-empty" class="empty-state hidden">No events in this archive.</div>
        <section id="archive-articles-results" class="results hidden" aria-live="polite"></section>
        <div id="archive-articles-empty" class="empty-state hidden">No articles in this archive.</div>
      </section>

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
      return '#record=' + encodeURIComponent(record.displayId || record.name || '');
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

    function detailLink(displayId) {
      return '#record=' + encodeURIComponent(String(displayId || '').trim());
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

    function renderDetail(record, allRecords) {
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
      // Issue #490: Linked Events section — shows events from
      // bundle.events whose linkedDisplayIds contains this
      // record's displayId.
      const linkedEvents = linkedEventsForRecord(record, allEvents);
      if (linkedEvents.length) {
        primarySections.push(
          '<section class="detail-section"><h4>Linked Events</h4><div class="related-list">' +
            linkedEvents.map(function(ev) {
              const evTitle = escapeHtml(ev.description || ev.kind || 'Untitled Event');
              const evMeta = escapeHtml((ev.kind || '') + (ev.dateRange ? ' · ' + ev.dateRange : ''));
              return '<div class="related-card"><strong>' + evTitle + '</strong><p>' + evMeta + '</p>' +
                '<div class="related-links"><a class="image-button" href="#event=' + encodeURIComponent(String(ev.displayId || ev.id || '')) + '">Open Event</a></div></div>';
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

    // Issue #490: Linked Events section — filters allEvents for
    // events whose linkedDisplayIds contains this record's displayId.
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
            const link = p.displayId ? '<div class="related-links"><a class="image-button" href="#record=' + encodeURIComponent(p.displayId) + '">Open Person Record</a></div>' : '';
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
            const link = ref.resolved && ref.displayId ? '<a class="record-link" href="#record=' + encodeURIComponent(ref.displayId) + '">' + escapeHtml(ref.name || ref.displayId) + '</a>' : escapeHtml(ref.name || ref.displayId || 'Unknown');
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

    function findRecordIndex(records, hash) {
      const match = String(hash || '').match(/^#record=(.+)$/);
      if (!match) {
        return -1;
      }
      const displayId = decodeURIComponent(match[1]);
      return records.findIndex(function(record) {
        return record.displayId === displayId;
      });
    }

    // Issue #490: hash routers for #event= and #article= hashes.
    function findEventIndex(events, hash) {
      const match = String(hash || '').match(/^#event=(.+)$/);
      if (!match) return -1;
      const id = decodeURIComponent(match[1]);
      return events.findIndex(function(ev) { return String(ev.displayId || ev.id || '') === id; });
    }

    function findArticleIndex(articles, hash) {
      const match = String(hash || '').match(/^#article=(.+)$/);
      if (!match) return -1;
      const id = decodeURIComponent(match[1]);
      return articles.findIndex(function(a) { return String(a.id || '') === id; });
    }

    function showListScreen() {
      document.getElementById('archive-list-screen').classList.remove('hidden');
      document.getElementById('archive-detail-screen').classList.add('hidden');
      document.querySelectorAll('.record-row').forEach(function(row) {
        row.classList.remove('active');
      });
    }

    function showDetailScreen(record, index, visibleCount, allRecords, allEvents) {
      document.getElementById('archive-list-screen').classList.add('hidden');
      document.getElementById('archive-detail-screen').classList.remove('hidden');
      document.getElementById('detail-content').innerHTML = renderDetail(record, allRecords, allEvents);
      document.getElementById('detail-position').textContent = 'Record ' + (index + 1) + ' of ' + visibleCount;
      window.scrollTo({ top: 0, behavior: 'smooth' });
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

    function updateResults(records, query) {
      const filtered = records
        .map(function(record, index) { return { record: record, index: index }; })
        .filter(function(item) {
          return matchesSearch(item.record, query);
        });

      const results = document.getElementById('archive-results');
      const empty = document.getElementById('archive-empty');
      const count = document.getElementById('result-count');

      results.innerHTML = filtered.map(function(item) {
        return renderRecord(item.record, item.index, records);
      }).join('');
      empty.style.display = filtered.length ? 'none' : 'block';
      count.textContent = filtered.length + (filtered.length === 1 ? ' record' : ' records');
      return filtered;
    }

    document.addEventListener('DOMContentLoaded', function() {
      // Issue #320 child #335: the bundle is now an object
      // with records + events arrays rather than a bare
      // array. Read .records so the per-Person list still
      // renders identically; events are exposed via
      // window.DIXIE_DATA.events for a future slot to
      // render an Events tab.
      const bundle = (window.DIXIE_DATA && typeof window.DIXIE_DATA === 'object') ? window.DIXIE_DATA : {};
      const records = Array.isArray(bundle.records) ? bundle.records : [];
      const events = Array.isArray(bundle.events) ? bundle.events : [];
      const articles = Array.isArray(bundle.articles) ? bundle.articles : [];
      window.__DIXIE_EVENTS__ = events;
      const searchInput = document.getElementById('archive-search');
      const previewStage = document.getElementById('image-preview-stage');
      let activeTab = 'persons';
      let filteredRecords = updateResults(records, '');

      // Issue #490: hide tabs with zero items — no empty tabs.
      if (!events.length) {
        const evTab = document.querySelector('[data-tab="events"]');
        if (evTab) evTab.classList.add('hidden');
      }
      if (!articles.length) {
        const artTab = document.querySelector('[data-tab="articles"]');
        if (artTab) artTab.classList.add('hidden');
      }

      // Issue #490: render events + articles lists.
      function updateEventResults() {
        const container = document.getElementById('archive-events-results');
        if (events.length) {
          container.innerHTML = events.map(function(ev, i) { return renderEventRow(ev, i); }).join('');
        }
      }
      function updateArticleResults() {
        const container = document.getElementById('archive-articles-results');
        if (articles.length) {
          container.innerHTML = articles.map(function(a, i) { return renderArticleRow(a, i); }).join('');
        }
      }
      updateEventResults();
      updateArticleResults();

      // Issue #490: tab switching.
      function switchTab(tab) {
        activeTab = tab;
        document.querySelectorAll('.tab-button').forEach(function(btn) {
          btn.classList.toggle('active', btn.getAttribute('data-tab') === tab);
        });
        var isPersons = tab === 'persons';
        var isEvents = tab === 'events';
        var isArticles = tab === 'articles';
        document.getElementById('archive-results').classList.toggle('hidden', !isPersons);
        var ae = document.getElementById('archive-empty'); if (ae) ae.classList.toggle('hidden', !isPersons);
        document.getElementById('archive-events-results').classList.toggle('hidden', !isEvents);
        var aee = document.getElementById('archive-events-empty'); if (aee) aee.classList.toggle('hidden', !isEvents);
        document.getElementById('archive-articles-results').classList.toggle('hidden', !isArticles);
        var aae = document.getElementById('archive-articles-empty'); if (aae) aae.classList.toggle('hidden', !isArticles);
      }

      function syncViewFromHash() {
        const hash = window.location.hash;
        // Issue #490: check #event= and #article= before #record=.
        var evIdx = findEventIndex(events, hash);
        if (evIdx >= 0) {
          document.getElementById('archive-list-screen').classList.add('hidden');
          document.getElementById('archive-detail-screen').classList.remove('hidden');
          document.getElementById('detail-content').innerHTML = renderEventDetail(events[evIdx], records);
          document.getElementById('detail-position').textContent = 'Event';
          window.scrollTo({ top: 0, behavior: 'smooth' });
          return;
        }
        var artIdx = findArticleIndex(articles, hash);
        if (artIdx >= 0) {
          document.getElementById('archive-list-screen').classList.add('hidden');
          document.getElementById('archive-detail-screen').classList.remove('hidden');
          document.getElementById('detail-content').innerHTML = renderArticleDetail(articles[artIdx]);
          document.getElementById('detail-position').textContent = 'Article';
          window.scrollTo({ top: 0, behavior: 'smooth' });
          return;
        }
        const matchIndex = findRecordIndex(records, hash);
        if (matchIndex < 0) {
          showListScreen();
          return;
        }
        const visibleIndex = filteredRecords.findIndex(function(item) {
          return item.index === matchIndex;
        });
        if (visibleIndex < 0) {
          filteredRecords = updateResults(records, searchInput.value);
        }
        const finalVisibleIndex = filteredRecords.findIndex(function(item) {
          return item.index === matchIndex;
        });
        if (finalVisibleIndex < 0) {
          showListScreen();
          return;
        }
        showDetailScreen(records[matchIndex], finalVisibleIndex, filteredRecords.length, records, events);
      }

      syncViewFromHash();

      searchInput.addEventListener('input', function(event) {
        filteredRecords = updateResults(records, event.target.value);
        if (!window.location.hash) {
          showListScreen();
          return;
        }
        syncViewFromHash();
      });

      document.addEventListener('click', function(event) {
        // Issue #490: tab switching.
        const tabButton = event.target.closest('.tab-button');
        if (tabButton) {
          switchTab(tabButton.getAttribute('data-tab'));
          if (window.location.hash) window.location.hash = '';
          return;
        }

        const viewButton = event.target.closest('[data-view-record]');
        if (viewButton) {
          const index = Number(viewButton.getAttribute('data-view-record'));
          if (!Number.isNaN(index) && records[index]) {
            window.location.hash = detailHash(records[index]);
          }
          return;
        }

        // Issue #490: event + article view-more buttons.
        const viewEventButton = event.target.closest('[data-view-event]');
        if (viewEventButton) {
          const index = Number(viewEventButton.getAttribute('data-view-event'));
          if (!Number.isNaN(index) && events[index]) {
            window.location.hash = '#event=' + encodeURIComponent(String(events[index].displayId || events[index].id || ''));
          }
          return;
        }

        const viewArticleButton = event.target.closest('[data-view-article]');
        if (viewArticleButton) {
          const index = Number(viewArticleButton.getAttribute('data-view-article'));
          if (!Number.isNaN(index) && articles[index]) {
            window.location.hash = '#article=' + encodeURIComponent(String(articles[index].id || ''));
          }
          return;
        }

        if (event.target.id === 'detail-back') {
          window.location.hash = '';
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

      window.addEventListener('hashchange', syncViewFromHash);
      window.addEventListener('resize', applyImageTransform);
    });
    // Issue #494: theme picker JS removed. The static archive ships
    // with Soft hardcoded; no theme switching inside the archive.
  </script>
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
