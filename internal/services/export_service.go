package services

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
)

const exportBatchSize = 500

var pdfURLPattern = regexp.MustCompile(`https?://[^\s<]+`)

type ExportService struct {
	db      *db.DB
	soldier *SoldierService
}

type ExportMetadata struct {
	AppVersion    string `json:"app_version"`
	SchemaVersion int    `json:"schema_version"`
	Format        string `json:"format"`
	Version       int    `json:"version"`
	GeneratedAt   string `json:"generated_at"`
}

type JSONExportDocument struct {
	Metadata ExportMetadata   `json:"metadata"`
	Soldiers []models.Soldier `json:"soldiers"`
}

type StaticArchiveRecord struct {
	DisplayID    string                     `json:"displayId"`
	EntryType    string                     `json:"entryType"`
	Name         string                     `json:"name"`
	Dates        string                     `json:"dates"`
	Rank         string                     `json:"rank,omitempty"`
	RankIn       string                     `json:"rankIn,omitempty"`
	RankOut      string                     `json:"rankOut,omitempty"`
	Unit         string                     `json:"unit,omitempty"`
	Location     string                     `json:"location,omitempty"`
	Notes        string                     `json:"notes,omitempty"`
	MaidenName   string                     `json:"maidenName,omitempty"`
	SpouseName   string                     `json:"spouseName,omitempty"`
	PensionID    string                     `json:"pensionId,omitempty"`
	AppID        string                     `json:"appId,omitempty"`
	PensionState string                     `json:"pensionState,omitempty"`
	HomeStatus   string                     `json:"homeStatus,omitempty"`
	HomeName     string                     `json:"homeName,omitempty"`
	ImagePath    string                     `json:"imagePath,omitempty"`
	Images       []StaticArchiveImage       `json:"images,omitempty"`
	Records      []StaticArchiveRecordEntry `json:"records,omitempty"`
	DisplayType  string                     `json:"displayType"`
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

const staticArchiveIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{ .ArchiveTitle }}</title>
  <script defer src="./data.js"></script>
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
        record.maidenName,
        record.spouseName,
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

    function renderRecord(record, index) {
      return '' +
        '<article class="record-row" data-record-index="' + index + '">' +
          '<div class="row-main">' +
            '<div class="row-meta">' +
              '<span class="pill">' + escapeHtml(record.displayType) + '</span>' +
              '<span class="pill">' + escapeHtml(record.displayId) + '</span>' +
            '</div>' +
            '<h3 class="row-title">' + escapeHtml(record.name) + '</h3>' +
            '<div class="row-summary">' +
              '<span><strong>Dates:</strong> ' + escapeHtml(record.dates || 'Not recorded') + '</span>' +
              '<span><strong>Unit:</strong> ' + escapeHtml(record.unit || 'Not recorded') + '</span>' +
              '<span><strong>Location:</strong> ' + escapeHtml(record.location || 'Not recorded') + '</span>' +
            '</div>' +
            (record.notes ? '<div class="row-excerpt">' + escapeHtml(excerpt(record.notes, 150)) + '</div>' : '') +
          '</div>' +
          '<button type="button" class="action-button" data-view-record="' + index + '">View More</button>' +
        '</article>';
    }

    function renderDetail(record) {
      const details = [
        ['Dates', record.dates || 'Not recorded'],
        ['Rank', record.rankOut || record.rank || record.rankIn || 'Not recorded'],
        ['Unit', record.unit || 'Not recorded'],
        ['Location', record.location || 'Not recorded'],
        ['Record ID', record.displayId || 'Not recorded']
      ];
      if (record.spouseName) {
        details.push(['Married To', record.spouseName]);
      }
      if (record.maidenName) {
        details.push(['Maiden Name', record.maidenName]);
      }
      if (record.homeStatus) {
        details.push(['Confederate Home Status', record.homeStatus]);
      }
      if (record.homeName) {
        details.push(['Confederate Home Name', record.homeName]);
      }
      if (record.pensionId) {
        details.push(['Pension ID', record.pensionId]);
      }
      if (record.appId) {
        details.push(['Application ID', record.appId]);
      }
      if (record.pensionState) {
        details.push(['Pension State', record.pensionState]);
      }

      const primarySections = [];
      const sideSections = [];
      if (record.notes) {
        primarySections.push('<section class="detail-section"><h4>Notes</h4><p>' + escapeHtml(record.notes) + '</p></section>');
      }
      if (record.records && record.records.length) {
        primarySections.push(
          '<section class="detail-section"><h4>Records</h4><ul>' +
            record.records.map(function(item) {
              const app = item.appId ? ' (' + escapeHtml(item.appId) + ')' : '';
              const detailsText = item.details ? '<br>' + escapeHtml(item.details) : '';
              return '<li><strong>' + escapeHtml(item.recordType || 'Record') + '</strong>' + app + detailsText + '</li>';
            }).join('') +
          '</ul></section>'
        );
      }
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
          '</div>' +
          '<h3>' + escapeHtml(record.name) + '</h3>' +
        '</div>' +
        '<div class="detail-layout">' +
          '<div>' +
            '<dl class="detail-grid">' +
              details.map(function(line) {
                return '<dt>' + escapeHtml(line[0]) + '</dt><dd>' + escapeHtml(line[1]) + '</dd>';
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

    function showDetailScreen(record, index, visibleCount) {
      document.getElementById('archive-list-screen').classList.add('hidden');
      document.getElementById('archive-detail-screen').classList.remove('hidden');
      document.getElementById('detail-content').innerHTML = renderDetail(record);
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
        return renderRecord(item.record, item.index);
      }).join('');
      empty.style.display = filtered.length ? 'none' : 'block';
      count.textContent = filtered.length + (filtered.length === 1 ? ' record' : ' records');
      return filtered;
    }

    document.addEventListener('DOMContentLoaded', function() {
      const records = Array.isArray(window.archiveData) ? window.archiveData : [];
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
        showDetailScreen(records[matchIndex], finalVisibleIndex, filteredRecords.length);
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

func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return &ExportService{db: database, soldier: soldier}
}

// ExportJSON writes a full hierarchical export document with metadata and
// soldiers/records/images, processing records in batches to avoid loading the
// entire dataset into memory at once.
func (e *ExportService) ExportJSON(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	payload := JSONExportDocument{
		Metadata: newExportMetadata("json", buildinfo.JSONExportVersion),
		Soldiers: []models.Soldier{},
	}
	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			enriched, err := e.soldier.GetByID(s.ID)
			if err != nil {
				return err
			}
			payload.Soldiers = append(payload.Soldiers, *enriched)
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}

	return enc.Encode(payload)
}

func (e *ExportService) ExportICalendar(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	soldiers, err := exportSoldiers(e.soldier)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, line := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		fmt.Sprintf("PRODID:-//%s v%s//Anniversaries v%d//EN", buildinfo.AppName, buildinfo.AppVersion, buildinfo.ICalendarExportVersion),
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:DixieData Anniversaries",
		fmt.Sprintf("X-DIXIEDATA-APP-VERSION:%s", buildinfo.AppVersion),
		fmt.Sprintf("X-DIXIEDATA-SCHEMA-VERSION:%d", buildinfo.SchemaVersion),
		fmt.Sprintf("X-DIXIEDATA-EXPORT-VERSION:%d", buildinfo.ICalendarExportVersion),
	} {
		if err := writeICalendarLine(f, line); err != nil {
			return err
		}
	}

	for _, soldier := range soldiers {
		if soldier.DeathMonth < 1 || soldier.DeathDay < 1 {
			continue
		}
		start := nextGoogleAnniversaryDate(soldier, now)
		start = time.Date(start.Year(), start.Month(), start.Day(), 9, 0, 0, 0, time.Local)
		end := start.Add(time.Hour)
		description := fmt.Sprintf("Database Number: %s\nUnit: %s\nBuried In: %s\nOriginal Death Date: %s\nGenerated by DixieData.",
			soldier.DisplayID,
			soldier.Unit,
			soldier.BuriedIn,
			soldierDeathLine(soldier),
		)

		lines := []string{
			"BEGIN:VEVENT",
			fmt.Sprintf("UID:%s", icalText("dixiedata-"+strings.ToLower(soldier.DisplayID)+"@dixiedata.local")),
			fmt.Sprintf("DTSTAMP:%s", now.Format("20060102T150405Z")),
			fmt.Sprintf("SUMMARY:%s", icalText("DixieData Anniversary: "+soldierDisplayName(soldier))),
			fmt.Sprintf("DESCRIPTION:%s", icalText(description)),
			fmt.Sprintf("DTSTART:%s", start.Format("20060102T150405")),
			fmt.Sprintf("DTEND:%s", end.Format("20060102T150405")),
			"RRULE:FREQ=YEARLY",
			"STATUS:CONFIRMED",
			"TRANSP:TRANSPARENT",
			"BEGIN:VALARM",
			"TRIGGER:-P1D",
			"ACTION:DISPLAY",
			fmt.Sprintf("DESCRIPTION:%s", icalText("Upcoming anniversary for "+soldierDisplayName(soldier))),
			"END:VALARM",
			"BEGIN:VALARM",
			"TRIGGER:-PT1H",
			"ACTION:DISPLAY",
			fmt.Sprintf("DESCRIPTION:%s", icalText("Anniversary in one hour for "+soldierDisplayName(soldier))),
			"END:VALARM",
			"END:VEVENT",
		}
		for _, line := range lines {
			if err := writeICalendarLine(f, line); err != nil {
				return err
			}
		}
	}

	return writeICalendarLine(f, "END:VCALENDAR")
}

// ExportCSV streams a flat CSV export of all soldiers, processing records in
// batches to avoid loading the entire dataset into memory at once.
func (e *ExportService) ExportCSV(outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	metadata := newExportMetadata("csv", buildinfo.CSVExportVersion)
	header := []string{"app_version", "schema_version", "export_version", "generated_at", "id", "display_id", "entry_type", "spouse_soldier_id", "maiden_name", "is_generated", "pension_id", "application_id", "first_name", "middle_name", "last_name", "rank", "rank_in", "rank_out", "unit", "pension_state", "confederate_home_status", "confederate_home_name", "birth_date", "death_date", "birth_info", "buried_in", "notes", "created_at"}
	if err := w.Write(header); err != nil {
		return err
	}

	page := 1
	for {
		batch, _, err := e.soldier.List(page, exportBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, s := range batch {
			row := []string{
				metadata.AppVersion,
				fmt.Sprintf("%d", metadata.SchemaVersion),
				fmt.Sprintf("%d", metadata.Version),
				metadata.GeneratedAt,
				fmt.Sprintf("%d", s.ID),
				s.DisplayID,
				s.EntryType,
				fmt.Sprintf("%d", s.SpouseSoldierID),
				s.MaidenName,
				fmt.Sprintf("%v", s.IsGenerated),
				s.PensionID,
				s.ApplicationID,
				s.FirstName,
				s.MiddleName,
				s.LastName,
				s.Rank,
				s.RankIn,
				s.RankOut,
				s.Unit,
				s.PensionState,
				s.ConfederateHomeStatus,
				s.ConfederateHomeName,
				s.BirthDate,
				s.DeathDate,
				s.BirthInfo,
				s.BuriedIn,
				s.Notes,
				s.CreatedAt,
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}

		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	return nil
}

func (e *ExportService) StaticArchiveFileName(now time.Time) (string, error) {
	owner, err := e.staticArchiveOwner()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("DixieData_Archive_%s_%s.zip", owner.FileStem, now.Format("2006-01-02")), nil
}

func (e *ExportService) ExportStaticArchive(outputPath, dataDir string) error {
	owner, err := e.staticArchiveOwner()
	if err != nil {
		return err
	}
	records, err := e.staticArchiveRecords()
	if err != nil {
		return err
	}

	exportRoot, err := os.MkdirTemp("", "dixiedata-static-archive-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(exportRoot)

	if err := copyDirectoryContents(filepath.Join(dataDir, "images"), filepath.Join(exportRoot, "images")); err != nil {
		return err
	}

	dataPayload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	dataJS := "const archiveData = " + string(dataPayload) + ";\nwindow.archiveData = archiveData;\n"
	if err := os.WriteFile(filepath.Join(exportRoot, "data.js"), []byte(dataJS), 0o644); err != nil {
		return err
	}

	indexHTML, err := renderStaticArchiveIndex(staticArchiveIndexData{
		ArchiveTitle: owner.DisplayName + "'s Civil War Research Archive",
		OwnerShort:   owner.DisplayName,
		Version:      buildinfo.AppVersion,
		Build:        buildinfo.BuildIdentity(),
		GeneratedAt:  time.Now().Format("January 2, 2006"),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(exportRoot, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}

	return zipDirectory(outputPath, exportRoot)
}

func (e *ExportService) ExportImages(outputPath string, images []models.Image) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	for _, image := range images {
		source, err := os.Open(image.FilePath)
		if err != nil {
			return err
		}

		entryName := image.FileName
		if entryName == "" {
			entryName = filepath.Base(image.FilePath)
		}

		entry, err := zipWriter.Create(entryName)
		if err != nil {
			source.Close()
			return err
		}

		if _, err := io.Copy(entry, source); err != nil {
			source.Close()
			return err
		}
		source.Close()
	}

	return nil
}

func (e *ExportService) ExportSoldierPDF(outputPath string, soldier models.Soldier) error {
	return e.exportSoldierPDF(outputPath, soldier, true)
}

func (e *ExportService) ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error {
	return e.exportSoldierPDF(outputPath, soldier, false)
}

func (e *ExportService) exportSoldierPDF(outputPath string, soldier models.Soldier, includeImages bool) error {
	pdf := newPDFDocument("Soldier Report", "soldier-pdf", buildinfo.SoldierPDFExportVersion)
	pdf.AddPage()

	pdf.SetFont("Times", "B", 20)
	pdf.CellFormat(0, 12, soldierDisplayName(soldier), "", 1, "", false, 0, "")
	pdf.SetFont("Times", "", 12)
	pdf.CellFormat(0, 8, fmt.Sprintf("Database Number: %s", soldier.DisplayID), "", 1, "", false, 0, "")
	pdf.Ln(2)

	writePDFField(pdf, "Record Type", displayEntryType(soldier))
	if isSoldierEntry(soldier) {
		writePDFField(pdf, "Rank In", soldier.RankIn)
		writePDFField(pdf, "Rank Out", displaySoldierRank(soldier))
		writePDFField(pdf, "Unit", soldier.Unit)
		writePDFField(pdf, "Pension State", soldier.PensionState)
		writePDFField(pdf, "Confederate Home Status", soldier.ConfederateHomeStatus)
		writePDFField(pdf, "Confederate Home Name", soldier.ConfederateHomeName)
		writePDFField(pdf, "Pension ID", soldier.PensionID)
		writePDFField(pdf, "Application ID", soldier.ApplicationID)
	} else {
		writePDFField(pdf, "Married To", strings.TrimSpace(soldier.SpouseName))
		writePDFField(pdf, "Maiden Name", soldier.MaidenName)
		if soldier.EntryType == "widow" {
			writePDFField(pdf, "Pension ID", soldier.PensionID)
			writePDFField(pdf, "Application ID", soldier.ApplicationID)
		}
	}
	writePDFField(pdf, "Death", soldierDeathLine(soldier))
	writePDFField(pdf, "Birth Info", soldier.BirthInfo)
	writePDFField(pdf, "Buried In", soldier.BuriedIn)
	writePDFSection(pdf, "Notes")
	writePDFBody(pdf, soldier.Notes)

	if len(soldier.Records) > 0 {
		writePDFSection(pdf, "Records")
		for _, record := range soldier.Records {
			line := record.RecordType
			if strings.TrimSpace(record.AppID) != "" {
				line += fmt.Sprintf(" (App: %s)", record.AppID)
			}
			writePDFBullet(pdf, line)
			if strings.TrimSpace(record.Details) != "" {
				writePDFBody(pdf, record.Details)
			}
		}
	}

	if includeImages && len(soldier.Images) > 0 {
		writePDFSection(pdf, "Images")
		for _, image := range soldier.Images {
			writePDFImageRow(pdf, image)
		}
	}

	writePDFExportMetadata(pdf, "soldier-pdf", buildinfo.SoldierPDFExportVersion, map[string]string{
		"Includes Images": fmt.Sprintf("%t", includeImages),
	})

	return pdf.OutputFileAndClose(outputPath)
}

func (e *ExportService) ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier) error {
	pdf := newPDFDocument("Monthly Anniversary Report", "monthly-pdf", buildinfo.MonthlyPDFExportVersion)
	pdf.AddPage()

	title := fmt.Sprintf("%s Anniversary Report", monthLabel(month))
	pdf.SetFont("Times", "B", 20)
	pdf.CellFormat(0, 12, title, "", 1, "", false, 0, "")
	pdf.SetFont("Times", "", 12)
	pdf.CellFormat(0, 8, "Includes soldier names and database numbers for the selected month.", "", 1, "", false, 0, "")
	pdf.Ln(2)

	days := make([]int, 0, len(calendar))
	for day := range calendar {
		if day == 0 {
			continue
		}
		days = append(days, day)
	}
	sort.Ints(days)

	if len(days) == 0 {
		writePDFBody(pdf, "No soldiers are recorded for this month.")
		writePDFExportMetadata(pdf, "monthly-pdf", buildinfo.MonthlyPDFExportVersion, nil)
		return pdf.OutputFileAndClose(outputPath)
	}

	for _, day := range days {
		soldiers := append([]models.Soldier(nil), calendar[day]...)
		sort.Slice(soldiers, func(i, j int) bool {
			left := strings.ToLower(soldierDisplayName(soldiers[i]))
			right := strings.ToLower(soldierDisplayName(soldiers[j]))
			return left < right
		})

		writePDFSection(pdf, fmt.Sprintf("%s %d", monthLabel(month), day))
		for _, soldier := range soldiers {
			writePDFBullet(pdf, fmt.Sprintf("%s - %s", soldierDisplayName(soldier), soldier.DisplayID))
		}
	}

	writePDFExportMetadata(pdf, "monthly-pdf", buildinfo.MonthlyPDFExportVersion, map[string]string{})

	return pdf.OutputFileAndClose(outputPath)
}

func newPDFDocument(title, format string, version int) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetTitle(title, false)
	pdf.SetAuthor(buildinfo.AppLabel(), false)
	pdf.SetCreator(fmt.Sprintf("%s %s export v%d", buildinfo.AppName, format, version), false)
	pdf.SetSubject(fmt.Sprintf("%s schema v%d", buildinfo.AppName, buildinfo.SchemaVersion), false)
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.SetCompression(false)
	return pdf
}

func newExportMetadata(format string, version int) ExportMetadata {
	return ExportMetadata{
		AppVersion:    buildinfo.AppVersion,
		SchemaVersion: buildinfo.SchemaVersion,
		Format:        format,
		Version:       version,
		GeneratedAt:   time.Now().Format(time.RFC3339),
	}
}

func exportSoldiers(soldierSvc *SoldierService) ([]models.Soldier, error) {
	var all []models.Soldier
	page := 1
	for {
		batch, _, err := soldierSvc.List(page, exportBatchSize)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < exportBatchSize {
			break
		}
		page++
	}
	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].DisplayID) < strings.ToLower(all[j].DisplayID)
	})
	return all, nil
}

func (e *ExportService) staticArchiveOwner() (staticArchiveOwner, error) {
	identity, err := e.db.UserIdentity()
	if err != nil {
		return staticArchiveOwner{}, err
	}
	firstName := strings.TrimSpace(identity.FirstName)
	lastName := strings.TrimSpace(identity.LastName)
	if firstName == "" || lastName == "" {
		return staticArchiveOwner{}, fmt.Errorf("user identity is incomplete")
	}
	initial := staticArchiveInitial(firstName)
	if initial == "" {
		return staticArchiveOwner{}, fmt.Errorf("user identity is incomplete")
	}
	fileStem := sanitizeStaticArchiveStem(initial + lastName)
	if fileStem == "" {
		return staticArchiveOwner{}, fmt.Errorf("user identity is incomplete")
	}
	return staticArchiveOwner{
		DisplayName: initial + ". " + lastName,
		FileStem:    fileStem,
	}, nil
}

func (e *ExportService) staticArchiveRecords() ([]StaticArchiveRecord, error) {
	batch, err := exportSoldiers(e.soldier)
	if err != nil {
		return nil, err
	}
	records := make([]StaticArchiveRecord, 0, len(batch))
	for _, item := range batch {
		soldier, err := e.soldier.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		records = append(records, newStaticArchiveRecord(*soldier))
	}
	sort.Slice(records, func(i, j int) bool {
		left := strings.ToLower(records[i].Name + " " + records[i].DisplayID)
		right := strings.ToLower(records[j].Name + " " + records[j].DisplayID)
		return left < right
	})
	return records, nil
}

func newStaticArchiveRecord(soldier models.Soldier) StaticArchiveRecord {
	record := StaticArchiveRecord{
		DisplayID:    strings.TrimSpace(soldier.DisplayID),
		EntryType:    strings.TrimSpace(soldier.EntryType),
		DisplayType:  displayEntryType(soldier),
		Name:         soldierDisplayName(soldier),
		Dates:        staticArchiveDateSummary(soldier),
		Rank:         strings.TrimSpace(soldier.Rank),
		RankIn:       strings.TrimSpace(soldier.RankIn),
		RankOut:      strings.TrimSpace(soldier.RankOut),
		Unit:         strings.TrimSpace(soldier.Unit),
		Location:     strings.TrimSpace(soldier.BuriedIn),
		Notes:        strings.TrimSpace(soldier.Notes),
		MaidenName:   strings.TrimSpace(soldier.MaidenName),
		SpouseName:   strings.TrimSpace(soldier.SpouseName),
		PensionID:    strings.TrimSpace(soldier.PensionID),
		AppID:        strings.TrimSpace(soldier.ApplicationID),
		PensionState: strings.TrimSpace(soldier.PensionState),
		HomeStatus:   strings.TrimSpace(soldier.ConfederateHomeStatus),
		HomeName:     strings.TrimSpace(soldier.ConfederateHomeName),
		Images:       make([]StaticArchiveImage, 0, len(soldier.Images)),
		Records:      make([]StaticArchiveRecordEntry, 0, len(soldier.Records)),
	}
	if record.HomeStatus == "None" {
		record.HomeStatus = ""
	}
	if record.PensionState == "None" {
		record.PensionState = ""
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

func staticArchiveDateSummary(soldier models.Soldier) string {
	birth := strings.TrimSpace(strings.ReplaceAll(dates.Display(soldier.BirthDate), "Not recorded", ""))
	death := strings.TrimSpace(strings.ReplaceAll(dates.Display(soldier.DeathDate), "Not recorded", ""))
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

func staticArchiveInitial(value string) string {
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return string(r)
		}
	}
	return ""
}

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

func zipDirectory(outputPath, root string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

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
}

func icalText(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		";", "\\;",
		",", "\\,",
		"\r\n", "\\n",
		"\n", "\\n",
	)
	return replacer.Replace(strings.TrimSpace(value))
}

func nextAnniversaryDate(soldier models.Soldier, now time.Time) time.Time {
	year := now.Year()
	for i := 0; i < 8; i++ {
		candidateYear := year + i
		candidate := time.Date(candidateYear, time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.UTC)
		if candidate.Month() != time.Month(soldier.DeathMonth) || candidate.Day() != soldier.DeathDay {
			continue
		}
		if !candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)) {
			return candidate
		}
	}
	return time.Date(now.Year(), time.Month(soldier.DeathMonth), soldier.DeathDay, 0, 0, 0, 0, time.UTC)
}

func writeICalendarLine(w io.Writer, line string) error {
	const maxLineLength = 75
	for len(line) > maxLineLength {
		if _, err := fmt.Fprintf(w, "%s\r\n ", line[:maxLineLength]); err != nil {
			return err
		}
		line = line[maxLineLength:]
	}
	_, err := fmt.Fprintf(w, "%s\r\n", line)
	return err
}

func writePDFSection(pdf *fpdf.Fpdf, title string) {
	pdf.Ln(4)
	pdf.SetFont("Times", "B", 15)
	pdf.CellFormat(0, 8, title, "", 1, "", false, 0, "")
	pdf.SetFont("Times", "", 12)
}

func writePDFExportMetadata(pdf *fpdf.Fpdf, format string, version int, details map[string]string) {
	writePDFSection(pdf, "Report Metadata")
	writePDFField(pdf, "App Version", buildinfo.AppVersion)
	writePDFField(pdf, "Schema Version", fmt.Sprintf("%d", buildinfo.SchemaVersion))
	writePDFField(pdf, "Export Format", format)
	writePDFField(pdf, "Export Version", fmt.Sprintf("%d", version))
	writePDFField(pdf, "Generated At", time.Now().Format(time.RFC3339))
	keys := make([]string, 0, len(details))
	for key := range details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writePDFField(pdf, key, details[key])
	}
}

func writePDFField(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Times", "B", 12)
	pdf.CellFormat(34, 8, label+":", "", 0, "", false, 0, "")
	pdf.SetFont("Times", "", 12)
	pdf.MultiCell(0, 8, emptyPDFValue(value), "", "", false)
}

func writePDFBody(pdf *fpdf.Fpdf, text string) {
	writePDFRichText(pdf, emptyPDFValue(text), 7)
}

func writePDFBullet(pdf *fpdf.Fpdf, text string) {
	pdf.SetFont("Times", "", 12)
	pdf.CellFormat(6, 7, "-", "", 0, "", false, 0, "")
	pdf.MultiCell(0, 7, emptyPDFValue(text), "", "", false)
}

func writePDFRichText(pdf *fpdf.Fpdf, text string, lineHeight float64) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for lineIndex, line := range lines {
		segments := pdfTextSegments(line)
		if len(segments) == 0 {
			segments = []pdfTextSegment{{Text: ""}}
		}
		for _, segment := range segments {
			if segment.Link != "" {
				pdf.SetFont("Times", "I", 12)
				pdf.SetTextColor(48, 87, 122)
				pdf.WriteLinkString(lineHeight, segment.Text, segment.Link)
				pdf.SetTextColor(0, 0, 0)
				pdf.SetFont("Times", "", 12)
				continue
			}
			pdf.SetFont("Times", "", 12)
			pdf.Write(lineHeight, segment.Text)
		}
		if lineIndex < len(lines)-1 {
			pdf.Ln(lineHeight)
		}
	}
	pdf.Ln(lineHeight)
}

type pdfTextSegment struct {
	Text string
	Link string
}

func pdfTextSegments(text string) []pdfTextSegment {
	matches := pdfURLPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return []pdfTextSegment{{Text: text}}
	}

	segments := make([]pdfTextSegment, 0, len(matches)*2+1)
	cursor := 0
	for _, match := range matches {
		start := match[0]
		end := match[1]
		if start > cursor {
			segments = append(segments, pdfTextSegment{Text: text[cursor:start]})
		}

		linkText, suffix := splitPDFURLSuffix(text[start:end])
		if linkText != "" {
			segments = append(segments, pdfTextSegment{Text: linkText, Link: linkText})
		}
		if suffix != "" {
			segments = append(segments, pdfTextSegment{Text: suffix})
		}
		cursor = end
	}
	if cursor < len(text) {
		segments = append(segments, pdfTextSegment{Text: text[cursor:]})
	}
	return segments
}

func splitPDFURLSuffix(value string) (string, string) {
	trimmed := strings.TrimRight(value, ".,;:!?)]}")
	return trimmed, value[len(trimmed):]
}

func writePDFImageRow(pdf *fpdf.Fpdf, image models.Image) {
	const thumbnailWidth = 34.0
	const rowHeight = 36.0

	_, pageHeight := pdf.GetPageSize()
	if pdf.GetY()+rowHeight > pageHeight-16 {
		pdf.AddPage()
	}

	x := pdf.GetX()
	y := pdf.GetY()
	imagePath := imagePathForPDF(image)

	if imagePath != "" {
		pdf.ImageOptions(imagePath, x, y, thumbnailWidth, 0, false, fpdf.ImageOptions{
			ImageType: strings.TrimPrefix(strings.ToLower(filepath.Ext(imagePath)), "."),
		}, 0, "")
	} else {
		pdf.Rect(x, y, thumbnailWidth, 22, "D")
	}

	pdf.SetXY(x+thumbnailWidth+4, y)
	title := image.FileName
	if strings.TrimSpace(image.Caption) != "" {
		title = fmt.Sprintf("%s - %s", image.FileName, image.Caption)
	}
	pdf.SetFont("Times", "", 12)
	pdf.MultiCell(0, 6, emptyPDFValue(title), "", "", false)
	pdf.SetXY(x+thumbnailWidth+4, y+10)
	pdf.MultiCell(0, 6, "DB Path: "+emptyPDFValue(image.FilePath), "", "", false)
	pdf.SetXY(x+thumbnailWidth+4, y+18)
	pdf.MultiCell(0, 6, "Full Path: "+emptyPDFValue(strings.TrimSpace(image.ResolvedPath)), "", "", false)

	if pdf.GetY() < y+rowHeight {
		pdf.SetY(y + rowHeight)
	}
}

func emptyPDFValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Not recorded"
	}
	return value
}

func soldierDisplayName(soldier models.Soldier) string {
	if isSoldierEntry(soldier) {
		return strings.TrimSpace(strings.TrimSpace(displaySoldierRank(soldier)) + " " + soldierFullName(soldier))
	}
	if name := soldierFullName(soldier); name != "" {
		return name
	}
	return displayEntryType(soldier)
}

func soldierFullName(soldier models.Soldier) string {
	return strings.Join(compactNameParts(soldier.FirstName, soldier.MiddleName, soldier.LastName), " ")
}

func displaySoldierRank(soldier models.Soldier) string {
	if strings.TrimSpace(soldier.RankOut) != "" {
		return strings.TrimSpace(soldier.RankOut)
	}
	if strings.TrimSpace(soldier.Rank) != "" {
		return strings.TrimSpace(soldier.Rank)
	}
	return strings.TrimSpace(soldier.RankIn)
}

func isSoldierEntry(soldier models.Soldier) bool {
	return strings.TrimSpace(soldier.EntryType) == "" || soldier.EntryType == "soldier"
}

func displayEntryType(soldier models.Soldier) string {
	switch soldier.EntryType {
	case "wife":
		return "Wife"
	case "widow":
		return "Widow"
	default:
		return "Soldier"
	}
}

func compactNameParts(parts ...string) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func soldierDeathLine(soldier models.Soldier) string {
	return strings.TrimSpace(strings.ReplaceAll(dates.Display(soldier.DeathDate), "Not recorded", ""))
}

func monthLabel(month int) string {
	if month < 1 || month > 12 {
		return "Unknown"
	}
	return time.Month(month).String()
}

func imagePathForPDF(image models.Image) string {
	candidate := strings.TrimSpace(image.ResolvedPath)
	if candidate == "" {
		candidate = strings.TrimSpace(image.FilePath)
	}
	if candidate == "" {
		return ""
	}
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return ""
	}
	switch strings.ToLower(filepath.Ext(candidate)) {
	case ".jpg", ".jpeg", ".png", ".gif":
	default:
		return ""
	}
	if !validPDFImage(candidate) {
		return ""
	}
	return candidate
}

func validPDFImage(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		return false
	}
	switch strings.ToLower(format) {
	case "jpeg", "png", "gif":
		return true
	default:
		return false
	}
}
