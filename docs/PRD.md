# Typst-based PDF Template System

## Problem Statement

The DixieData PDF export system is built on a hand-rolled `go-pdf/fpdf` coordinate-grid renderer. Adding or removing a field requires editing Go source; changing the visual theme (colors, fonts, margins) requires editing nine hard-coded RGB callsites and sixty-plus CSS rgba variants across two layers. The renderer bakes in eleven "correction values" (e.g. `pageHeight-16` at `internal/archive/pdf_layout.go:1128`, `pdf.GetY() > 230` at `internal/archive/pdf_layout.go:1160`) that exist only because fpdf's layout model is primitive. Orphan/widow control is weak. The visual design system is duplicated between the PDF export path and the static-archive HTML path.

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

The fpdf path remains the default for all exports during a phased migration. The Typst path becomes the default for migrated exports. Eventually the fpdf path is removed.

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
10. As a researcher using DixieData, I want to export a printable archive with a registry of all Person Records, so that I have a print-friendly index of the Local Archive.
11. As a researcher using DixieData, I want a group divider page between records in a bulk export when I group by unit or pension state, so that the output is organized.
12. As a researcher using DixieData, I want all export options (orientation, printer-friendly, include-images, group-by, filter, sort) to continue working unchanged, so that the migration is non-breaking.
13. As a developer working on DixieData, I want to add a new export template by creating a single `.typ` file, so that I do not need to edit Go source.
14. As a developer working on DixieData, I want to add a new field to a record card by editing one `.typ` file, so that field selection is a template concern.
15. As a developer working on DixieData, I want to change a color or font by editing one `theme.typ` file, so that theme tokens are centralized.
16. As a developer working on DixieData, I want to change page margins by editing one `theme.typ` file, so that geometry is centralized.
17. As a developer working on DixieData, I want the fpdf path to remain functional during the migration, so that the test surface stays green.
18. As a researcher using DixieData, I want exports to look the same as they do today (close enough, not byte-identical), so that the migration is visually non-disruptive.
19. As a developer working on DixieData, I want orphan/widow control handled by the renderer, so that I do not have to hand-roll keep-together logic.
20. As a developer working on DixieData, I want the static-archive HTML to use the same design system as the PDF exports, so that the duplication between `internal/archive/static_archive.go` and `internal/templates/layout.templ` is eliminated.
21. As a developer iterating on template design, I want a CLI tool that renders a template against a real record in seconds, so that I can iterate without rebuilding DixieData.
22. As a developer iterating on template design, I want a web UI in the tuning tool that shows the rendered PDF inline, so that I can iterate visually.
23. As a developer iterating on template design, I want a side-by-side comparison of the fpdf baseline and the Typst render for the same record, so that I can see how close I am to the original.
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
35. As a researcher, I want the static archive to remain interactive (search, image overlay zoom), so that the unification does not regress UX.
36. As a developer, I want the printer-friendly export option to suppress the footer and switch the palette to black-on-white, so that ink usage is reduced.
37. As a developer, I want the fpdf path to be removed in a final phase, so that the Go module is clean.
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
- The Go side uses `github.com/Dadido3/go-typst` (CLI shell-out) to invoke the bundled binary.
- Data flows from Go to the template via `sys.inputs` (a JSON object).
- Errors from `go-typst` are returned as structured Go errors with line/column info.

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

- **Phase 0 — scaffolding and baseline capture.** Renderer interface, TypstRenderer skeleton, template discovery, smoke test that produces a trivial PDF. Baseline capture: render every record through every fpdf export and save to `tools/tune/baseline/`. The tuning tool's CLI skeleton. Annotation convention defined.
- **Phase 1 — bulk export migration.** Six record-card templates (Soldier/Spouse/Widow × Landscape/Portrait) plus `group_divider.typ` and `biography_appendix.typ`. The bulk export uses Typst by default; fpdf remains as fallback behind a UI toggle.
- **Phase 2 — remaining exports and static archive.** `anniversary.typ`, `analytics_summary.typ`, `printable_archive_registry.typ`. Static archive unification: `static_archive_index.typ` (HTML output) wrapping in a JS shell.
- **Phase 3 — retire fpdf.** Remove the fpdf path; remove `go-pdf/fpdf` from `go.mod`; delete `internal/archive/pdf_layout.go`. Update the audit findings documents to reflect the new state.

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

- Replacing the Typst binary with a WASM-embedded version. Considered and rejected; the user picked CLI shell-out via `go-typst`.
- A visual diff tool that overlays two rasterized PDFs. The `compare` subcommand produces text diffs; visual diff is manual.
- Letting end users author templates. The author is the developer; templates ship with the application.
- A template marketplace or per-user override directory. The program directory is the only source of templates.
- Migrating the Wails desktop app to use the new renderer. The renderer is a drop-in for the existing methods; the Wails frontend is untouched until Phase 3 confirms the migration is solid.
- Replacing the DixieData database schema or the data access layer. The migration is renderer-only.
- Backing up or migrating existing Local Archives. The new renderer reads the same SQLite database the same way.

## Further Notes

- The audit findings documents (`docs/audit/layout-theming-*.md`) are resolved by this PRD. After Phase 3, the 9 hard-coded RGB triplets, 60+ rgba variants, and 11 correction values are all gone. The audit deliverables should be updated to reflect the new state.
- The `theme.json` audit deliverable (`docs/audit/layout-theming-token-schema.md`) is the source of truth for the design tokens. The Typst `theme.typ` is a near-verbatim port.
- The phased migration means the fpdf path is live for the duration of Phases 0–2. The test surface stays green. Phase 3 deletes it.
- The tuning tool's `serve` subcommand is a small Go HTTP server with templ-rendered HTML (using the same template engine DixieData uses, since the templates directory is shared). It is not a full web app; it is a developer tool.
- The annotation feedback loop is a convention, not a system. The agent reads `.md` files alongside renders. The convention is: filename ends in `.md`, content is free-form markdown, the most recent one for a `(template, record)` pair is the source of truth.
- The Typst binary is pinned to a specific version (Typst 0.15 as of June 2026). The bundled binary version is documented in `THIRDPARTY.md` and updated when the tool is re-bundled.
- The bundled binary is ~30–50 MB per platform. Three platform binaries in `<program-dir>/bin/` is ~150 MB worst case. This is a one-time install cost.
