# Typst-based PDF Template System

## Problem Statement

The DixieData PDF export system was built on a hand-rolled `go-pdf/fpdf` coordinate-grid renderer. Adding or removing a field required editing Go source; changing the visual theme (colors, fonts, margins) required editing nine hard-coded RGB callsites and sixty-plus CSS rgba variants across two layers. The renderer baked in eleven "correction values" (e.g. `pageHeight-16` at `internal/archive/pdf_layout.go:1128`, `pdf.GetY() > 230` at `internal/archive/pdf_layout.go:1160`) that existed only because fpdf's layout model is primitive. Orphan/widow control was weak. The visual design system was duplicated between the PDF export path and the static-archive HTML path. (This PRD is now historical: the fpdf path was removed in slice 7. The current export pipeline is typst-only; the duplication, the hard-coded RGB triplets, the correction values, and the orphan/widow control problem are all gone. This Problem Statement section is preserved to document the original motivation.)

A researcher using DixieData cannot:

- Add a new PDF export format without rebuilding the application.
- Adjust the theme (colors, fonts, margins) without editing Go source.
- Iterate quickly on the visual design of an export — the build cycle is minutes.

We want a system where adding a new export template is a single-file operation, theme changes are a single-file operation, and the design system is unified between the PDF and HTML outputs.

## Solution

Adopt Typst (a markup-based typesetting system) as the rendering engine for all PDF exports. Templates are `.typ` files in the program's `templates/` directory. The application discovers templates at startup, picks one based on the record type and export options, and invokes the Typst compiler to produce a PDF.

A new **`dixiedata-tune`** tool lives in the same repository as a separate Go module. It reads the same `templates/` directory and the DixieData database (read-only) and renders PDFs on demand. The tool provides:

- A CLI for scripted iteration: `dixiedata-tune render --template=soldier_landscape --record=42 --out=out.pdf`.
- A web UI for visual iteration: pick a record, pick a template, click render, see the PDF inline.
- A side-by-side comparison view: render the same record with the fpdf baseline and the Typst template, show both PDFs.
- A **markdown annotation feedback loop**: the researcher writes free-form notes in a `.md` file alongside the rendered PDF. The agent reads the annotations before the next render.

The visual design is captured in a single `theme.typ` file. The audit's `theme.json` schema (see `docs/audit/layout-theming-token-schema.md`) ports almost verbatim into Typst `let` bindings.

The fpdf path remains the default for all exports during a phased migration. The Typst path becomes the default for migrated exports. Eventually the fpdf path is removed. (Done: the fpdf path is now fully removed. The typst path is the only path. Slice 7 completed.)

## User Stories

1. As a researcher using DixieData, I want to export a Soldier's record card to a PDF, so that I can share it with collaborators.
2. As a researcher using DixieData, I want to export a Spouse Record's card to a PDF, so that I can share it.
3. As a researcher using DixieData, I want to export a Widow Record's card to a PDF, so that I can share it.
4. As a researcher using DixieData, I want to export a Soldier's record card in portrait orientation, so that I can print it in a portrait binder.
5. As a researcher using DixieData, I want to export a Spouse Record's card in portrait orientation, so that I can print it in a portrait binder.
6. As a researcher using DixieData, I want to export a Widow Record's card in portrait orientation, so that I can print it in a portrait binder.
7. As a researcher using DixieData, I want to export a full biography appendix, so that I have a complete narrative alongside the record card.
8. As a researcher using DixieData, I want to export a monthly anniversary report, so that I can see which Soldiers have a service anniversary this month.
9. As a researcher using DixieData, I want to export an analytics summary, so that I can see the archive's statistics at a glance.
10. As a researcher using DixieData, I want a printable-archive mode of the bulk export (one record per page, image included, bounded biography excerpt) so that I can print the Local Archive in a binder.
11. As a researcher using DixieData, I want a group divider page between records in a bulk export when I group by unit or pension state, so that the output is organized.
12. As a researcher using DixieData, I want all export options (orientation, printer-friendly, include-images, group-by, filter, sort) to continue working unchanged, so that the migration is non-breaking.
13. As a developer working on DixieData, I want to add a new export template by creating a single `.typ` file, so that I do not need to edit Go source.
14. As a developer working on DixieData, I want to add a new field to a record card by editing one `.typ` file, so that field selection is a template concern.
15. As a developer working on DixieData, I want to change a color or font by editing one `theme.typ` file, so that theme tokens are centralized.
16. As a developer working on DixieData, I want to change page margins by editing one `theme.typ` file, so that geometry is centralized.
17. ~~As a developer working on DixieData, I want the fpdf path to remain functional during the migration, so that the test surface stays green.~~ (Fulfilled by slices 0-6; superseded by slice 7.)
18. As a researcher using DixieData, I want exports to look the same as they do today (close enough, not byte-identical), so that the migration is visually non-disruptive.
19. As a developer working on DixieData, I want orphan/widow control handled by the renderer, so that I do not have to hand-roll keep-together logic.
20. As a developer working on DixieData, I want the static-archive HTML to use the same design system as the PDF exports, so that the duplication between `internal/archive/static_archive.go` and `internal/templates/layout.templ` is eliminated.
21. As a developer iterating on template design, I want a CLI tool that renders a template against a real record in seconds, so that I can iterate without rebuilding DixieData.
22. As a developer iterating on template design, I want a web UI in the tuning tool that shows the rendered PDF inline, so that I can iterate visually.
23. ~~As a developer iterating on template design, I want a side-by-side comparison of the fpdf baseline and the Typst render for the same record, so that I can see how close I am to the original.~~ (Replaced: the fpdf baseline is gone. The tune tool renders the same record through the production typst path that the appshell uses, so iterating on a template produces a faithful preview. There is no separate baseline.)
24. As a researcher providing feedback on a template, I want to write free-form markdown notes alongside the rendered PDF, so that I can describe what to change in a structured way.
25. As a developer iterating on a template, I want the agent to read my annotation markdown before the next render, so that my feedback drives the next iteration.
26. As a developer, I want annotation markdown files to persist across agent sessions, so that feedback is not lost when a session ends.
27. As a developer, I want the Typst compiler binary to be bundled with the application, so that the application works out of the box without a separate install.
28. As a developer, I want the tuning tool to read the DixieData database in read-only mode, so that the tool cannot corrupt user data.
29. As a developer, I want the tuning tool to be a separate Go module in the same repository, so that it has its own dependencies and build cycle.
30. As a developer, I want the tuning tool to share the template engine code with DixieData, so that templates are the source of truth.
31. As a researcher, I want the bulk export to handle Soldier, Spouse, and Widow records with the appropriate fields for each, so that the output is correct for each record subtype.
32. As a developer, I want each `.typ` template to declare its metadata (record types, orientation, description) in a comment block, so that the export UI can list available templates.
33. As a developer, I want the application to discover `.typ` files at startup, so that adding a template does not require a code change.
34. As a developer, I want the visual comparison baseline to be captured before any Typst code is written, so that there is something to compare against.
35. As a researcher, I want the static archive to be migrated to Typst HTML output as a thin slice, so that the design-system duplication is killed. The static archive's interactivity (search, image overlay zoom) is intentionally deferred to a separate static-archive overhaul; the slice accepts that the static archive may look and behave differently.
36. As a developer, I want the printer-friendly export option to suppress the footer and switch the palette to black-on-white, so that ink usage is reduced.
37. ~~As a developer, I want the fpdf path to be removed in a final phase, so that the Go module is clean.~~ (Done: `go-pdf/fpdf` removed from `go.mod`. The export pipeline is typst-only.)
38. As a developer, I want every existing export test to continue passing through the migration, so that the test surface is preserved.
39. As a developer, I want the audit's hard-coded literals (RGB triplets, rgba variants, correction values) to be eliminated by the migration, so that the audit findings are resolved.
40. As a researcher, I want the export UI to show a dropdown of available templates per record type, so that I can pick the format I want.

## Implementation Decisions

### Module structure

- The Typst-based renderer lives in a new `internal/render` package.
- The `Renderer` interface has two implementations: `FpdfRenderer` (existing behavior) and `TypstRenderer` (new behavior).
- A `Registry` discovers templates from `<program-dir>/templates/` and resolves `PrintSettings` to a concrete template.
- The tuning tool lives in `tools/tune/` as a separate Go module.
- To allow the tuning tool to import DixieData code, the relevant packages are extracted from `internal/` to `pkg/`. The minimum set is `pkg/render`, `pkg/encode`, and `pkg/templatespec`.

### Template system

- Templates are `.typ` files in `<program-dir>/templates/`.
- Each template declares metadata in a comment block at the top: `name`, `record_types`, `orientation`, `export_types`, `description`.
- Common theme tokens live in `templates/common/theme.typ`. Every template imports this.
- Common components (section title, field row, image panel, records table) live in `templates/common/components.typ`.
- Template discovery scans the directory at startup; no registration step is required.
- The `PrintSettings` struct gets one new field: `Template string`. Default is empty; the `Registry` resolves it based on the record type and orientation.

### Typst integration

- The Typst compiler binary is bundled in `<program-dir>/bin/typst(.exe)`.
- The Go side shells out to the bundled binary via `exec.Command` directly (no `go-typst` wrapper). On Windows the child is spawned with `CREATE_NO_WINDOW` so PDF export does not flash a black console.
- Data flows from Go to the template via `sys.inputs` (a JSON object).
- Errors from `typst compile` are surfaced verbatim from stderr (line/column info included).

### Tuning tool

- A new Go module at `tools/tune/go.mod` imports DixieData's `pkg/render`, `pkg/encode`, and `pkg/templatespec` packages.
- The tool reads the DixieData SQLite database in read-only mode (`?mode=ro`).
- CLI subcommands: `render`, `list-templates`, `list-records`, `capture-baseline`, `compare`, `serve`.
- The `serve` subcommand starts a small web server with a UI for picking a template, picking a record, rendering, and viewing the PDF inline.
- The web UI also shows a side-by-side comparison: the fpdf baseline PDF and the Typst PDF.
- The tool watches the `templates/` directory and hot-reloads templates when they change.

### Visual comparison

- A `tools/tune/baseline/` directory contains the fpdf-rendered PDFs (one per record, per export type) captured before any Typst code is written.
- The `capture-baseline` subcommand renders every record through every fpdf export and saves the output.
- The `compare` subcommand renders the same record through both the fpdf and Typst paths, saves both, and produces a markdown report with extracted text diffs and any layout divergences the agent notices.
- The "close enough" bar is: the Typst output should be visually recognizable as the same export, with the same fields, same color palette, same header/footer, and same two-column record card structure. It does not need to be byte-identical.

### Feedback loop

- For each render, the tool produces a sidecar file: `render_<n>_<template>_<record>.md` alongside the PDF.
- The markdown is a free-form notes file the researcher writes. Common sections: "what looks right", "what needs to change", "specific values to adjust".
- The next render of the same `(template, record)` pair reads the most recent annotation file for that pair as context.
- The agent's behavior: before rendering, check for an existing annotation file. If present, read it and incorporate the feedback into the next render.

### Phase boundaries

The migration is 8 slices: 1 prefactor + 7 implementation. The 7 implementation slices match the issue list; the prefactor is added because the existing PDF code is inlined in `ExportService` (a 1626-line god struct) and must be extracted before a `Renderer` interface can be built around it.

- **Slice 0 (prefactor) — extract PDF methods from `ExportService`.** Move `ExportSoldierPDF`, `ExportSoldierPDFWithoutImages`, `ExportMonthlyAnniversaryPDF`, `ExportFullDatabasePDF`, `ExportAnalyticsSummaryPDF`, and the supporting `brandedPDFDocument` / `newPDFDocument` / `pdfBranding` helpers into a new `internal/render` package. The existing methods on `ExportService` become thin facades that call the new package. No behavior change. No fpdf removal. This is the seam that makes the `Renderer` interface possible.

- **Slice 1 (Phase 0) — scaffolding and baseline capture.** Renderer interface, TypstRenderer that shells out to the bundled binary directly, template discovery, `tools/tune/` separate Go module, `dixiedata-tune` CLI with `render`/`list-templates`/`list-records`/`capture-baseline`/`compare` subcommands, baseline capture of every fpdf export to `tools/tune/baseline/`, annotation convention, smoke test that produces a trivial PDF. After this slice, the Typst pipeline works for a one-line template; all fpdf behavior is unchanged.

- **Slice 2 (Phase 1a) — first real record card.** `templates/soldier_landscape.typ` renders all current Soldier record card fields with the current visual design. `templates/common/theme.typ` filled in with real palette and type-scale. `templates/common/components.typ` with reusable functions. `dixiedata-tune compare` works for Soldier landscape. At least one Soldier record passes the "close enough to be recognizable" visual bar.

- **Slice 3 (Phase 1b) — all 6 record card variants.** `soldier_portrait.typ`, `spouse_landscape.typ`, `spouse_portrait.typ`, `widow_landscape.typ`, `widow_portrait.typ` — clones of the Soldier landscape pattern.

- **Slice 4 (Phase 1c) — bulk export wiring.** `ExportFullDatabasePDF` renders through the Typst path by default. A UI toggle in `share.templ` lets the user fall back to the fpdf path. All existing fpdf tests pass. The bulk export's visual output is recognizable as the same export.

- **Slice 5 (Phase 2a) — special-purpose templates.** `biography_appendix.typ`, `anniversary.typ`, `analytics_summary.typ`, `group_divider.typ`. The corresponding DixieData export entry points use the Typst path; fpdf remains as fallback.

- **Slice 6 (Phase 2b) — minimal static archive unification.** `templates/static_archive_index.typ` produces the static archive page structure as Typst HTML output. The existing `staticArchiveIndexHTML` constant in `internal/archive/static_archive.go` is replaced. The 27 alpha-variant rgba duplications are gone. The static archive's interactivity (search, image overlay zoom) is intentionally deferred to a separate static-archive overhaul.

- **Slice 7 (Phase 3) — retire fpdf.** Remove the fpdf path; remove `go-pdf/fpdf` from `go.mod`; delete `internal/archive/pdf_layout.go`; delete `FpdfRenderer`; remove the UI toggle. Update the audit findings documents. (Done.)

### Data flow

- Each export entry point becomes a single call: `Registry.Render(ctx, settings, recordType, data, out)`.
- The `data` is a `map[string]any` containing the record, its sub-records, branding, app metadata, and the original `PrintSettings` under `options`.
- The template reads via `sys.inputs.soldier`, `sys.inputs.options`, `sys.inputs.branding`, `sys.inputs.app`.

### Printer-friendly override

- Applied at the template level. Each template reads `sys.inputs.options.printerFriendly` and decides its own behavior (drop footer, switch palette to black-on-white).
- The `theme.typ` exposes a `printer-friendly` palette as a `let` binding. Templates that need it import it conditionally.

## Testing Decisions

A good test exercises external behavior, not implementation. The migration is renderer-internal, so the tests are:

- **Smoke tests.** Render a known record through each template; assert the output is a non-empty PDF. Reused across phases.
- **Metadata extraction.** Assert the template metadata comment block parses correctly; record types and orientation are detected.
- **Side-by-side text diff.** The `compare` subcommand produces a text-level diff between fpdf and Typst outputs; assert that the same set of field labels and values appear in both.
- **Visual regression.** Manual. Side-by-side PDFs viewed in a PDF viewer. No automated visual diff — Typst output is not byte-identical to fpdf output by design.
- **Test surface stability.** All existing tests in `internal/archive/export_service_test.go` continue to pass until Phase 3 (the fpdf path remains live). New tests cover the Typst path; old tests stay untouched.

Prior art: the existing `internal/archive/export_service_test.go` tests render real records through the fpdf path. The new tests follow the same shape: open a test record, render, assert on output.

## Out of Scope

- Replacing the Typst binary with a WASM-embedded version. Considered and rejected; the user picked CLI shell-out via direct `exec.Command`.
- A visual diff tool that overlays two rasterized PDFs. The `compare` subcommand produces text diffs; visual diff is manual.
- Letting end users author templates. The author is the developer; templates ship with the application.
- A template marketplace or per-user override directory. The program directory is the only source of templates.
- Migrating the Wails desktop app to use the new renderer. The renderer is a drop-in for the existing methods; the Wails frontend is untouched until Phase 3 confirms the migration is solid.
- Replacing the DixieData database schema or the data access layer. The migration is renderer-only.
- Backing up or migrating existing Local Archives. The new renderer reads the same SQLite database the same way.

## Further Notes

- The audit findings documents (`docs/audit/layout-theming-*.md`) are resolved by this PRD. After Phase 3, the 9 hard-coded RGB triplets, 60+ rgba variants, and 11 correction values are all gone. The audit deliverables should be updated to reflect the new state.
- The `theme.json` audit deliverable (`docs/audit/layout-theming-token-schema.md`) is the source of truth for the design tokens. The Typst `theme.typ` is a near-verbatim port.
- The phased migration means the fpdf path is live for the duration of Phases 0–2. The test surface stays green. Phase 3 deletes it. (Done: the fpdf path is gone. The only PDF export path is the typst-backed one.)
- The tuning tool's `serve` subcommand is a small Go HTTP server with templ-rendered HTML (using the same template engine DixieData uses, since the templates directory is shared). It is not a full web app; it is a developer tool.
- The annotation feedback loop is a convention, not a system. The agent reads `.md` files alongside renders. The convention is: filename ends in `.md`, content is free-form markdown, the most recent one for a `(template, record)` pair is the source of truth.
- The Typst binary is pinned to a specific version (Typst 0.15 as of June 2026). The bundled binary version is documented in `THIRDPARTY.md` and updated when the tool is re-bundled.
- The bundled binary is ~30–50 MB per platform. Three platform binaries in `<program-dir>/bin/` is ~150 MB worst case. This is a one-time install cost.
- **Static archive has its own upcoming audit and overhaul.** The static archive today (in `internal/archive/static_archive.go`) has 27 alpha-variant rgba duplications with the live design system, a 490-line inline `<style>` block, interactive JS for search and image overlay zoom, and a structure that predates the audit. The Phase 2b slice in this PRD is intentionally minimal — it moves the static archive onto Typst HTML output to kill the design-system duplication, but does not preserve visual fidelity or interactive behavior. A separate, follow-up audit and overhaul of the static archive will address structure, interactivity, accessibility, and visual design. The follow-up work is out of scope for this PRD; it will get its own PRD, its own issues, and its own phased plan when prioritized.

---

# UI/UX Audit Findings (June 2026)

This section captures a separate UI/UX audit performed against the live application shell (`frontend/` + `internal/appshell/` + `internal/templates/`). It is independent of the Typst PDF migration above and addresses design-system consistency, accessibility, and small redundancies in the rendered HTML/HTMX surface.

## Current State

DixieData is a single-window Wails desktop app serving an HTMX-driven SPA where Go templates emit complete pages into `body` and one ~3,800-line vanilla JS IIFE progressively enhances them. The design system lives in `<style>` inside `internal/templates/layout.templ:60-230`, not in `app.css` or `tailwind.config.js`. Visual components (`.primary-button`, `.secondary-button`, `.danger-button`, `.field-input`, `.card`, `.pill-link`, `.ghost-link`, `.toast-card`, `.layout-mode-option`, `.google-progress-*`, `.ui-debug-*`) are defined once and reused consistently across 20+ templates.

## Design Language

- **Palette:** `#8d7440` (bronze), `#c5ab68`/`#cfb77a` (gold), `#22303d`/`#1f2b38` (slate), `#6f2c26`/`#54211d` (oxblood), `#fff8e7`/`#f5f1e6` (parchment) — Civil-War archival themed.
- **Type:** `"Helvetica Neue", Arial, sans-serif` body; `.gold` class uses Georgia/serif. Uppercase + `tracking-[0.18em]`–`[0.28em]` for section labels.
- **Breakpoint:** single 1000px split-screen/relaxed toggle.
- **ARIA:** `aria-live` regions on toasts, feedback, merge-review, scratchpad; `[aria-busy]` styling; `aria-pressed` on layout toggle.

## Redundancy Found (Minimal)

1. **Startup placeholder duplicated.** `frontend/index.html:14-22` and `internal/appshell/app.go:155-181` emit the same loading card with slightly different copy.
2. **Layout breakpoint logic duplicated.** `internal/templates/layout.templ:14-32` inline script and `frontend/app.js:152-158` both implement the same 1000px constant (`splitScreenBreakpointPx`).
3. **Inline `<style>` in `frontend/index.html:6-13`** duplicates CSS already in `layout.templ:38-44`.
4. **2–3 inline-styled `<button>` strings** hand-built in Go handlers (`app.go:1379`, `google_handlers.go:163`) instead of templ components.

## Genuine Gaps (Priority Order)

### Priority 1 — Form labels disconnected from inputs (Accessibility)

- **Evidence:** `internal/templates/soldier_card.templ:177-244`, `entry_form.templ:60,145,449`, `layout.templ:504-522` — every `<label class="…">` lacks `for=`, every `<input>` lacks `id=`.
- **User impact:** Screen readers cannot announce the input's purpose; clicking the label does not focus the field. Largest concrete a11y deficit.
- **Solution:** Mechanical `for=`/`id=` pairing across templ files. Templ variables interpolate cleanly.
- **No redundancy:** Confirmed — no existing label/input pairings to conflict with.
- **Scope estimate:** ~30 form fields, ~30 minutes of work, no new architecture.

### Priority 2 — Hex tokens scattered, no CSS variables

- **Evidence:** `#8d7440`, `#22303d`, `#6f2c26`, `#c5ab68` recur 20–40+ times across `frontend/app.css` and `internal/templates/*.templ`. `tailwind.config.js:5` `theme.extend: {}` is empty.
- **User impact:** Theme tweaks (e.g. dark mode) require find/replace in dozens of files. Not user-visible today but blocks future consistency work.
- **Solution:** Promote 4–6 most-used colors to CSS custom properties in `:root` inside `layout.templ:38` (the existing inline-style home). Swap ~10 most-common literals. Do NOT migrate every `rgba(…, 0.x)` alpha variant — ~30 distinct alpha values exist for legitimate layering reasons.
- **Scope estimate:** ~1 hour. Defer unless dark-mode or theme toggle is on the roadmap.

### Priority 3 — Remove duplicate startup placeholder

- **Evidence:** `frontend/index.html:14-22` and `internal/appshell/app.go:155-181` both emit the same loading card.
- **User impact:** Drift risk — copy or styling changes in one place silently fail to apply to the other.
- **Solution:** Have `app.go:handleCalendar` redirect (HTTP 302) to `/` for first-load, OR delete the `index.html` loading card and rely on the Go-rendered one. Pick one source of truth.
- **Scope estimate:** ~10 minutes.

## Concerns (Not Gaps)

- `frontend/app.js` is 3,789 lines in one IIFE. Functionally organized: storage helpers, drafts, browse, toasts, HTMX bridge. Splitting into ES modules would add CSP surface without functional gain in a Wails sandbox.
- `internal/appshell/app.go` is 2,230 LOC — the "god class" extraction work is already tracked (issue #42). Not a UI/UX issue.
- `tailwind.config.js` `theme.extend` is empty — adding tokens there is a parallel option to CSS variables, but the inline `<style>` block in `layout.templ` is the actual stylesheet entry point today.

## Design Philosophy Compliance

**High compliance.** The team has done the hard part: extracted consistent button/input/card classes, used them across 20+ templates, namespaced client storage (`dixiedata.*`), and given HTMX swap targets stable IDs. The inline-hex-color pattern is the main shortcut, and it is consistent within itself. The app reads as one product, not 12 pages stitched together.

## NOT Recommended

- Refactoring `frontend/app.js` into ES modules. It works, the Wails sandbox doesn't need them, and the file is logically sectioned.
- Migrating every `rgba(…, 0.x)` to a token — at least 30 distinct alpha variants exist for legitimate layering reasons (`.12`, `.18`, `.25`, `.35`, `.45`, `.55`, `.7`, `.8`).
- Extracting every Go `fmt.Fprintf` HTML to a templ file — only `app.go:1379` and `google_handlers.go:163` are worth converting (2 instances).
- Adding a UI component library. The existing `.primary-button` / `.secondary-button` / `.field-input` already are the component library.

## Recommendation

1. **Add `for=`/`id=` pairs to form labels** (Priority 1) — biggest user-visible win, mechanical change. Filable as one GitHub issue.
2. **Delete the duplicate startup placeholder** (Priority 3) — small, prevents drift. Filable as one GitHub issue.
3. **Promote the 4–6 most-used hex colors to CSS variables** (Priority 2) — only if a future theme toggle is on the roadmap. Optional, file as a separate issue gated on roadmap decision.

Each priority is small enough to ship in a single pass later. The work is non-blocking and orthogonal to the Typst PDF migration in the rest of this PRD.

---

# UI/UX Audit Round 2 (June 2026): Density & Feedback Placement

A second audit pass, driven by user feedback that the existing design "looks right" but the page flow forces too much scrolling and that action confirmations (e.g. backup load) are buried mid-page instead of being visible. This section captures both problems and proposes targeted fixes.

## Context

The existing app already has a toast system (`frontend/app.js:2400-2421`, `layout.templ:150-167`, toast region at `layout.templ:423`). Toasts are fixed top-center, `z-80`, `aria-live="polite"`, with a dismiss button. **They currently auto-dismiss at 4200 ms** (`app.js:2421`) — Round 2 changes this to manual-dismiss-only. The Go backend already pipes `X-DixieData-Toast` headers through `setToastHeader` / `setToastHeaderWithType` (`exports_handlers.go:251-268`); the client reads them at `app.js:2842-2874`. Many handlers already trigger toasts; some still write to inline status divs instead.

## Problem 1 — Density / Scrolling

The app is functionally clean but several surfaces grow too tall, forcing unnecessary scroll on 13" laptop screens. Findings:

### Data rows / tables
- **`browse.templ:25-145`** — Browse filters panel is 380+ px tall (column toggles + selection status). Collapse column toggles behind `<details>` when 4+ columns selected.
- **`browse.templ:51-56`** — page-size options are 50-250; add 25-row option for screen-bound archives.
- **`calendar.templ:228`** — day cells are `min-h-[115px]` (6 rows × 7 cols = ~700 px before any click). Drop to `min-h-[90px]` below `xl`.
- **`calendar.templ:74-83`** — quote panel + month header strip = ~280 px of vertical chrome before the grid renders.

### Buttons / controls
- **`layout.templ:103`** — `.ghost-link` has no padding, sits shorter than adjacent pill buttons in `browse.templ:96`, `entry_form.templ:789-793`. Add `py-1`.
- **`entry_form.templ:813-819`** — radio cards (`.rounded-2xl px-4 py-3` × 2) take 60 px for a binary choice. Collapse to inline.
- **`share.templ:105-131`** — Export button cards stack title + description; 7 cards = ~450 px before any status appears. Collapse to single line.

### Forms / sections
- **`entry_form.templ:119`** — main form is flat `card rounded-3xl p-5 sm:p-8 space-y-5`. Bio/Notes/Source Records should wrap in `<details>` (Soldier detail already does this at `soldier_card.templ:450-540`).
- **`entry_form.templ:284-289`** — 4-5 Special-case panels each ~80 px of chrome stacked vertically.

### Modals / dialogs
- **`layout.templ:489`, `share.templ:197, 551`** — three modals use `p-6 sm:p-8` (32 px each side). Tighten to `p-6 sm:p-7`.
- **`layout.templ:518`** — feedback textarea defaults to `rows="5"` (~120 px); reduce to 4.
- **`soldier_card.templ:332-368`** + **`calendar.templ:49-77`** — Export popouts overlap page content below `xl` (`z-20`, not modal-grade).

## Problem 2 — Buried Feedback

The app has 16 inline status divs. Some host actionable content (must stay inline); most just announce action results (should become toasts).

### Existing toast system summary
- `showToast(message, kind)` (`app.js:2400-2421`)
- `.toast-card` (`layout.templ:150-167`) — fixed top-center, dismiss button
- Toast region `[data-toast-region]` (`layout.templ:423`) — `aria-live="polite" aria-atomic="true"`
- Auto-dismiss at 4200 ms (currently). **Change to manual-only for Round 2.**
- Backend: `X-DixieData-Toast` / `X-DixieData-Toast-Type` headers, `setToastHeader` helpers

### Buried feedback sites — full inventory

| File:line | Action | Severity | aria-live? | Toast-safe? | Notes |
|---|---|---|---|---|---|
| `share.templ:194` (`#share-status`) | Export/Import/Feedback-log + Merge review | Mixed | No | Mostly yes; merge already triggers `setToastHeader` from `app.go:99` | Memorial JSON preview at `app.go:1377-1381` embeds a Confirm button — must stay inline |
| `share.templ:548` (`#google-status`) | 10+ Google Drive/Calendar/Sheets buttons | Mixed | No | Yes for connect/disconnect/sync; no for `google_handlers.go:163` dry-run (embeds "Run Sync Now" button) | |
| `entry_form.templ:441` (`#form-image-import-status`) | Add Images From Computer | Info | No | Yes — `app.go:777` already redirects | |
| `soldier_card.templ:589` (`#soldier-export-status`) | PDF/JPG export | Info | No | Partial — contains clickable `file://` link. Use `runtime.BrowserOpenURL` + toast | |
| `soldier_card.templ:647` (`#image-download-status`) | Image add/download/delete | Info | No | Yes for counts; no for in-place badge refresh (`hx-swap="none"`) | |
| `soldier_card.templ:535` (`#review-resolution-status`) | Flag/Resolve review | Critical | No | Yes — `app.go:166, 175` already toast | |
| `entry_form.templ:794` (`#settings-status`) | Initialize Data (typed-confirm) | Critical | No | Yes after typed-confirm passes | |
| `entry_form.templ:897` (`#settings-update-status`) | Check/Apply update + Export Backup | Critical | No | Yes for "Saved update source"; keep `LastApply` card inline | |
| `entry_form.templ:804` (`#settings-orphan-results`) | Image orphan scan/cleanup | Info | No | Yes for action; keep result list inline | |
| `entry_form.templ:824` (`#settings-quality-results`) | Quality scan + apply | Info | No | Yes for action; keep result list inline | |
| `insights.templ:33` (`#insights-export-status`) | Insights PDF export | Info | No | Same `exportLinkMarkup` issue | |
| `insights.templ:108` (`#insights-audit-status`) | Duplicate audit | Info | No | Keep "Last scan" timestamp inline (persistent metadata); migrate action result to toast | |
| `calendar.templ:33` (`#calendar-export-status`) | Monthly PDF export | Info | No | Same `exportLinkMarkup` issue | |
| `share.templ:403` (`#merge-review-loaded-status`) | Merge Review initial render | Info | Yes | No — initial render metadata, not action feedback | |
| `layout.templ:525` (`#feedback-form-status`) | Feedback submit | Info | Yes | Yes — `app_feedback.go:90` already toast; status div is redundant. Drop it. | |
| `layout.templ:450` (`data-floating-scratchpad-status`) | Scratchpad save from floating dock | Info | Yes | Yes — optional migration | |

### Critical findings — migrate first
1. **`#share-status` for Merge Review Keep Local/Shared/Both** — high-traffic critical; inline text is overwritten by next merge. Already has `setToastHeader` from `app.go:99, 129, 146`.
2. **`#google-status`** — 10+ buttons funnel through one tiny div; during 30s sync the status is unreadable deep in the panel.
3. **`#settings-update-status`** — Software updates + backup export results sit at the bottom of a 200-line panel; user has already scrolled away.
4. **`#review-resolution-status`** — destructive-ish action; status buried under the Review Queue card.
5. **Export link divs** (`#soldier-export-status`, `#insights-export-status`, `#calendar-export-status`) — success path: trigger `runtime.BrowserOpenURL` + toast. Error path: keep inline so user can retry.

### NOT candidates for toast migration
- Memorial JSON preview confirm button (`app.go:1377-1381`)
- Google dry-run sync button (`google_handlers.go:163`)
- Merge review loaded count (`share.templ:403`) — initial render metadata
- Orphan/quality result lists (`entry_form.templ:804, 824`) — contain actionable lists

## Proposed Changes (priority order)

### Priority 1 — Switch toast to manual-dismiss-only
- Remove the 4200 ms auto-dismiss timer (`app.js:2421`); toast stays until user clicks the dismiss button.
- ~10 minutes. Single-file change. Reversible.

### Priority 2 — Migrate buried feedback to manual-dismiss toasts
- Add `setToastHeader` to ~6 Go handlers where missing: `google_handlers.go` (connect/disconnect/sync, 6+ spots), `soldiers_handlers.go` image routes, `settings_handlers.go` initialize, `app_update.go` check/apply.
- Remove or repurpose status divs in `share.templ:194`, `soldier_card.templ:589,647`, `entry_form.templ:794`, `insights.templ:33`.
- Keep inline status divs that host actionable content (Memorial JSON confirm, Google dry-run, orphan/quality lists).
- For export links: replace `exportLinkMarkup` with `runtime.BrowserOpenURL` + toast "PDF saved to Downloads".
- Estimated effort: ~3 hours.

### Priority 3 — Density pass (data rows + buttons)
- Collapse Browse column toggles behind `<details>` (`browse.templ:25-145`).
- Add 25-row page-size option (`browse.templ:51-56`).
- Drop calendar cell `min-h-[115px]` → `min-h-[90px]` below `xl` (`calendar.templ:228`).
- Add `py-1` to `.ghost-link` (`layout.templ:103`).
- Collapse radio cards in `entry_form.templ:813-819`.
- Estimated effort: ~2 hours.

### Priority 4 — Density pass (forms + modals)
- Wrap Bio/Notes/Source Records in `<details>` (`entry_form.templ:382`).
- Tighten modal padding `p-6 sm:p-8` → `p-6 sm:p-7` (3 sites).
- Drop feedback textarea default rows 5 → 4 (`layout.templ:518`).
- Estimated effort: ~2 hours.

## Design Philosophy Compliance

All recommendations keep existing `.primary-button / .secondary-button / .pill-link` tokens and the `aria-live="polite"` toast region. No new colors, no new component classes. The split-screen / relaxed layout-mode toggle already lets users choose density; these fixes reduce the *minimum* density so even relaxed mode fits more on a 13" laptop without scroll.

## NOT Recommended

- Adding a toast queue / stacking UI — current `flex-direction: column` in `.toast-region` (`layout.templ:140-148`) already stacks. More than 3 toasts becomes visual noise.
- Re-architecting with a global notification drawer — over-engineering for the 16 sites in scope.
- Migrating status-then-redirect pattern (e.g. `merge_review` import → toast in URL state, `app.js:2450-2455`) — already works.
- Adding a compact/comfortable data-row toggle on Browse — page-size dropdown already gives users control.

## Recommendation

Ship in this order (each is small enough for one PR):

1. **Priority 1** — Switch toast to manual-dismiss (~10 min, one-liner behavior change).
2. **Priority 2** — Migrate buried feedback (~3 hrs).
3. **Priority 3** — Density pass A (~2 hrs).
4. **Priority 4** — Density pass B (~2 hrs).

All orthogonal to the Typst PDF migration above and to the Round 1 issues (#51, #52, #53).

