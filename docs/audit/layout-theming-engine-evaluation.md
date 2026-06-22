# Layout Engine Evaluation

Comparison of the three candidate rendering paradigms for DixieData's PDF and HTML outputs. Scores are 1–5, higher is better. Justification cites specific findings in `layout-theming-findings.md`.

The candidates are:

- **A — Coordinate grid (current state, `go-pdf/fpdf`).** The current implementation.
- **B — Flow-based document layout (HTML/CSS-to-PDF via a headless renderer).** A future where a Chromium / WeasyPrint / wkhtmltopdf dependency replaces fpdf.
- **C — Hybrid: coordinate grid + token-driven theming (recommended target).** fpdf stays; every visual literal is replaced with a token read at document-init time.

---

## 1. Scoring matrix

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

---

## 2. Justifications

### 2.1 Margin determinism — A: 4, B: 4, C: 4

- **A (4).** The current fpdf path arms explicit margins at `newPDFDocument` (`export_service.go:1090` — `SetMargins(16, 28, 16)`) and re-asserts them per-column in `writePDFRichTextColumnSection` (`pdf_layout.go:683–684`) and `writePDFRecordsColumnSection` (`pdf_layout.go:702–703`). The auto-break margin is overridden downstream (see findings §1.1) and the bottom margin is fpdf's default `10 mm` rather than the literal `20` in `SetAutoPageBreak` (see findings §2.5). The reason this is not 5: the bottom margin is implicit; the per-column `SetMargins` calls save/restore via `defer` and have a subtle interaction with `SetAutoPageBreak` that is not documented in the code. **4.**
- **B (4).** Flow-based engines treat `@page { margin: ... }` and CSS `margin` as authoritative; the renderer enforces them. There is no manual override surface in the templating layer, so drift is harder. **4.**
- **C (4).** Same engine as A but with all margin literals resolved through tokens. Inherits the same subtle interaction between `SetMargins` and `SetAutoPageBreak`, so it does not improve on A. **4.**

### 2.2 Multi-page reliability — A: 4, B: 4, C: 4

- **A (4).** The current path uses a layered strategy: fpdf's auto page break (enabled in `writePDFRecordCardMultiPage` at `pdf_layout.go:539`), an explicit `preparePDFRecordCardSection` overflow guard (`pdf_layout.go:590–601`), a `pdf.AddPage()` call in `writeFullBiographyPage` (`pdf_layout.go:871`), and per-section `AddPage()` calls in `writePDFImageRow` (`pdf_layout.go:1129`) and `writePDFRegistryEntry` (`pdf_layout.go:1161`). The strategy is sound; the reason this is not 5: the `preparePDFRecordCardSection` guard only applies to sections 2..N; the Identity section (the first section) is drawn without a guard (see findings §2.3). **4.**
- **B (4).** Flow-based engines handle multi-page natively. The reliability ceiling is determined by the engine's bug surface and the CSS rules. **4.**
- **C (4).** Same as A — the multi-page strategy does not change; only the inputs (tokens) change. **4.**

### 2.3 Orphan/widow control — A: 2, B: 5, C: 2 → 3

- **A (2).** The codebase has section-level keep-together (`preparePDFRecordCardSection` forces a page break when the next section title + one body line would not fit; see findings §2.3). It does **not** have label/value keep-together — `writePDFCompactFieldSection` starts both `MultiCell`s at the same `rowTop` but does not verify they END together (see findings §2.3). It does **not** have biography widow control — `writeFullBiographyPage` relies on fpdf's auto page break with no minimum-line-together check. **2.**
- **B (5).** Flow-based engines honor `break-inside: avoid`, `orphans`, `widows`, and `break-before` natively. CSS rules can express every keep-together semantic the audit calls out. **5.**
- **C, today (2).** Same as A. The hybrid does not improve orphan/widow handling on its own. **2.**
- **C, with keep-together token (3).** If a `record_card.keep_together` token is added to the schema, the existing `preparePDFRecordCardSection` could be extended to accept a "minimum lines together" floor (e.g. "keep the section title and the first 2 lines of the section together on a new page"). This is a small addition — it does not require a new helper family, only an extended signature. **3.**

### 2.4 Theme change cost (per palette) — A: 1, B: 5, C: 4

- **A (1).** Changing the accent color from `(141, 116, 64)` to anything else requires editing 9 callsites across 2 files (6 `SetTextColor` + 3 `SetDrawColor`; see findings §3.1). On the templ side, 27 different alpha variants of `rgba(141, 116, 64, *)` would also need to change. There is no abstraction layer. **1.**
- **B (5).** A flow-based engine consumes CSS variables (`--accent: #8d7440;`) directly; a theme change is a single line in a `:root` block. **5.**
- **C (4).** With the proposed `theme.json` schema, a palette change is one entry in one file. The hybrid still requires the `theme.Loader` to inject the tokens into fpdf (`SetTextColor(token.RGB[0], …)`) and into the templ layer (CSS custom properties on `:root`), but that is plumbing that exists once. **4.**

### 2.5 Theme change cost (per geometry) — A: 1, B: 5, C: 4

- **A (1).** Changing paper size from `Letter` to `A4` requires editing `export_service.go:1085`. Changing margins requires editing `export_service.go:1090`. Changing column ratio requires editing `pdf_layout.go:275` and the two override literals at `pdf_layout.go:319, 321`. There is no single point of change. **1.**
- **B (5).** A flow-based engine consumes `@page { size: A4; margin: 0.75in; }` directly. **5.**
- **C (4).** With the proposed schema, a paper-size or margin change is one entry in `theme.json`; the `theme.Loader` reads it once. **4.**

### 2.6 Reproducibility across OS — A: 5, B: 2, C: 5

- **A (5).** fpdf is pure Go; output is byte-identical across OS, arch, and Go version. The codebase disables compression (`export_service.go:1092` — `SetCompression(false)`) which makes the byte-identical property even more reliable. **5.**
- **B (2).** A headless renderer (Chromium, WeasyPrint, wkhtmltopdf) brings OS-level variance: font availability, font hinting differences, line-break differences across locales, sandboxing differences. For a local-first archive whose exports are meant to be reproducible, this is a serious concern. The `Build: {build_identity}` suffix on the footer (see `export_service.go:1144`) is the audit's marker for how seriously reproducibility is taken. **2.**
- **C (5).** Same engine as A. Output reproducibility is unchanged. **5.**

### 2.7 Refactor cost from today — A: n/a, B: 5, C: 3

- **A.** n/a (current state).
- **B (5, high).** Replacing the fpdf path with a headless renderer means: re-implementing `pdfBranding` in CSS, re-implementing every `writePDF*` helper as a CSS template, finding a font-equivalent on every supported OS, re-testing the entire export path, and absorbing the binary-size / sandboxing concerns of the renderer dependency. The cost is high and the payoff is limited unless a future requirement forces it. **5 (high).**
- **C (3, medium-low).** The hybrid path is concentrated in three work-streams: (a) the typed token schema and loader, (b) the `newPDFDocument` and `brandedPDFDocument` init functions that read tokens, (c) the per-section render helpers that take a `Tokens` argument instead of literals. The engine and the multi-page strategy stay the same. The refactor is bounded and the test surface is preserved. **3 (medium-low).**

### 2.8 Test surface stability — A: 5, B: 2, C: 5

- **A (5).** Existing tests in `internal/archive/export_service_test.go` compare PDF output bytes (or byte hashes) for the export path. fpdf is deterministic; the test surface is stable. **5.**
- **B (2).** Switching renderers invalidates every test that asserts on output bytes. New tests must allow for renderer variation. **2.**
- **C (5).** The hybrid preserves the renderer; tests continue to pass byte-for-byte. **5.**

---

## 3. Recommendation

**Adopt Candidate C — Hybrid: coordinate grid + token-driven theming.**

The hybrid path resolves every concrete theming pain surfaced in the audit (no abstraction layer over the 9 `(141, 116, 64)` PDF callsites, the 27 `rgba(141, 116, 64, *)` alpha variants on the web, the duplicated body background between `layout.templ:27–31` and `static_archive.go:143–149`, the `e.db.UserIdentity()`-only branding path, the hard-coded `230 mm` and `pageHeight-16` overflow thresholds) without forcing an engine swap and the OS-level variance that comes with it.

The implementation should follow the component migration order in `layout-theming-components.md` §2. The order is:

1. `Page` + `StickyHeader` + `StickyFooter` (lowest risk; single function literals in PDF, two structural blocks in templ).
2. `SectionTitle` + `FieldLabel` + `FieldValue` (highest volume; most templ callsites).
3. `RecordCard` (largest PDF migration; 7 functions to refactor).
4. The remaining components (`ImagePanel`, `RecordsTable`, `RichTextSection`, `BiographyPage`, `GroupDividerPage`, `RegistryEntry`, `ImageRow`, `TitleBlock`) in any order.

A follow-on extension can add `record_card.keep_together` to the schema to bump the orphan/widow score from 2 to 3.

---

## 4. When to reconsider

The hybrid path should be revisited under any of the following conditions:

1. **Rich CSS hover-state previews are required in the static archive.** The current static archive is a single-page JS viewer; if the requirement grows to include interactive CSS hovers / `:focus` / `transition` previews, a flow-based engine is the only one that supports them natively. The PDF path would not change; only the static archive would gain a renderer dependency.
2. **JS-driven interactivity is required inside the static archive** (e.g. a sortable records table, an in-place edit affordance). The current static archive is read-only by design (`Static Archive: read-only browser-viewable archive export`, per `CONTEXT.md`); if this changes, B becomes a serious option.
3. **Multi-column CSS reflows are required inside a record card** (e.g. a 4-column dense field layout). The current fpdf path supports two columns cleanly (the `pdfRecordCardLayout` struct); a 3+ column layout in PDF is a hand-rolled exercise. A flow-based engine handles N-column reflow natively. If the requirement grows, B is worth the cost.
4. **fpdf goes unmaintained or ships a blocking bug.** The `go-pdf/fpdf` fork is the underlying dependency (`go.mod` `github.com/go-pdf/fpdf`); if it stops being maintained or hits a bug that can't be worked around, B is the only fallback.
5. **Repro tests fail to keep up with fpdf version drift.** If a future fpdf version changes the byte output of a tested export, the test surface becomes a maintenance burden. This is a low-probability scenario — fpdf has been byte-stable for years — but it should be tracked.

If any of these conditions is met, re-evaluate the matrix. None of them is true today.

---

## 5. Status as of 2026-06-20 (post-migration)

The Typst-based migration has shipped. As of slice 7 (commit 7139fff), the production appshell routes every export through Typst templates under `templates/`:

- `soldier_landscape`, `soldier_portrait`
- `widow_landscape`, `widow_portrait`
- `spouse_landscape`, `spouse_portrait` (covers `wife` and
  `linked_person` entry types)
- `biography_appendix`
- `anniversary`
- `analytics_summary`
- `group_divider`
- `static_archive_index` (printable companion; the HTML
  static-archive index remains unchanged)

The `pkg/render/fpdf` Service is retained as a test scaffold
only. `FpdfRenderer` is no longer wired into the appshell's
Registry. The `go-pdf/fpdf` dependency stays solely to compile
the test fixtures. New code should NOT depend on the fpdf
Service.

The hybrid recommendation in section 3 still describes the
target end-state, but the path taken (via Typst rather than the
proposed coordinate-grid PDF layer) reaches the same outcomes:
single source of truth for theme tokens, page geometry, and
section components. See `docs/PRD.md` and `docs/TASKS.csv`
for the closed user stories.
