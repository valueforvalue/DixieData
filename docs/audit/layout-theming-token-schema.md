# Token Schema Proposal

This is the typed configuration shape for layout and theming. It is **proposed only** — no Go code is written during the audit. Every field below is traceable to a finding in `layout-theming-findings.md`.

---

## 1. Schema file shape

`theme.json` lives alongside the template files in the program directory. It is loaded once at document-init time by a `theme.Loader` and resolved into a `theme.Theme` value that both the PDF and templ layers consume.

```json
{
  "schema_version": "1.0.0",
  "page": {
    "paper_size": "letter",
    "orientation": "landscape",
    "margins": {
      "top":    { "value": 0.75, "unit": "in" },
      "bottom": { "value": 0.75, "unit": "in" },
      "left":   { "value": 0.75, "unit": "in" },
      "right":  { "value": 0.75, "unit": "in" }
    },
    "header_offset": { "value": 0.4, "unit": "in" },
    "footer_offset": { "value": 0.4, "unit": "in" },
    "auto_break_margin": { "value": 20, "unit": "mm" }
  },
  "record_card": {
    "left_column_ratio": 0.52,
    "column_gap":        { "value": 8,  "unit": "mm" },
    "section_gap":       { "value": 4,  "unit": "mm" },
    "field_row_gap":     { "value": 1,  "unit": "mm" },
    "label_width_clamp": { "min": { "value": 28, "unit": "mm" }, "max": { "value": 44, "unit": "mm" } }
  },
  "shrink": {
    "sequence": [1.0, 0.94, 0.88, 0.82, 0.76],
    "min_readable_scale": 0.76
  },
  "type_scale": {
    "primary_font":   "Helvetica",
    "display_font":   "Times",
    "monospace_font": "Courier",
    "scale": 1.0,
    "section_title":   { "size": { "value": 9, "unit": "pt" }, "line": { "value": 6,  "unit": "pt" } },
    "field_label":     { "size": { "value": 8, "unit": "pt" }, "line": { "value": 4.5,"unit": "pt" } },
    "field_value":     { "size": { "value": 9, "unit": "pt" }, "line": { "value": 4.5,"unit": "pt" } },
    "body":            { "size": { "value": 9, "unit": "pt" }, "line": { "value": 5,  "unit": "pt" } },
    "image_label":     { "size": { "value": 8, "unit": "pt" }, "line": { "value": 4,  "unit": "pt" } },
    "image_thumbnail": { "width": { "value": 34, "unit": "mm" }, "height": { "value": 22, "unit": "mm" } },
    "image_row":       { "height": { "value": 36, "unit": "mm" } },
    "bullet_indent":   { "value": 6, "unit": "mm" },
    "image_panel":     { "height": { "value": 64, "unit": "mm" } },
    "biography":       { "size": { "value": 11, "unit": "pt" }, "line": { "value": 6, "unit": "pt" } },
    "header":          { "size": { "value": 10, "unit": "pt" } },
    "footer":          { "size": { "value": 8,  "unit": "pt" } }
  },
  "wrap": {
    "rich_text_multiplier": 1.18,
    "records_multiplier":   1.18
  },
  "overflow": {
    "image_row_bottom_guard": { "value": 16, "unit": "mm" },
    "registry_break_y":       { "value": 230, "unit": "mm" }
  },
  "palette": {
    "accent":         { "rgb": [141, 116, 64],  "hex": "#8d7440" },
    "accent_strong":  { "rgb": [168, 138, 70],  "hex": "#a88a46" },
    "text_primary":   { "rgb": [34,  48,  61],  "hex": "#22303d" },
    "text_secondary": { "rgb": [68,  82,  96],  "hex": "#445260" },
    "text_muted":     { "rgb": [113, 128, 142], "hex": "#71808e" },
    "link":           { "rgb": [48,  87,  122], "hex": "#30577a" },
    "danger":         { "rgb": [84,  33,  29],  "hex": "#54211d" },
    "divider":        { "rgb": [141, 116, 64],  "hex": "#8d7440" },
    "panel_fill":     { "hex": "#fff8e7" }
  },
  "borders": {
    "divider_weight":       { "value": 0.2, "unit": "mm" },
    "panel_radius":         { "value": 0.8, "unit": "rem" },
    "panel_border_weight":  { "value": 1,   "unit": "px" }
  },
  "branding": {
    "header": { "from": "user_identity.branding_name", "suffix": "'s Civil War Research Archive" },
    "footer": { "template": "Made with DixieData | Version: {app_version} | Build: {build_identity}" }
  },
  "overrides": {
    "printer_friendly": {
      "palette": {
        "text_primary":   { "rgb": [0, 0, 0], "hex": "#000000" },
        "text_secondary": { "rgb": [0, 0, 0], "hex": "#000000" }
      },
      "branding": { "footer": null }
    },
    "portrait_compact": {
      "record_card": { "left_column_ratio": 0.6 }
    },
    "no_images": {
      "record_card": { "left_column_ratio": 0.43 }
    },
    "biography_appendix": {
      "type_scale": {
        "biography":   { "size": { "value": 10, "unit": "pt" }, "line": { "value": 6, "unit": "pt" } },
        "section_title": { "size": { "value": 10, "unit": "pt" }, "line": { "value": 7, "unit": "pt" } }
      }
    }
  }
}
```

---

## 2. Field-level documentation

Every field, with trace to a finding in the audit, units, defaults, and semantic role.

### 2.1 `schema_version`

- **Purpose.** Identifies the schema revision. Future revisions branch on this.
- **Type.** `string`, semver.
- **Default.** `"1.0.0"`.
- **Required.** Yes.
- **Validation.** `loader` rejects values it doesn't recognize.

### 2.2 `page`

- **Trace.** `export_service.go:1085` (`fpdf.New(..., "Letter", "")`), `export_service.go:1090` (`SetMargins(16, 28, 16)`), `export_service.go:1091` (`SetAutoPageBreak(true, 20)`).
- **Role.** Document-wide geometry. Read once at document-init time; never modified thereafter.

#### `page.paper_size`

- **Purpose.** Paper size for the fpdf document.
- **Type.** `string` (`"letter"`, `"a4"`, `"legal"`, `"a3"`) **or** `{ "width": { "value": ..., "unit": "mm" }, "height": { "value": ..., "unit": "mm" } }`.
- **Default.** `"letter"` (matches `export_service.go:1085` literal `"Letter"`).
- **Required.** Yes.

#### `page.orientation`

- **Purpose.** Page orientation.
- **Type.** `string` (`"portrait"` / `"landscape"`).
- **Default.** `"landscape"`.
- **Note.** Today this is driven by `PDFOptions.Orientation` via `PrintSettings` (see findings §1.2). The token is the *base* value; `PrintSettings.Orientation` is a delta override.

#### `page.margins.{top,bottom,left,right}`

- **Purpose.** Page margins.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** All `0.75 in` (currently `16 mm` left/right, `28 mm` top; bottom is fpdf's default `10 mm`).
- **Trace.** `export_service.go:1090` and the absence of an explicit bottom margin set.
- **Required.** Yes.

#### `page.header_offset` / `page.footer_offset`

- **Purpose.** Y offset from the top of the page (header) and from the bottom (footer) where the sticky header/footer anchors.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `0.4 in`.
- **Trace.** `export_service.go:1105` (`pdf.SetY(10)`) and `export_service.go:1117` (`pdf.SetY(-11)`) — the literals get replaced.

#### `page.auto_break_margin`

- **Purpose.** Distance from the page bottom at which fpdf's auto page break triggers.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `20 mm` (matches `export_service.go:1091`).
- **Trace.** `export_service.go:1091`; this is the only place it's set at document-init time, and it is currently overridden by `writePDFRecordCard` / `writePDFRecordCardMultiPage`.

### 2.3 `record_card`

- **Trace.** `pdf_layout.go:255–272` (struct) and `pdf_layout.go:273–293` (defaults).
- **Role.** Geometry for the two-column record card. Inherited by all per-export record card variants.

#### `record_card.left_column_ratio`

- **Purpose.** Width of the left column as a fraction of the content area.
- **Type.** `number` in `[0, 1]`.
- **Default.** `0.52`.
- **Overrides.**
  - `portrait_compact`: `0.6` (matches `pdf_layout.go:319`).
  - `no_images`: `0.43` (matches `pdf_layout.go:321`).

#### `record_card.column_gap`

- **Purpose.** Gap between left and right columns.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `8 mm` (matches `pdf_layout.go:282`).

#### `record_card.section_gap`

- **Purpose.** Vertical gap between sections.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `4 mm` (matches `pdf_layout.go:283`).

#### `record_card.field_row_gap`

- **Purpose.** Vertical gap between field rows inside a compact field section.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `1 mm` (matches `pdf_layout.go:281`).

#### `record_card.label_width_clamp`

- **Purpose.** Min/max width for the label column inside a compact field section.
- **Type.** `{ "min": { "value": ..., "unit": ... }, "max": { "value": ..., "unit": ... } }`.
- **Default.** `min: 28 mm`, `max: 44 mm` (matches the `28` and `44` literals in `pdf_layout.go:404–407` and `pdf_layout.go:619–622`).
- **Trace.** Two literal clamps at `pdf_layout.go:404–407` and `pdf_layout.go:619–622`. Currently no override; this token makes the clamp a configuration concern.

### 2.4 `shrink`

- **Trace.** `pdf_layout.go:271` (constant), `pdf_layout.go:327` (slice literal).
- **Role.** Shrink loop that tries to fit the record card on a single page.

#### `shrink.sequence`

- **Purpose.** Ordered list of scale factors tried by `choosePDFRecordCardLayout`.
- **Type.** `array<number>` (each value in `(0, 1]`).
- **Default.** `[1.0, 0.94, 0.88, 0.82, 0.76]`.

#### `shrink.min_readable_scale`

- **Purpose.** Floor scale factor.
- **Type.** `number` in `(0, 1]`.
- **Default.** `0.76` (matches `pdf_layout.go:271`).
- **Trace.** `pdf_layout.go:271` constant `minReadablePDFRecordCardScale`.

### 2.5 `type_scale`

- **Trace.** `pdf_layout.go:255–269` (struct fields), `pdf_layout.go:273–289` (defaults), `pdf_layout.go:878` (biography literal), `pdf_layout.go:1106, 1121` (header/footer sizes), `pdf_layout.go:1125–1127` (image thumbnail/row constants).
- **Role.** Font family, font size, and line height for every typed element.

#### `type_scale.primary_font` / `display_font` / `monospace_font`

- **Purpose.** Font family names for the three roles.
- **Type.** `string`.
- **Default.** `"Helvetica"`, `"Times"`, `"Courier"`.
- **Note.** Only `Helvetica` and `Times` are actually used in the PDF today (`pdf_layout.go:234, 238, 242, ...`). `Courier` is reserved for future rich-text mono support.

#### `type_scale.scale`

- **Purpose.** Global font scale multiplier (1.0 = no scaling).
- **Type.** `number` ≥ 0.
- **Default.** `1.0`.
- **Use.** All size/line tokens are multiplied by this at resolve time.

#### `type_scale.<element>`

- **Purpose.** Per-element font size + line height.
- **Type.** `{ "size": { "value": number, "unit": "pt" | "mm" | "in" }, "line"?: { "value": number, "unit": "pt" | "mm" | "in" } }`.
- **Element → default → trace:**

| Element | Default size | Default line | Source |
|---|---|---|---|
| `section_title` | `9 pt` | `6 pt` | `pdf_layout.go:276–277` |
| `field_label` | `8 pt` | `4.5 pt` | `pdf_layout.go:278, 280` |
| `field_value` | `9 pt` | `4.5 pt` | `pdf_layout.go:279, 280` |
| `body` | `9 pt` | `5 pt` | `pdf_layout.go:287, 288` |
| `image_label` | `8 pt` | `4 pt` | `pdf_layout.go:285, 286` |
| `image_thumbnail` | width `34 mm`, height `22 mm` | n/a | `pdf_layout.go:1125–1126` |
| `image_row` | height `36 mm` | n/a | `pdf_layout.go:1127` |
| `image_panel` | height `64 mm` | n/a | `pdf_layout.go:284` |
| `bullet_indent` | `6 mm` | n/a | `pdf_layout.go:289` |
| `biography` | `11 pt` | `6 pt` | `pdf_layout.go:878` (hard-coded) |
| `header` | `10 pt` | n/a | `export_service.go:1106` |
| `footer` | `8 pt` | n/a | `export_service.go:1121` |

### 2.6 `wrap`

- **Trace.** `pdf_layout.go:414` (`* 1.18`), `pdf_layout.go:430` (`* 1.18`).
- **Role.** Wrap multipliers for text-density estimation.

#### `wrap.rich_text_multiplier` / `wrap.records_multiplier`

- **Purpose.** Padding multiplier on `BodyLineHeight` when estimating section height for wrapped text.
- **Type.** `number` ≥ 1.0.
- **Default.** `1.18` for both (matches the two literal `1.18` values).

### 2.7 `overflow`

- **Trace.** `pdf_layout.go:1128` (`pageHeight-16`), `pdf_layout.go:1160` (`pdf.GetY() > 230`).
- **Role.** Hard-coded bottom thresholds that the renderer should respect.

#### `overflow.image_row_bottom_guard`

- **Purpose.** Distance from page bottom at which the image-row helper triggers `pdf.AddPage()`.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `16 mm` (matches `pdf_layout.go:1128`).

#### `overflow.registry_break_y`

- **Purpose.** Y position at which the registry-entry helper triggers `pdf.AddPage()`.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `230 mm` (matches `pdf_layout.go:1160` literal `230`).

### 2.8 `palette`

- **Trace.** Every `pdf.SetTextColor` and `pdf.SetDrawColor` in `pdf_layout.go` and `export_service.go`, plus the corresponding `#hex` literals in `*.templ` and `static_archive.go`.
- **Role.** Canonical color tokens with **both** an `rgb` triplet (for fpdf `SetTextColor` / `SetDrawColor`) and a `hex` string (for CSS / templ).

#### Token table

| Token | `rgb` | `hex` | PDF callsites | Templ/static-archive callsites |
|---|---|---|---|---|
| `accent` | `[141, 116, 64]` | `#8d7440` | 6 text + 3 draw = 9 | every `.templ` file, `static_archive.go:129` (`--gold-dark`) |
| `accent_strong` | `[168, 138, 70]` | `#a88a46` | not used in PDF | `layout.templ:36` (`.gold` color), `layout.templ:51` (focus ring), `static_archive.go:128` (`--gold`) |
| `text_primary` | `[34, 48, 61]` | `#22303d` | 9 text = 9 | every `.templ` file, `static_archive.go:130` (`--ink`) |
| `text_secondary` | `[68, 82, 96]` | `#445260` | 8 text = 8 | `calendar.templ`, `calendar_day.templ`, `static_archive.go:131` (`--muted`) |
| `text_muted` | `[113, 128, 142]` | `#71808e` | not used in PDF | `calendar.templ:27, 35, 43` only |
| `link` | `[48, 87, 122]` | `#30577a` | `pdf_layout.go:1066` | not used as a hex in `.templ` |
| `danger` | `[84, 33, 29]` | `#54211d` | not used in PDF | `layout.templ:76, 79` (`.danger-button`) |
| `divider` | `[141, 116, 64]` | `#8d7440` | (same as `accent`; semantic alias) | (same as `accent`) |
| `panel_fill` | — | `#fff8e7` | not used in PDF | `layout.templ:76, 87, 142`, `static_archive.go:125, 127, 455` |

**Notes on the gap between PDF and web palettes.** The "danger" red used heavily in the templ layer (`#6f2c26`, `#7b1e2b`) has **no PDF equivalent** — it does not appear in any fpdf call. The token table above maps the actual PDF triplets; the web-only "danger" hexes are reserved for a future `palette.danger_alt` extension once the PDF export grows to use them.

### 2.9 `borders`

- **Trace.** fpdf's default `Line` width (currently no `SetLineWidth` call exists); CSS `border-radius: 0.8rem` at `layout.templ:47`; `border: 1px solid …` at `layout.templ:40, 46, 66, 71, 76, 88, 130, 157, 171`.
- **Role.** Border weight, panel radius, panel border weight.

#### `borders.divider_weight`

- **Purpose.** Stroke width for `pdf.Line` calls.
- **Type.** `{ "value": number, "unit": "mm" | "in" | "pt" }`.
- **Default.** `0.2 mm` (fpdf's default — no current `SetLineWidth` exists, so today the divider weight is whatever fpdf ships).

#### `borders.panel_radius`

- **Purpose.** `border-radius` for `.card` and friends.
- **Type.** `{ "value": number, "unit": "rem" | "px" | "em" }`.
- **Default.** `0.8 rem` (matches `layout.templ:47`).

#### `borders.panel_border_weight`

- **Purpose.** `border-width` for `.card` and friends.
- **Type.** `{ "value": number, "unit": "px" | "pt" | "mm" }`.
- **Default.** `1 px` (matches `layout.templ:40, 46, 66, 71, 76, 88, 130, 157, 171`).

### 2.10 `branding`

- **Trace.** `export_service.go:1143` (`"'s Civil War Research Archive"`), `export_service.go:1144` (`"Made with DixieData | Version: "`, `" | Build: "`).

#### `branding.header.from` / `branding.header.suffix`

- **Purpose.** The header title is `<from> + <suffix>`.
- **Type.** `string` (`from` is a path: `"user_identity.branding_name"`; `suffix` is a literal).
- **Default.** `suffix: "'s Civil War Research Archive"`.

#### `branding.footer.template`

- **Purpose.** The footer text template; supports `{app_version}` and `{build_identity}` placeholders.
- **Type.** `string`.
- **Default.** `"Made with DixieData | Version: {app_version} | Build: {build_identity}"`.

#### `branding.footer = null`

- **Effect.** Footer suppressed (used by `printer_friendly` override — see `export_service.go:1113` `if !printerFriendly` guard).

### 2.11 `overrides`

- **Trace.** `PDFOptions.Normalize` is the existing override surface (`pdf_layout.go:64`); `writePDFRecordCard` lines 319/321 are the existing per-mode override surface; `printerFriendly` suppresses the footer (`export_service.go:1113`); `biography_appendix` re-uses the biography page renderer with a different label (`pdf_layout.go:858–860`).
- **Role.** Pre-defined override bundles that can be activated by name when resolving the final theme for a given export.

| Override name | Activated when | What it changes |
|---|---|---|
| `printer_friendly` | `PDFOptions.PrinterFriendly == true` | black-on-white text; footer suppressed |
| `portrait_compact` | `usesPortraitCompactRecordCardLayout(soldier, options) == true` | `record_card.left_column_ratio` → `0.6` |
| `no_images` | `!options.IncludeImages` | `record_card.left_column_ratio` → `0.43` |
| `biography_appendix` | `shouldAppendSingleRecordBiographyPage(...)` or `writePrintableBiographyAppendixPage(...)` | bumps biography font size to `10 pt` and section title to `10 pt / 7 pt` |

---

## 3. Typed Go struct

The JSON shape above mirrors this Go struct. Every value is a leaf type that survives JSON round-trips. Unit-bearing values are a small `theme.Measure` struct so units cannot drift.

```go
package theme

type Theme struct {
    SchemaVersion string `json:"schema_version"`
    Page          Page
    RecordCard    RecordCard
    Shrink        Shrink
    TypeScale     TypeScale
    Wrap          Wrap
    Overflow      Overflow
    Palette       Palette
    Borders       Borders
    Branding      Branding
    Overrides     map[string]Override
}

type Page struct {
    PaperSize        PaperSize `json:"paper_size"`
    Orientation      string    `json:"orientation"`
    Margins          Margins   `json:"margins"`
    HeaderOffset     Measure   `json:"header_offset"`
    FooterOffset     Measure   `json:"footer_offset"`
    AutoBreakMargin  Measure   `json:"auto_break_margin"`
}

type PaperSize interface{ apply(*fpdf.Fpdf) }   // or: union { Named string; Explicit SizeMM }
type Margins struct{ Top, Bottom, Left, Right Measure }
type RecordCard struct {
    LeftColumnRatio float64 `json:"left_column_ratio"`
    ColumnGap       Measure `json:"column_gap"`
    SectionGap      Measure `json:"section_gap"`
    FieldRowGap     Measure `json:"field_row_gap"`
    LabelWidthClamp Clamp   `json:"label_width_clamp"`
}
type Clamp struct{ Min, Max Measure }

type Shrink struct {
    Sequence         []float64 `json:"sequence"`
    MinReadableScale float64   `json:"min_readable_scale"`
}

type TypeScale struct {
    PrimaryFont   string  `json:"primary_font"`
    DisplayFont   string  `json:"display_font"`
    MonospaceFont string  `json:"monospace_font"`
    Scale         float64 `json:"scale"`
    SectionTitle  Element `json:"section_title"`
    FieldLabel    Element `json:"field_label"`
    FieldValue    Element `json:"field_value"`
    Body          Element `json:"body"`
    ImageLabel    Element `json:"image_label"`
    ImagePanel    Box     `json:"image_panel"`
    ImageThumb    Box     `json:"image_thumbnail"`
    ImageRow      Box     `json:"image_row"`
    BulletIndent  Measure `json:"bullet_indent"`
    Biography     Element `json:"biography"`
    Header        Element `json:"header"`
    Footer        Element `json:"footer"`
}

type Element struct {
    Size Measure `json:"size"`
    Line Measure `json:"line"`
}
type Box struct {
    Width  Measure `json:"width,omitempty"`
    Height Measure `json:"height"`
}

type Wrap struct {
    RichTextMultiplier float64 `json:"rich_text_multiplier"`
    RecordsMultiplier   float64 `json:"records_multiplier"`
}

type Overflow struct {
    ImageRowBottomGuard Measure `json:"image_row_bottom_guard"`
    RegistryBreakY      Measure `json:"registry_break_y"`
}

type Palette struct {
    Accent        Color  `json:"accent"`
    AccentStrong  Color  `json:"accent_strong"`
    TextPrimary   Color  `json:"text_primary"`
    TextSecondary Color  `json:"text_secondary"`
    TextMuted     Color  `json:"text_muted"`
    Link          Color  `json:"link"`
    Danger        Color  `json:"danger"`
    Divider       Color  `json:"divider"`
    PanelFill     HexOnly `json:"panel_fill"`
}

type Color struct {
    RGB [3]int  `json:"rgb"`
    Hex string `json:"hex"`
}
type HexOnly struct{ Hex string `json:"hex"` }

type Borders struct {
    DividerWeight     Measure `json:"divider_weight"`
    PanelRadius       Measure `json:"panel_radius"`
    PanelBorderWeight Measure `json:"panel_border_weight"`
}

type Branding struct {
    Header BrandingHeader `json:"header"`
    Footer BrandingFooter `json:"footer"`
}
type BrandingHeader struct {
    From   string `json:"from"`
    Suffix string `json:"suffix"`
}
type BrandingFooter struct {
    Template *string `json:"template"` // nil = suppressed
}

type Override = map[string]any  // each override is itself a partial Theme

type Measure struct {
    Value float64 `json:"value"`
    Unit  string  `json:"unit"`
}
```

This is illustrative; the actual implementation can refine (e.g. a `Measure.ToMM()` method that converts the `value` to millimeters based on the `unit` field).

---

## 4. Token-resolution rules

1. **Source of truth.** The JSON is the only source of truth at runtime. The typed Go struct is loaded from it.
2. **Unit normalization.** `Measure` values are converted to a single internal unit (millimeters for geometry, points for type scale) at load time. The `unit` field is preserved in the struct for round-tripping, but all math uses the normalized form.
3. **Override precedence.** When a theme is resolved for a specific export, the resolution order is:
   1. The base `theme.json` is loaded.
   2. The override bundles listed in `overrides` whose activation conditions are met (e.g. `printer_friendly` if `PDFOptions.PrinterFriendly`) are applied **on top of** the base, in a deterministic order documented in code.
   3. `PrintSettings` / `PDFOptions` per-export deltas (orientation, paper size) are applied last.
4. **Token references.** A token may reference another token by `"$name"` (e.g. `"color": "$accent"`). References are resolved depth-first; cycles are rejected by the loader.
5. **Leaf-only overrides.** An override bundle may only redefine leaves, not structure. `record_card.section_count` (if it existed) is not a leaf and cannot be overridden.
6. **No silent fallbacks.** If a required field is missing, the loader fails fast. Defaults are the values in `theme.json`, not hidden in code.

---

## 5. Validation rules

| Rule | Where | What happens on violation |
|---|---|---|
| `schema_version` matches a known major | loader | reject with `ErrUnsupportedSchema` |
| Every `Measure.unit` ∈ {`mm`, `in`, `pt`, `px`, `rem`, `em`} | loader | reject with `ErrBadUnit` |
| `Palette.<color>.rgb` values in `[0, 255]` | loader | reject |
| `Palette.<color>.hex` matches `^#[0-9a-fA-F]{6}$` | loader | reject |
| `record_card.left_column_ratio` in `[0, 1]` | loader | reject |
| `shrink.min_readable_scale` in `(0, 1]` and ≤ `min(shrink.sequence)` | loader | reject |
| Every `type_scale.<element>.size.value > 0` | loader | reject |
| `branding.footer.template` (when non-nil) is a string with at most `{app_version}` and `{build_identity}` placeholders | loader | reject |
| Token references (`"$name"`) form an acyclic graph | loader | reject with `ErrCyclicToken` |
| Override bundle JSON shape matches the top-level `Theme` shape | loader | reject |
| Every required field present (no `null`/missing) | loader | reject with `ErrMissingField` listing all missing fields |

These rules live in the `theme.Loader` package; the loader exposes `Load(path string) (*Theme, error)` and `LoadFromBytes(data []byte) (*Theme, error)`.
