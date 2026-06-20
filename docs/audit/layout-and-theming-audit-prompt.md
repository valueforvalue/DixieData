# Layout & Theming Diagnostic Audit Prompt

## Purpose

Isolate **formatting definitions** (page geometry, margins, colors, fonts, spacing, borders, grid columns) from the **template compilation engine** (the Go/Templ/HTML/CSS delivery stack and the `go-pdf/fpdf` rendering pipeline) in DixieData.

This audit is **read-only**. No application code is to be modified, refactored, or written. The output is a diagnostic artifact: a structured map of *where* visual rules live today, *how rigid* they are, and *what a portable theming seam would need to look like* before any implementation is proposed.

Scope of formatting concerns:

- Page geometry (paper size, orientation, margins)
- Grid columns and section spacing
- Line height, font scale, bullet indents, section gaps
- Color palette (text, accent, borders, dividers, fills)
- Typography (family, weight, size)
- Header / footer branding
- Orphan / widow / multi-page overflow handling for tabular historical and soldier record cards

Out of scope: domain logic, data collection, persistence, archive import/export mechanics, frontend SPA behavior beyond the static HTML the templates emit.

---

## 1. Fine-Grained Layout & Theming Diagnostics

For each item below, record the exact file path, the line range, the literal value used, and whether the value is **structural** (defines a layout primitive), **thematic** (color/font/branding), or **hybrid** (ties structure to a specific theme).

### 1.1 Structural Constraints

1.1.1. Enumerate every place where page margins are declared as literal numbers or unit-bearing strings. Cover at minimum:
- The initial `pdf.SetMargins(...)` call that arms every exported PDF document.
- Any inner `pdf.SetMargins(...)` calls that re-define margins mid-page (column-section helpers, multi-page record cards, biography appendices).
- Per-export-type overrides (single-soldier PDF vs. bulk archive vs. printable biography appendix vs. static archive preview).
- The HTML template page container (the `<body>` / `.page` / outer wrapper rules in `internal/templates/layout.templ`) and any `@media print` block that mirrors PDF geometry.
- The CSS-side `@page` rule (if present) and `body { margin: ... }` declarations that govern browser printing of static archives.

1.1.2. Enumerate every place where line spacing is set. Distinguish:
- PDF `CellFormat` / `MultiCell` height arguments.
- PDF line-height arguments passed to `writePDFRichText`, `writePDFRichTextSized`, `writePDFRichTextColumnSection`.
- CSS `line-height` declarations in template files.
- Templ-level `style="line-height: ..."` overrides passed through DTOs.

1.1.3. Enumerate every page border / divider rule:
- `pdf.SetDrawColor(...)` + `pdf.Line(...)` calls that draw header underlines, footer rules, and group-divider separators.
- CSS `border`, `border-top`, `border-bottom`, `border-color`, `border-radius` rules used as structural dividers vs. component chrome.
- The `.gold-border`, `.ribbon`, `.ghost-link` style class families in `layout.templ` and how each is consumed by other templates.

1.1.4. Enumerate grid column definitions:
- The `LeftWidthRatio` and `ColumnGap` fields of `pdfRecordCardLayout` and every callsite that reads them.
- The CSS grid / flex definitions in templates (`.record-card`, `.field-grid`, `.columns-*`) used for two-column record layouts.
- The HTML `<table>` structures (if any) that establish tabular column widths for historical records.

1.1.5. For each of the above, classify the value as:
- **Hard-coded literal** — embedded in a Go expression or CSS rule with no indirection.
- **Default-only** — referenced through a `defaultXxx()` constructor but never overridden by configuration.
- **Per-export configurable** — surfaced through `PrintSettings` or `PDFOptions` (or their JSON tag set).

### 1.2 Layout Boundaries, Multi-Page Overflow, Orphan / Widow Handling

1.2.1. Trace the multi-page strategy for tabular soldier record cards:
- The single-vs-multi-page branch in `writePDFRecordCard` and the delegation to `writePDFRecordCardMultiPage`.
- The `choosePDFRecordCardLayout` shrink loop (the `[]float64{1, 0.94, 0.88, 0.82, 0.76}` scale sequence and the `minReadablePDFRecordCardScale` floor).
- The `estimatePDFRecordCardHeight` / `estimatePDFCompactFieldSectionHeight` / `estimatePDFRichTextSectionHeight` / `estimatePDFRecordsSectionHeight` family of pre-flight measurements.
- The `preparePDFRecordCardSection` overflow guard that calls `pdf.AddPage()` when a section would cross the bottom margin.

1.2.2. Identify any orphan / widow handling — explicit or implicit:
- PDF: does any helper insert a keep-together guard around a label/value pair, a section title and its first field, or a record block and its first continuation line?
- CSS: are `orphans`, `widows`, `break-inside: avoid`, `page-break-inside: avoid`, `break-after`, or `break-before` declared anywhere? If yes, list each rule with its selector.
- Template-level: do any templ components render a `keep-together` wrapper class?

1.2.3. Map the boundary mechanics for the printable biography appendix and full-biography page:
- The single-record biography page path and the appendix page path.
- The conditions under which each is appended (look at `shouldAppendSingleRecordBiographyPage`, `writeSingleRecordBiographyPage`, `writePrintableBiographyAppendixPage`, `writeFullBiographyPage`).
- Whether biography flow re-uses `pdfRecordCardLayout` or applies its own line height / font scale.

1.2.4. Identify the static-archive HTML counterpart:
- How the browser-rendered HTML in `internal/archive/static_archive.go` approximates the PDF page geometry.
- Whether column ratios, line heights, and section gaps are duplicated from `pdfRecordCardLayout` as CSS literals or pulled from a shared token (today: literal duplication; expected: literal duplication).

### 1.3 Thematic Elements

1.3.1. Enumerate the canonical color palette by locating every `SetTextColor`, `SetDrawColor`, `SetFillColor` call in `internal/archive/pdf_layout.go` and `internal/archive/export_service.go`. Group occurrences by RGB triplet (e.g., `(141, 116, 64)`, `(34, 48, 61)`, `(68, 82, 96)`, `(48, 87, 122)`). For each triplet, record the semantic role (accent gold, primary text, secondary text, link blue, etc.).

1.3.2. Enumerate every CSS hex color used in `internal/templates/*.templ` files. Group by role:
- Text colors (primary, secondary, muted, link, danger).
- Border / divider colors (gold ribbon, ghost outline, danger).
- Fill colors (panel background, button, danger fill).
- Map each hex to its nearest PDF RGB triplet equivalent if one exists.

1.3.3. Enumerate every font family and size literal:
- PDF: the `"Helvetica"`, `"Times"` strings and the integer/float sizes used at every callsite.
- CSS: the `font-family` declarations (Helvetica Neue / Arial / sans-serif, Georgia / Times New Roman / serif, monospace) and the named scale classes (e.g., `text-xs`, `text-sm`, `text-base`, `text-lg`, `text-xl` if present in the Tailwind config).
- The Tailwind config file (`tailwind.config.js`) and any theme extension that defines custom colors, fonts, or spacing.

1.3.4. Locate header and footer branding:
- The `pdf.SetHeaderFuncMode(...)` callback in `brandedPDFDocument`.
- The `pdf.SetFooterFunc(...)` callback (printer-friendly off path only).
- The `pdfBranding` struct fields (`ArchiveTitle`, `FooterText`) and the `pdfBranding()` method that derives them from `UserIdentity`.
- The HTML header/footer analogues in the templ layout (`.app-header`, `.app-footer`, `.branding-*`).

1.3.5. Evaluate the change cost matrix — for each thematic element, answer:
- Can a user change this **per export type** (PDF vs. printable biography vs. static archive HTML) without recompiling?
- Can a user change this **globally** without editing Go source or templ files?
- Can a user change this **per archive** (each user identity / brand) without code change?
- Is the change a single-file edit, a multi-file edit, or a structural rewrite?

Score each on: `trivial` (one literal), `local` (one file, one function), `cross-file` (multiple files, same domain), `cross-layer` (Go + templ + CSS + Tailwind config), `structural` (requires new abstraction).

---

## 2. Layout & Theme Configuration Strategy

Propose a **unified styling configuration framework** that lives alongside the template files in the program directory. The framework is described here as an architectural target — no code is written during this audit. Every proposal below must be paired with a diagnosis from Section 1 that justifies its necessity.

### 2.1 Centralized Design Token Schema

Define a portable configuration format — recommended shape: a `theme.json` file co-located with the template files, with a companion **typed Go struct** that mirrors the JSON shape and exposes the tokens to both PDF and templ/CSS consumers.

Required token categories:

- **Page geometry**: `paper_size` (`"letter"` / `"a4"` / explicit width+height in mm), `orientation` (`"portrait"` / `"landscape"`), `margins` (`top`, `bottom`, `left`, `right` in mm or in-format strings like `"0.75in"`), `header_offset`, `footer_offset`.
- **Grid**: `record_card.left_column_ratio`, `record_card.column_gap`, `record_card.section_gap`, `section.title_to_body_gap`.
- **Type scale**: `font_scale` (global multiplier), per-element sizes (`section_title`, `field_label`, `field_value`, `body`, `image_label`, `bullet_indent`), line heights (`section_title_line`, `field_line_height`, `body_line_height`), per-family families (`primary`, `serif_display`, `monospace`).
- **Color palette**: named tokens (`accent`, `accent_strong`, `text_primary`, `text_secondary`, `text_muted`, `link`, `danger`, `divider`, `panel_fill`, `printer_safe_text`, `printer_safe_muted`), each carrying an `rgb` triplet for PDF and a `hex` string for CSS.
- **Borders**: `divider.weight`, `divider.color_token`, `panel.radius`, `panel.border_weight`, `panel.border_color_token`.
- **Printability**: `printer_friendly.*` overrides (typically `text_primary = "#000"`, footer/header suppressed, dividers dropped).

Schema constraints:

- Single source of truth. Tokens resolve identically for PDF, CSS-in-templ, and standalone CSS.
- Format-independent units in the JSON: prefer millimeters for page geometry and points (`pt`) for font sizes; reject mixed unit strings without an explicit `unit` field.
- Token references allowed (`"color": "$accent"`) so per-export overrides only need to redefine the smallest possible leaf.
- A versioned schema (`"schema_version": "1.0.0"`) so future revisions can branch cleanly.

### 2.2 Declarative Page Layouts

Page geometry must be **set declaratively** without reaching into the data collection path. Concretely:

- The PDF document initializer (`newPDFDocument` / `brandedPDFDocument`) reads page geometry exclusively from the token store.
- `pdf.SetMargins(...)` and `pdf.SetAutoPageBreak(...)` derive their arguments from tokens, not from literals.
- `PrintSettings` / `PDFOptions` retain only **deltas** (e.g., user toggles printer-friendly mode, picks orientation override); the base geometry always comes from tokens.
- A change to paper size (Letter → A4) or orientation requires editing one token, not hunting literals across `pdf_layout.go` and `export_service.go`.
- `fpdf.New(orientation, "mm", size, "")` accepts the `size` argument from the token; do not hardcode `"Letter"`.

Audit checklist — for every literal surfaced in Section 1, decide:
- Does this literal belong in `theme.json`?
- Does it belong in `PrintSettings`?
- Does it belong in the per-export-type config (e.g., a `biography` section that overrides `record_card`)?
- Is it a derived value (computed from others) and therefore not a candidate for externalization?

### 2.3 Component-Level Overrides

Specific structural elements must inherit global tokens while supporting fine-grained overrides at the template layer. Required component classes:

- **`RecordCard`**: the two-column soldier record layout. Inherits `record_card.*` tokens. Supports a per-export override (e.g., the printable biography appendix uses a single-column variant).
- **`Table`**: tabular historical records. Inherits `grid.*` and `borders.*`. Supports column-specific override for sticky header behavior, zebra striping, and row density.
- **`StickyHeader`**: page header that repeats on every page. Inherits `header.*` and `branding.*`. Supports per-export override (e.g., suppressed in printer-friendly mode).
- **`StickyFooter`**: page footer. Same inheritance model as header.
- **`SectionTitle`**: small-caps style section header inside a record card. Inherits `section.*`. Supports per-component override (e.g., oversized variant in group divider pages).
- **`ImagePanel`**: the optional portrait/image panel. Inherits `image_panel.*`. Supports per-export override for `printable_archive` (typically hidden).

Override semantics:

- Each component has a `theme_path` (e.g., `record_card.section_title`) that resolves a token subtree.
- A component can declare a `local_overrides` block at the template layer that selectively replaces leaf tokens (e.g., `{ "color": "$danger" }` to recolor a single subsection).
- Overrides must not redefine structure — they may only adjust visual properties. Structural changes (column count, section order) remain code-level.

---

## 3. Updated Minimal Working Example

Provide a **revised, conceptual** architectural example. This is illustrative, not code to be merged. Every token must trace back to a Section 1 finding.

### 3.1 The Config Matrix — `theme.json`

```json
{
  "schema_version": "1.0.0",
  "page": {
    "paper_size": "letter",
    "orientation": "landscape",
    "margins": {
      "top": "0.75in",
      "bottom": "0.75in",
      "left": "0.75in",
      "right": "0.75in"
    },
    "header_offset": "0.4in",
    "footer_offset": "0.4in"
  },
  "record_card": {
    "left_column_ratio": 0.52,
    "column_gap": "6mm",
    "section_gap": "3mm",
    "field_row_gap": "0.7mm"
  },
  "type_scale": {
    "primary_font": "Helvetica",
    "display_font": "Times",
    "scale": 1.0,
    "section_title": { "size": 9, "line": 6 },
    "field_label":   { "size": 8, "line": 4.5 },
    "field_value":   { "size": 9, "line": 4.5 },
    "body":          { "size": 9, "line": 5 },
    "image_label":   { "size": 8, "line": 4 },
    "bullet_indent": "1.6mm"
  },
  "palette": {
    "accent":          { "rgb": [141, 116, 64],  "hex": "#8d7440" },
    "accent_strong":   { "rgb": [168, 138, 70],  "hex": "#a88a46" },
    "text_primary":    { "rgb": [34, 48, 61],    "hex": "#22303d" },
    "text_secondary":  { "rgb": [68, 82, 96],    "hex": "#445260" },
    "text_muted":      { "rgb": [113, 128, 142], "hex": "#71808e" },
    "link":            { "rgb": [48, 87, 122],   "hex": "#30577a" },
    "danger":          { "rgb": [84, 33, 29],    "hex": "#54211d" },
    "divider":         { "rgb": [141, 116, 64],  "hex": "#8d7440" },
    "panel_fill":      { "hex": "#fff8e7" }
  },
  "borders": {
    "divider_weight": "0.2mm",
    "panel_radius": "0.8rem",
    "panel_border_weight": "1px"
  },
  "branding": {
    "header": { "from": "user_identity.branding_name", "suffix": "'s Civil War Research Archive" },
    "footer": { "template": "Made with DixieData | Version: {app_version} | Build: {build_identity}" }
  },
  "overrides": {
    "printer_friendly": {
      "palette": {
        "text_primary": { "rgb": [0, 0, 0], "hex": "#000000" },
        "text_secondary": { "rgb": [0, 0, 0], "hex": "#000000" }
      },
      "branding": { "footer": null }
    },
    "printable_archive": {
      "record_card": { "left_column_ratio": 1.0, "column_gap": "0mm" }
    },
    "biography_appendix": {
      "type_scale": { "body": { "size": 10, "line": 6 }, "section_title": { "size": 10, "line": 7 } }
    }
  }
}
```

Each token must be traceable to a Section 1 finding — e.g., `palette.accent.rgb = [141, 116, 64]` cites the triplet used at `pdf.SetTextColor(141, 116, 64)` in `pdf_layout.go`.

### 3.2 The Injected Component — `RecordCard` Template Snippet

Conceptual shape (illustrative; the actual implementation is templ + Go):

```templ
// templ pseudocode — describes the shape, not the syntax to be written
templ RecordCard(soldier models.Soldier, theme Theme, override ComponentOverride) {
    @LayoutFrame(theme.Page.Margins, theme.Page.PaperSize) {
        @StickyHeader(theme.Branding.Header.From(soldier.Owner))
        <div class={ recordCardClasses(theme, override) }>
            @RecordCardColumn("left",
                width = theme.RecordCard.LeftColumnRatio,
                gap   = theme.RecordCard.ColumnGap,
                sections = {
                    SectionTitle("Identity & Vital Details"),
                    FieldSection(identityFields(soldier), theme.TypeScale.FieldLabel, theme.TypeScale.FieldValue),
                    SectionTitle("Service & Archive Details"),
                    FieldSection(serviceFields(soldier), theme.TypeScale.FieldLabel, theme.TypeScale.FieldValue),
                },
                overrides = override.Left)
            @RecordCardColumn("right",
                width = 1 - theme.RecordCard.LeftColumnRatio,
                gap   = theme.RecordCard.ColumnGap,
                sections = {
                    if theme.Components.ImagePanel.Enabled {
                        ImagePanel(...)
                    }
                    FieldSection(householdFields(soldier), ...),
                    RichTextSection(narrativeText, ...),
                    RecordsSection(soldier.Records, ...),
                },
                overrides = override.Right)
        </div>
        @StickyFooter(theme.Branding.Footer.Template)
    }
}
```

Key properties demonstrated:

- The component receives `theme` (global tokens) and `override` (component-level deltas).
- Margins and paper size come from `theme.Page.*`, never from literals.
- Column ratios and gaps come from `theme.RecordCard.*`.
- Typography comes from `theme.TypeScale.*`.
- Colors are referenced by **token name** (`palette.accent`), never by literal RGB/hex inside the component.
- Branding strings come from `theme.Branding.*`, which is data-driven from user identity + version metadata.
- The override bag lets a caller flip the card to single-column (`PrintableArchive` override) or shrink the body type (`BiographyAppendix` override) without touching the component itself.

---

## 4. Layout Engine Evaluation

Compare the candidate underlying layout paradigms for the PDF and HTML rendering paths. Score each paradigm against the criteria below, using the Section 1 findings as evidence.

### 4.1 Candidate A — Exact Coordinate Grid Mapping (current state, `go-pdf/fpdf`)

- **Current behavior**: every visual element is placed at an `(x, y)` coordinate derived from running counters (`pdf.GetY()`, `pdf.GetX()`), fixed geometry constants, and a hand-tuned shrink loop in `choosePDFRecordCardLayout`.
- **Margin enforcement**: relies on `pdf.SetMargins` + `pdf.SetAutoPageBreak` + an explicit `pdf.AddPage()` guard in `preparePDFRecordCardSection`. Predictable but verbose; orphan/widow control is implicit (the multi-page path re-flattens sections in order).
- **Multi-page reliability**: high — explicit page breaks for each section that would cross the bottom margin. Cost: large surface area of `estimate*` helpers that must stay in sync with the actual render helpers.
- **Theme portability**: low — every literal is inline; a palette change requires source edits.
- **Refactor cost to support `theme.json`**: medium — a token resolver at document-init time plus a thin wrapper that swaps literals for token reads; the helper geometry stays the same.

### 4.2 Candidate B — Flow-Based Document Layout (e.g., HTML/CSS-to-PDF via headless renderer)

- **Behavior**: layout is described in CSS; the renderer reflows text, splits pages, and applies orphan/widow rules natively.
- **Margin enforcement**: native via `@page` + CSS `margin`. Strong support for `break-inside: avoid`, `orphans`, `widows`.
- **Multi-page reliability**: high — the engine handles page breaks per the CSS rules; no manual `AddPage()` arithmetic.
- **Theme portability**: very high — CSS variables (`:root { --accent: #8d7440; }`) consume tokens directly; a theme change is a variable override.
- **Refactor cost to support `theme.json`**: low for CSS, high for Go — requires a headless rendering dependency (Chromium, WeasyPrint, or wkhtmltopdf), packaging concerns (binary size, sandboxing), and a different testing surface.
- **Risk**: deterministic output is harder to guarantee; rendering cost is higher; archival outputs may differ across OS/shell versions. For a local-first archive that emphasizes reproducible exports, this risk is non-trivial.

### 4.3 Candidate C — Hybrid: Coordinate Grid + Token-Driven Theming (recommended target)

- **Behavior**: keep the `go-pdf/fpdf` coordinate-grid engine for page-perfect determinism. Replace every literal surfaced in Section 1 with a token read at document-init time. Wrap the section-render helpers in a `RecordCard` component that resolves tokens once per card and passes them through.
- **Margin enforcement**: unchanged engine, but all margin literals resolve through tokens. Orphan/widow control remains explicit; future work could add a `keep_together` token that flips the section-flatten behavior.
- **Multi-page reliability**: unchanged — same proven page-break logic, same `estimate*` helper family.
- **Theme portability**: high — a single `theme.json` change re-skins the entire PDF family and the static-archive HTML.
- **Refactor cost**: medium-low — no engine swap; the work is concentrated in (a) the typed token schema, (b) the document initializer that resolves tokens, (c) the section render helpers that take a `Tokens` argument instead of literals.
- **Determinism**: preserved — same engine, same arithmetic, only the inputs change.

### 4.4 Scoring Matrix

Score each candidate 1–5 against each criterion; justify each score with Section 1 evidence.

| Criterion | A — Coord Grid (today) | B — Flow-Based | C — Hybrid (target) |
|---|---|---|---|
| Margin determinism | 4 | 4 | 4 |
| Multi-page reliability | 4 | 4 | 4 |
| Orphan/widow control | 2 | 5 | 2 (now), 3 (with keep-together token) |
| Theme change cost (per palette) | 1 | 5 | 4 |
| Theme change cost (per geometry) | 1 | 5 | 4 |
| Reproducibility across OS | 5 | 2 | 5 |
| Refactor cost from today | n/a | 5 (high) | 3 (medium-low) |
| Test surface stability | 5 | 2 | 5 |

Recommend the hybrid path unless a future requirement forces flow-based rendering (e.g., rich CSS hover-state previews, JS-driven interactivity in static archive previews).

---

## 5. Audit Deliverables

Produce the following artifacts at the end of the audit. No application code is to be written.

### 5.1 Findings Document

A single markdown file (suggested path: `docs/audit/layout-theming-findings.md`) containing:

1. **Section 1.1 register** — every structural literal: file, line, value, classification (hard-coded / default-only / configurable).
2. **Section 1.2 register** — the multi-page strategy map: every helper involved in page-break logic, with line refs.
3. **Section 1.3 register** — the color palette register: every RGB triplet and CSS hex, grouped by semantic role, with file+line citations.
4. **Change-cost matrix** — the scoring table from Section 1.3.5.
5. **Per-export-type theme map** — which theme tokens apply to PDF bulk export vs. PDF single-record vs. printable biography appendix vs. static archive HTML.

### 5.2 Token Schema Proposal

A second markdown file (suggested path: `docs/audit/layout-theming-token-schema.md`) containing:

1. The full `theme.json` shape with every field documented (purpose, units, default, semantic role).
2. The typed Go struct that mirrors the JSON.
3. The token-resolution rules (override precedence, reference syntax).
4. The validation rules (required fields, allowed unit strings, RGB ranges).

### 5.3 Component Inventory

A third markdown file (suggested path: `docs/audit/layout-theming-components.md`) containing:

1. The list of components from Section 2.3 with their inheritance paths.
2. For each component, the current templ/PDF callsite(s) that would migrate to it.
3. The override surface each component exposes.

### 5.4 Engine Evaluation Report

A fourth markdown file (suggested path: `docs/audit/layout-theming-engine-evaluation.md`) containing:

1. The filled-in scoring matrix from Section 4.4.
2. The recommendation with explicit risk callouts.
3. The conditions under which the recommendation should be revisited.

---

## 6. Audit Constraints

- **Read-only.** Do not edit, create, or delete application source files, templates, CSS, or configuration files during the audit. The only artifacts produced are the four markdown files in Section 5 plus this prompt itself.
- **Evidence-first.** Every finding must cite a file path and line range. No speculative claims about how a system "probably" works.
- **No code proposals in the findings.** The schema in Section 3 is illustrative — the actual proposed schema lives in `docs/audit/layout-theming-token-schema.md` and must be backed by evidence from Section 1.
- **Domain terminology** must follow `CONTEXT.md`. Use **Person Record**, **Source Record**, **Soldier**, **Display ID**, **Local Archive**, **Shared Archive**, **Backup Archive**, **Static Archive** as defined there.
- **Audit boundaries** must respect the existing Grey Box: `internal/presentation/views.go` is the adapter that turns domain objects into DTOs/ViewModels consumed by `internal/templates`. Theming changes that need new data fields must travel through the ViewModel layer, not by reaching past it into the database or domain services.
- **Pre-release audits must end with the four deliverables.** No "see code for details" placeholders.