# Component Inventory

The visual components that the rendering stack produces, mapped to the templates and PDF helpers that would migrate to each component, and the override surface each one exposes.

This is **proposed only** — no code is written. The migration targets are the existing PDF helpers and templ templates; the components below describe the *shape* the migration should converge on.

---

## 1. Component registry

Each component is identified by:

- **Name** — the canonical name.
- **Inheritance path** — the `theme.json` subtree it reads by default.
- **Current PDF callsite(s)** — the helpers in `pdf_layout.go` / `export_service.go` that render this component today.
- **Current templ callsite(s)** — the `.templ` files and lines that render the analogous markup today.
- **Override surface** — what callers can adjust at the template layer.

---

### 1.1 `Page`

- **Inheritance path.** `theme.page.*`
- **Purpose.** Establishes document-wide geometry. Owns the fpdf document constructor and the page margin/auto-break state.
- **Current PDF callsite(s).** `newPDFDocument` (`export_service.go:1084–1094`), called by `brandedPDFDocument` (`export_service.go:1096–1131`).
- **Current templ callsite(s).** No equivalent — templ renders inside an HTML `<body>` whose margins come from `layout.templ:30` (`body { ... }`) and CSS shell (`layout.templ:219–228`).
- **Override surface.**
  - `paper_size`, `orientation`, `margins.*`, `header_offset`, `footer_offset`, `auto_break_margin`.
  - The override path: `PrintSettings.Orientation` (per-export delta on `page.orientation`).
- **Notes.** Today `newPDFDocument` is the only place paper size and margins are armed. A `Page` component would replace the literal `fpdf.New(..., "Letter", "")` and the literal `SetMargins(16, 28, 16)` with token reads.

---

### 1.2 `StickyHeader`

- **Inheritance path.** `theme.branding.header` + `theme.palette.text_primary` + `theme.palette.divider` + `theme.type_scale.header.*`
- **Purpose.** Repeating page header on every PDF page (or single page if `ApplyToFirstPage: true`).
- **Current PDF callsite(s).** `brandedPDFDocument` body lines `export_service.go:1102–1112` (the `pdf.SetHeaderFuncMode` callback).
- **Current templ callsite(s).** The templ layout's top header is `<header class="top-shell ...">` at `layout.templ:415` — but that is a *single-page* SPA header, not a sticky multi-page header. The closest equivalent in the static archive is `<header class="hero">` at `static_archive.go:616`.
- **Override surface.**
  - `palette.text_primary` (header text color, currently `(34, 48, 61)`).
  - `palette.divider` (header rule color, currently `(141, 116, 64)`).
  - `branding.header.from`, `branding.header.suffix` (text content).
  - `enabled: false` for `printer_friendly` mode (no header, since printer-friendly also drops footer).

---

### 1.3 `StickyFooter`

- **Inheritance path.** `theme.branding.footer` + `theme.palette.text_secondary` + `theme.palette.divider` + `theme.type_scale.footer.*`
- **Purpose.** Repeating page footer.
- **Current PDF callsite(s).** `brandedPDFDocument` body lines `export_service.go:1114–1129` (the `pdf.SetFooterFunc` callback).
- **Current templ callsite(s).** Live app footer is `<footer class="mt-8 text-right text-xs text-slate-600" data-build-identity=…>` at `layout.templ:509`. Static archive footer is the raw `<footer>` element at `static_archive.go:656`.
- **Override surface.**
  - `palette.text_secondary` (footer text color, currently `(68, 82, 96)`).
  - `palette.divider` (footer rule color, currently `(141, 116, 64)`).
  - `branding.footer.template` (text content; `null` to suppress).
  - `enabled: false` for `printer_friendly` mode (footer suppressed — see `export_service.go:1113` `if !printerFriendly`).

---

### 1.4 `RecordCard`

- **Inheritance path.** `theme.record_card.*` + `theme.type_scale.*` + `theme.palette.*`
- **Purpose.** The two-column record card that displays a Person Record's identity, service, household, image, narrative, and source records.
- **Current PDF callsite(s).**
  - `writePDFRecordCard` (`pdf_layout.go:468–530`) — single-page path.
  - `writePDFRecordCardMultiPage` (`pdf_layout.go:532–588`) — multi-page fallback.
  - `choosePDFRecordCardLayout` (`pdf_layout.go:313–333`) — picks the layout that fits.
  - `defaultPDFRecordCardLayout` / `pdfRecordCardLayout.scaled` (`pdf_layout.go:273–312`).
  - `pdfRecordCardLayout` struct (`pdf_layout.go:255–272`).
- **Current templ callsite(s).** The two-column record card markup is not a single named component in templ — it is scattered across `soldier_card.templ` (the per-soldier detail page), `share.templ` (the export page where settings are configured), and `review_queue.templ` (which renders side-by-side compare cards). The closest analogue is the `SoldierDetail` block in `soldier_card.templ:250–470` (the actual record page), which uses `responsive-two-col` for the field grid and ad-hoc `lg:grid-cols-2` for image and family panels.
- **Override surface.**
  - `record_card.left_column_ratio`, `record_card.column_gap`, `record_card.section_gap`, `record_card.field_row_gap`, `record_card.label_width_clamp`.
  - `type_scale.section_title.*`, `type_scale.field_label.*`, `type_scale.field_value.*`, `type_scale.body.*`.
  - `palette.accent` (section title color, `(141, 116, 64)`).
  - `palette.text_primary` (body text, `(34, 48, 61)`).
  - `palette.text_secondary` (field labels, `(68, 82, 96)`).
  - `layout_mode: "single_column" | "two_column"` — overrides the left/right split for printable archive.
  - `local_overrides` bag at the call site (e.g. `SoldierCompareCard` may set `palette.accent = palette.danger` to draw attention to a discrepancy).

---

### 1.5 `SectionTitle`

- **Inheritance path.** `theme.type_scale.section_title.*` + `theme.palette.accent`
- **Purpose.** The small-caps-style section heading inside a record card.
- **Current PDF callsite(s).** `writePDFSection` (`pdf_layout.go:232–239`), `writePDFCompactFieldSection` title cell (`pdf_layout.go:609–610`), `writePDFImagePanel` title cell (`pdf_layout.go:649–650`), `writePDFRichTextColumnSection` title cell (`pdf_layout.go:686–688`), `writePDFRecordsColumnSection` "Records" header (`pdf_layout.go:705–707`), `writePDFGroupDividerPage` label (`pdf_layout.go:455–458`).
- **Current templ callsite(s).** Every `.templ` file uses small-caps section headings via `text-[#8d7440]` + `0.18em`/`0.22em`/`0.24em`/`0.26em`/`0.28em` letter-spacing. Representative lines: `browse.templ:144`, `camaraderie.templ:33`, `conflict_ledger.templ:36`, `insights.templ:23, 49, 57, 64, 71, 80, 89, 121, 144`, `recovery.templ:16`, `research_log.templ:33`, `research_pack.templ:33`, `review_queue.templ`, `share.templ:47, 73, 124, 142, 163, 282, 306, 326`, `soldier_card.templ:277, 284, 320, 348, 355, 380, 387, 393, 399, 406`, `timeline.templ:31`, `calendar_day.templ:23`.
- **Override surface.**
  - `size`, `line` (from `theme.type_scale.section_title`).
  - `color` (token name, e.g. `"accent"` for normal sections, `"danger"` for review-queue compare cards).
  - `letter_spacing` (currently scattered across 5 different `em` values in templ).
  - `case` (`"upper"`, `"title"`, `"none"`).

---

### 1.6 `FieldLabel` and `FieldValue`

- **Inheritance path.** `theme.type_scale.field_label.*` / `theme.type_scale.field_value.*` + `theme.palette.text_secondary` / `theme.palette.text_primary`
- **Purpose.** The label and value pair inside a compact field section.
- **Current PDF callsite(s).** `writePDFCompactFieldSection` (`pdf_layout.go:603–642`), `writePDFInlineField` (`pdf_layout.go:981–999`), `writePDFField` (`pdf_layout.go:1005–1012`).
- **Current templ callsite(s).** `soldier_card.templ` uses Tailwind utilities `text-sm text-slate-500` (label) and `text-[#22303d]` (value) for the field grid (see `soldier_card.templ:317, 362, 990`). The static archive's `.detail-grid` at `static_archive.go:369` uses `var(--muted)` for `dt` and the inherited `--ink` for `dd`.
- **Override surface.**
  - `size`, `line` per type-scale entry.
  - `color` per palette entry.
  - `style` (bold, italic) for value (today: `writePDFInlineField` uses `pdf.SetFont("Helvetica", "I", 10)` for maiden-name italic at `pdf_layout.go:988`).
  - `align` (left, right) — the static archive detail row uses right-aligned labels (`.detail-grid dt` at `static_archive.go:376`), the live templ version uses left-aligned.

---

### 1.7 `ImagePanel`

- **Inheritance path.** `theme.type_scale.image_panel.*` + `theme.type_scale.image_label.*` + `theme.type_scale.image_thumbnail.*` + `theme.palette.accent` (border)
- **Purpose.** The optional portrait/image panel inside a record card.
- **Current PDF callsite(s).** `writePDFImagePanel` (`pdf_layout.go:644–667`), `estimatePDFImagePanelHeight` (`pdf_layout.go:669–675`), the `ImagePanel` callsite in `writePDFRecordCardMultiPage` at `pdf_layout.go:567–574`.
- **Current templ callsite(s).** The images section of `soldier_card.templ:478–520` uses `card rounded-2xl p-3`, `border-slate-200 bg-white/70`, `accent-[#7b1e2b]` for the select-all checkbox, and `h-40 w-full rounded-xl bg-slate-50` for the thumbnail. None of this is theme-driven.
- **Override surface.**
  - `height` (token: `theme.type_scale.image_panel.height`).
  - `border_color`, `border_width`, `border_radius`.
  - `enabled: false` for `printable_archive` mode (typically hidden — see `PDFOptions.PrintableArchive` usage in `pdf_layout.go:832`).

---

### 1.8 `RecordsTable`

- **Inheritance path.** `theme.record_card.column_gap` + `theme.type_scale.body.*` + `theme.palette.text_primary` + `theme.palette.accent` (header color)
- **Purpose.** The "Records" section that lists Source Records at the bottom of the right column.
- **Current PDF callsite(s).** `writePDFRecordsColumnSection` (`pdf_layout.go:696–717`).
- **Current templ callsite(s).** The Source Records section in `soldier_card.templ:471–476` (`<h3 class="text-lg">Source Records</h3>` + per-record card); the records list in `review_queue.templ:152–166` (a 3-column CSS grid with `grid-template-columns: [minmax(10rem,1.3fr)_minmax(0,1fr)_minmax(0,1fr)]`).
- **Override surface.**
  - `column_gap`, `section_gap` (inherited from `theme.record_card`).
  - `palette.accent` (header text color).
  - `density: "compact" | "comfortable"` (row height).
  - `zebra: "on" | "off"` (alternating row backgrounds).
  - `sticky_header: "on" | "off"` (only meaningful in CSS).

---

### 1.9 `RichTextSection`

- **Inheritance path.** `theme.type_scale.body.*` + `theme.palette.text_primary` + `theme.palette.link`
- **Purpose.** Renders the narrative (biography / service narrative) block of a record card.
- **Current PDF callsite(s).** `writePDFRichTextColumnSection` (`pdf_layout.go:677–694`), `writePDFRichText` (`pdf_layout.go:1040–1042`), `writePDFRichTextSized` (`pdf_layout.go:1056–1085`).
- **Current templ callsite(s).** The biography wrapper in `soldier_card.templ:347–349` uses `rgba(141,116,64,0.28) bg-[rgba(255,248,230,0.48)]` and `0.24em text-[#8d7440]`. The detail-section body in `static_archive.go:391` uses `color: var(--muted); line-height: 1.6; white-space: pre-wrap;`.
- **Override surface.**
  - `size`, `line` (inherited from `theme.type_scale.body`).
  - `color` (token name; today `text_primary` for narrative, `link` for clickable segments).
  - `wrap_multiplier` (currently 1.18, hard-coded in `pdf_layout.go:414` and `pdf_layout.go:430`).
  - `link_color` (token name; today `palette.link` for `(48, 87, 122)`).

---

### 1.10 `BiographyPage`

- **Inheritance path.** `theme.type_scale.biography.*` + `theme.type_scale.section_title.*` + `theme.palette.accent` (title)
- **Purpose.** The standalone "Full Biography" page (single-record and printable appendix variants).
- **Current PDF callsite(s).** `writeFullBiographyPage` (`pdf_layout.go:866–879`), `shouldAppendSingleRecordBiographyPage` (`pdf_layout.go:830–836`), `writeSingleRecordBiographyPage` (`pdf_layout.go:862–864`), `writePrintableBiographyAppendixPage` (`pdf_layout.go:858–860`).
- **Current templ callsite(s).** No analogous standalone page in templ.
- **Override surface.**
  - `biography_appendix` override (see `theme.json` `overrides.biography_appendix`): bumps size/line metrics to give the appendix more breathing room.
  - `enabled: false` when `PDFOptions.PrintableArchive` or `usesPortraitRecordPDFLayout(options)` (see `pdf_layout.go:832`).
  - `label` (string, default `"Full Biography"`, overridden to `"Full Biography Appendix"` for the printable appendix path).

---

### 1.11 `GroupDividerPage`

- **Inheritance path.** `theme.palette.accent` (label) + `theme.palette.text_primary` (title) + `theme.palette.text_secondary` (subtitle) + `theme.type_scale.section_title.*` (label font)
- **Purpose.** A page that introduces a group of records (by unit, pension state, etc.) when `PrintSettings.GroupBy*` is set.
- **Current PDF callsite(s).** `writePDFGroupDividerPage` (`pdf_layout.go:453–466`), called from `export_service.go:1025`.
- **Current templ callsite(s).** No templ analogue.
- **Override surface.**
  - `label`, `title`, `subtitle` (text).
  - `palette.accent`, `palette.text_primary`, `palette.text_secondary`.
  - `title_size` (currently `maxFloat(20, 28 - float64(level)*2)` at `pdf_layout.go:459` — derived).

---

### 1.12 `RegistryEntry`

- **Inheritance path.** `theme.type_scale.field_value.*` + `theme.palette.accent` (rule) + `theme.palette.text_primary` (name) + `theme.palette.text_secondary` (sub-line) + `theme.overflow.registry_break_y`
- **Purpose.** A single line in the printable archive's registry index.
- **Current PDF callsite(s).** `writePDFRegistryEntry` (`pdf_layout.go:1159–1179`).
- **Current templ callsite(s).** The "Archive List" view in `static_archive.go:637–644` (the `.results` section filled by JS).
- **Override surface.**
  - `registry_break_y` (page-break Y position).
  - `palette.accent` (rule color).
  - `name_size`, `sub_size` (currently 13 / 10 at `pdf_layout.go:1166, 1169`).

---

### 1.13 `ImageRow`

- **Inheritance path.** `theme.type_scale.image_thumbnail.*` + `theme.type_scale.image_row.*` + `theme.overflow.image_row_bottom_guard`
- **Purpose.** A horizontal row of image thumbnails.
- **Current PDF callsite(s).** `writePDFImageRow` (`pdf_layout.go:1122–1157`).
- **Current templ callsite(s).** The images section in `soldier_card.templ:484–520`.
- **Override surface.**
  - `image_thumbnail.width`, `image_thumbnail.height`.
  - `image_row.height`.
  - `image_row_bottom_guard`.
  - `palette.accent` (border color; today `(141, 116, 64)`).

---

### 1.14 `TitleBlock`

- **Inheritance path.** `theme.type_scale.section_title.*` (large) + `theme.palette.text_primary` (title) + `theme.palette.text_secondary` (subtitle)
- **Purpose.** Title and subtitle at the top of a page (e.g. group divider, biography page).
- **Current PDF callsite(s).** `writePDFTitleBlock` (`pdf_layout.go:241–251`) — uses `Times B 20` for title and `Helvetica 10` for subtitle.
- **Current templ callsite(s).** The hero h1 in `static_archive.go:167` (`Georgia "Times New Roman" serif; font-size: clamp(1.45rem, 2.8vw, 2.2rem); line-height: 1.15; color: #cfb77a;`).
- **Override surface.**
  - `title_font`, `title_size` (currently `display_font: "Times"`, `size: 20`).
  - `subtitle_font`, `subtitle_size` (currently `primary_font: "Helvetica"`, `size: 10`).
  - `title_color`, `subtitle_color`.

---

## 2. Component-to-callsite migration table

This table is the actionable "where do I start?" map for the implementation phase that follows the audit.

| Component | Today's PDF callsite(s) | Today's templ callsite(s) | Effort |
|---|---|---|---|
| `Page` | `newPDFDocument` (`export_service.go:1084–1094`) | `body { … }` in `layout.templ:30–35` | small — single init point |
| `StickyHeader` | `brandedPDFDocument` header callback (`export_service.go:1102–1112`) | `layout.templ:415–420` (`.top-shell`), `static_archive.go:616–623` (`.hero`) | small — single function literal in PDF, two structural blocks in templ |
| `StickyFooter` | `brandedPDFDocument` footer callback (`export_service.go:1114–1129`) | `layout.templ:509` (raw `<footer>`), `static_archive.go:490, 656` (`footer { … }` + raw `<footer>`) | small |
| `RecordCard` | `writePDFRecordCard` (`pdf_layout.go:468–530`) + `writePDFRecordCardMultiPage` (`pdf_layout.go:532–588`) + `pdfRecordCardLayout` struct + `defaultPDFRecordCardLayout` (`pdf_layout.go:255–293`) + `choosePDFRecordCardLayout` (`pdf_layout.go:313–333`) + the 5 `estimatePDF*` helpers | scattered across `soldier_card.templ:250–470`, `share.templ`, `review_queue.templ` | **large** — 7 PDF functions migrate; templ side is a *new* component class |
| `SectionTitle` | 5 PDF callsites (`pdf_layout.go:609, 649, 686, 705, 455`) | 50+ templ callsites (every file with a small-caps heading) | medium — small Go helper, large templ helper |
| `FieldLabel` / `FieldValue` | `writePDFCompactFieldSection` (`pdf_layout.go:603–642`), `writePDFInlineField` (`pdf_layout.go:981–999`), `writePDFField` (`pdf_layout.go:1005–1012`) | `soldier_card.templ:317, 362, 990` (Tailwind `text-slate-500` + `text-[#22303d]`), `static_archive.go:369` (`.detail-grid`) | medium |
| `ImagePanel` | `writePDFImagePanel` (`pdf_layout.go:644–667`) + `estimatePDFImagePanelHeight` (`pdf_layout.go:669–675`) | `soldier_card.templ:478–520` | small |
| `RecordsTable` | `writePDFRecordsColumnSection` (`pdf_layout.go:696–717`) | `soldier_card.templ:471–476`, `review_queue.templ:152–166` | medium |
| `RichTextSection` | `writePDFRichTextColumnSection` (`pdf_layout.go:677–694`) + `writePDFRichText*` (`pdf_layout.go:1040–1085`) | `soldier_card.templ:347–349` (biography wrapper), `static_archive.go:391` (`.detail-section p, li`) | medium |
| `BiographyPage` | `writeFullBiographyPage` (`pdf_layout.go:866–879`) + 2 wrapper one-liners (`pdf_layout.go:858–864`) + `shouldAppendSingleRecordBiographyPage` (`pdf_layout.go:830–836`) | n/a | small |
| `GroupDividerPage` | `writePDFGroupDividerPage` (`pdf_layout.go:453–466`) | n/a | small |
| `RegistryEntry` | `writePDFRegistryEntry` (`pdf_layout.go:1159–1179`) | `static_archive.go:637–644` (`.results` section) | small |
| `ImageRow` | `writePDFImageRow` (`pdf_layout.go:1122–1157`) | `soldier_card.templ:484–520` | small |
| `TitleBlock` | `writePDFTitleBlock` (`pdf_layout.go:241–251`) | `static_archive.go:167` (`.hero h1`) | small |

---

## 3. Override surface — summary

The `local_overrides` bag on a component instance lets a caller redefine leaf tokens *for that one component only*. Examples drawn from the current code:

| Component | Local override scenario | Today's literal workaround |
|---|---|---|
| `RecordCard` | `printable_archive` flips to single-column | `writePDFRecordCard` calls `writePDFRichTextColumnSection` with full width (no left/right) for the narrative — currently un-tokenized; this would become `layout_mode: "single_column"` |
| `RecordCard` | compare-card uses danger-colored section title to draw attention to a discrepancy | `review_queue.templ:124, 143` already inlines `border-[rgba(111,44,38,0.35)] bg-[rgba(111,44,38,0.08)] text-[#6f2c26]` — would become `local_overrides: { "section_title": { "color": "danger" } }` |
| `SectionTitle` | group-divider label is "Grouped by …" instead of the usual section name | `writePDFGroupDividerPage` calls `writePDFSection` with the label as the section title — no override needed; this is data, not style |
| `RecordsTable` | zebra striping | `browse.templ:155` uses `bg-[rgba(246,241,228,0.46)]` on row hover — would become `zebra: "on"` |
| `ImagePanel` | hidden in printable archive | `PDFOptions.PrintableArchive` flag — the override would be `enabled: false` |

---

## 4. ViewModel-layer seam

The existing boundary must be preserved. `internal/presentation/views.go` is the only adapter that turns domain objects into DTOs/ViewModels. The component layer is **presentation-only**; any new theme-aware data the components need must travel through the ViewModel layer.

What this means in practice:

- A new theme field on the ViewModel side is required only if a component needs a theme decision the `theme.Theme` cannot make on its own (e.g. "which sections of this Person Record are incomplete and should be highlighted in danger color"). Today no such field exists; today's call site for compare-card danger highlighting is purely visual.
- `UserIdentity.BrandingName` is already read directly by `pdfBranding()` (`export_service.go:1135`). A `Theme` component can read the same source without touching the ViewModel layer for branding.
- The audit found no theme fields on the ViewModel side today (`views.go` has no structs; all ViewModel structs in `internal/viewmodel/` are theme-agnostic). The implementation phase should preserve this: theme tokens do not enter the ViewModel layer.

---

## 5. What does not need to be a component

A few things that look like components but are actually data or runtime conditions:

- The `Person Record` content of a record card is data, not a component. The data shape is owned by `internal/models` and the ViewModel layer.
- The export settings UI in `share.templ` is a form, not a visual component. It is rendered with the same Tailwind utilities as everything else.
- The `pdfRecordCardLayout` struct itself is not a component; it is a **layout values bag** that the `RecordCard` component reads at construction time. After migration, the struct's fields become token reads (`pdfRecordCardLayout.LeftWidthRatio` → `theme.RecordCard.LeftColumnRatio`).
