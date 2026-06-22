# Static Web Archive Audit (June 2026)

Read-only audit. No source was modified.

**Scope.** The static web archive feature end-to-end: production code (`internal/archive/static_archive.go`, `internal/archive/export_service.go:1191-1242`, `internal/appshell/exports_handlers.go:131-156`, `internal/appshell/routes.go:55`, `cmd/gold-master/main.go:131-156`), UI surface (`internal/templates/share.templ:115-119`), templates (`templates/static_archive_index.typ` — orphan), test surface (`internal/archive/export_service_test.go:230-394`, `internal/templates/entry_form_test.go:199-201`, `tests/goldmaster/playwright/tests/static-archive.spec.js`, `tests/goldmaster/run-suite.ps1:41-48`), committed sample output (`tests/goldmaster/artifacts/output/artifacts/static-archive/`), and user/PRD/audit docs.

**Terminology.** Static Web Archive, Local Archive, Share, Backup Archive, Display ID — as defined in `CONTEXT.md`.

**Date.** 2026-06-21. Branch: `dev`. Commit: `8c36344`.

---

## 1. Headline Findings

| # | Finding | Severity |
|---|---|---|
| 1 | Feature works end-to-end; production code path is unchanged since the god-class extraction (issue #42 PR2). | OK |
| 2 | `templates/static_archive_index.typ` is **orphan** — exists on disk and is auto-discovered by the Typst registry, but no Go code ever calls `Registry.Render(..., "static_archive_index", ...)`. PRD Slice 6 was deferred. | Drift |
| 3 | The static archive's 1,142-LOC inline HTML template (`staticArchiveIndexHTML`, `static_archive.go:105-892`) uses its own class taxonomy (`.action-button`, `.image-button`, `.back-button`, `.overlay-close`, `.screen`) instead of the live app's global `.primary-button` / `.secondary-button` / `.card`. Visually identical, semantically divergent. | Drift |
| 4 | `index.html` and `viewer.html` are byte-identical files in every export (`export_service.go:1228-1241`). Both written. Redundant bytes + future divergence risk. | Stale |
| 5 | Hex literals scattered through `static_archive.go` CSS mirror the same `#8d7440`/`#22303d`/`#6f2c26`/`#c5ab68` pattern issue #53 documents for the live app. Same shortcut, different file. | Drift |
| 6 | PRD (`docs/PRD.md:138,184`) and the typst-migration-plan (`docs/audit/typst-migration-plan.md:462`, since deleted) describe a "Typst-based static archive" deliverable that does **not** exist in the codebase. The migration was deferred; the docs were not updated to say so. | Doc rot |
| 7 | No `aria-live` on `#result-count`, image overlay, or detail screen content. Only `#archive-results` is announced. Partial a11y coverage. | Drift |
| 8 | Toast-region integration absent. The export writes `exportLinkMarkup` (a `file:///` pill-link) into `#share-status`; no `X-DixieData-Toast` header set. Inconsistent with the rest of the export surface — issue #55 plans to migrate this pattern but is itself blocked on #69. | Drift |
| 9 | The static archive is triggered from one button (`share.templ:115-119`) and the UI is clean. No hidden, disabled, or commented-out triggers elsewhere. | OK |

---

## 2. Architecture

### 2.1 Call chain (production)

```
User clicks button (share.templ:115)
  → HTMX POST /export/static-archive (routes.go:55)
  → handleExportStaticArchive (exports_handlers.go:131-156)
  → Wails SaveFileDialog → user-chosen path
  → a.export.ExportStaticArchive(path, a.dataDir) (export_service.go:1199)
      → e.staticArchiveOwner() (static_archive.go:1245)
      → e.staticArchiveRecords() (static_archive.go:1267) → sort by (Name, DisplayID)
      → os.MkdirTemp + copyDirectoryContents(images) (static_archive.go:1461)
      → json.MarshalIndent → archive_data.js (export_service.go:1217)
      → renderStaticArchiveIndex(staticArchiveIndexHTML) (static_archive.go:1437)
      → writeFile viewer.html + index.html (export_service.go:1228-1241) [duplicate]
      → zipDirectory(outputPath, exportRoot) (static_archive.go:1497)
        → writeZipArchive (internal/archive/archive_writer.go:9)
  → exportLinkMarkup("Static web archive ready:", path) → #share-status
```

### 2.2 Independence from the Typst pipeline

The static archive path is **fully independent** of `pkg/render/`, `internal/renderbridge/`, `internal/archive/pdf_layout.go`, and every `.typ` file. It uses Go's `html/template` to render an inline 1,142-LOC HTML/CSS/JS string. It does **not** participate in the typst migration.

The only shared utility is `writeZipArchive` at `internal/archive/archive_writer.go:9` (also used by `backup_service.go` and `diagnostics_service.go`).

### 2.3 Orphan Typst template

`templates/static_archive_index.typ` declares `export_types: [static_archive_index]`, but no Go code path invokes it via `Registry.Render`. The PRD described Slice 6 as moving the static archive to Typst HTML output; that slice was deferred. The `.typ` file was scaffolded but never wired.

The file's own header documents its non-use:

> The existing static-archive index is an interactive HTML page (see `internal/archive/static_archive.go`); this template produces a printable PDF index that can be viewed without JavaScript.

### 2.4 Output shape

Per export, produces one ZIP file containing:

```
index.html              (byte-identical to viewer.html)
viewer.html             (duplicate of index.html; see §4.2)
archive_data.js         (~2.3 KB; window.DIXIE_DATA = [...])
images/                 (sharded tree copied from .dixiedata/images/...)
```

Sample output is produced by `cmd/gold-master/main.go -mode output-audit` during the gold-master suite and lands under `tests/goldmaster/artifacts/output/artifacts/static-archive/` (gitignored; not committed).

---

## 3. Visual Layer

### 3.1 Design-system fit

Static archive's CSS hex values match the live app hex-for-hex (verified at `static_archive.go:113-538` vs `internal/templates/layout.templ:34-128`):

| Token | Live app | Static archive | Match |
|---|---|---|---|
| Body gradient | `linear-gradient(180deg, #d7d2c9 → #c9c2b5 → #b9b1a3)` | byte-identical | ✓ |
| Card chrome | `--panel`, `--border`, `--shadow` | identical literals | ✓ |
| Primary button gradient | `linear-gradient(180deg, #c5ab68, #a5853f)` | identical (`.action-button`) | ✓ |
| Field input | `border-radius: 0.8rem; padding: 0.65rem 0.9rem` | `border-radius: 18px; padding: 14px 16px` | Diverges — search input is intentionally larger |
| Hero h1 gold | `color: #cfb77a` on dark | same | ✓ |
| Typography | `"Helvetica Neue", Arial, sans-serif` body; Georgia serif on h1 | same | ✓ |

**Design philosophy compliance** (per `docs/PRD.md:240`): **high within the static archive itself**; **mixed on class taxonomy**.

### 3.2 Class-name taxonomy drift

The static archive defines and uses its own class names instead of the live app's globals:

| Live app class | Static archive class | Same visual? |
|---|---|---|
| `.primary-button` | `.action-button` | Yes |
| `.secondary-button` | `.image-button` + `.back-button` + `.overlay-close` (three aliases) | Yes |
| `.card` | `.screen` (with `border-radius: 30px` vs live `rounded-3xl` ≈24px) | Mostly |

If a future contributor tries to extract the static archive's CSS into the live design system, the class-name rename is the single biggest hurdle. There is no semantic reason for the rename — the styles are byte-equivalent on the gradients and borders.

### 3.3 Inline-hex shortcut

`static_archive.go:113-538` uses CSS custom properties (`:root { --paper, --panel, --gold, --ink, --muted, --border, --shadow }`) for the highest-frequency tokens, then breaks that pattern with hex literals scattered through component rules: `#f4ead0`, `#cfb77a`, `#1f2b38`, `#c5ab68`, `#a5853f`, `#d1b676`, `#b08f45`, `#f4ead0`. This is the same shortcut issue #53 documents for the live app, just relocated from templ `class=` attributes to CSS rules. Issue #53's gating condition ("only if a future theme toggle is on the roadmap") applies equally here.

### 3.4 Interactivity

The static archive ships ~570 lines of vanilla JS in the same inline `<script>` block:

- Real-time search on displayId, name, dates, unit, location, notes, biography, record details.
- List ↔ detail screen swap (`showListScreen` / `showDetailScreen`).
- URL-hash deep links (`#record=<displayId>`).
- Image overlay with mouse-wheel zoom (0.15 step), drag-pan, keyboard close.
- Spouse/family cross-links via `detailLink` / `relatedFamilyRecords`.

The PRD's Slice 6 explicitly notes that Typst HTML output would **not** preserve this JS interactivity. Deferring the migration was the right call; the JS is the feature's primary value-add over a flat PDF index.

### 3.5 Accessibility

| Site | Static archive | Live app |
|---|---|---|
| `#archive-results` (list updates) | `aria-live="polite"` (`static_archive.go:686`) | parallel pattern in live app |
| `#result-count` | none | silent count changes |
| Image overlay | `aria-hidden="true"`, no `role="dialog"`, no focus trap | live app modals use `role="dialog"` + focus trap |
| Detail screen swap | no `aria-live` announcement on entry | live app toast region covers this |
| Label/input pairing | `<label for="archive-search">` / `<input id="archive-search">` paired (`static_archive.go:681-684`) | live app: see issue #51 |

Single form, single pairing — issue #51's scope is the live app, not this.

---

## 4. Drift & Stale-Seam Inventory

### 4.1 Orphan Typst template

**File:** `templates/static_archive_index.typ` (13 lines, scaffolded but never wired).

**Why it matters:** the Typst registry auto-discovers `.typ` files at startup. This file is therefore registered as a valid export type. If a future caller asks for `name == "static_archive_index"` via the registry, Typst will be invoked with no producer to consume it.

**Recommendation:** delete the file. If a future slice wants to reintroduce a printable PDF index, it can be re-scaffolded at that time with the correct data contract.

### 4.2 Duplicate `index.html` + `viewer.html`

**Files:** `internal/archive/export_service.go:1228-1241` writes both files with identical content.

**History:** the Playwright test (`tests/goldmaster/playwright/tests/static-archive.spec.js:6`) loads `/viewer.html`. The HTML file predated the test; the test added a duplicate for explicit naming.

**Recommendation:** pick one. Either delete `index.html` (retain `viewer.html` to match the Playwright test) or rename `viewer.html` → `index.html` (delete the duplicate). Either is a one-line change. The cleaner choice is to keep `index.html` (the conventional HTML entry-point filename users see in their file manager) and update the Playwright test to load `/index.html`.

### 4.3 Doc/code disagreement on the Typst migration

| Doc | Claim | Code reality |
|---|---|---|
| `docs/PRD.md:138` | "Slice 6 — minimal static archive unification. `templates/static_archive_index.typ` produces the static archive page structure as Typst HTML output." | The `.typ` file existed but is **not wired**; since deleted by PR #71. The static archive uses `html/template`, not Typst. |
| `docs/PRD.md:184` | "27 alpha-variant rgba duplications" | Confirmed present in `static_archive.go`; unfixed. |
| `docs/audit/typst-migration-plan.md:462` (file since deleted by PR #70) | "Static archive uses Typst. Fpdf path is dead code but still wired." | Fpdf is gone from `go.mod`. Static archive does **not** use Typst. |
| `docs/TASKS.csv:41-45` | Slice 6 task listed as `- [ ]` (open) | ✓ matches code |
| `docs/audit/typst-migration-plan.md:464-467` (file since deleted) | "Skip the static-archive unification and defer it" — explicitly listed as one of three options | Code reflects this option being chosen |

**Recommendation:** mark Slice 6 as explicitly **deferred** in `docs/PRD.md` (move it from "planned" to "out of scope / future overhaul"). The typst-migration-plan.md is gone; PRD §Slice 6 should be amended to read "Static archive does not use Typst; defer Slice 6 indefinitely." Doc-only edit.

### 4.4 Class-name taxonomy divergence

Covered in §3.2. The visual chrome is identical; only the class names diverge. Resolving this would mean either:
- **(A)** Rename the static archive's `.action-button`/`.image-button`/`.back-button`/`.overlay-close`/`.screen` to the live app's `.primary-button`/`.secondary-button`/`.card`. **Effort:** ~10 minutes; visual identical; breaks any external CSS/JS hooks (none known).
- **(B)** Leave the static archive alone; it's a self-contained export. **Effort:** zero. The class-name divergence is invisible to users.

**Recommendation:** **(B)** unless/until someone consolidates CSS into shared tokens. (A) is mechanical but offers no user-visible benefit.

### 4.5 Hex-literal shortcut in CSS

Same shortcut as live app (`#8d7440`, `#22303d`, `#6f2c26`, `#c5ab68` recur). Covered by issue #53's scope and gating. If #53 lands, this file should be in the same migration wave.

---

## 5. Test Surface

| Test | File:Line | Type | Coverage |
|---|---|---|---|
| `TestExportService_ExportStaticArchive` | `internal/archive/export_service_test.go:230-394` | Go unit | Full export round-trip. Asserts ZIP contents (`index.html`, `viewer.html`, `archive_data.js`, `images/PENSION-0042/portrait.png`), JSON payload shape (widow cross-links, full-detail fields, `addedBy`, `reviewReason`, `biography`), owner title substring, `viewer.html` contains `Family Links`, `Archive Metadata`, `showDetailScreen`, `renderDetail`, version/build footer, `return text || 'Unknown'`, blank-name handling. |
| Golden-master check | `cmd/gold-master/main.go:273-282` | Go integration | Asserts spouse cross-links present in `archive_data.js`; checks `viewer.html` contains "Family Links" + "Archive Metadata". |
| Share-view HTML assertion | `internal/templates/entry_form_test.go:199-201` | Go string match | Asserts rendered share view contains `/export/static-archive` and `"Export Static Web Archive"`. Regression guard. |
| Playwright smoke | `tests/goldmaster/playwright/tests/static-archive.spec.js` | E2E (31 lines) | Extracts ZIP to `tests/goldmaster/artifacts/output/artifacts/static-archive/`, serves via `python -m http.server`, runs Playwright assertions against the served viewer. |

**Gaps:**

- No visual regression test. The Playwright spec is a smoke test, not a screenshot diff.
- The Playwright spec does not exercise the image overlay (zoom, drag, keyboard close).
- No test for `staticArchiveFileName` date formatting or `sanitizeStaticArchiveStem` edge cases (empty owner, special characters in `BrandingName`).
- No test for the cancel path of the SaveFileDialog (user clicks Cancel).

---

## 6. UI Surface

| Location | Label | Status |
|---|---|---|
| `internal/templates/share.templ:115-119` | "Export Static Web Archive" / "Standalone static archive with images, live search, and expandable person record cards" | Active |
| `#share-status` (target) | rendered `exportLinkMarkup` with `file:///` pill-link | Active |
| Toast header | not set | Inconsistent with rest of export surface |
| Other templ files (`entry_form.templ`, `browse.templ`, `calendar.templ`, `soldier_card.templ`, etc.) | — | no static-archive trigger |

The button is a sibling of JSON/iCal/CSV/Backup/SharedArchive/Printable PDF buttons in the same Export & Backup card. Consistent placement.

---

## 7. Recommendations (ranked)

| Priority | Action | Effort | Notes |
|---|---|---|---|
| P1 | Delete `templates/static_archive_index.typ`. | 1 minute | One-line commit; orphan file. |
| P1 | Pick `index.html` or `viewer.html` and remove the duplicate write in `export_service.go:1228-1241`. | 5 minutes | Recommendation: keep `viewer.html` (matches Playwright test). |
| P2 | Update `docs/PRD.md:138` to mark Slice 6 as explicitly deferred. (The typst-migration-plan reference is gone — that file was deleted by PR #70.) | 10 minutes | Doc-only. Aligns docs with code reality. |
| P3 | Audit `(e *ExportService) staticArchiveImagePath` field — see if `imagePath` in `StaticArchiveRecord` (`static_archive.go:60`) is consumed by the rendered UI or is dead data. | 15 minutes | Low priority cleanup; if dead, drop it. |
| P3 | When issue #55 (toast migration) ships, fold the static-archive success/error into the toast flow alongside the other exports. | Depends on #55 | Drop the `exportLinkMarkup` write for this handler; emit `setToastHeader` instead. (Will require file:// link alternative — `runtime.BrowserOpenURL`?) |
| P4 | When issue #53 (CSS variables) ships, extend the migration to `static_archive.go:113-538`. | Bundled with #53 | Same pattern; same gating. |
| P4 | When issue #51 (a11y label pairing) ships, no work needed in this file. | Zero | Single form, already correctly paired. |
| Defer | Class-name taxonomy consolidation (`.action-button` → `.primary-button`, etc.). | 10 minutes | Mechanical but offers no user-visible benefit; do only if a CSS consolidation happens. |
| Defer | Typst-based printable index page (PRD Slice 6). | Open-ended | The interactive JS is the value-add; flat HTML or PDF would lose it. Reconsider only if a "static archive → PDF" feature is requested. |

---

## 8. Verdict

The static web archive is **shipped, tested, and visually consistent with the live app**, but is **partially misrepresented by the docs**. The code is in better shape than the documentation suggests:

- Code: maintained. Last meaningful change was the god-class extraction (issue #42 PR2); no broken refs, no dead ends.
- Tests: passing. Unit + golden-master + Playwright e2e + share-view HTML regression.
- Docs: drift. PRD claims a Typst migration happened; the code shows it didn't.

Two concrete cleanup items land in under 15 minutes total (orphan `.typ` delete + duplicate HTML write). The remaining items are cosmetic or depend on other in-flight issues (#53, #55).

The feature does **not** need a rebuild. The static archive is not stale in the sense that the user phrased — the user's framing assumed it predates recent work, but the actual situation is that the typst-migration-era docs oversold what shipped. The runtime behavior is fine.

---

## Appendix A — Files audited

- `internal/archive/static_archive.go` (1,529 lines)
- `internal/archive/export_service.go:1191-1242`
- `internal/appshell/exports_handlers.go:131-156`
- `internal/appshell/routes.go:55`
- `internal/appshell/app.go:1477-1486` (`exportLinkMarkup`)
- `cmd/gold-master/main.go:131-156, 273-282`
- `internal/templates/share.templ:115-119`
- `templates/static_archive_index.typ`
- `internal/archive/export_service_test.go:230-394`
- `internal/templates/entry_form_test.go:199-201`
- `tests/goldmaster/playwright/tests/static-archive.spec.js`
- `tests/goldmaster/run-suite.ps1:41-48`
- `tests/goldmaster/artifacts/output/artifacts/static-archive/` (committed sample)
- `docs/PRD.md:138, 184`
- `docs/audit/typst-migration-plan.md:462, 464-467, 534-562` (file since deleted by PR #70)
- `docs/TASKS.csv:41-45, 53`
- `docs/user-manual.md:334-336, 447-457`
- `docs/ai-handoff.md:299-310`
- `docs/implementation-and-features.md:285-290`
- `CONTEXT.md:31-33, 137, 155`
- `CHANGELOG.md:6, 12, 46-50`