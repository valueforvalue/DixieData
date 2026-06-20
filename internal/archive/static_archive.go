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
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
)


// --- StaticArchiveRecord/Image/Entry types ---
type StaticArchiveRecord struct {
	DisplayID         string                     `json:"displayId"`
	EntryType         string                     `json:"entryType"`
	DisplayType       string                     `json:"displayType"`
	Name              string                     `json:"name"`
	Dates             string                     `json:"dates"`
	Prefix            string                     `json:"prefix,omitempty"`
	FirstName         string                     `json:"firstName,omitempty"`
	MiddleName        string                     `json:"middleName,omitempty"`
	LastName          string                     `json:"lastName,omitempty"`
	Suffix            string                     `json:"suffix,omitempty"`
	Rank              string                     `json:"rank,omitempty"`
	RankIn            string                     `json:"rankIn,omitempty"`
	RankOut           string                     `json:"rankOut,omitempty"`
	Unit              string                     `json:"unit,omitempty"`
	Location          string                     `json:"location,omitempty"`
	BirthDate         string                     `json:"birthDate,omitempty"`
	DeathDate         string                     `json:"deathDate,omitempty"`
	BirthInfo         string                     `json:"birthInfo,omitempty"`
	Biography         string                     `json:"biography,omitempty"`
	Notes             string                     `json:"notes,omitempty"`
	MaidenName        string                     `json:"maidenName,omitempty"`
	RelationshipLabel string                     `json:"relationshipLabel,omitempty"`
	SpouseName        string                     `json:"spouseName,omitempty"`
	SpouseDisplayID   string                     `json:"spouseDisplayId,omitempty"`
	PensionID         string                     `json:"pensionId,omitempty"`
	AppID             string                     `json:"appId,omitempty"`
	PensionState      string                     `json:"pensionState,omitempty"`
	HomeStatus        string                     `json:"homeStatus,omitempty"`
	HomeName          string                     `json:"homeName,omitempty"`
	NeedsReview       bool                       `json:"needsReview,omitempty"`
	ReviewReason      string                     `json:"reviewReason,omitempty"`
	AddedBy           string                     `json:"addedBy,omitempty"`
	LastEditedBy      string                     `json:"lastEditedBy,omitempty"`
	LastEditedAt      string                     `json:"lastEditedAt,omitempty"`
	LastEditedFields  string                     `json:"lastEditedFields,omitempty"`
	ImagePath         string                     `json:"imagePath,omitempty"`
	Images            []StaticArchiveImage       `json:"images,omitempty"`
	Records           []StaticArchiveRecordEntry `json:"records,omitempty"`
}

type StaticArchiveImage struct {
	FileName string `json:"fileName"`
	Caption  string `json:"caption,omitempty"`
	FilePath string `json:"filePath"`
}

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
	ArchiveTitle string
	OwnerShort   string
	Version      string
	Build        string
	GeneratedAt  string
}


// --- staticArchiveIndexHTML template ---
const staticArchiveIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .ArchiveTitle }}</title>
  <script defer src="./archive_data.js"></script>
  <style>
    :root {
      color-scheme: light;
      --paper: #d7d2c9;
      --panel: rgba(223, 228, 234, 0.92);
      --panel-strong: rgba(255, 251, 241, 0.96);
      --panel-dark: rgba(36, 48, 61, 0.92);
      --border: rgba(141, 116, 64, 0.82);
      --gold: #a88a46;
      --gold-dark: #8d7440;
      --ink: #22303d;
      --muted: #445260;
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

    function showListScreen() {
      document.getElementById('archive-list-screen').classList.remove('hidden');
      document.getElementById('archive-detail-screen').classList.add('hidden');
      document.querySelectorAll('.record-row').forEach(function(row) {
        row.classList.remove('active');
      });
    }

    function showDetailScreen(record, index, visibleCount, allRecords) {
      document.getElementById('archive-list-screen').classList.add('hidden');
      document.getElementById('archive-detail-screen').classList.remove('hidden');
      document.getElementById('detail-content').innerHTML = renderDetail(record, allRecords);
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
      const records = Array.isArray(window.DIXIE_DATA) ? window.DIXIE_DATA : [];
      const searchInput = document.getElementById('archive-search');
      const previewStage = document.getElementById('image-preview-stage');
      let filteredRecords = updateResults(records, '');

      function syncViewFromHash() {
        const matchIndex = findRecordIndex(records, window.location.hash);
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
        showDetailScreen(records[matchIndex], finalVisibleIndex, filteredRecords.length, records);
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
        const viewButton = event.target.closest('[data-view-record]');
        if (viewButton) {
          const index = Number(viewButton.getAttribute('data-view-record'));
          if (!Number.isNaN(index) && records[index]) {
            window.location.hash = detailHash(records[index]);
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


// --- newStaticArchiveRecord ---
func newStaticArchiveRecord(soldier models.Soldier, idIndex map[int64]models.Soldier) StaticArchiveRecord {
	record := StaticArchiveRecord{
		DisplayID:         strings.TrimSpace(soldier.DisplayID),
		EntryType:         strings.TrimSpace(soldier.EntryType),
		DisplayType:       displayEntryType(soldier),
		Name:              soldierDisplayName(soldier),
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
				record.SpouseName = soldierDisplayName(linked)
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
	defer source.Close()
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
			defer source.Close()
			_, err = io.Copy(entry, source)
			return err
		})
	})
}

