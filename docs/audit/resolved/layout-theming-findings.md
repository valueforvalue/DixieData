# Layout & Theming Findings

> **STATUS: RESOLVED.** Findings addressed by the Typst PDF migration (PRD §Typst migration, slice 7 / commit 7139fff). Retained for historical reference. Path citations in this document refer to the fpdf-era rendering stack that was retired; the equivalent code now lives in `pkg/render/`. Findings about hard-coded RGB triplets, alpha-variant rgba, and correction values are now obsolete because the Typst templates drive styling through `templates/common/theme.typ` and `templates/common/record_card.typ`. See `docs/PRD.md` for the live state.

Read-only audit. No source was modified.

**Scope.** Every literal that defines visual output across the PDF rendering stack (`internal/archive/pdf_layout.go`, `internal/archive/export_service.go`), the templ view layer (`internal/templates/*.templ`), the static-archive browser viewer (`internal/archive/static_archive.go`), the ViewModel adapter (`internal/presentation/views.go`), and the Tailwind config (`tailwind.config.js`).

**Terminology.** Person Record, Source Record, Soldier, Display ID, Local Archive, Shared Archive, Backup Archive, Static Archive — as defined in `CONTEXT.md`.

---

## 1. Structural Constraints Register

### 1.1 Page margins

| File | Line | Literal | Classification |
|---|---|---|---|
| export_service.go | 1090 | `pdf.SetMargins(16, 28, 16)` — `newPDFDocument` | **Hard-coded literal** — armed on every exported PDF |
| pdf_layout.go | 683–684 | `defer pdf.SetMargins(leftMargin, topMargin, rightMargin)` then `pdf.SetMargins(x, topMargin, pageWidth-(x+width))` — `writePDFRichTextColumnSection` | **Structural** — narrows page for the narrative column |
| pdf_layout.go | 702–703 | same pattern — `writePDFRecordsColumnSection` | **Structural** — narrows page for the records column |

Notes:

- `newPDFDocument` only ever sets margins to `16/28/16` mm. `fpdf.SetMargins` has no bottom argument; fpdf's default bottom is `10` mm. The codebase never sets an explicit bottom margin.
- `pdf.SetAutoPageBreak(true, 20)` is called once at `export_service.go:1091` but is immediately overridden by `writePDFRecordCard` / `writePDFRecordCardMultiPage`, which re-set the auto-break margin to whatever `pdf.GetMargins()` currently reports for `bottomMargin` (i.e. fpdf's default `10`, not the literal `20`). See §4.2.

### 1.2 Page size, orientation, document constructor

| File | Line | Literal | Classification |
|---|---|---|---|
| export_service.go | 1085 | `fpdf.New(orientation, "mm", "Letter", "")` — `newPDFDocument` | **Hard-coded** — paper size and unit are baked in |
| PDFOptions.Orientation | pdf_layout.go:42 | `json:"orientation"` | **Per-export configurable** — `"P"` or `"L"` after `Normalize("L", true)` |

Only one `fpdf.New(...)` call exists in the entire module. The `size` argument is a string literal `"Letter"`. A future A4 path requires editing this one line plus the JSON-side field.

### 1.3 Line spacing

PDF cell heights are baked into `CellFormat` / `MultiCell` literals or pulled from the `pdfRecordCardLayout` struct defaults. The only per-call literal heights that escape the struct are:

| File | Line | Literal | Role |
|---|---|---|---|
| pdf_layout.go | 236 | `CellFormat(0, 8, ...)` | section title cell |
| pdf_layout.go | 244 | `CellFormat(0, 10, ...)` | title block |
| pdf_layout.go | 248 | `MultiCell(0, 6, ...)` | title-block subtitle |
| pdf_layout.go | 404–407 | `28` and `44` mm clamp on `labelWidth` | compact field section estimator |
| pdf_layout.go | 414 | `* 1.18` | rich-text section wrap multiplier |
| pdf_layout.go | 430 | `* 1.18` | records section wrap multiplier |
| pdf_layout.go | 457, 461, 465 | `7`, `11`, `6` | group-divider page cells |
| pdf_layout.go | 619–622 | `28`, `44`, `width - labelWidth - 3` (3 mm gap) | compact field section render |
| pdf_layout.go | 631 | `x + labelWidth + 3` (3 mm gap) | compact field render |
| pdf_layout.go | 646, 660, 664 | `x+2`, `panelY+2`, `width-4`, `panelHeight-14`, `panelY+panelHeight-10` (mm insets) | image panel |
| pdf_layout.go | 878 | `writePDFRichTextSized(pdf, ..., 6, 11)` | biography page body — line height `6`, font size `11` |
| pdf_layout.go | 986, 993, 994 | `34`, `6` mm (width/height) | `writePDFInlineField` |
| pdf_layout.go | 1008, 1011 | `34`, `8` mm | `writePDFField` |
| pdf_layout.go | 1020, 1021 | `6`, `7` mm | `writePDFBullet` (unsized) |
| pdf_layout.go | 1125–1128 | `thumbnailWidth = 34.0`, `thumbnailHeight = 22.0`, `rowHeight = 36.0`, `pageHeight-16` | `writePDFImageRow` (pageHeight guard uses `16` not the document bottom margin) |
| pdf_layout.go | 1152 | `MultiCell(0, 6, ...)` | image-row title |
| pdf_layout.go | 1161 | `pdf.GetY() > 230` | registry entry page guard (literal `230` mm) |
| pdf_layout.go | 1168, 1171 | `7`, `5` | registry entry cells |
| pdf_layout.go | 1178 | `pdf.Ln(2)` | registry entry tail gap |
| export_service.go | 1105, 1110, 1111, 1117, 1120, 1127 | `10`, `17`, `3`, `-11`, `1`, `4` | header/footer callback Y positions and gaps |
| export_service.go | 1064, 1077 | `pdf.Ln(2)` | analytics separators |

CSS line-heights are essentially absent from the templ layer. The only raw `line-height` declaration is `layout.templ:207` (`line-height: 1.15;` for nav links and dock button). Every other line-height flows through Tailwind `leading-*` utilities (`leading-relaxed`, `leading-none`, `leading-6`, `leading-5`).

### 1.4 Borders and dividers

PDF draw calls are limited. The full set:

| File | Line | Literal | Role |
|---|---|---|---|
| pdf_layout.go | 1164 | `pdf.Line(16, pdf.GetY(), 200, pdf.GetY())` | registry horizontal rule (x1=`16`, x2=`200`) |
| pdf_layout.go | 1135 | `pdf.Rect(x, y, 34.0, 22.0, "D")` | image-row thumbnail stroke |
| export_service.go | 1110 | `pdf.Line(leftMargin, 17, pageWidth-rightMargin, 17)` | header underline |
| export_service.go | 1119 | `pdf.Line(leftMargin, pdf.GetY(), pageWidth-rightMargin, pdf.GetY())` | footer rule |

No `pdf.SetLineWidth` call exists anywhere — all rules use fpdf's default `0.2` mm. No `pdf.SetFillColor` call exists anywhere.

CSS borders concentrate in `layout.templ:38–48, 66, 71, 76, 88, 130, 140, 144, 157, 171` (raw `border:` / `border-color:` declarations) and in thousands of Tailwind `border-*` utility occurrences across every `.templ` file.

### 1.5 Grid columns

| File | Line | Literal | Role |
|---|---|---|---|
| pdf_layout.go | 256 | `LeftWidthRatio float64` (struct field) | record-card left-column width as a fraction of content width |
| pdf_layout.go | 263 | `ColumnGap float64` (struct field) | gap between left and right columns |
| pdf_layout.go | 275 | default `LeftWidthRatio: 0.52` | default two-column split |
| pdf_layout.go | 319 | `base.LeftWidthRatio = 0.6` | portrait-compact override |
| pdf_layout.go | 321 | `base.LeftWidthRatio = 0.43` | no-images override |
| pdf_layout.go | 282 | default `ColumnGap: 8` | default 8 mm gap |
| pdf_layout.go | 283 | default `SectionGap: 4` | default 4 mm section gap |

The only PDF grid spans the two-column record card. All other PDF layouts are full-width (biography page, group divider page, registry page, image row, analytics summary).

CSS grid definitions:

- `layout.templ:231` — `.responsive-two-col { grid-template-columns: repeat(2, minmax(0, 1fr)); }`
- `layout.templ:235` — `.responsive-span-2 { grid-column: span 2 / span 2; }`
- `layout.templ:239` — `.calendar-layout { grid-template-columns: minmax(0, 1fr) 390px; }`
- `static_archive.go:284` — `.record-row { display: grid; gap: 14px; grid-template-columns: minmax(0, 1fr) auto; }`
- `static_archive.go:369` — `.detail-grid { display: grid; grid-template-columns: auto 1fr; gap: 10px 12px; }`
- `static_archive.go:441` — `.detail-grid.compact` (reduced gap/font-size)
- `static_archive.go:463` — `.detail-layout { display: grid; gap: 18px; grid-template-columns: minmax(0, 1.15fr) minmax(280px, 0.85fr); }`

No `.record-card`, `.field-grid`, or `.columns-*` class exists. The two-column record layout on the web is a Tailwind utility soup, not a reusable class.

### 1.6 Classification summary

| Class | Count | Examples |
|---|---|---|
| **Hard-coded literal** | ~25 | `SetMargins(16, 28, 16)`, `fpdf.New(..., "Letter", ...)`, `[]float64{1, 0.94, 0.88, 0.82, 0.76}`, all `pdf.SetTextColor` / `pdf.SetDrawColor` RGB triplets, the `6`/`11` biography page line-height/font-size, the `28/44` mm label-width clamp, the `thumbnailWidth = 34.0` image constants |
| **Default-only** | 15 (struct fields) | `LeftWidthRatio: 0.52`, `SectionTitleFontSize: 9`, `FieldLabelFontSize: 8`, `FieldValueFontSize: 9`, `FieldLineHeight: 4.5`, `FieldRowGap: 1`, `ColumnGap: 8`, `SectionGap: 4`, `ImagePanelHeight: 64`, `ImageLabelFontSize: 8`, `ImageLabelLine: 4`, `BodyFontSize: 9`, `BodyLineHeight: 5`, `BulletIndent: 6`, `SectionTitleLine: 6` — all in `defaultPDFRecordCardLayout` (lines 273–293) |
| **Per-export configurable** | 4 | `PDFOptions.Orientation`, `PDFOptions.PrinterFriendly`, `PDFOptions.IncludeImages`, `PDFOptions.PrintableArchive` (all in `pdf_layout.go:41–45`) |

The only two tokens a user can change without recompiling are orientation and printer-friendly mode. Everything else is in the source.

---

## 2. Multi-page / Overflow / Orphan / Widow Register

### 2.1 Multi-page strategy map

`writePDFRecordCard` (`pdf_layout.go:468–530`) — single-page entry point.

- Captures `pageWidth, pageHeight, leftMargin, topMargin, rightMargin, bottomMargin` (lines 470–476).
- Calls `choosePDFRecordCardLayout` (line 477) to get a layout and a `fitsSinglePage` boolean.
- If `!fitsSinglePage` → `writePDFRecordCardMultiPage(...)` and return (lines 478–481).
- Otherwise renders two columns (left/right) on a single page (lines 488–529), having suspended auto page break for the render (line 486, deferred restore on line 485).

`writePDFRecordCardMultiPage` (`pdf_layout.go:532–588`) — multi-page fallback.

- Renders full-content-width sections in fixed order.
- Each section is preceded by a `preparePDFRecordCardSection` overflow guard.
- Sections, in order: Identity (no guard), Service, Household, optional Image Panel, narrative (`writePDFRichTextColumnSection`), Records (`writePDFRecordsColumnSection`).
- Enables auto page break for the render (line 539, deferred restore on line 538).

`choosePDFRecordCardLayout` (`pdf_layout.go:313–333`) — layout picker.

- Applies `base.LeftWidthRatio` override for portrait-compact (`0.6`) or no-images (`0.43`).
- If portrait and not portrait-compact → returns `base.scaled(0.76), false` (forces multi-page).
- Otherwise iterates the shrink sequence `[]float64{1, 0.94, 0.88, 0.82, 0.76}` (line 327) and returns the first scale whose `estimatePDFRecordCardHeight` fits the available height.
- Falls through to `base.scaled(0.76), false` if no scale fits.

`pdfRecordCardLayout.scaled` (`pdf_layout.go:295–312`) — applies per-field `maxFloat(...)` floors so a scale of `0.76` cannot collapse below minimum readable dimensions.

`preparePDFRecordCardSection` (`pdf_layout.go:590–601`) — the overflow guard.

```go
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
```

Callers and their `minHeight` floors (each includes the section-title line + one body line):

| Line | Section | `minHeight` |
|---|---|---|
| 547–551 | Identity & Vital Details | **none — first section, no guard** |
| 555 | Service & Archive Details | `layout.SectionTitleLine + layout.FieldLineHeight` |
| 562 | Household & Context | `layout.SectionTitleLine + layout.FieldLineHeight` |
| 571 | Image Panel | `estimatePDFImagePanelHeight(layout, hasTitle)` |
| 580 | Biography (narrative) | `layout.SectionTitleLine + layout.BodyLineHeight` |
| 586 | Records | `layout.SectionTitleLine + layout.BodyLineHeight` |

### 2.2 Estimate family

| Function | Lines | Measures |
|---|---|---|
| `estimatePDFRecordCardHeight` | 335–388 | total card height = max(left column sum, right column sum) |
| `estimatePDFCompactFieldSectionHeight` | 390–412 | one compact field section (label + value) — clamps `labelWidth` to `[28, 44]` mm |
| `estimatePDFRichTextSectionHeight` | 414–416 | `SectionTitleLine + wrappedMultilineCount * BodyLineHeight * 1.18` |
| `estimatePDFRecordsSectionHeight` | 418–431 | records section, type+id line + details per row, with `* 1.18` wrap multiplier |
| `estimatePDFImagePanelHeight` | 669–675 | `ImagePanelHeight + 2`, plus `SectionTitleLine` if titled |
| `wrappedPDFLineCount` | 433–441 | fpdf `SplitText` based |
| `wrappedPDFMultilineCount` | 442–451 | fpdf `SplitText` based, multiline aware |

### 2.3 Orphan / widow / keep-together — finding

- **Page break before section title:** Yes, at the section level only. `preparePDFRecordCardSection` accepts a `minHeight` that always includes the section title line + at least one body line, so a section title is never placed at the bottom of a page unless its title and one body line can also fit.
- **Section title + first field together:** Yes, via the same mechanism. The first field is not specifically measured; the floor is title-line + one body-line height.
- **Label + value pair on the same page:** Partially. `writePDFCompactFieldSection` (`pdf_layout.go:603–642`) starts both `MultiCell`s at `rowTop` (line 622 captures the row top, lines 625 and 633 both render from it), so the label and value START together. There is **no** pre-row `if pdf.GetY() > pageHeight - bottomMargin { AddPage }` test, so the pair can be split by fpdf's auto page break mid-row when a label or value wraps to many lines.
- **First section (Identity) keep-together:** No. Lines 547–551 draw Identity from `startY = pdf.GetY()` without an overflow guard. If `startY` is too low, Identity will be drawn against the auto-break margin only.
- **Biography block keep-together:** No. `writeFullBiographyPage` (`pdf_layout.go:866–879`) calls `writePDFRichTextSized` with a single text stream and relies on fpdf's auto page break to split it. There is no minimum-line-together check.
- **Hard-coded bottom thresholds that are not tied to the document's bottom margin:**
  - `writePDFImageRow` line 1128: `pageHeight-16` (16 mm hard-coded).
  - `writePDFRegistryEntry` line 1160: `pdf.GetY() > 230` (230 mm hard-coded).

### 2.4 Biography page flow

- `shouldAppendSingleRecordBiographyPage` (`pdf_layout.go:830–836`) — returns true when not `PrintableArchive`, not portrait, and biography has non-empty free text.
- `writeSingleRecordBiographyPage` (`pdf_layout.go:862–864`) — one-liner that calls `writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography")`.
- `writePrintableBiographyAppendixPage` (`pdf_layout.go:858–860`) — one-liner that calls `writeFullBiographyPage(pdf, soldier, printerFriendly, "Full Biography Appendix")`.
- `writeFullBiographyPage` (`pdf_layout.go:866–879`) — does **not** construct a `pdfRecordCardLayout`. Uses its own `writePDFTitleBlock` (line 871), `writePDFSection(pdf, "Biography")` (line 876), and `writePDFRichTextSized(pdf, ..., 6, 11)` (line 878). The `6` and `11` are **hard-coded** — they bypass the layout struct entirely.

### 2.5 Static-archive HTML — the browser counterpart

`internal/archive/static_archive.go` (1528 lines) embeds a single 490-line `<style>` block (lines 122–612) inside the `staticArchiveIndexHTML` template constant. It uses CSS custom properties for token reuse:

```css
:root {
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
```

This is the **only** place in the codebase that has any tokenization at all. The tokens are used only within `static_archive.go`; they do not propagate to the live `layout.templ` and not to the PDF. Every other visual literal in the project is a duplicated RGB triplet or hex string.

The HTML wraps everything in `<div class="shell">` (line 614) with `max-width: 1280px; margin: 0 auto; padding: 0 20px 32px;` (line 152). There is no `@page` rule, no `.page` class, no `.sheet` class, no `@media print` block. Two screen views (`.screen.list-screen` and `.screen.detail-screen`) stand in for "pages". Body background (lines 143–149) is a four-layer gradient that matches `layout.templ:27–31` byte-for-byte — a literal duplication of the live app's body background.

---

## 3. Color Palette Register

### 3.1 PDF RGB triplets (`pdf.SetTextColor` / `pdf.SetDrawColor`)

| RGB | Semantic role | Count | Callsites |
|---|---|---|---|
| `(141, 116, 64)` | Gold/bronze — section titles, headers, accent | 8 | pdf_layout.go:235, 456, 609, 649, 687, 706 (text); 1163 (rule); export_service.go:1109, 1118 (header/footer rules) |
| `(34, 48, 61)` | Dark navy — primary body text | 9 | pdf_layout.go:237, 243, 460, 636, 691, 985, 1007, 1167; export_service.go:1107 |
| `(68, 82, 96)` | Slate — secondary text (labels, subtitles) | 8 | pdf_layout.go:247, 464, 627, 662, 992, 1010, 1170; export_service.go:1122 |
| `(48, 87, 122)` | Link blue | 1 | pdf_layout.go:1066 (`writePDFRichTextSized` link segments) |
| `(0, 0, 0)` | Black — link-segment color reset | 1 | pdf_layout.go:1068 |

The only RGB triplet that appears as both text color and draw color is `(141, 116, 64)`. There is no fill-color use; no `pdf.SetFillColor` call exists.

### 3.2 CSS hex literals — palette map

Cross-referenced against the PDF triplets, the hexes used in `*.templ` resolve to the same palette:

| Token (proposed) | PDF triplet | Hex (CSS) | Used in PDF | Used in templ/static-archive |
|---|---|---|---|---|
| `accent` | (141, 116, 64) | `#8d7440` | section titles, rules, header/footer | `browse.templ`, `calendar.templ`, `camaraderie.templ`, `conflict_ledger.templ`, `entry_form.templ`, `insights.templ`, `layout.templ`, `recovery.templ`, `research_collections.templ`, `research_log.templ`, `research_pack.templ`, `review_queue.templ`, `share.templ`, `soldier_card.templ`, `timeline.templ`, `calendar_day.templ`, `static_archive.go` (as `--gold-dark`) |
| `accent_strong` | (168, 138, 70) | `#a88a46` | not used | `layout.templ:36` (`.gold`), `static_archive.go:128` (as `--gold`) |
| `text_primary` | (34, 48, 61) | `#22303d` | body text, titles | every `.templ` file plus `static_archive.go:130` (as `--ink`) |
| `text_secondary` | (68, 82, 96) | `#445260` | field labels, subtitles | `calendar.templ`, `calendar_day.templ`, `static_archive.go:131` (as `--muted`) |
| `text_muted` | (113, 128, 142) | `#71808e` | not used (closest `(68, 82, 96)` is used) | `calendar.templ:27,35,43` only |
| `link` | (48, 87, 122) | `#30577a` | rich-text link segments | not used as a hex in any `.templ` file |
| `danger` | (84, 33, 29) | `#54211d` | not used (closest `(111, 44, 38)` is the actual hex) | `layout.templ:76` (`.danger-button` border) |
| `danger_alt` (no PDF equivalent) | — | `#6f2c26` | — | "needs review" pill across `browse.templ`, `calendar_day.templ`, `conflict_ledger.templ`, `entry_form.templ`, `review_queue.templ`, `share.templ`, `soldier_card.templ`, `timeline.templ` |
| `panel_fill` | — | `#fff8e7` | not used | `layout.templ:76,87,142`, `static_archive.go:125,127,455` (as `--panel-strong`) |
| `panel_fill_alt` | — | `#f2ede1` | not used | `layout.templ`, `calendar.templ` |
| `success_text` | — | `#1f5b3b` | not used | `calendar.templ` (today badge) |
| `success_text_alt` | — | `#29522d` / `#78a373` | not used | `recovery.templ`, `entry_form.templ` (success state) |

The "danger" color in PDF is the navy `(34, 48, 61)` for body text; there is no PDF red. The web "danger" hex `#6f2c26` and `#7b1e2b` have **no PDF equivalent at all** — danger-styled UI elements (review queue pills, conflict ledger, scrape errors) do not appear in any PDF output path.

### 3.3 CSS rgba token transparency ladder (sample)

The same RGB triplet appears at many different alpha levels. Distinct `rgba(141,116,64, *)` forms used:

`0.14`, `0.16`, `0.18`, `0.2`, `0.22`, `0.24`, `0.25`, `0.26`, `0.28`, `0.3`, `0.32`, `0.34`, `0.35`, `0.38`, `0.4`, `0.45`, `0.48`, `0.55`, `0.7`, `0.78`, `0.82`, `0.85`, `0.86`, `0.88`, `0.92`, `0.97`

This is the strongest signal of accidental theme drift: a single accent color is being copy-pasted at 27 different alpha levels, with no semantic mapping between the level and the role (border vs. fill vs. hover vs. focus).

### 3.4 Font families and sizes

PDF font families used: `Helvetica` (regular, bold, italic) and `Times` (bold, only in `writePDFTitleBlock` line 242 and `writePDFGroupDividerPage` line 459 and `writePDFRegistryEntry` line 1166). No other font family is referenced.

PDF font sizes used: 4, 5, 6, 7, 8, 9, 10, 11, 13, 20, plus the dynamic `28 - level*2` in group dividers. Most of these are pulled from `pdfRecordCardLayout` struct fields (9, 8, 4.5, 5, 6, 4); the raw `7, 8, 10, 11, 13, 20` literals are escape hatches in section title, group divider, header/footer, and biography-page code.

CSS font families: only two declarations. `body` uses `"Helvetica Neue", Arial, sans-serif` (`layout.templ:33`); `.gold` uses `Georgia, "Times New Roman", serif` (`layout.templ:36`). Every other templ file inherits `body`'s family via cascade.

CSS font sizes: `text-xs` (very common), `text-sm`, `text-base`, `text-lg`, `text-xl`, `text-2xl`, `text-3xl`, `text-4xl`, plus arbitrary bracket values (`text-[0.65rem]`, `text-[0.68rem]`, `text-[0.76rem]`, `text-[0.78rem]`, `text-[0.95rem]`, `text-[1.5rem]`). The Tailwind config defines no custom font sizes (`theme.extend: {}`).

### 3.5 Header and footer branding

- PDF header (`export_service.go:1102–1112`): `pdf.SetHeaderFuncMode` callback. Calls `pdfBranding()` to get `branding.ArchiveTitle`, sets font Helvetica B 10, text color `(34, 48, 61)`, draws a `(141, 116, 64)` horizontal rule at Y=17, then a `pdf.Ln(3)`. Applied to the first page too.
- PDF footer (`export_service.go:1114–1129`): `pdf.SetFooterFunc` callback, **only when `!printerFriendly`** (line 1113). Sets Y=-11, draws a `(141, 116, 64)` rule, then renders `branding.FooterText` in Helvetica size 8, color `(68, 82, 96)`, centered.
- `pdfBranding` struct (`pdf_layout.go:220–223`): two fields, `ArchiveTitle string` and `FooterText string`.
- `pdfBranding()` method (`export_service.go:1133–1146`): reads `e.db.UserIdentity()`, returns `pdfBranding{ ArchiveTitle: owner + "'s Civil War Research Archive", FooterText: "Made with DixieData | Version: " + buildinfo.AppVersion + " | Build: " + buildinfo.BuildIdentity() }`. The `'s Civil War Research Archive` suffix and `Made with DixieData` prefix are hard-coded string literals in this method.

The HTML header/footer analogues requested in the audit (`.app-header`, `.app-footer`, `.branding-*`) **do not exist**. The closest equivalents are:

- `layout.templ:415` — `<header class="top-shell flex flex-wrap items-center justify-between gap-3 rounded-[1.7rem] border border-[#8d7440] bg-[rgba(36,48,61,0.92)] …">` (the live-app top header).
- `layout.templ:509` — `<footer class="mt-8 text-right text-xs text-slate-600" data-build-identity=…>` (the live-app footer).
- `static_archive.go:656` — raw `<footer>` element styled at line 490 with `font-size: 0.88rem; border-top: 1px solid rgba(141, 116, 64, 0.18);`.

The `data-build-identity` attribute on the live footer is the only place a build identity token appears in the templ layer; `buildinfo.AppVersion` and `buildinfo.BuildIdentity()` are read by `pdfBranding()` only.

---

## 4. Change-Cost Matrix

For each thematic element, can a user change it without recompiling? Per export type? Globally? Per archive? What's the edit footprint?

| Element | Per export type | Globally | Per archive | Edit cost |
|---|---|---|---|---|
| PDF paper size | no (baked in `fpdf.New`) | no | no | **structural** — change `pdf_layout.go:1085` |
| PDF page margins | no (`SetMargins(16, 28, 16)` baked) | no | no | **structural** — change `export_service.go:1090` |
| PDF auto-break margin | no | no | no | **trivial** — change `export_service.go:1091` (but it's already overridden downstream) |
| PDF orientation | yes (`PDFOptions.Orientation`) | yes | yes | **trivial** — already configurable |
| PDF printer-friendly mode | yes (`PDFOptions.PrinterFriendly`) | yes | yes | **trivial** — already configurable |
| Record-card column ratio (left width) | no (default only, 2 hard-coded overrides) | no | no | **cross-file** — edit `pdf_layout.go:275,319,321` |
| Column gap, section gap, field row gap, image panel height, all font sizes, all line heights | no (struct defaults only) | no | no | **cross-file** — edit `pdf_layout.go:275–291` |
| Shrink-loop scale sequence `{1, 0.94, 0.88, 0.82, 0.76}` | no | no | no | **trivial** — `pdf_layout.go:327` |
| Minimum readable scale `0.76` | no | no | no | **trivial** — `pdf_layout.go:271` |
| Image-row thumbnail size `34.0 × 22.0` and row height `36.0` | no | no | no | **local** — `pdf_layout.go:1125–1127` |
| Registry entry page-break Y literal `230` | no | no | no | **local** — `pdf_layout.go:1161` |
| Image-row bottom guard `pageHeight-16` | no | no | no | **local** — `pdf_layout.go:1128` |
| Biography page line height `6` and font size `11` | no | no | no | **local** — `pdf_layout.go:878` |
| Header title color (PDF triplet `(34, 48, 61)`) | no | no | no | **cross-file** — every `SetTextColor(34, 48, 61)` call |
| Section-title color (PDF triplet `(141, 116, 64)`) | no | no | no | **cross-file** — 6 `SetTextColor(141, 116, 64)` calls plus 3 `SetDrawColor(141, 116, 64)` rules |
| Field label color (PDF triplet `(68, 82, 96)`) | no | no | no | **cross-file** — 7 `SetTextColor(68, 82, 96)` calls |
| Link blue `(48, 87, 122)` | no | no | no | **local** — single call at `pdf_layout.go:1066` |
| Header font size (PDF) `10` | no | no | no | **local** — `export_service.go:1106` |
| Header text (`<owner>'s Civil War Research Archive`) | no | yes (via `UserIdentity.BrandingName`) | yes (per user identity) | **trivial** for owner; **local** for the suffix string at `export_service.go:1143` |
| Footer text (`Made with DixieData | Version: … | Build: …`) | no | yes (AppVersion/BuildIdentity) | yes (per build) | **local** — `export_service.go:1144` |
| `body` font family in templ | no | no | no | **local** — `layout.templ:33` |
| `.gold` font family (Georgia) in templ | no | no | no | **local** — `layout.templ:36` |
| CSS color tokens (`#8d7440`, `#22303d`, `#445260`, etc.) | no | no | no | **cross-layer** — every `.templ` file plus `static_archive.go` |
| Body background (4-layer gradient) | no | no | no | **cross-file** — `layout.templ:27–31` AND `static_archive.go:143–149` (literal duplication) |
| `.record-card` / `.field-grid` / `.columns-*` classes | n/a | n/a | n/a | n/a — **do not exist** |
| `.app-header`, `.app-footer`, `.branding-*` classes | n/a | n/a | n/a | n/a — **do not exist**; live header is `.top-shell`, live footer is raw `<footer>` |

The single most expensive change is the **PDF triplet palette**: changing the accent color from `(141, 116, 64)` to anything else requires editing 9 callsites across 2 files. Doing the same on the web requires editing 27 alpha-level rgba variants of `rgba(141, 116, 64, …)` across every `.templ` file plus the `--gold-dark` token in `static_archive.go`.

---

## 5. Per-export-type Theme Map

The codebase has four export paths. Each inherits a different mix of the literals above.

| Export path | Drives | Page geometry source | Color source | Font source | Header/footer source |
|---|---|---|---|---|---|
| **PDF bulk export** (`ExportFullDatabasePDF`, `export_service.go:999–1050`) | `pdfRecordCardLayout` (full set) + `pdfBranding` + `PDFOptions.Orientation` + `PDFOptions.PrinterFriendly` + `PDFOptions.IncludeImages` | `newPDFDocument` → `fpdf.New(..., "Letter", ...)` + `SetMargins(16, 28, 16)` | literal RGB triplets at every `SetTextColor` / `SetDrawColor` callsite | literal `SetFont("Helvetica", ..., ...)` and `SetFont("Times", "B", ...)` calls | `brandedPDFDocument` → `pdfBranding()` → `UserIdentity.BrandingName` + buildinfo |
| **PDF single-Soldier export** (`exportSoldierPDF`, `export_service.go:903`) | same as bulk, plus optional biography page (`writeSingleRecordBiographyPage` at `pdf_layout.go:862`) | same | same | same | same |
| **Printable biography appendix** (called from single-Soldier export) | `writeFullBiographyPage` with label `"Full Biography Appendix"` (`pdf_layout.go:858–860`) | new page via `pdf.AddPage()` at `pdf_layout.go:870` | inherits all prior colors; **adds no new ones** | `Times B 20` title, `Helvetica 10` subtitle, `Helvetica B 9` section, `Helvetica 11` body — **bypasses `pdfRecordCardLayout`** with literal `6`/`11` at `pdf_layout.go:878` | inherits `brandedPDFDocument` header/footer |
| **Static Archive HTML** (`staticArchiveIndexHTML`, `static_archive.go`) | none — pure HTML+CSS, no domain-driven theming | `.shell` `max-width: 1280px`; no `@page` rule | **the only tokenized place**: 9 CSS custom properties on `:root` (`--paper`, `--panel`, `--panel-strong`, `--panel-dark`, `--border`, `--gold`, `--gold-dark`, `--ink`, `--muted`, `--shadow`) | `body { font-family: "Helvetica Neue", Arial, sans-serif; }`; `.hero h1` and `.panel-head h2` use `Georgia, "Times New Roman", serif` | raw `<footer>` element with `border-top: 1px solid rgba(141, 116, 64, 0.18)`; no header branding text on archive page other than `<h1>{{ .ArchiveTitle }}</h1>` (the static archive's title comes from the export metadata, not `UserIdentity`) |

**Drift summary:**

- The PDF and templ layers each carry their own copy of the same gold/cream/navy palette, but the PDF uses 5 RGB triplets and the templ layer uses 28+ hex values plus 60+ rgba variants.
- `static_archive.go` has *internal* tokenization (CSS custom properties) but the tokens resolve to the same hex/rgba values that appear literally in every other `.templ` file.
- `layout.templ` and `static_archive.go` duplicate the body background gradient byte-for-byte (`layout.templ:27–31` and `static_archive.go:143–149`).
- The biography page does not consume `pdfRecordCardLayout`; it uses its own hard-coded font/line metrics.
- The "danger" red is web-only; no PDF output path uses it.
- The ViewModel layer (`internal/presentation/views.go`) carries no theme fields. `views.go` is a 165-line adapter file that wraps domain types into ViewModel types; the ViewModel structs themselves (in `internal/viewmodel/`, not in scope) similarly carry no theme/style/color/font fields.

---

## 6. Theming seams that do not exist

- No `.record-card` class. The two-column record layout is a `pdfRecordCardLayout` struct in Go and a Tailwind utility soup in templ.
- No `.field-grid` or `.columns-*` classes. The two-column responsive layout uses `.responsive-two-col` and per-template `grid-template-columns: minmax(0, 320px) minmax(0, 1fr)` literals.
- No `.app-header`, `.app-footer`, or `.branding-*` classes.
- No `@media print` block. The static archive is not designed to print.
- No `.ribbon` class.
- No viewmodel-side theme tokens. The ViewModel layer is theme-agnostic.
- No Tailwind theme extension. `tailwind.config.js` has `theme.extend: {}`.

The only place any tokenization exists is `static_archive.go`'s `:root { --paper: ...; --gold: ...; ... }` block, and that tokenization is local to that one file.
