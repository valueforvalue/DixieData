# Typst-based PDF Template System — Implementation Plan

## Decisions (locked from interview)

| Decision | Value |
|---|---|
| Rendering engine | Typst 0.15 (current) via `go-typst` (CLI shell-out) |
| DSL shape | Pure Typst markup — `.typ` files in program directory |
| Typst binary | Bundled in `<program-dir>/bin/`, ~30–50 MB per platform |
| PrintSettings name | Keep — add one field: `Template string` |
| Override bundles | In `theme.typ` (Typst-side) |
| Static archive | Unify via Typst HTML export — same `.typ` files compile to HTML |
| Template count | 11 (3 subtypes × 2 orientations + 5 special-purpose) |
| Template discovery | Auto-discover `.typ` files in `<program-dir>/templates/` — drop a file, no Go change |
| Printer-friendly override | Applied at template level — each template reads `sys.inputs.options.printerFriendly` and decides its own behavior |
| Migration shape | Phased — new path alongside fpdf, migrate one export at a time |

---

## Goals

1. Make adding a new PDF export template a single-file operation.
2. Make field selection a template-level decision, not a Go-level one.
3. Make theme (colors, fonts, margins) a single file (`theme.typ`) instead of 11 PDF callsites + 60+ rgba variants.
4. Preserve all 7 existing export entry points and their options.
5. Eliminate the 11 hard-coded "correction values" the audit found in the fpdf path.
6. Lift orphan/widow control from audit score 2/5 to 5/5 (Typst native).
7. Unify the static-archive HTML design system with the PDF design system.

---

## Architecture

```
PrintSettings { Template string }  →  ExportService
                                          │
                                          ▼
                                  ┌───────────────┐
                                  │   Renderer    │  (interface)
                                  └───────┬───────┘
                                          │
                          ┌───────────────┴───────────────┐
                          ▼                               ▼
                  ┌───────────────┐               ┌───────────────┐
                  │  FpdfRenderer │               │ TypstRenderer │
                  │  (Phase 0-2)  │               │  (Phase 1+)   │
                  └───────────────┘               └───────┬───────┘
                                                         │
                                                         ▼
                                                ┌────────────────┐
                                                │  go-typst CLI  │
                                                └────────┬───────┘
                                                         │
                                                         ▼
                                                   bin/typst(.exe)
                                                  (bundled binary)

                ┌──────────────────────────────────────────────┐
                │  <program-dir>/templates/  (auto-discovered)  │
                ├──────────────────────────────────────────────┤
                │  common/                                      │
                │    theme.typ       palette, type-scale,       │
                │                   geometry, branding strings  │
                │    components.typ  section-title, field-row,  │
                │                   image-panel, records-table  │
                │    layout.typ      page setup, header, footer │
                │                                                  │
                │  soldier_landscape.typ   (record card, default)│
                │  soldier_portrait.typ                         │
                │  spouse_landscape.typ                          │
                │  spouse_portrait.typ                           │
                │  widow_landscape.typ                           │
                │  widow_portrait.typ                            │
                │  biography_appendix.typ                       │
                │  anniversary.typ                               │
                │  analytics_summary.typ                         │
                │  group_divider.typ                             │
                │  printable_archive_registry.typ               │
                │  (static_archive_index.typ — HTML output)     │
                └──────────────────────────────────────────────┘
```

### Package layout

```
internal/
  render/
    renderer.go         # Renderer interface
    fpdf.go             # FpdfRenderer (existing ExportService methods, refactored)
    typst.go            # TypstRenderer (go-typst wrapper)
    registry.go         # template discovery, PrintSettings → Template resolution
    encode.go           # data → sys.inputs JSON encoding
  export/               # the existing internal/archive package, refactored to call render
templates/              # <program-dir>/templates/, in the program's working dir
  common/
    theme.typ
    components.typ
    layout.typ
  soldier_landscape.typ
  ... (etc)
bin/
  typst.exe            # Windows
  typst                # macOS, Linux
```

---

## Renderer interface

```go
package render

type Template struct {
    Name     string            // "soldier_landscape" — used by Registry
    Engine   string            // "typst" or "fpdf"
    Path     string            // resolved path to .typ file (typst) or Go func name (fpdf)
    Metadata TemplateMetadata // record types, orientation, etc.
}

type TemplateMetadata struct {
    RecordTypes  []string // ["soldier", "spouse", "widow"]
    Orientation  string   // "landscape" | "portrait" | "any"
    ExportTypes  []string // ["record_card", "biography", "analytics", ...]
    Description  string   // shown in the export UI
}

type Renderer interface {
    // Render a template with the given data, write to out.
    // Returns ErrTemplateNotFound, ErrRenderFailed, etc.
    Render(ctx context.Context, tpl Template, data map[string]any, out io.Writer) error

    // List templates this renderer can serve.
    List(ctx context.Context) ([]Template, error)
}

type Registry struct {
    typst   Renderer
    fpdf    Renderer
    dir     string // <program-dir>/templates
}

func (r *Registry) Resolve(ps PrintSettings, recordType string) (Template, error) {
    // 1. If ps.Template is set and a .typ file with that name exists, return it.
    // 2. Otherwise, fall back to the default for (recordType, orientation).
    // 3. If no Typst template matches, return the FpdfRenderer template.
}

func (r *Registry) Render(ctx context.Context, ps PrintSettings, recordType string, data map[string]any, out io.Writer) error {
    tpl, err := r.Resolve(ps, recordType)
    if err != nil { return err }

    var r Renderer
    switch tpl.Engine {
    case "typst": r = r.typst
    case "fpdf":  r = r.fpdf
    default:      return fmt.Errorf("unknown engine %q", tpl.Engine)
    }
    return r.Render(ctx, tpl, data, out)
}
```

---

## Data flow

Each export becomes a single Go call:

```go
func (e *ExportService) ExportSoldierPDF(out string, s models.Soldier, ps PrintSettings) error {
    data := encodeForTypst(s, ps, e.pdfBranding())
    return e.registry.Render(ctx, ps, "soldier", data, out)
}

func encodeForTypst(s models.Soldier, ps PrintSettings, br pdfBranding) map[string]any {
    return map[string]any{
        "soldier": s,
        "biography": s.Biography,
        "records":   s.SourceRecords,
        "household": s.Household,
        "images":    s.Images,
        "options":   ps,
        "branding":  br,
        "app": map[string]string{
            "version":        buildinfo.AppVersion,
            "build_identity": buildinfo.BuildIdentity(),
        },
    }
}
```

The `options` field is the existing `PrintSettings` struct (with the new `Template` field). It serializes to:

```json
{
  "scope": "all",
  "orientation": "landscape",
  "printerFriendly": false,
  "fullBiographyPage": false,
  "sortBy": "last_name",
  "groupByUnit": false,
  "...": "..."
}
```

A `.typ` template reads everything via `sys.inputs`:

```typst
#let opts = sys.inputs.options
#let s = sys.inputs.soldier
#let branding = sys.inputs.branding
```

---

## `theme.typ` (centralized tokens)

```typst
// templates/common/theme.typ
#let palette = (
  accent:         rgb("#8d7440"),
  accent_strong:  rgb("#a88a46"),
  text_primary:   rgb("#22303d"),
  text_secondary: rgb("#445260"),
  text_muted:     rgb("#71808e"),
  link:           rgb("#30577a"),
  danger:         rgb("#54211d"),
  divider:        rgb("#8d7440"),
  panel_fill:     rgb("#fff8e7"),
)

#let type-scale = (
  section_title: (size: 9pt, line: 6pt),
  field_label:   (size: 8pt, line: 4.5pt),
  field_value:   (size: 9pt, line: 4.5pt),
  body:          (size: 9pt, line: 5pt),
  image_label:   (size: 8pt, line: 4pt),
  header:        (size: 10pt),
  footer:        (size: 8pt),
  biography:     (size: 11pt, line: 6pt),
)

#let geometry = (
  page_margin:    (top: 0.75in, bottom: 0.75in, left: 0.75in, right: 0.75in),
  column_gap:     8mm,
  section_gap:    4mm,
  field_row_gap:  1mm,
  record_card_left_ratio: 52%,
  image_panel_height: 64mm,
)

#let branding = (
  header_suffix: "'s Civil War Research Archive",
  footer_template: "Made with DixieData | Version: {app_version} | Build: {build_identity}",
)
```

This is the **only** place colors, fonts, and margins are defined. Every template imports this. A palette change is one file.

---

## `components.typ` (reusable pieces)

```typst
// templates/common/components.typ
#import "theme.typ"

#let section-title(title) = {
  text(size: theme.type-scale.section_title.size, weight: "bold", fill: theme.palette.accent, tracking: 0.18em)[
    #title
  ]
  v(theme.geometry.section_gap)
}

#let field-row(label, value) = grid(
  columns: (auto, 3mm, 1fr),
  text(size: theme.type-scale.field_label.size, fill: theme.palette.text_secondary, weight: "bold")[#label],
  [],
  text(size: theme.type-scale.field_value.size, fill: theme.palette.text_primary)[#value],
)

#let field-row-opt(label, value) = if value != none and value != "" [
  #field-row(label, value)
]

#let image-panel(image, label) = {
  if image != none {
    block(
      stroke: theme.palette.accent + 0.85pt,
      inset: 2mm,
      radius: 0.8rem,
      width: 100%,
    )[
      #image(height: theme.geometry.image_panel_height)
      #v(2mm)
      #text(size: theme.type-scale.image_label.size, fill: theme.palette.text_secondary)[#label]
    ]
  }
}

#let records-table(records) = {
  if records.len() > 0 [
    = #section-title("Records")
    #for r in records [
      #field-row(r.type, r.app_id)
      #if r.details != none and r.details != "" [
        #text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)[#r.details]
      ]
      #v(2mm)
    ]
  ]
}
```

---

## Sample template — `soldier_landscape.typ`

```typst
#import "common/theme.typ"
#import "common/components.typ"

#let opts = sys.inputs.options
#let s = sys.inputs.soldier
#let branding = sys.inputs.branding

#set page(
  paper: "us-letter",
  margin: theme.geometry.page_margin,
  header: align(center, text(
    size: theme.type-scale.header.size,
    weight: "bold",
    fill: theme.palette.text_primary,
  )[#branding.archive_title]),
  footer: if not opts.printerFriendly {
    align(center, text(
      size: theme.type-scale.footer.size,
      fill: theme.palette.text_secondary,
    )[#branding.footer_text])
  },
)

#set text(
  font: ("Helvetica Neue", "Arial"),
  size: theme.type-scale.body.size,
  fill: theme.palette.text_primary,
)

// Two-column record card
#grid(
  columns: (theme.geometry.record_card_left_ratio, theme.geometry.column_gap, 1fr),
  [
    // Left column: identity + service
    #section-title("Identity & Vital Details")
    #field-row("Display ID", s.display_id)
    #field-row("Name", s.name)
    #field-row("Birth", s.birth_date)
    #field-row-opt("Death", s.death_date)
    #field-row-opt("Burial", s.burial)

    #v(theme.geometry.section_gap)
    #section-title("Service & Archive Details")
    #field-row("Unit", s.unit)
    #field-row-opt("Rank", s.rank)
    #field-row("Pension State", s.pension_state)
    #field-row("Confederate Home", s.confederate_home_status)
  ],
  [],
  [
    // Right column
    #if opts.includeImages and s.portrait_image != none [
      #image-panel(s.portrait_image, s.name)
    ]

    #section-title("Household & Context")
    #field-row-opt("Spouse", s.spouse_name)
    #field-row-opt("Children", s.children)

    #v(theme.geometry.section_gap)
    #section-title("Biography")
    #if s.biography != none and s.biography != "" [
      #text(size: theme.type-scale.body.size, fill: theme.palette.text_primary)[
        #s.biography
      ]
    ]

    #v(theme.geometry.section_gap)
    #records-table(s.records)
  ],
)
```

That's the entire soldier record card. 60 lines. Compare to `writePDFRecordCard` (`pdf_layout.go:468–530`) — 62 lines of Go with 5 hard-coded colors, 6 hard-coded fonts, 4 hard-coded geometry values, 3 hard-coded overflow thresholds.

---

## Phased migration

### Phase 0 — Scaffolding (no behavior change)

**Goal:** Typst pipeline works end-to-end for one trivial case. Fpdf still does every real export.

**Tasks:**

1. Add `internal/render/renderer.go` with the `Renderer` interface.
2. Add `internal/render/typst.go` wrapping `go-typst`. Use `typst.CLI` with `ExecutablePath` pointing at `<program-dir>/bin/typst(.exe)`.
3. Add `internal/render/registry.go` with `Registry` and template discovery.
4. Add the `Template` field to `PrintSettings` (`pdf_layout.go:34`).
5. Add `<program-dir>/templates/` to the program directory layout.
6. Add `<program-dir>/bin/` and document where to drop the bundled `typst(.exe)`.
7. Add `templates/soldier_landscape.typ` that renders the soldier's name and Display ID (no styling beyond a single page).
8. Add a `templates/common/theme.typ` with the bare minimum (a `palette` dict, a `type-scale` dict).
9. Add `internal/render/encode.go` to serialize the test data to `sys.inputs`.
10. Add a debug-flagged entry point `ExportTypstPreview(out, soldier)` in `ExportService`.
11. Add a smoke test: assert the debug entry point produces a non-empty PDF.

**Out of scope:** No existing export changes. No `share.templ` UI changes. No template discovery for record types beyond Soldier.

**Deliverable:** `typst compile` runs, produces a PDF. All existing exports unchanged. All existing tests pass.

**Risk:** The `go-typst` library is unproven against our input shapes. Mitigate with the smoke test before proceeding.

---

### Phase 1 — Migrate the bulk export

**Goal:** `ExportFullDatabasePDF` uses Typst. Other exports unchanged.

**Tasks:**

1. Write `templates/soldier_landscape.typ` (the full record card, as in the sketch above).
2. Write `templates/soldier_portrait.typ` (portrait-compact variant: tighter column ratio, smaller fonts).
3. Write `templates/spouse_landscape.typ`, `spouse_portrait.typ`, `widow_landscape.typ`, `widow_portrait.typ` — clones with the subtype-specific fields swapped.
4. Write `templates/group_divider.typ` for group divider pages.
5. Write `templates/common/components.typ` with the full set of reusable pieces.
6. Write `templates/biography_appendix.typ` for the full-biography page.
7. Refactor `ExportFullDatabasePDF` to call `Registry.Render` with the appropriate template per record.
8. Add a `share.templ` UI toggle: "Use Typst renderer" (default ON in Phase 1; user can flip off to fall back to fpdf).
9. Add a test: render a single soldier via both the fpdf and typst paths; assert both produce valid PDFs.
10. Add visual regression: open both PDFs side-by-side, eyeball. Document any divergences and fix in the template.

**Out of scope:** Single-Soldier export, anniversary, analytics, printable archive, static archive.

**Deliverable:** Bulk export uses Typst. Old fpdf path still available as a fallback behind the UI toggle.

**Risk:** Image embedding. Typst's `image()` can take raw bytes via `bytes.decode()`, but the existing `image_service.go` returns paths. Need to confirm bytes round-trip cleanly through `go-typst`'s `Data` field. Test with a real soldier image.

---

### Phase 2 — Migrate the rest, one at a time

**Goal:** All exports use Typst. Fpdf path still available as a fallback.

**Tasks (in order):**

1. `ExportSoldierPDF` — `soldier_landscape.typ` + `biography_appendix.typ`.
2. `ExportMonthlyAnniversaryPDF` — `anniversary.typ`.
3. `ExportPrintableArchivePDF` — `printable_archive_registry.typ`.
4. `ExportAnalyticsSummaryPDF` — `analytics_summary.typ`.
5. Unify static archive — `static_archive_index.typ` (HTML output) replacing `staticArchiveIndexHTML` in `static_archive.go`.
6. Test each migration with the fpdf fallback and the typst path side-by-side.

**Deliverable:** All exports use Typst. Static archive uses Typst. Fpdf path is dead code but still wired.

**Risk:** The static archive unification is the biggest unknown. The current `static_archive.go` has interactive JS (image overlay zoom, search filter). Typst HTML output is static — no JS. Need to either:
- (a) Ship a thin JS layer that hydrates the static HTML for interactivity.
- (b) Accept a no-JS static archive and document the loss.
- (c) Skip the static-archive unification and defer it.

Recommend (a). The JS layer is small and the gain in design system unification is large.

---

### Phase 3 — Retire fpdf

**Goal:** Single rendering path. Clean module.

**Tasks:**

1. Remove `FpdfRenderer` and the underlying fpdf path from `ExportService`.
2. Remove `go-pdf/fpdf` from `go.mod`.
3. Delete `internal/archive/pdf_layout.go`.
4. Delete `internal/archive/pdfium_windows.go`, `internal/archive/pdfium_nonwindows.go` (these exist for rasterization; check if anything else uses them).
5. Remove the `fpdf` toggle from `share.templ`.
6. Update the audit deliverables in `docs/audit/layout-theming-*.md` to reflect the new state (the 9 hard-coded RGB triplets, the 60+ rgba variants, and the 11 "correction values" are all gone).
7. Final test pass: every existing export test must still pass.
8. Document the new template authoring workflow in `docs/templates.md`.

**Deliverable:** One rendering path. Smaller Go module. Documented template system.

**Risk:** Low. By Phase 3, all exports have been migrated and tested. The fpdf removal is just deletion.

---

## Discovery: how a new template appears in the UI

`Registry.List` scans `<program-dir>/templates/` at startup:

```
templates/
├── common/
│   ├── theme.typ
│   ├── components.typ
│   └── layout.typ
├── soldier_landscape.typ
├── ...
```

For each top-level `.typ` file, the registry:
1. Reads the file's first 20 lines.
2. Looks for a `#metadata` block (Typst comment) declaring name, record types, orientation, export types.
3. If found, registers the template.
4. If absent, falls back to a metadata file `soldier_landscape.json` alongside the `.typ`.

The metadata block convention (sketch):

```typst
// metadata:
//   name: soldier_landscape
//   record_types: [soldier]
//   orientation: landscape
//   export_types: [record_card]
//   description: Standard soldier record card (landscape)

#import "common/theme.typ"
... (template body)
```

The export UI (`share.templ`) shows a dropdown of discovered templates, grouped by record type. The default is selected based on the current record being exported.

**Adding a new template:** drop a `.typ` file in `templates/`. Restart the app. New template appears in the dropdown.

---

## Static archive unification

The static archive (`static_archive.go`) currently has:
- A 490-line inline `<style>` block with 9 CSS custom properties.
- A 70-line HTML body with hand-written structure.
- Interactive JS (search, image overlay zoom) embedded.

**The Typst path:**

1. `templates/static_archive_index.typ` produces the page using the same `theme.typ`.
2. The Typst HTML output is the page body, wrapped in a thin HTML shell with the existing JS for interactivity.
3. The CSS custom properties become inline `<style>` rules in the shell, sourced from `theme.typ` values serialized to CSS variables at build time.

**The hard part:** The interactive JS. The current `static_archive.go` has image overlay zoom (`static_archive.go:445–487`) and search filter (`static_archive.go:624–670`). These are user-facing. Without them, the static archive is significantly less useful.

**Mitigation:** Keep the JS layer. The shell HTML wraps the Typst-generated body and re-attaches the same event listeners by element ID. The data the JS needs (record list, image list) gets emitted as a `<script type="application/json">` block in the shell.

**Risk:** The CSS custom property names need to be stable. The shell's `<style>` block references them; the Typst body needs to use them. This is solvable but adds a small CSS-to-Typst naming convention.

---

## Risk summary

| Phase | Risk | Mitigation |
|---|---|---|
| 0 | `go-typst` doesn't handle our input shapes | Smoke test before proceeding |
| 1 | Image bytes don't round-trip cleanly | Test with a real soldier image in Phase 0 |
| 1 | Visual regression on bulk export | Side-by-side comparison; document divergences |
| 2 | Static archive loses interactivity | Wrap Typst body in JS shell |
| 3 | Hidden fpdf dependencies in `image_service.go` | Audit `pdfium_*.go` first |
| All | Typst version drift | Pin bundled binary version; document in `THIRDPARTY.md` |
| All | Performance regression | 1000-record bulk export benchmark; Typst is typically faster than fpdf for this volume but verify |

---

## Files to create

```
internal/render/
  renderer.go        # Renderer interface, Template, TemplateMetadata
  fpdf.go            # FpdfRenderer (existing methods, refactored)
  typst.go           # TypstRenderer (go-typst wrapper)
  registry.go        # Registry, template discovery
  encode.go          # data → sys.inputs JSON encoder
  encode_test.go
  registry_test.go
  typst_test.go      # smoke test

internal/export/
  print_settings.go  # PrintSettings with new Template field (moved from pdf_layout.go)
  ... (refactored from internal/archive/export_service.go)

templates/
  common/
    theme.typ
    components.typ
    layout.typ
  soldier_landscape.typ
  soldier_portrait.typ
  spouse_landscape.typ
  spouse_portrait.typ
  widow_landscape.typ
  widow_portrait.typ
  biography_appendix.typ
  anniversary.typ
  analytics_summary.typ
  group_divider.typ
  printable_archive_registry.typ
  static_archive_index.typ

bin/
  typst.exe          # Windows (downloaded from typst/typst releases)
  typst              # macOS, Linux

docs/
  templates.md       # how to author a new template
  THIRDPARTY.md      # pinned Typst version, license
```

## Files to delete (Phase 3)

```
internal/archive/pdf_layout.go
internal/archive/pdfium_windows.go
internal/archive/pdfium_nonwindows.go
```

(After confirming no other code references them.)

## Files to modify

```
go.mod                                       # add go-typst, remove go-pdf/fpdf
internal/archive/export_service.go          # refactor to call Registry.Render
internal/templates/share.templ              # add template selector UI
docs/audit/layout-theming-*.md              # update audit findings
CONTEXT.md                                   # no change
```

---

## Open items

1. **Pin the Typst version.** 0.15 is current (June 2026). Pin in `THIRDPARTY.md`. Re-evaluate on each upstream release.
2. **Font files.** The current `pdf_layout.go` uses `Helvetica` (no actual font file embedded — fpdf uses a builtin). Typst does not have built-in fonts. The bundled binary needs TTFs for "Helvetica Neue", "Arial", "Georgia", "Times New Roman". These are not free. Options: (a) ship open-source equivalents (Liberation Sans, Liberation Serif), (b) require the user to provide fonts, (c) bundle Adobe-licensed fonts (not redistributable). Recommend (a) for now.
3. **Migration of `pdfBranding()`.** Today it reads `e.db.UserIdentity()`. The Typst path can do the same. No change needed; the function moves from `export_service.go` to the new `internal/render` package.
4. **`pdfium_*.go`.** These exist for PDF rasterization (probably for the printable archive preview). Need to confirm what uses them before deletion in Phase 3.

---

## Done criteria

- [ ] Phase 0: `typst compile` produces a valid PDF for a test soldier. Fpdf path unchanged.
- [ ] Phase 1: `ExportFullDatabasePDF` produces a valid PDF via Typst. Old fpdf path still wired.
- [ ] Phase 2: All 7 export entry points produce valid PDFs via Typst. Static archive produces valid HTML.
- [ ] Phase 3: `go-pdf/fpdf` removed from `go.mod`. `pdf_layout.go` deleted. All existing export tests pass.
- [ ] Documentation: `docs/templates.md` describes how to add a new template.
- [ ] Third-party: `THIRDPARTY.md` lists the pinned Typst version and license.
- [ ] Audit: `docs/audit/layout-theming-findings.md` updated to reflect the new state.

No code written. No source files modified. Plan ready for review.
