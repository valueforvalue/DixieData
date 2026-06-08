package archive

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"errors"
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
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/valueforvalue/DixieData/internal/buildinfo"
	"github.com/valueforvalue/DixieData/internal/confederatehomestatus"
	"github.com/valueforvalue/DixieData/internal/dates"
	"github.com/valueforvalue/DixieData/internal/db"
	"github.com/valueforvalue/DixieData/internal/models"
	"github.com/valueforvalue/DixieData/internal/pensionstate"
	"github.com/valueforvalue/DixieData/internal/persondisplay"
	"github.com/xuri/excelize/v2"
)

const exportBatchSize = 500

var pdfURLPattern = regexp.MustCompile(`https?://[^\s<]+`)

type ExportService struct {
	db         *db.DB
	soldier    *SoldierService
	rasterizer pdfToJPEGRasterizer
}

type pdfToJPEGRasterizer interface {
	Rasterize(pdfPath, outputDir string) ([]string, error)
}

type PrintSettings struct {
	Scope                        string  `json:"scope"`
	Orientation                  string  `json:"orientation"`
	PrinterFriendly              bool    `json:"printerFriendly"`
	FullBiographyPage            bool    `json:"fullBiographyPage"`
	SortBy                       string  `json:"sortBy"`
	GroupByUnit                  bool    `json:"groupByUnit"`
	GroupByPensionState          bool    `json:"groupByPensionState"`
	GroupByConfederateHomeStatus bool    `json:"groupByConfederateHomeStatus"`
	GroupByBuriedIn              bool    `json:"groupByBuriedIn"`
	FilterBuriedIn               []string `json:"filterBuriedIn"`
	FilterEntryTypes             []string `json:"filterEntryTypes"`
	FilterUnits                  []string `json:"filterUnits"`
	FilterPensionStates          []string `json:"filterPensionStates"`
	FilterConfederateHomeStatus  []string `json:"filterConfederateHomeStatuses"`
	ExportAll                    bool    `json:"exportAll"`
	SelectedIDs                  []int64 `json:"selectedIds"`
}

type PDFOptions struct {
	Orientation      string `json:"orientation"`
	PrinterFriendly  bool   `json:"printerFriendly"`
	IncludeImages    bool   `json:"includeImages"`
	PrintableArchive bool   `json:"printableArchive"`
}

const (
	PrintSortLastName  = "last_name"
	PrintSortBirthYear = "birth_year"
	PrintSortDeathYear = "death_year"

	PrintScopeAll      = "all"
	PrintScopeFiltered = "filtered"
	PrintScopeSelected = "selected"

	printFilterUnknownValue = "__unknown__"
)

func (s PrintSettings) Normalize() PrintSettings {
	options := PDFOptions{
		Orientation:     s.Orientation,
		PrinterFriendly: s.PrinterFriendly,
		IncludeImages:   false,
	}.Normalize("L", false)
	s.Orientation = options.Orientation
	s.PrinterFriendly = options.PrinterFriendly
	s.SortBy = strings.TrimSpace(strings.ToLower(s.SortBy))
	switch s.SortBy {
	case PrintSortBirthYear, PrintSortDeathYear:
	default:
		s.SortBy = PrintSortLastName
	}
	s.Scope = normalizePrintScope(s.Scope, s.ExportAll, s.SelectedIDs)
	s.SelectedIDs = normalizeSelectedPrintIDs(s.SelectedIDs)
	s.FilterBuriedIn = normalizePrintFilterValues(s.FilterBuriedIn)
	s.FilterEntryTypes = normalizePrintFilterValues(s.FilterEntryTypes)
	s.FilterUnits = normalizePrintFilterValues(s.FilterUnits)
	s.FilterPensionStates = normalizePrintFilterValues(s.FilterPensionStates)
	s.FilterConfederateHomeStatus = normalizePrintFilterValues(s.FilterConfederateHomeStatus)
	if s.Scope == PrintScopeFiltered && !s.HasFilters() {
		s.Scope = PrintScopeAll
	}
	s.ExportAll = s.Scope == PrintScopeAll
	return s
}

func (s PrintSettings) HasFilters() bool {
	return len(s.FilterBuriedIn) > 0 ||
		len(s.FilterEntryTypes) > 0 ||
		len(s.FilterUnits) > 0 ||
		len(s.FilterPensionStates) > 0 ||
		len(s.FilterConfederateHomeStatus) > 0
}

func normalizePrintScope(scope string, exportAll bool, selectedIDs []int64) string {
	switch strings.TrimSpace(strings.ToLower(scope)) {
	case PrintScopeFiltered:
		return PrintScopeFiltered
	case PrintScopeSelected:
		return PrintScopeSelected
	case PrintScopeAll:
		return PrintScopeAll
	}
	if !exportAll || len(selectedIDs) > 0 {
		return PrintScopeSelected
	}
	return PrintScopeAll
}

func normalizePrintFilterValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return strings.ToLower(normalized[i]) < strings.ToLower(normalized[j])
	})
	return normalized
}

func (o PDFOptions) Normalize(defaultOrientation string, _ bool) PDFOptions {
	o.Orientation = strings.TrimSpace(strings.ToUpper(o.Orientation))
	switch o.Orientation {
	case "P", "L":
	default:
		o.Orientation = strings.TrimSpace(strings.ToUpper(defaultOrientation))
		if o.Orientation != "P" && o.Orientation != "L" {
			o.Orientation = "P"
		}
	}
	return o
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

type pdfField struct {
	Label         string
	Value         string
	EmptyValue    string
	ShowWhenEmpty bool
}

func defaultPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "N/A"}
}

func blankPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, ShowWhenEmpty: true}
}

func unknownDatePDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "Unknown", ShowWhenEmpty: true}
}

func naPDFField(label, value string) pdfField {
	return pdfField{Label: label, Value: value, EmptyValue: "N/A", ShowWhenEmpty: true}
}

func (field pdfField) renderedValue() string {
	trimmed := strings.TrimSpace(field.Value)
	switch strings.ToUpper(trimmed) {
	case "", "NONE", "NA", "N/A", "NOT RECORDED":
		return sanitizePDFText(field.EmptyValue)
	default:
		return sanitizePDFText(trimmed)
	}
}

func (field pdfField) visible() bool {
	trimmed := strings.TrimSpace(field.Value)
	switch strings.ToUpper(trimmed) {
	case "", "NONE", "NA", "N/A", "NOT RECORDED":
		return field.ShowWhenEmpty
	default:
		return true
	}
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

type pdfBranding struct {
	ArchiveTitle string
	FooterText   string
}

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

func NewExportService(database *db.DB, soldier *SoldierService) *ExportService {
	return &ExportService{
		db:         database,
		soldier:    soldier,
		rasterizer: newPDFJPEGRasterizer(),
	}
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

func (e *ExportService) ExportExcel(outputPath string) error {
	const (
		archiveSheet  = "Archive Export"
		metadataSheet = "Metadata"
		spouseSheet   = "Linked Relationships"
	)

	soldiers, err := exportDetailedSoldiers(e.soldier, nil)
	if err != nil {
		return err
	}
	sort.Slice(soldiers, func(i, j int) bool {
		leftLast := strings.ToLower(strings.TrimSpace(soldiers[i].LastName))
		rightLast := strings.ToLower(strings.TrimSpace(soldiers[j].LastName))
		if leftLast != rightLast {
			return leftLast < rightLast
		}
		leftName := strings.ToLower(strings.TrimSpace(soldierFullName(soldiers[i])))
		rightName := strings.ToLower(strings.TrimSpace(soldierFullName(soldiers[j])))
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(strings.TrimSpace(soldiers[i].DisplayID)) < strings.ToLower(strings.TrimSpace(soldiers[j].DisplayID))
	})

	workbook := excelize.NewFile()
	workbook.SetSheetName(workbook.GetSheetName(0), archiveSheet)
	workbook.NewSheet(metadataSheet)
	workbook.NewSheet(spouseSheet)

	headerStyle, textStyle, dateStyle, wrapStyle, err := excelExportStyles(workbook)
	if err != nil {
		return err
	}

	metadata := newExportMetadata("xlsx", buildinfo.XLSXExportVersion)
	spouseIndex := map[int64]models.Soldier{}
	for _, soldier := range soldiers {
		spouseIndex[soldier.ID] = soldier
	}

	metadataHeaders := []string{"app_version", "schema_version", "export_version", "generated_at", "format"}
	metadataValues := []string{
		metadata.AppVersion,
		fmt.Sprintf("%d", metadata.SchemaVersion),
		fmt.Sprintf("%d", metadata.Version),
		metadata.GeneratedAt,
		metadata.Format,
	}
	metadataWidths, err := writeExcelHeaderRow(workbook, metadataSheet, metadataHeaders, headerStyle)
	if err != nil {
		return err
	}
	for index, value := range metadataValues {
		cell, _ := excelize.CoordinatesToCellName(index+1, 2)
		if err := workbook.SetCellValue(metadataSheet, cell, value); err != nil {
			return err
		}
		if err := workbook.SetCellStyle(metadataSheet, cell, cell, textStyle); err != nil {
			return err
		}
		updateExcelColumnWidth(metadataWidths, index, value)
	}
	if err := finalizeExcelSheet(workbook, metadataSheet, metadataWidths, 2); err != nil {
		return err
	}

	archiveHeaders := []string{
		"app_version", "schema_version", "export_version", "generated_at",
		"db_id", "display_id", "entry_type",
		"linked_spouse_db_id", "linked_spouse_display_id", "linked_spouse_name",
		"relationship_label", "maiden_name", "is_generated", "pension_id", "application_id",
		"prefix", "first_name", "middle_name", "last_name", "suffix",
		"rank", "rank_in", "rank_out", "unit", "pension_state",
		"confederate_home_status", "confederate_home_name",
		"birth_date", "death_date", "birth_info", "buried_in", "biography", "notes",
		"added_by", "last_edited_by", "last_edited_fields", "last_edited_at",
		"created_at", "updated_at", "records_count", "images_count",
	}
	archiveWidths, err := writeExcelHeaderRow(workbook, archiveSheet, archiveHeaders, headerStyle)
	if err != nil {
		return err
	}
	for rowIndex, soldier := range soldiers {
		rowNumber := rowIndex + 2
		spouse, spouseLinked := spouseIndex[soldier.SpouseSoldierID]
		linkedSpouseDisplayID := ""
		linkedSpouseName := strings.TrimSpace(soldier.SpouseName)
		if spouseLinked {
			linkedSpouseDisplayID = strings.TrimSpace(spouse.DisplayID)
			if linkedSpouseName == "" {
				linkedSpouseName = strings.TrimSpace(soldierFullName(spouse))
			}
		}
		values := []string{
			metadata.AppVersion,
			fmt.Sprintf("%d", metadata.SchemaVersion),
			fmt.Sprintf("%d", metadata.Version),
			metadata.GeneratedAt,
			fmt.Sprintf("%d", soldier.ID),
			soldier.DisplayID,
			soldier.EntryType,
			func() string {
				if soldier.SpouseSoldierID <= 0 {
					return ""
				}
				return fmt.Sprintf("%d", soldier.SpouseSoldierID)
			}(),
			linkedSpouseDisplayID,
			linkedSpouseName,
			soldier.RelationshipLabel,
			soldier.MaidenName,
			fmt.Sprintf("%t", soldier.IsGenerated),
			soldier.PensionID,
			soldier.ApplicationID,
			soldier.Prefix,
			soldier.FirstName,
			soldier.MiddleName,
			soldier.LastName,
			soldier.Suffix,
			soldier.Rank,
			soldier.RankIn,
			soldier.RankOut,
			soldier.Unit,
			soldier.PensionState,
			soldier.ConfederateHomeStatus,
			soldier.ConfederateHomeName,
			soldier.BirthDate,
			soldier.DeathDate,
			soldier.BirthInfo,
			soldier.BuriedIn,
			soldier.Biography,
			soldier.Notes,
			soldier.AddedBy,
			soldier.LastEditedBy,
			soldier.LastEditedFields,
			soldier.LastEditedAt,
			soldier.CreatedAt,
			soldier.UpdatedAt,
			fmt.Sprintf("%d", len(soldier.Records)),
			fmt.Sprintf("%d", len(soldier.Images)),
		}
		for columnIndex, value := range values {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowNumber)
			switch archiveHeaders[columnIndex] {
			case "db_id", "records_count", "images_count", "schema_version", "export_version":
				parsed, parseErr := strconv.Atoi(value)
				if parseErr == nil {
					if err := workbook.SetCellValue(archiveSheet, cell, parsed); err != nil {
						return err
					}
				} else if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
			case "birth_date", "death_date":
				displayValue, err := setExcelHistoricalDateCell(workbook, archiveSheet, cell, value, dateStyle, textStyle)
				if err != nil {
					return err
				}
				updateExcelColumnWidth(archiveWidths, columnIndex, displayValue)
				continue
			case "display_id", "linked_spouse_display_id", "app_version", "generated_at", "entry_type", "linked_spouse_name", "relationship_label", "maiden_name", "pension_id", "application_id", "prefix", "first_name", "middle_name", "last_name", "suffix", "rank", "rank_in", "rank_out", "unit", "pension_state", "confederate_home_status", "confederate_home_name", "birth_info", "buried_in", "biography", "notes", "added_by", "last_edited_by", "last_edited_fields", "last_edited_at", "created_at", "updated_at":
				if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
				if err := workbook.SetCellStyle(archiveSheet, cell, cell, textStyle); err != nil {
					return err
				}
				if archiveHeaders[columnIndex] == "birth_info" || archiveHeaders[columnIndex] == "biography" || archiveHeaders[columnIndex] == "notes" || archiveHeaders[columnIndex] == "last_edited_fields" {
					if err := workbook.SetCellStyle(archiveSheet, cell, cell, wrapStyle); err != nil {
						return err
					}
				}
			case "is_generated":
				if err := workbook.SetCellValue(archiveSheet, cell, soldier.IsGenerated); err != nil {
					return err
				}
			default:
				if err := workbook.SetCellValue(archiveSheet, cell, value); err != nil {
					return err
				}
			}
			updateExcelColumnWidth(archiveWidths, columnIndex, value)
		}
	}
	if err := finalizeExcelSheet(workbook, archiveSheet, archiveWidths, len(soldiers)+1); err != nil {
		return err
	}

	spouseHeaders := []string{
		"record_display_id", "record_name", "record_entry_type",
		"linked_spouse_db_id", "linked_spouse_display_id", "linked_spouse_name", "linked_spouse_entry_type",
		"relationship_label", "maiden_name",
	}
	spouseWidths, err := writeExcelHeaderRow(workbook, spouseSheet, spouseHeaders, headerStyle)
	if err != nil {
		return err
	}
	spouseRow := 2
	for _, soldier := range soldiers {
		if soldier.SpouseSoldierID <= 0 && strings.TrimSpace(soldier.SpouseName) == "" && strings.TrimSpace(soldier.RelationshipLabel) == "" && strings.TrimSpace(soldier.MaidenName) == "" {
			continue
		}
		spouse, spouseLinked := spouseIndex[soldier.SpouseSoldierID]
		rowValues := []string{
			soldier.DisplayID,
			soldierDisplayName(soldier),
			displayEntryType(soldier),
			func() string {
				if soldier.SpouseSoldierID <= 0 {
					return ""
				}
				return fmt.Sprintf("%d", soldier.SpouseSoldierID)
			}(),
			func() string {
				if spouseLinked {
					return spouse.DisplayID
				}
				return ""
			}(),
			func() string {
				if strings.TrimSpace(soldier.SpouseName) != "" {
					return soldier.SpouseName
				}
				if spouseLinked {
					return soldierFullName(spouse)
				}
				return ""
			}(),
			func() string {
				if spouseLinked {
					return displayEntryType(spouse)
				}
				return ""
			}(),
			soldier.RelationshipLabel,
			soldier.MaidenName,
		}
		for columnIndex, value := range rowValues {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, spouseRow)
			if err := workbook.SetCellValue(spouseSheet, cell, value); err != nil {
				return err
			}
			if err := workbook.SetCellStyle(spouseSheet, cell, cell, textStyle); err != nil {
				return err
			}
			updateExcelColumnWidth(spouseWidths, columnIndex, value)
		}
		spouseRow++
	}
	if err := finalizeExcelSheet(workbook, spouseSheet, spouseWidths, spouseRow-1); err != nil {
		return err
	}

	activeSheet, _ := workbook.GetSheetIndex(archiveSheet)
	workbook.SetActiveSheet(activeSheet)
	return workbook.SaveAs(outputPath)
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
	dtstamp := now.UTC()
	for _, line := range []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		fmt.Sprintf("PRODID:-//%s v%s//Memorial Anniversaries v%d//EN", buildinfo.AppName, buildinfo.AppVersion, buildinfo.ICalendarExportVersion),
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:DixieData Memorial Anniversaries",
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
		description := strings.Join(compactICalendarDescriptionLines(
			"Record ID: "+emptyPDFValue(strings.TrimSpace(soldier.DisplayID)),
			"Entry Type: "+displayEntryType(soldier),
			"Full Name: "+emptyPDFValue(strings.TrimSpace(recordPDFTitle(soldier))),
			func() string {
				rank := strings.TrimSpace(displaySoldierRank(soldier))
				if rank == "" {
					return ""
				}
				return "Rank: " + rank
			}(),
			func() string {
				rankIn := strings.TrimSpace(soldier.RankIn)
				if rankIn == "" {
					return ""
				}
				return "Rank In: " + rankIn
			}(),
			func() string {
				rankOut := strings.TrimSpace(soldier.RankOut)
				if rankOut == "" {
					return ""
				}
				return "Rank Out: " + rankOut
			}(),
			"Unit: "+emptyPDFValue(strings.TrimSpace(soldier.Unit)),
			"Buried In: "+emptyPDFValue(strings.TrimSpace(soldier.BuriedIn)),
			"Original Death Date: "+emptyPDFValue(soldierDeathLine(soldier)),
			"Generated by DixieData.",
		), "\n")

		lines := []string{
			"BEGIN:VEVENT",
			fmt.Sprintf("UID:%s", icalText("dixiedata-"+strings.ToLower(soldier.DisplayID)+"@dixiedata.local")),
			fmt.Sprintf("DTSTAMP:%s", dtstamp.Format("20060102T150405Z")),
			fmt.Sprintf("SUMMARY:%s", icalText("Memorial Anniversary: "+soldierDisplayName(soldier))),
			fmt.Sprintf("DESCRIPTION:%s", icalText(description)),
			fmt.Sprintf("DTSTART:%s", start.Format("20060102T150405")),
			fmt.Sprintf("DTEND:%s", end.Format("20060102T150405")),
			"RRULE:FREQ=YEARLY",
			"STATUS:CONFIRMED",
			"TRANSP:TRANSPARENT",
			"BEGIN:VALARM",
			"TRIGGER:-P1D",
			"ACTION:DISPLAY",
			fmt.Sprintf("DESCRIPTION:%s", icalText("Upcoming memorial anniversary for "+soldierDisplayName(soldier))),
			"END:VALARM",
			"BEGIN:VALARM",
			"TRIGGER:-PT1H",
			"ACTION:DISPLAY",
			fmt.Sprintf("DESCRIPTION:%s", icalText("Memorial anniversary in one hour for "+soldierDisplayName(soldier))),
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
	header := []string{"app_version", "schema_version", "export_version", "generated_at", "id", "display_id", "entry_type", "spouse_soldier_id", "relationship_label", "maiden_name", "is_generated", "pension_id", "application_id", "prefix", "first_name", "middle_name", "last_name", "suffix", "rank", "rank_in", "rank_out", "unit", "pension_state", "confederate_home_status", "confederate_home_name", "birth_date", "death_date", "birth_info", "buried_in", "biography", "notes", "added_by", "last_edited_by", "last_edited_fields", "last_edited_at", "created_at", "updated_at"}
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
				s.RelationshipLabel,
				s.MaidenName,
				fmt.Sprintf("%v", s.IsGenerated),
				s.PensionID,
				s.ApplicationID,
				s.Prefix,
				s.FirstName,
				s.MiddleName,
				s.LastName,
				s.Suffix,
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
				s.Biography,
				s.Notes,
				s.AddedBy,
				s.LastEditedBy,
				s.LastEditedFields,
				s.LastEditedAt,
				s.CreatedAt,
				s.UpdatedAt,
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

func excelExportStyles(workbook *excelize.File) (int, int, int, int, error) {
	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "22303D"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F2EDE1"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "bottom", Color: "8D7440", Style: 1},
		},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	textStyle, err := workbook.NewStyle(&excelize.Style{
		NumFmt:    49,
		Alignment: &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	dateStyle, err := workbook.NewStyle(&excelize.Style{
		CustomNumFmt: excelStringPtr("mm/dd/yyyy"),
		Alignment:    &excelize.Alignment{Vertical: "top"},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	wrapStyle, err := workbook.NewStyle(&excelize.Style{
		NumFmt: 49,
		Alignment: &excelize.Alignment{
			Vertical: "top",
			WrapText: true,
		},
	})
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return headerStyle, textStyle, dateStyle, wrapStyle, nil
}

func writeExcelHeaderRow(workbook *excelize.File, sheet string, headers []string, headerStyle int) ([]float64, error) {
	widths := make([]float64, len(headers))
	for index, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := workbook.SetCellValue(sheet, cell, header); err != nil {
			return nil, err
		}
		if err := workbook.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return nil, err
		}
		updateExcelColumnWidth(widths, index, header)
	}
	return widths, nil
}

func finalizeExcelSheet(workbook *excelize.File, sheet string, widths []float64, rowCount int) error {
	if rowCount > 0 && len(widths) > 0 {
		lastColumn, _ := excelize.ColumnNumberToName(len(widths))
		if err := workbook.AutoFilter(sheet, fmt.Sprintf("A1:%s%d", lastColumn, rowCount), []excelize.AutoFilterOptions{}); err != nil {
			return err
		}
	}
	for index, width := range widths {
		column, _ := excelize.ColumnNumberToName(index + 1)
		if err := workbook.SetColWidth(sheet, column, column, width); err != nil {
			return err
		}
	}
	return workbook.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	})
}

func updateExcelColumnWidth(widths []float64, index int, value string) {
	if index < 0 || index >= len(widths) {
		return
	}
	estimated := float64(len(strings.TrimSpace(value)))*1.15 + 2
	if estimated < 10 {
		estimated = 10
	}
	if estimated > 64 {
		estimated = 64
	}
	if estimated > widths[index] {
		widths[index] = estimated
	}
}

func setExcelHistoricalDateCell(workbook *excelize.File, sheet, cell, canonical string, dateStyle, textStyle int) (string, error) {
	if dateValue, ok := excelDateValue(canonical); ok {
		if err := workbook.SetCellValue(sheet, cell, dateValue); err != nil {
			return "", err
		}
		if err := workbook.SetCellStyle(sheet, cell, cell, dateStyle); err != nil {
			return "", err
		}
		return dateValue.Format("01/02/2006"), nil
	}
	if err := workbook.SetCellValue(sheet, cell, canonical); err != nil {
		return "", err
	}
	if err := workbook.SetCellStyle(sheet, cell, cell, textStyle); err != nil {
		return "", err
	}
	return canonical, nil
}

func excelDateValue(canonical string) (time.Time, bool) {
	partial, err := dates.ParseCanonical(strings.TrimSpace(canonical))
	if err != nil {
		return time.Time{}, false
	}
	if partial.Year <= 0 || partial.Month <= 0 || partial.Day <= 0 {
		return time.Time{}, false
	}
	return time.Date(partial.Year, time.Month(partial.Month), partial.Day, 0, 0, 0, 0, time.UTC), true
}

func compactICalendarDescriptionLines(lines ...string) []string {
	compacted := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			compacted = append(compacted, trimmed)
		}
	}
	return compacted
}

func excelStringPtr(value string) *string {
	return &value
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
	dataJS := "window.DIXIE_DATA = " + string(dataPayload) + ";\n"
	if err := os.WriteFile(filepath.Join(exportRoot, "archive_data.js"), []byte(dataJS), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(exportRoot, "viewer.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(exportRoot, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return err
	}

	return zipDirectory(outputPath, exportRoot)
}

func (e *ExportService) ExportImages(outputPath string, images []models.Image) error {
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return err
	}

	usedNames := map[string]bool{}
	for _, image := range images {
		source, err := os.Open(image.FilePath)
		if err != nil {
			return err
		}

		entryName := image.FileName
		if entryName == "" {
			entryName = filepath.Base(image.FilePath)
		}
		destPath := uniqueCopiedImagePath(outputPath, entryName, usedNames)
		target, err := os.Create(destPath)
		if err != nil {
			source.Close()
			return err
		}
		if _, err := io.Copy(target, source); err != nil {
			target.Close()
			source.Close()
			return err
		}
		if err := target.Close(); err != nil {
			source.Close()
			return err
		}
		source.Close()
	}

	return nil
}

func uniqueCopiedImagePath(rootDir, fileName string, usedNames map[string]bool) string {
	base := strings.TrimSpace(fileName)
	if base == "" {
		base = "image"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		stem = "image"
	}
	candidate := base
	index := 2
	for {
		fullPath := filepath.Join(rootDir, candidate)
		key := strings.ToLower(candidate)
		if !usedNames[key] {
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				usedNames[key] = true
				return fullPath
			}
		}
		candidate = fmt.Sprintf("%s-%d%s", stem, index, ext)
		index++
	}
}

func (e *ExportService) ExportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	return e.exportSoldierPDF(outputPath, soldier, options.Normalize("L", true))
}

func (e *ExportService) ExportSoldierPDFWithoutImages(outputPath string, soldier models.Soldier) error {
	return e.exportSoldierPDF(outputPath, soldier, PDFOptions{Orientation: "L", IncludeImages: false})
}

func (e *ExportService) ExportSoldierJPG(outputPath string, soldier models.Soldier, options PDFOptions) ([]string, error) {
	options = options.Normalize("L", true)
	outputPath = ensureJPGOutputPath(outputPath)

	tempDir, err := os.MkdirTemp(filepath.Dir(outputPath), ".dixiedata-soldier-jpg-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary JPG export directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "record.pdf")
	if err := e.exportSoldierPDF(pdfPath, soldier, options); err != nil {
		return nil, err
	}

	renderedDir := filepath.Join(tempDir, "pages")
	if err := os.MkdirAll(renderedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create temporary JPG page directory: %w", err)
	}

	renderedPaths, err := e.rasterizer.Rasterize(pdfPath, renderedDir)
	if err != nil {
		return nil, err
	}
	if len(renderedPaths) == 0 {
		return nil, errors.New("PDF rasterizer did not produce any JPG pages")
	}

	finalPaths := make([]string, len(renderedPaths))
	for i := range renderedPaths {
		finalPaths[i] = jpgPagePath(outputPath, i+1)
	}

	if err := removeExistingJPGArtifacts(outputPath); err != nil {
		return nil, err
	}
	for i, renderedPath := range renderedPaths {
		if err := os.Rename(renderedPath, finalPaths[i]); err != nil {
			return nil, fmt.Errorf("save JPG page %d: %w", i+1, err)
		}
	}
	return finalPaths, nil
}

func (e *ExportService) exportSoldierPDF(outputPath string, soldier models.Soldier, options PDFOptions) error {
	options = options.Normalize("L", true)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Record Card", "soldier-pdf", buildinfo.SoldierPDFExportVersion, "", options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()

	writePDFTitleBlock(pdf, recordPDFTitle(soldier), fmt.Sprintf("%s - %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)))
	writePDFRecordCard(pdf, soldier, options)
	if shouldAppendSingleRecordBiographyPage(soldier, options) {
		writeSingleRecordBiographyPage(pdf, soldier, options.PrinterFriendly)
	}

	return pdf.OutputFileAndClose(outputPath)
}

func ensureJPGOutputPath(outputPath string) string {
	if strings.EqualFold(filepath.Ext(outputPath), ".jpg") {
		return outputPath
	}
	return outputPath + ".jpg"
}

func jpgPagePath(outputPath string, pageNumber int) string {
	outputPath = ensureJPGOutputPath(outputPath)
	if pageNumber <= 1 {
		return outputPath
	}
	ext := filepath.Ext(outputPath)
	stem := strings.TrimSuffix(outputPath, ext)
	return fmt.Sprintf("%s-page-%03d%s", stem, pageNumber, ext)
}

func removeExistingJPGArtifacts(outputPath string) error {
	primaryPath := ensureJPGOutputPath(outputPath)
	if err := os.Remove(primaryPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove existing JPG export: %w", err)
	}

	ext := filepath.Ext(primaryPath)
	stem := strings.TrimSuffix(primaryPath, ext)
	matches, err := filepath.Glob(stem + "-page-*" + ext)
	if err != nil {
		return fmt.Errorf("list existing JPG page exports: %w", err)
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing JPG page export %s: %w", match, err)
		}
	}
	return nil
}

func (e *ExportService) ExportMonthlyAnniversaryPDF(outputPath string, month int, calendar map[int][]models.Soldier, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Monthly Anniversary Report", "monthly-pdf", buildinfo.MonthlyPDFExportVersion, pdfFooterMetadata("monthly-pdf", buildinfo.MonthlyPDFExportVersion), options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()

	title := fmt.Sprintf("%s Anniversary Report", monthLabel(month))
	writePDFTitleBlock(pdf, title, "Includes soldier names and database numbers for the selected month.")

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

	return pdf.OutputFileAndClose(outputPath)
}

func (e *ExportService) ExportFullDatabasePDF(outputPath string, settings PrintSettings) error {
	settings = settings.Normalize()
	pdf, err := e.brandedPDFDocument(settings.Orientation, "Printable Archive Registry", "database-pdf", buildinfo.DatabasePDFExportVersion, pdfFooterMetadata("database-pdf", buildinfo.DatabasePDFExportVersion), settings.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()
	writePDFTitleBlock(pdf, "Printable Archive Registry", "Full database export with concise record pages, captioned primary images, and bounded biography excerpts that continue onto additional pages when needed.")

	var selectedIDs []int64
	if settings.Scope == PrintScopeSelected {
		selectedIDs = settings.SelectedIDs
	}
	soldiers, err := exportDetailedSoldiers(e.soldier, selectedIDs)
	if err != nil {
		return err
	}
	if settings.Scope == PrintScopeFiltered {
		soldiers = filterPrintableSoldiers(soldiers, settings)
	}
	if len(soldiers) == 0 {
		writePDFBody(pdf, "No records are currently stored in this archive.")
		return pdf.OutputFileAndClose(outputPath)
	}

	sortPrintableSoldiers(soldiers, settings)
	groupOrder := selectedPrintGroups(settings)
	lastGroupValues := map[string]string{}
	firstRecord := true

	for _, soldier := range soldiers {
		for _, groupChange := range changedPrintGroups(lastGroupValues, soldier, groupOrder, firstRecord) {
			pdf.AddPage()
			writePDFGroupDividerPage(pdf, groupChange.Label, groupChange.Title, groupChange.Level)
		}
		firstRecord = false
		pdf.AddPage()
		writePDFTitleBlock(
			pdf,
			recordPDFTitle(soldier),
			fmt.Sprintf("%s | %s | Captioned primary image + concise biography excerpt", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)),
		)
		writePDFRecordCard(pdf, soldier, PDFOptions{Orientation: settings.Orientation, PrinterFriendly: settings.PrinterFriendly, IncludeImages: true, PrintableArchive: true})
		if settings.FullBiographyPage {
			writePrintableBiographyAppendixPage(pdf, soldier, settings.PrinterFriendly)
		}
	}

	return pdf.OutputFileAndClose(outputPath)
}

func (e *ExportService) ExportAnalyticsSummaryPDF(outputPath string, snapshot AnalyticsSnapshot, options PDFOptions) error {
	options = options.Normalize("P", false)
	pdf, err := e.brandedPDFDocument(options.Orientation, "Archive Summary Report", "analytics-pdf", buildinfo.AnalyticsPDFExportVersion, pdfFooterMetadata("analytics-pdf", buildinfo.AnalyticsPDFExportVersion), options.PrinterFriendly)
	if err != nil {
		return err
	}
	pdf.AddPage()
	writePDFTitleBlock(pdf, "Archive Summary Report", "High-level archive analytics covering burial density, Confederate Home participation, record types, pension geography, unit representation, and decade trends.")

	writePDFSection(pdf, "Record Types")
	writePDFBullet(pdf, fmt.Sprintf("Soldiers: %d", snapshot.RecordTypes.TotalSoldiers))
	writePDFBullet(pdf, fmt.Sprintf("Spouses (Wives & Widows): %d", snapshot.RecordTypes.TotalWivesWidows))
	writePDFBullet(pdf, fmt.Sprintf("Linked People: %d", snapshot.RecordTypes.TotalLinkedPeople))

	writePDFSection(pdf, "Top Cemeteries")
	writePDFAnalyticsRows(pdf, snapshot.CemeteryDensity, "No burial locations are recorded yet.")

	writePDFSection(pdf, "Confederate Home Participation")
	writePDFBody(pdf, "Status breakdown")
	writePDFAnalyticsRows(pdf, snapshot.ConfederateHomeStatus, "No Confederate Home statuses are recorded yet.")
	pdf.Ln(2)
	writePDFBody(pdf, "Most frequent home names")
	writePDFAnalyticsRows(pdf, snapshot.ConfederateHomeNames, "No Confederate Home names are recorded yet.")

	writePDFSection(pdf, "Pension Distribution")
	writePDFAnalyticsRows(pdf, snapshot.PensionDistribution, "No pension states are recorded yet.")

	writePDFSection(pdf, "Unit Representation")
	writePDFAnalyticsRows(pdf, snapshot.UnitRepresentation, "No units are recorded yet.")

	writePDFSection(pdf, "Chronological Overview")
	writePDFBody(pdf, "Birth decades")
	writePDFAnalyticsRows(pdf, snapshot.BirthDecadeDistribution, "No birth decades are recorded yet.")
	pdf.Ln(2)
	writePDFBody(pdf, "Death decades")
	writePDFAnalyticsRows(pdf, snapshot.DeathDecadeDistribution, "No death decades are recorded yet.")

	return pdf.OutputFileAndClose(outputPath)
}

func newPDFDocument(orientation, title, format string, version int) *fpdf.Fpdf {
	pdf := fpdf.New(orientation, "mm", "Letter", "")
	pdf.SetTitle(title, false)
	pdf.SetAuthor(buildinfo.AppLabel(), false)
	pdf.SetCreator(fmt.Sprintf("%s %s export v%d", buildinfo.AppName, format, version), false)
	pdf.SetSubject(fmt.Sprintf("%s schema v%d", buildinfo.AppName, buildinfo.SchemaVersion), false)
	pdf.SetMargins(16, 28, 16)
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetCompression(false)
	return pdf
}

func (e *ExportService) brandedPDFDocument(orientation, title, format string, version int, footerDetail string, printerFriendly bool) (*fpdf.Fpdf, error) {
	branding, err := e.pdfBranding()
	if err != nil {
		return nil, err
	}
	pdf := newPDFDocument(orientation, title, format, version)
	pdf.SetHeaderFuncMode(func() {
		pageWidth, _ := pdf.GetPageSize()
		leftMargin, _, rightMargin, _ := pdf.GetMargins()
		pdf.SetY(10)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(34, 48, 61)
		pdf.CellFormat(0, 5, sanitizePDFText(branding.ArchiveTitle), "", 1, "L", false, 0, "")
		pdf.SetDrawColor(141, 116, 64)
		pdf.Line(leftMargin, 17, pageWidth-rightMargin, 17)
		pdf.Ln(3)
	}, true)
	if !printerFriendly {
		pdf.SetFooterFunc(func() {
			pageWidth, _ := pdf.GetPageSize()
			leftMargin, _, rightMargin, _ := pdf.GetMargins()
			pdf.SetY(-11)
			pdf.SetDrawColor(141, 116, 64)
			pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())
			pdf.Ln(1)
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(68, 82, 96)
			footerText := sanitizePDFText(branding.FooterText)
			if strings.TrimSpace(footerDetail) != "" {
				footerText = footerText + " | " + sanitizePDFText(footerDetail)
			}
			pdf.CellFormat(0, 4, footerText, "", 0, "C", false, 0, "")
		})
	}
	return pdf, nil
}

func (e *ExportService) pdfBranding() (pdfBranding, error) {
	identity, err := e.db.UserIdentity()
	if err != nil {
		return pdfBranding{}, err
	}
	owner := strings.TrimSpace(identity.BrandingName())
	if owner == "" {
		return pdfBranding{}, fmt.Errorf("user identity is incomplete")
	}
	return pdfBranding{
		ArchiveTitle: owner + "'s Civil War Research Archive",
		FooterText:   "Made with DixieData | Version: " + buildinfo.AppVersion + " | Build: " + buildinfo.BuildIdentity(),
	}, nil
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

func exportDetailedSoldiers(soldierSvc *SoldierService, selectedIDs []int64) ([]models.Soldier, error) {
	batch, err := exportSoldiers(soldierSvc)
	if err != nil {
		return nil, err
	}
	if len(selectedIDs) > 0 {
		selectedSet := make(map[int64]struct{}, len(selectedIDs))
		for _, id := range selectedIDs {
			selectedSet[id] = struct{}{}
		}
		filtered := make([]models.Soldier, 0, len(selectedIDs))
		for _, item := range batch {
			if _, ok := selectedSet[item.ID]; ok {
				filtered = append(filtered, item)
			}
		}
		batch = filtered
	}
	all := make([]models.Soldier, 0, len(batch))
	for _, item := range batch {
		soldier, err := soldierSvc.GetByID(item.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, *soldier)
	}
	return all, nil
}

type printGroupChange struct {
	Key   string
	Label string
	Value string
	Title string
	Level int
}

func printablePDFMetadataDetails(settings PrintSettings) map[string]string {
	settings = settings.Normalize()
	metadata := map[string]string{
		"Includes Images":     "true",
		"Full Biography Page": fmt.Sprintf("%t", settings.FullBiographyPage),
		"Sort By":             printableSortLabel(settings.SortBy),
		"Group By":            printableGroupSummary(settings),
	}
	switch settings.Scope {
	case PrintScopeSelected:
		metadata["Export Scope"] = fmt.Sprintf("Selected records (%d)", len(settings.SelectedIDs))
	case PrintScopeFiltered:
		metadata["Export Scope"] = printableFilterScopeSummary(settings)
	default:
		metadata["Export Scope"] = "All records"
	}
	metadata["Printer Friendly"] = fmt.Sprintf("%t", settings.PrinterFriendly)
	metadata["Orientation"] = pdfOrientationLabel(settings.Orientation)
	return metadata
}

func printableFilterScopeSummary(settings PrintSettings) string {
	settings = settings.Normalize()
	if !settings.HasFilters() {
		return "All records"
	}
	return fmt.Sprintf("Filtered records (%d active filter family)", activePrintableFilterFamilyCount(settings))
}

func activePrintableFilterFamilyCount(settings PrintSettings) int {
	settings = settings.Normalize()
	count := 0
	for _, values := range [][]string{
		settings.FilterBuriedIn,
		settings.FilterEntryTypes,
		settings.FilterUnits,
		settings.FilterPensionStates,
		settings.FilterConfederateHomeStatus,
	} {
		if len(values) > 0 {
			count++
		}
	}
	return count
}

func printableSortLabel(sortBy string) string {
	switch strings.TrimSpace(sortBy) {
	case PrintSortBirthYear:
		return "Chronological by Birth Year"
	case PrintSortDeathYear:
		return "Chronological by Death Year"
	default:
		return "Alphabetical by Last Name"
	}
}

func printableGroupSummary(settings PrintSettings) string {
	fields := selectedPrintGroups(settings.Normalize())
	if len(fields) == 0 {
		return "None"
	}
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, printGroupLabel(field))
	}
	return strings.Join(labels, ", ")
}

func selectedPrintGroups(settings PrintSettings) []string {
	fields := []string{}
	if settings.GroupByUnit {
		fields = append(fields, "unit")
	}
	if settings.GroupByPensionState {
		fields = append(fields, "pension_state")
	}
	if settings.GroupByConfederateHomeStatus {
		fields = append(fields, "confederate_home_status")
	}
	if settings.GroupByBuriedIn {
		fields = append(fields, "buried_in")
	}
	return fields
}

func normalizeSelectedPrintIDs(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	normalized := make([]int64, 0, len(values))
	for _, value := range values {
		if value < 1 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized
}

func filterPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) []models.Soldier {
	settings = settings.Normalize()
	if settings.Scope != PrintScopeFiltered || !settings.HasFilters() {
		return soldiers
	}
	filtered := make([]models.Soldier, 0, len(soldiers))
	for _, soldier := range soldiers {
		if matchesPrintableFilters(soldier, settings) {
			filtered = append(filtered, soldier)
		}
	}
	return filtered
}

func matchesPrintableFilters(soldier models.Soldier, settings PrintSettings) bool {
	settings = settings.Normalize()
	return matchesPrintableFilterFamily(settings.FilterBuriedIn, printableBuriedInFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterEntryTypes, printableEntryTypeFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterUnits, printableUnitFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterPensionStates, printablePensionStateFilterValue(soldier)) &&
		matchesPrintableFilterFamily(settings.FilterConfederateHomeStatus, printableConfederateHomeStatusFilterValue(soldier))
}

func matchesPrintableFilterFamily(selected []string, actual string) bool {
	if len(selected) == 0 {
		return true
	}
	for _, candidate := range selected {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(actual)) {
			return true
		}
	}
	return false
}

func printableBuriedInFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(soldier.BuriedIn)
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printableEntryTypeFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(strings.ToLower(soldier.EntryType))
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printableUnitFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(soldier.Unit)
	if value == "" {
		return printFilterUnknownValue
	}
	return value
}

func printablePensionStateFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(pensionstate.Normalize(soldier.PensionState))
	if omitPDFValue(value) {
		return printFilterUnknownValue
	}
	return value
}

func printableConfederateHomeStatusFilterValue(soldier models.Soldier) string {
	value := strings.TrimSpace(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	if omitPDFValue(value) {
		return printFilterUnknownValue
	}
	return value
}

func changedPrintGroups(previous map[string]string, soldier models.Soldier, groupOrder []string, firstRecord bool) []printGroupChange {
	changes := []printGroupChange{}
	startLevel := len(groupOrder)
	if firstRecord {
		startLevel = 0
	} else {
		for index, field := range groupOrder {
			value := printGroupValue(soldier, field)
			if previous[field] != value {
				startLevel = index
				break
			}
		}
	}
	if startLevel >= len(groupOrder) {
		return changes
	}
	for index := startLevel; index < len(groupOrder); index++ {
		field := groupOrder[index]
		value := printGroupValue(soldier, field)
		previous[field] = value
		changes = append(changes, printGroupChange{
			Key:   field,
			Label: printGroupLabel(field),
			Value: value,
			Title: printGroupTitle(field, value),
			Level: index,
		})
	}
	return changes
}

func sortPrintableSoldiers(soldiers []models.Soldier, settings PrintSettings) {
	settings = settings.Normalize()
	groupOrder := selectedPrintGroups(settings)
	sort.Slice(soldiers, func(i, j int) bool {
		left := soldiers[i]
		right := soldiers[j]

		for _, field := range groupOrder {
			leftValue := printGroupSortKey(left, field)
			rightValue := printGroupSortKey(right, field)
			if leftValue != rightValue {
				return leftValue < rightValue
			}
		}

		switch settings.SortBy {
		case PrintSortBirthYear:
			leftYear, leftHasYear := printBirthYear(left)
			rightYear, rightHasYear := printBirthYear(right)
			if result, decided := compareOptionalYears(leftYear, leftHasYear, rightYear, rightHasYear); decided {
				return result
			}
			leftDate := strings.TrimSpace(left.BirthDate)
			rightDate := strings.TrimSpace(right.BirthDate)
			if leftDate != rightDate {
				return leftDate < rightDate
			}
		case PrintSortDeathYear:
			leftYear, leftHasYear := printDeathYear(left)
			rightYear, rightHasYear := printDeathYear(right)
			if result, decided := compareOptionalYears(leftYear, leftHasYear, rightYear, rightHasYear); decided {
				return result
			}
			leftDate := strings.TrimSpace(left.DeathDate)
			rightDate := strings.TrimSpace(right.DeathDate)
			if leftDate != rightDate {
				return leftDate < rightDate
			}
		default:
			leftLast := strings.ToLower(strings.TrimSpace(left.LastName))
			rightLast := strings.ToLower(strings.TrimSpace(right.LastName))
			if leftLast != rightLast {
				return leftLast < rightLast
			}
		}

		leftName := strings.ToLower(strings.TrimSpace(soldierFullName(left)))
		rightName := strings.ToLower(strings.TrimSpace(soldierFullName(right)))
		if leftName != rightName {
			return leftName < rightName
		}
		return strings.ToLower(strings.TrimSpace(left.DisplayID)) < strings.ToLower(strings.TrimSpace(right.DisplayID))
	})
}

func compareOptionalYears(left int, leftOK bool, right int, rightOK bool) (bool, bool) {
	switch {
	case leftOK && rightOK && left != right:
		return left < right, true
	case leftOK != rightOK:
		return leftOK, true
	default:
		return false, false
	}
}

func printBirthYear(soldier models.Soldier) (int, bool) {
	if year := printYearFromCanonical(strings.TrimSpace(soldier.BirthDate)); year > 0 {
		return year, true
	}
	if year := firstFourDigitYear(strings.TrimSpace(soldier.BirthInfo)); year > 0 {
		return year, true
	}
	return 0, false
}

func printDeathYear(soldier models.Soldier) (int, bool) {
	if soldier.DeathYear > 0 {
		return soldier.DeathYear, true
	}
	if year := printYearFromCanonical(strings.TrimSpace(soldier.DeathDate)); year > 0 {
		return year, true
	}
	return 0, false
}

func printYearFromCanonical(value string) int {
	if len(value) < 4 {
		return 0
	}
	year := strings.TrimSpace(value[len(value)-4:])
	if len(year) != 4 {
		return 0
	}
	if year == "0000" {
		return 0
	}
	parsed, err := strconv.Atoi(year)
	if err != nil {
		return 0
	}
	return parsed
}

func firstFourDigitYear(value string) int {
	match := regexp.MustCompile(`\b(1[0-9]{3}|20[0-9]{2})\b`).FindString(value)
	if match == "" {
		return 0
	}
	parsed, err := strconv.Atoi(match)
	if err != nil {
		return 0
	}
	return parsed
}

func printGroupLabel(field string) string {
	switch field {
	case "unit":
		return "Unit"
	case "pension_state":
		return "Pension State"
	case "confederate_home_status":
		return "Confederate Home Status"
	case "buried_in":
		return "Burial Location"
	default:
		return "Group"
	}
}

func printGroupSortKey(soldier models.Soldier, field string) string {
	if field == "buried_in" && strings.TrimSpace(soldier.BuriedIn) == "" {
		return "\uffff"
	}
	return strings.ToLower(printGroupValue(soldier, field))
}

func printGroupValue(soldier models.Soldier, field string) string {
	switch field {
	case "unit":
		return emptyPDFValue(strings.TrimSpace(soldier.Unit))
	case "pension_state":
		return emptyPDFValue(strings.TrimSpace(soldier.PensionState))
	case "confederate_home_status":
		return emptyPDFValue(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	case "buried_in":
		value := strings.TrimSpace(soldier.BuriedIn)
		if value == "" {
			return "Location Unknown"
		}
		return value
	default:
		return "N/A"
	}
}

func printGroupTitle(field, value string) string {
	if field == "buried_in" {
		return "Cemetery: " + value
	}
	return value
}

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
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(0, 8, title, "", 1, "", false, 0, "")
	pdf.SetTextColor(34, 48, 61)
	pdf.SetFont("Helvetica", "", 11)
}

func writePDFTitleBlock(pdf *fpdf.Fpdf, title, subtitle string) {
	pdf.SetFont("Times", "B", 20)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(0, 10, emptyPDFValue(title), "", 1, "", false, 0, "")
	if strings.TrimSpace(subtitle) != "" {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(68, 82, 96)
		pdf.MultiCell(0, 6, sanitizePDFText(strings.TrimSpace(subtitle)), "", "L", false)
	}
	pdf.Ln(1)
}

type pdfRecordCardLayout struct {
	LeftWidthRatio       float64
	SectionTitleFontSize float64
	SectionTitleLine     float64
	FieldLabelFontSize   float64
	FieldValueFontSize   float64
	FieldLineHeight      float64
	FieldRowGap          float64
	ColumnGap            float64
	SectionGap           float64
	ImagePanelHeight     float64
	ImageLabelFontSize   float64
	ImageLabelLine       float64
	BodyFontSize         float64
	BodyLineHeight       float64
	BulletIndent         float64
}

const minReadablePDFRecordCardScale = 0.76

func defaultPDFRecordCardLayout() pdfRecordCardLayout {
	return pdfRecordCardLayout{
		LeftWidthRatio:       0.52,
		SectionTitleFontSize: 9,
		SectionTitleLine:     6,
		FieldLabelFontSize:   8,
		FieldValueFontSize:   9,
		FieldLineHeight:      4.5,
		FieldRowGap:          1,
		ColumnGap:            8,
		SectionGap:           4,
		ImagePanelHeight:     64,
		ImageLabelFontSize:   8,
		ImageLabelLine:       4,
		BodyFontSize:         9,
		BodyLineHeight:       5,
		BulletIndent:         6,
	}
}

func (layout pdfRecordCardLayout) scaled(scale float64) pdfRecordCardLayout {
	return pdfRecordCardLayout{
		LeftWidthRatio:       layout.LeftWidthRatio,
		SectionTitleFontSize: layout.SectionTitleFontSize * scale,
		SectionTitleLine:     maxFloat(4.2, layout.SectionTitleLine*scale),
		FieldLabelFontSize:   layout.FieldLabelFontSize * scale,
		FieldValueFontSize:   layout.FieldValueFontSize * scale,
		FieldLineHeight:      maxFloat(3.4, layout.FieldLineHeight*scale),
		FieldRowGap:          maxFloat(0.4, layout.FieldRowGap*scale),
		ColumnGap:            maxFloat(5, layout.ColumnGap*scale),
		SectionGap:           maxFloat(2, layout.SectionGap*scale),
		ImagePanelHeight:     maxFloat(38, layout.ImagePanelHeight*scale),
		ImageLabelFontSize:   layout.ImageLabelFontSize * scale,
		ImageLabelLine:       maxFloat(3.2, layout.ImageLabelLine*scale),
		BodyFontSize:         layout.BodyFontSize * scale,
		BodyLineHeight:       maxFloat(3.8, layout.BodyLineHeight*scale),
		BulletIndent:         maxFloat(4.5, layout.BulletIndent*scale),
	}
}

func choosePDFRecordCardLayout(pdf *fpdf.Fpdf, soldier models.Soldier, startY float64, options PDFOptions) (pdfRecordCardLayout, bool) {
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	availableHeight := pageHeight - bottomMargin - startY - 18
	base := defaultPDFRecordCardLayout()
	if usesPortraitCompactRecordCardLayout(soldier, options) {
		base.LeftWidthRatio = 0.6
	} else if !options.IncludeImages {
		base.LeftWidthRatio = 0.43
	}
	if usesPortraitRecordPDFLayout(options) && !usesPortraitCompactRecordCardLayout(soldier, options) {
		return base.scaled(minReadablePDFRecordCardScale), false
	}
	for _, scale := range []float64{1, 0.94, 0.88, 0.82, minReadablePDFRecordCardScale} {
		layout := base.scaled(scale)
		if estimatePDFRecordCardHeight(pdf, soldier, options, layout) <= availableHeight {
			return layout, true
		}
	}
	return base.scaled(minReadablePDFRecordCardScale), false
}

func estimatePDFRecordCardHeight(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions, layout pdfRecordCardLayout) float64 {
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	leftWidth := contentWidth * layout.LeftWidthRatio
	rightWidth := contentWidth - leftWidth - layout.ColumnGap
	usesPortraitCompact := usesPortraitCompactRecordCardLayout(soldier, options)

	leftHeight := estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordIdentityFields(soldier), layout)
	leftHeight += layout.SectionGap
	leftHeight += estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordServiceFields(soldier, false), layout)
	if usesPortraitCompact {
		householdHeight := estimatePDFCompactFieldSectionHeight(pdf, leftWidth, recordHouseholdFields(soldier, false), layout)
		if householdHeight > 0 {
			leftHeight += layout.SectionGap
			leftHeight += householdHeight
		}
		if len(soldier.Records) > 0 {
			leftHeight += layout.SectionGap
			leftHeight += estimatePDFRecordsSectionHeight(pdf, leftWidth, soldier.Records, layout)
		}
	}

	rightHeight := 0.0
	if options.IncludeImages {
		if imagePath, _ := firstRecordCardImage(soldier, false); imagePath != "" {
			rightHeight += estimatePDFImagePanelHeight(layout, recordPDFImageSectionTitle(options) != "")
		}
	}
	if !usesPortraitCompact {
		householdHeight := estimatePDFCompactFieldSectionHeight(pdf, rightWidth, recordHouseholdFields(soldier, false), layout)
		if householdHeight > 0 {
			if rightHeight > 0 {
				rightHeight += layout.SectionGap
			}
			rightHeight += householdHeight
		}
	}
	_, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		if rightHeight > 0 {
			rightHeight += layout.SectionGap
		}
		rightHeight += estimatePDFRichTextSectionHeight(pdf, rightWidth, narrativeText, layout)
	}
	if !usesPortraitCompact && len(soldier.Records) > 0 {
		if rightHeight > 0 {
			rightHeight += layout.SectionGap
		}
		rightHeight += estimatePDFRecordsSectionHeight(pdf, rightWidth, soldier.Records, layout)
	}

	return maxFloat(leftHeight, rightHeight)
}

func estimatePDFCompactFieldSectionHeight(pdf *fpdf.Fpdf, width float64, fields []pdfField, layout pdfRecordCardLayout) float64 {
	if !hasVisiblePDFField(fields) {
		return 0
	}
	labelWidth := width * 0.32
	if labelWidth < 28 {
		labelWidth = 28
	}
	if labelWidth > 44 {
		labelWidth = 44
	}
	valueWidth := width - labelWidth - 3
	height := layout.SectionTitleLine
	for _, field := range fields {
		if !field.visible() {
			continue
		}
		labelLines := wrappedPDFLineCount(pdf, sanitizePDFText(field.Label), labelWidth, "Helvetica", "B", layout.FieldLabelFontSize)
		valueLines := wrappedPDFLineCount(pdf, field.renderedValue(), valueWidth, "Helvetica", "", layout.FieldValueFontSize)
		height += float64(maxInt(labelLines, valueLines))*layout.FieldLineHeight + layout.FieldRowGap
	}
	return height
}

func estimatePDFRichTextSectionHeight(pdf *fpdf.Fpdf, width float64, text string, layout pdfRecordCardLayout) float64 {
	return layout.SectionTitleLine + float64(wrappedPDFMultilineCount(pdf, emptyPDFValue(text), width, "Helvetica", "", layout.BodyFontSize))*layout.BodyLineHeight*1.18
}

func estimatePDFRecordsSectionHeight(pdf *fpdf.Fpdf, width float64, records []models.Record, layout pdfRecordCardLayout) float64 {
	height := layout.SectionTitleLine
	for _, record := range records {
		line := record.RecordType
		if strings.TrimSpace(record.AppID) != "" {
			line += fmt.Sprintf(" (App: %s)", record.AppID)
		}
		height += float64(wrappedPDFLineCount(pdf, emptyPDFValue(line), width-layout.BulletIndent, "Helvetica", "", layout.BodyFontSize)) * layout.BodyLineHeight
		if strings.TrimSpace(record.Details) != "" {
			height += float64(wrappedPDFMultilineCount(pdf, emptyPDFValue(record.Details), width, "Helvetica", "", layout.BodyFontSize)) * layout.BodyLineHeight * 1.18
		}
	}
	return height
}

func wrappedPDFLineCount(pdf *fpdf.Fpdf, text string, width float64, family, style string, size float64) int {
	pdf.SetFont(family, style, size)
	lines := pdf.SplitText(sanitizePDFText(strings.TrimSpace(text)), width)
	if len(lines) == 0 {
		return 1
	}
	return len(lines)
}

func wrappedPDFMultilineCount(pdf *fpdf.Fpdf, text string, width float64, family, style string, size float64) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		count += wrappedPDFLineCount(pdf, line, width, family, style, size)
	}
	if count == 0 {
		return 1
	}
	return count
}

func writePDFGroupDividerPage(pdf *fpdf.Fpdf, label, value string, level int) {
	pdf.SetY(maxFloat(pdf.GetY(), 46))
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(0, 7, sanitizePDFText("Grouped by "+label), "", 1, "L", false, 0, "")
	pdf.Ln(float64(level) * 2)
	pdf.SetFont("Times", "B", maxFloat(20, 28-float64(level)*2))
	pdf.SetTextColor(34, 48, 61)
	pdf.MultiCell(0, 11, sanitizePDFText(emptyPDFValue(value)), "", "L", false)
	pdf.Ln(2)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.MultiCell(0, 6, sanitizePDFText("The following record pages belong to this section."), "", "L", false)
}

func writePDFRecordCard(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions) {
	options = options.Normalize("L", true)
	startY := pdf.GetY()
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	_, _, _, bottomMargin := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	layout, fitsSinglePage := choosePDFRecordCardLayout(pdf, soldier, startY, options)
	if !fitsSinglePage {
		writePDFRecordCardMultiPage(pdf, soldier, options, layout)
		return
	}
	leftWidth := contentWidth * layout.LeftWidthRatio
	gapWidth := layout.ColumnGap
	rightWidth := contentWidth - leftWidth - gapWidth
	rightX := leftMargin + leftWidth + gapWidth
	usesPortraitCompact := usesPortraitCompactRecordCardLayout(soldier, options)
	defer pdf.SetAutoPageBreak(true, bottomMargin)
	pdf.SetAutoPageBreak(false, bottomMargin)

	leftY := writePDFCompactFieldSection(pdf, leftMargin, startY, leftWidth, "Identity & Vital Details", recordIdentityFields(soldier), layout)
	leftY = writePDFCompactFieldSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, "Service & Archive Details", recordServiceFields(soldier, options.PrinterFriendly), layout)
	if usesPortraitCompact {
		householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
		if hasVisiblePDFField(householdFields) {
			leftY = writePDFCompactFieldSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, "Household & Context", householdFields, layout)
		}
		if len(soldier.Records) > 0 {
			leftY = writePDFRecordsColumnSection(pdf, leftMargin, leftY+layout.SectionGap, leftWidth, soldier.Records, layout, options.PrinterFriendly)
		}
	}

	rightY := startY
	if options.IncludeImages {
		if imagePath, imageLabel := firstRecordCardImage(soldier, options.PrinterFriendly); imagePath != "" {
			rightY = writePDFImagePanel(pdf, rightX, rightY, rightWidth, recordPDFImageSectionTitle(options), imagePath, imageLabel, layout)
		}
	}
	if !usesPortraitCompact {
		householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
		if hasVisiblePDFField(householdFields) {
			if rightY > startY {
				rightY += layout.SectionGap
			}
			rightY = writePDFCompactFieldSection(pdf, rightX, rightY, rightWidth, "Household & Context", householdFields, layout)
		}
	}
	narrativeTitle, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		if rightY > startY {
			rightY += layout.SectionGap
		}
		rightY = writePDFRichTextColumnSection(pdf, rightX, rightY, rightWidth, narrativeTitle, narrativeText, layout)
	}
	if !usesPortraitCompact && len(soldier.Records) > 0 {
		if rightY > startY {
			rightY += layout.SectionGap
		}
		rightY = writePDFRecordsColumnSection(pdf, rightX, rightY, rightWidth, soldier.Records, layout, options.PrinterFriendly)
	}

	pdf.SetY(maxPDFY(leftY, rightY) + 2)
}

func writePDFRecordCardMultiPage(pdf *fpdf.Fpdf, soldier models.Soldier, options PDFOptions, layout pdfRecordCardLayout) {
	options = options.Normalize("L", true)
	startY := pdf.GetY()
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, bottomMargin := pdf.GetMargins()
	contentWidth := pageWidth - leftMargin - rightMargin
	defer pdf.SetAutoPageBreak(true, bottomMargin)
	pdf.SetAutoPageBreak(true, bottomMargin)

	currentY := startY
	wroteSection := false

	identityFields := recordIdentityFields(soldier)
	if hasVisiblePDFField(identityFields) {
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Identity & Vital Details", identityFields, layout)
		wroteSection = true
	}

	serviceFields := recordServiceFields(soldier, options.PrinterFriendly)
	if hasVisiblePDFField(serviceFields) {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.FieldLineHeight)
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Service & Archive Details", serviceFields, layout)
		wroteSection = true
	}

	householdFields := recordHouseholdFields(soldier, options.PrinterFriendly)
	if hasVisiblePDFField(householdFields) {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.FieldLineHeight)
		currentY = writePDFCompactFieldSection(pdf, leftMargin, currentY, contentWidth, "Household & Context", householdFields, layout)
		wroteSection = true
	}

	if options.IncludeImages {
		if imagePath, imageLabel := firstRecordCardImage(soldier, options.PrinterFriendly); imagePath != "" {
			currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, estimatePDFImagePanelHeight(layout, recordPDFImageSectionTitle(options) != ""))
			currentY = writePDFImagePanel(pdf, leftMargin, currentY, contentWidth, recordPDFImageSectionTitle(options), imagePath, imageLabel, layout)
			wroteSection = true
		}
	}

	narrativeTitle, narrativeText := recordPDFNarrativeSection(soldier, options)
	if strings.TrimSpace(narrativeText) != "" {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.BodyLineHeight)
		currentY = writePDFRichTextColumnSection(pdf, leftMargin, currentY, contentWidth, narrativeTitle, narrativeText, layout)
		wroteSection = true
	}

	if len(soldier.Records) > 0 {
		currentY = preparePDFRecordCardSection(pdf, currentY, wroteSection, layout.SectionGap, layout.SectionTitleLine+layout.BodyLineHeight)
		currentY = writePDFRecordsColumnSection(pdf, leftMargin, currentY, contentWidth, soldier.Records, layout, options.PrinterFriendly)
		wroteSection = true
	}

	if wroteSection {
		pdf.SetY(currentY + 2)
	}
}

func preparePDFRecordCardSection(pdf *fpdf.Fpdf, currentY float64, wroteSection bool, gap, minHeight float64) float64 {
	if wroteSection {
		currentY += gap
	}
	_, pageHeight := pdf.GetPageSize()
	_, _, _, bottomMargin := pdf.GetMargins()
	if currentY+minHeight > pageHeight-bottomMargin {
		pdf.AddPage()
		return pdf.GetY()
	}
	return currentY
}

func writePDFCompactFieldSection(pdf *fpdf.Fpdf, x, y, width float64, title string, fields []pdfField, layout pdfRecordCardLayout) float64 {
	if !hasVisiblePDFField(fields) {
		return y
	}
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
	currentY := pdf.GetY()
	labelWidth := width * 0.32
	if labelWidth < 28 {
		labelWidth = 28
	}
	if labelWidth > 44 {
		labelWidth = 44
	}
	valueWidth := width - labelWidth - 3
	for _, field := range fields {
		if !field.visible() {
			continue
		}
		rowTop := currentY
		pdf.SetXY(x, rowTop)
		pdf.SetFont("Helvetica", "B", layout.FieldLabelFontSize)
		pdf.SetTextColor(68, 82, 96)
		pdf.MultiCell(labelWidth, layout.FieldLineHeight, sanitizePDFText(field.Label), "", "L", false)
		labelBottom := pdf.GetY()
		pdf.SetXY(x+labelWidth+3, rowTop)
		if strings.TrimSpace(field.Label) == "Maiden Name" {
			pdf.SetFont("Helvetica", "I", layout.FieldValueFontSize)
		} else {
			pdf.SetFont("Helvetica", "", layout.FieldValueFontSize)
		}
		pdf.SetTextColor(34, 48, 61)
		pdf.MultiCell(valueWidth, layout.FieldLineHeight, field.renderedValue(), "", "L", false)
		valueBottom := pdf.GetY()
		currentY = maxPDFY(labelBottom, valueBottom) + layout.FieldRowGap
	}
	return currentY
}

func writePDFImagePanel(pdf *fpdf.Fpdf, x, y, width float64, title, imagePath, label string, layout pdfRecordCardLayout) float64 {
	panelY := y
	if strings.TrimSpace(title) != "" {
		pdf.SetXY(x, y)
		pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
		pdf.SetTextColor(141, 116, 64)
		pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
		panelY = pdf.GetY()
	}
	panelHeight := layout.ImagePanelHeight
	imageX, imageY, imageWidth, imageHeight, ok := fitPDFImageToBounds(imagePath, x+2, panelY+2, width-4, panelHeight-14)
	if ok {
		pdf.ImageOptions(imagePath, imageX, imageY, imageWidth, imageHeight, false, fpdf.ImageOptions{
			ImageType: strings.TrimPrefix(strings.ToLower(filepath.Ext(imagePath)), "."),
		}, 0, "")
	}
	pdf.SetXY(x+2, panelY+panelHeight-10)
	pdf.SetFont("Helvetica", "", layout.ImageLabelFontSize)
	pdf.SetTextColor(68, 82, 96)
	if strings.TrimSpace(label) != "" {
		pdf.MultiCell(width-4, layout.ImageLabelLine, emptyPDFValue(label), "", "L", false)
	}
	return panelY + panelHeight + 2
}

func estimatePDFImagePanelHeight(layout pdfRecordCardLayout, includeTitle bool) float64 {
	height := layout.ImagePanelHeight + 2
	if includeTitle {
		height += layout.SectionTitleLine
	}
	return height
}

func writePDFRichTextColumnSection(pdf *fpdf.Fpdf, x, y, width float64, title, text string, layout pdfRecordCardLayout) float64 {
	if strings.TrimSpace(text) == "" {
		return y
	}
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, topMargin, rightMargin, _ := pdf.GetMargins()
	defer pdf.SetMargins(leftMargin, topMargin, rightMargin)
	pdf.SetMargins(x, topMargin, pageWidth-(x+width))
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, title, "", 1, "L", false, 0, "")
	pdf.SetX(x)
	pdf.SetFont("Helvetica", "", layout.BodyFontSize)
	pdf.SetTextColor(34, 48, 61)
	writePDFRichTextSized(pdf, emptyPDFValue(text), layout.BodyLineHeight, layout.BodyFontSize)
	return pdf.GetY()
}

func writePDFRecordsColumnSection(pdf *fpdf.Fpdf, x, y, width float64, records []models.Record, layout pdfRecordCardLayout, printerFriendly bool) float64 {
	if len(records) == 0 {
		return y
	}
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, topMargin, rightMargin, _ := pdf.GetMargins()
	defer pdf.SetMargins(leftMargin, topMargin, rightMargin)
	pdf.SetMargins(x, topMargin, pageWidth-(x+width))
	pdf.SetXY(x, y)
	pdf.SetFont("Helvetica", "B", layout.SectionTitleFontSize)
	pdf.SetTextColor(141, 116, 64)
	pdf.CellFormat(width, layout.SectionTitleLine, "Records", "", 1, "L", false, 0, "")
	for _, record := range records {
		line := record.RecordType
		if strings.TrimSpace(record.AppID) != "" {
			line += fmt.Sprintf(" (App: %s)", record.AppID)
		}
		pdf.SetX(x)
		writePDFBulletSized(pdf, line, layout)
		details := pdfFreeTextValue(record.Details, printerFriendly)
		if strings.TrimSpace(details) != "" {
			pdf.SetX(x)
			writePDFRichTextSized(pdf, emptyPDFValue(details), layout.BodyLineHeight, layout.BodyFontSize)
		}
	}
	return pdf.GetY()
}

func recordIdentityFields(soldier models.Soldier) []pdfField {
	fields := []pdfField{
		blankPDFField("Prefix", soldier.Prefix),
		blankPDFField("First Name", soldier.FirstName),
		blankPDFField("Middle Name", soldier.MiddleName),
		blankPDFField("Last Name", soldier.LastName),
		defaultPDFField("Suffix", soldier.Suffix),
		unknownDatePDFField("Birth Date", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death Date", soldierDeathLine(soldier)),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Buried In", soldier.BuriedIn),
	}
	return fields
}

func recordServiceFields(soldier models.Soldier, printerFriendly bool) []pdfField {
	pensionStateField := naPDFField("Pension State", pensionstate.Normalize(soldier.PensionState))
	homeStatusField := naPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
	homeNameField := confederateHomeNamePDFField(soldier)
	if printerFriendly {
		pensionStateField = defaultPDFField("Pension State", pensionstate.Normalize(soldier.PensionState))
		homeStatusField = defaultPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus))
		homeNameField = defaultPDFField("Confederate Home Name", pdfConfederateHomeName(soldier))
	}
	fields := []pdfField{
		defaultPDFField("Record Type", displayEntryType(soldier)),
		blankPDFField("Rank In", soldier.RankIn),
		blankPDFField("Rank Out", displaySoldierRank(soldier)),
		blankPDFField("Unit", soldier.Unit),
		pensionStateField,
		defaultPDFField("Pension ID", soldier.PensionID),
		defaultPDFField("Application ID", soldier.ApplicationID),
		homeStatusField,
		homeNameField,
	}
	return fields
}

func recordHouseholdFields(soldier models.Soldier, printerFriendly bool) []pdfField {
	fields := []pdfField{
		defaultPDFField("Spouse", soldier.SpouseName),
		defaultPDFField("Linked Spouse Record", func() string {
			if printerFriendly {
				return ""
			}
			if soldier.SpouseSoldierID > 0 {
				if strings.TrimSpace(soldier.SpouseName) != "" {
					return fmt.Sprintf("%s (DB ID %d)", strings.TrimSpace(soldier.SpouseName), soldier.SpouseSoldierID)
				}
				return fmt.Sprintf("DB ID %d", soldier.SpouseSoldierID)
			}
			return ""
		}()),
		defaultPDFField("Maiden Name", soldier.MaidenName),
	}
	return fields
}

func firstRecordCardImage(soldier models.Soldier, printerFriendly bool) (string, string) {
	for _, image := range soldier.Images {
		if !image.IsPrimary {
			continue
		}
		if imagePath := imagePathForPDF(image); imagePath != "" {
			return imagePath, pdfImageCaption(image)
		}
	}
	for _, image := range soldier.Images {
		if imagePath := imagePathForPDF(image); imagePath != "" {
			return imagePath, pdfImageCaption(image)
		}
	}
	return "", ""
}

func recordPDFImageSectionTitle(options PDFOptions) string {
	return ""
}

func recordPDFNarrativeSection(soldier models.Soldier, options PDFOptions) (string, string) {
	if options.PrintableArchive {
		return "Biography", printableArchiveBiographyText(soldier, options.PrinterFriendly)
	}
	if shouldAppendSingleRecordBiographyPage(soldier, options) {
		return "", ""
	}
	text := strings.TrimSpace(soldier.PDFExcerptOverride)
	if text == "" {
		text = soldier.Biography
	}
	return "Biography", pdfFreeTextValue(text, options.PrinterFriendly)
}

func usesPortraitCompactRecordCardLayout(soldier models.Soldier, options PDFOptions) bool {
	options = options.Normalize("L", true)
	if options.PrintableArchive || !usesPortraitRecordPDFLayout(options) || !options.IncludeImages {
		return false
	}
	imagePath, _ := firstRecordCardImage(soldier, options.PrinterFriendly)
	return imagePath != ""
}

func usesPortraitRecordPDFLayout(options PDFOptions) bool {
	return strings.TrimSpace(strings.ToUpper(options.Normalize("L", true).Orientation)) != "L"
}

func shouldAppendSingleRecordBiographyPage(soldier models.Soldier, options PDFOptions) bool {
	options = options.Normalize("L", true)
	if options.PrintableArchive || usesPortraitRecordPDFLayout(options) {
		return false
	}
	return strings.TrimSpace(pdfFreeTextValue(soldier.Biography, options.PrinterFriendly)) != ""
}

func printableArchiveBiographyText(soldier models.Soldier, printerFriendly bool) string {
	text := strings.TrimSpace(soldier.PDFExcerptOverride)
	if text == "" {
		text = soldier.Biography
	}
	return truncatePDFText(pdfFreeTextValue(text, printerFriendly), 480)
}

func truncatePDFText(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if maxRunes <= 0 || trimmed == "" {
		return trimmed
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func writePrintableBiographyAppendixPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool) {
	writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography Appendix")
}

func writeSingleRecordBiographyPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool) {
	writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography")
}

func writeFullBiographyPage(pdf *fpdf.Fpdf, soldier models.Soldier, printerFriendly bool, label string) {
	biography := pdfFreeTextValue(soldier.Biography, printerFriendly)
	if strings.TrimSpace(biography) == "" {
		return
	}
	pdf.AddPage()
	writePDFTitleBlock(
		pdf,
		recordPDFTitle(soldier),
		fmt.Sprintf("%s | %s | %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier), label),
	)
	writePDFSection(pdf, "Biography")
	writePDFRichTextSized(pdf, emptyPDFValue(biography), 6, 11)
}

func pdfImageCaption(image models.Image) string {
	caption := strings.TrimSpace(image.Caption)
	if caption == "" {
		return ""
	}
	if strings.EqualFold(caption, strings.TrimSpace(image.FileName)) {
		return ""
	}
	if filePath := strings.TrimSpace(image.FilePath); filePath != "" && strings.EqualFold(caption, filepath.Base(filePath)) {
		return ""
	}
	if looksLikePDFImageFileName(caption) {
		return ""
	}
	return caption
}

func looksLikePDFImageFileName(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	base := filepath.Base(trimmed)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tif", ".tiff":
		return strings.TrimSuffix(base, ext) != ""
	default:
		return false
	}
}

func recordPDFTitle(soldier models.Soldier) string {
	if name := strings.TrimSpace(soldier.GetFullName()); name != "" {
		return name
	}
	return soldierDisplayName(soldier)
}

func maxPDFY(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func hasVisiblePDFField(fields []pdfField) bool {
	for _, field := range fields {
		if field.visible() {
			return true
		}
	}
	return false
}

func omitPDFValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	switch strings.ToUpper(trimmed) {
	case "NONE", "NA", "N/A", "NOT RECORDED":
		return true
	default:
		return false
	}
}

func pdfFreeTextValue(value string, printerFriendly bool) string {
	trimmed := strings.TrimSpace(value)
	if !printerFriendly {
		return trimmed
	}
	cleaned := pdfURLPattern.ReplaceAllString(trimmed, "")
	cleaned = regexp.MustCompile(`\[\[([^\]]+)\]\]`).ReplaceAllString(cleaned, "$1")
	cleaned = strings.TrimSpace(strings.Join(strings.Fields(cleaned), " "))
	return cleaned
}

func pdfOrientationLabel(value string) string {
	if strings.TrimSpace(strings.ToUpper(value)) == "L" {
		return "Landscape"
	}
	return "Portrait"
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func writePDFInlineField(pdf *fpdf.Fpdf, field pdfField, width float64) {
	x := pdf.GetX()
	y := pdf.GetY()
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(34, 6, field.Label, "", 0, "", false, 0, "")
	if strings.TrimSpace(field.Label) == "Maiden Name" {
		pdf.SetFont("Helvetica", "I", 10)
	} else {
		pdf.SetFont("Helvetica", "", 10)
	}
	pdf.SetTextColor(68, 82, 96)
	pdf.SetXY(x+34, y)
	pdf.MultiCell(width-34, 6, field.renderedValue(), "", "L", false)
	if pdf.GetY() < y+6 {
		pdf.SetY(y + 6)
	}
	pdf.SetX(x)
}

func pdfFooterMetadata(format string, version int) string {
	return fmt.Sprintf("Generated %s | %s v%d | App %s | Schema %d", time.Now().Format("2006-01-02 15:04"), format, version, buildinfo.AppVersion, buildinfo.SchemaVersion)
}

func writePDFField(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(34, 8, label+":", "", 0, "", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.MultiCell(0, 8, emptyPDFValue(value), "", "", false)
}

func writePDFBody(pdf *fpdf.Fpdf, text string) {
	writePDFRichText(pdf, emptyPDFValue(text), 7)
}

func writePDFBullet(pdf *fpdf.Fpdf, text string) {
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(6, 7, "-", "", 0, "", false, 0, "")
	pdf.MultiCell(0, 7, emptyPDFValue(text), "", "", false)
}

func writePDFAnalyticsRows(pdf *fpdf.Fpdf, items []AnalyticsCount, emptyMessage string) {
	if len(items) == 0 {
		writePDFBody(pdf, emptyMessage)
		return
	}
	for _, item := range items {
		writePDFBullet(pdf, fmt.Sprintf("%s - %d", item.Label, item.Count))
	}
}

func writePDFBulletSized(pdf *fpdf.Fpdf, text string, layout pdfRecordCardLayout) {
	pdf.SetFont("Helvetica", "", layout.BodyFontSize)
	pdf.CellFormat(layout.BulletIndent, layout.BodyLineHeight, "-", "", 0, "", false, 0, "")
	pdf.MultiCell(0, layout.BodyLineHeight, emptyPDFValue(text), "", "", false)
}

func writePDFRichText(pdf *fpdf.Fpdf, text string, lineHeight float64) {
	writePDFRichTextSized(pdf, text, lineHeight, 10)
}

func analyticsPDFMetadataDetails(snapshot AnalyticsSnapshot) map[string]string {
	return map[string]string{
		"Top cemeteries": fmt.Sprintf("%d", len(snapshot.CemeteryDensity)),
		"Home statuses":  fmt.Sprintf("%d", len(snapshot.ConfederateHomeStatus)),
		"Pension states": fmt.Sprintf("%d", len(snapshot.PensionDistribution)),
		"Top units":      fmt.Sprintf("%d", len(snapshot.UnitRepresentation)),
		"Birth decades":  fmt.Sprintf("%d", len(snapshot.BirthDecadeDistribution)),
		"Death decades":  fmt.Sprintf("%d", len(snapshot.DeathDecadeDistribution)),
		"Record types":   fmt.Sprintf("%d soldiers / %d spouses / %d linked people", snapshot.RecordTypes.TotalSoldiers, snapshot.RecordTypes.TotalWivesWidows, snapshot.RecordTypes.TotalLinkedPeople),
	}
}

func writePDFRichTextSized(pdf *fpdf.Fpdf, text string, lineHeight, fontSize float64) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for lineIndex, line := range lines {
		segments := pdfTextSegments(line)
		if len(segments) == 0 {
			segments = []pdfTextSegment{{Text: ""}}
		}
		for _, segment := range segments {
			if segment.Link != "" {
				pdf.SetFont("Helvetica", "I", fontSize)
				pdf.SetTextColor(48, 87, 122)
				pdf.WriteLinkString(lineHeight, segment.Text, segment.Link)
				pdf.SetTextColor(0, 0, 0)
				pdf.SetFont("Helvetica", "", fontSize)
				continue
			}
			pdf.SetFont("Helvetica", "", fontSize)
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
	const thumbnailHeight = 22.0
	const rowHeight = 36.0

	_, pageHeight := pdf.GetPageSize()
	if pdf.GetY()+rowHeight > pageHeight-16 {
		pdf.AddPage()
	}

	x := pdf.GetX()
	y := pdf.GetY()
	imagePath := imagePathForPDF(image)
	pdf.Rect(x, y, thumbnailWidth, thumbnailHeight, "D")

	if imagePath != "" {
		imageX, imageY, imageWidth, imageHeight, ok := fitPDFImageToBounds(imagePath, x, y, thumbnailWidth, thumbnailHeight)
		if ok {
			pdf.ImageOptions(imagePath, imageX, imageY, imageWidth, imageHeight, false, fpdf.ImageOptions{
				ImageType: strings.TrimPrefix(strings.ToLower(filepath.Ext(imagePath)), "."),
			}, 0, "")
		}
	}

	pdf.SetXY(x+thumbnailWidth+4, y)
	title := pdfImageCaption(image)
	if title == "" {
		title = "Image"
	}
	pdf.SetFont("Helvetica", "", 10)
	pdf.MultiCell(0, 6, emptyPDFValue(title), "", "", false)

	if pdf.GetY() < y+rowHeight {
		pdf.SetY(y + rowHeight)
	}
}

func writePDFRegistryEntry(pdf *fpdf.Fpdf, soldier models.Soldier) {
	if pdf.GetY() > 230 {
		pdf.AddPage()
	}
	pdf.SetDrawColor(141, 116, 64)
	pdf.Line(16, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(3)
	pdf.SetFont("Times", "B", 13)
	pdf.SetTextColor(34, 48, 61)
	pdf.CellFormat(0, 7, soldierDisplayName(soldier), "", 1, "", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(68, 82, 96)
	pdf.CellFormat(0, 5, fmt.Sprintf("%s - %s", emptyPDFValue(strings.TrimSpace(soldier.DisplayID)), displayEntryType(soldier)), "", 1, "", false, 0, "")
	for _, line := range registryEntryLines(soldier) {
		if !line.visible() {
			continue
		}
		writePDFInlineField(pdf, line, 178)
	}
	pdf.Ln(2)
}

func recordCardFields(soldier models.Soldier) []pdfField {
	fields := []pdfField{
		defaultPDFField("Record Type", displayEntryType(soldier)),
		defaultPDFField("Full Name", soldierFullName(soldier)),
		defaultPDFField("Display ID", soldier.DisplayID),
		unknownDatePDFField("Birth", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death", soldierDeathLine(soldier)),
	}
	if strings.TrimSpace(soldier.Prefix) != "" {
		fields = append(fields, blankPDFField("Prefix", soldier.Prefix))
	}
	if strings.TrimSpace(soldier.Suffix) != "" {
		fields = append(fields, defaultPDFField("Suffix", soldier.Suffix))
	}
	if isSoldierEntry(soldier) {
		fields = append(fields,
			blankPDFField("Rank In", soldier.RankIn),
			blankPDFField("Rank Out", displaySoldierRank(soldier)),
			blankPDFField("Unit", soldier.Unit),
			naPDFField("Pension State", pensionstate.Normalize(soldier.PensionState)),
			naPDFField("Confederate Home Status", confederatehomestatus.Normalize(soldier.ConfederateHomeStatus)),
			confederateHomeNamePDFField(soldier),
		)
	} else {
		fields = append(fields,
			defaultPDFField("Married To", soldier.SpouseName),
			defaultPDFField("Maiden Name", soldier.MaidenName),
		)
	}
	fields = append(fields,
		defaultPDFField("Pension ID", soldier.PensionID),
		defaultPDFField("Application ID", soldier.ApplicationID),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Buried In", soldier.BuriedIn),
	)
	return fields
}

func registryEntryLines(soldier models.Soldier) []pdfField {
	lines := []pdfField{
		unknownDatePDFField("Birth Date", dates.DisplayUnknown(soldier.BirthDate)),
		unknownDatePDFField("Death Date", soldierDeathLine(soldier)),
		defaultPDFField("Birth Info", soldier.BirthInfo),
		defaultPDFField("Service Summary", soldierServiceLine(soldier)),
		defaultPDFField("Pension / Application", strings.TrimSpace(strings.Join(compactPDFValues(pensionstate.Normalize(soldier.PensionState), soldier.PensionID, soldier.ApplicationID), " | "))),
		defaultPDFField("Confederate Home", strings.TrimSpace(strings.Join(compactPDFValuesExcludingNA(confederatehomestatus.Normalize(soldier.ConfederateHomeStatus), soldier.ConfederateHomeName), " | "))),
		defaultPDFField("Buried In", soldier.BuriedIn),
	}
	if strings.TrimSpace(soldier.SpouseName) != "" || strings.TrimSpace(soldier.MaidenName) != "" {
		if spouseName := strings.TrimSpace(soldier.SpouseName); spouseName != "" {
			lines = append(lines, defaultPDFField("Spouse", spouseName))
		}
		if maidenName := strings.TrimSpace(soldier.MaidenName); maidenName != "" {
			lines = append(lines, defaultPDFField("Maiden Name", maidenName))
		}
	}
	if strings.TrimSpace(soldier.Notes) != "" {
		lines = append(lines, defaultPDFField("Notes", soldier.Notes))
	}
	if len(soldier.Records) > 0 {
		recordLines := make([]string, 0, len(soldier.Records))
		for _, record := range soldier.Records {
			recordLines = append(recordLines, strings.TrimSpace(strings.Join(compactPDFValues(record.RecordType, record.AppID, record.Details), " | ")))
		}
		lines = append(lines, defaultPDFField("Records", strings.Join(recordLines, "\n")))
	}
	return lines
}

func compactPDFValues(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func compactPDFValuesExcludingNA(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && trimmed != confederatehomestatus.NotApplicable {
			result = append(result, trimmed)
		}
	}
	return result
}

func emptyPDFValue(value string) string {
	sanitized := sanitizePDFText(strings.TrimSpace(value))
	if sanitized == "" {
		return "N/A"
	}
	return sanitized
}

func sanitizePDFText(value string) string {
	replaced := strings.NewReplacer(
		"\u2018", "'",
		"\u2019", "'",
		"\u201c", `"`,
		"\u201d", `"`,
		"\u2013", "-",
		"\u2014", "-",
		"\u2026", "...",
		"\u2022", "-",
		"\u00a0", " ",
	).Replace(value)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return r
		case r < 32 || r > 126:
			return ' '
		default:
			return r
		}
	}, replaced)
}

func soldierDisplayName(soldier models.Soldier) string {
	if name := soldierFullName(soldier); name != "" {
		return name
	}
	return displayEntryType(soldier)
}

func soldierFullName(soldier models.Soldier) string {
	return persondisplay.FullName(persondisplay.NameParts{
		Prefix:               soldier.Prefix,
		ShowPrefixBeforeName: soldier.ShowPrefixBeforeName,
		FirstName:            soldier.FirstName,
		MiddleName:           soldier.MiddleName,
		LastName:             soldier.LastName,
		Suffix:               soldier.Suffix,
	})
}

func soldierServiceLine(soldier models.Soldier) string {
	return persondisplay.SoldierServiceLine(soldier.RankOut, soldier.Rank, soldier.RankIn, soldier.Unit)
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
	case "linked_person":
		return "Person Record"
	default:
		return "Soldier"
	}
}

func soldierDeathLine(soldier models.Soldier) string {
	return dates.DisplayUnknown(soldier.DeathDate)
}

func pdfConfederateHomeName(soldier models.Soldier) string {
	if confederatehomestatus.Normalize(soldier.ConfederateHomeStatus) == confederatehomestatus.NotApplicable {
		return confederatehomestatus.NotApplicable
	}
	return strings.TrimSpace(soldier.ConfederateHomeName)
}

func confederateHomeNamePDFField(soldier models.Soldier) pdfField {
	if confederatehomestatus.Normalize(soldier.ConfederateHomeStatus) == confederatehomestatus.NotApplicable {
		return naPDFField("Confederate Home Name", confederatehomestatus.NotApplicable)
	}
	return defaultPDFField("Confederate Home Name", strings.TrimSpace(soldier.ConfederateHomeName))
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

func fitPDFImageToBounds(imagePath string, x, y, maxWidth, maxHeight float64) (float64, float64, float64, float64, bool) {
	if maxWidth <= 0 || maxHeight <= 0 {
		return x, y, 0, 0, false
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return x, y, 0, 0, false
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return x, y, 0, 0, false
	}

	width := float64(config.Width)
	height := float64(config.Height)
	scale := maxWidth / width
	if height*scale > maxHeight {
		scale = maxHeight / height
	}
	if scale <= 0 {
		return x, y, 0, 0, false
	}

	fittedWidth := width * scale
	fittedHeight := height * scale
	fittedX := x + (maxWidth-fittedWidth)/2
	fittedY := y + (maxHeight-fittedHeight)/2
	return fittedX, fittedY, fittedWidth, fittedHeight, true
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
