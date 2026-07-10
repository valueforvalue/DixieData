# Feature: Articles (free-form essays with Person Record references)

> **Status:** Rescoped 2026-07-05 to align with `docs/agents/feature-protocol.md` (post-#320 sequence: Ticket close-out law, TDD discipline, 5-section feature template, Slice 0 prefactor, Slice 0.5 RED test). 15 locked decisions from the prior spec retained inline; the 13-section meta-zoo around them (Schema sketch / Test plan / Files / Related lists) folded into the Slice plan where each one is actionable. Re-lock after Slice 0 prefactor completes. Locked decision #12 (auto-save / local draft) clarified 2026-07-05 — "no server auto-save" was correct but misleading; the Soldier/new-style localStorage draft persistence IS in scope and reuseable here.
> **Triage labels:** `enhancement`, `needs-triage`, `priority:high`, `cohort:v-next`, `area:backend, area:db, area:docs, area:export, area:frontend, area:templates`
> **Applies to:** `dev` (no PR until issue is triaged and Slice 0 prefactor completes)

## Summary

Add an "Article" entity to the Local Archive — a markdown-bodied long-form essay with inline Person Record references (`[Private John Doe](#person/D-00123)`), live sanitized preview, manual snapshot lifecycle, and PDF / Static HTML / raw markdown exports. Articles ship in v61 as a parallel primary entity to Person Records (sibling to #320 Event Records), riding the same DB and the existing Local / Shared / Backup / Static archive shapes with a new top-level `articles[]` array.

## User story

> As a researcher writing a regimental history or pension-court narrative, I want a dedicated long-form essay surface with markdown source, Person Record inline references, and PDF / Static HTML export, so that I can publish the finished writing without it getting mixed into the per-Person-Record `biography` field or scattered across Source Record bodies.

## Acceptance criteria

- [ ] Researcher can `/articles/new` to create a new Article with title + subtitle + markdown body; lands on `/articles/{id}` with the rendered detail page.
- [ ] Researcher can edit `/articles/{id}/edit`, type markdown, see sanitized HTML preview alongside (debounce 250ms); Save commits via the same Article row.
- [ ] **Local-draft persistence** on both `/articles/new` and `/articles/{id}/edit` — same UX as `/soldiers/new` + `/soldiers/{id}/edit`: form carries a `data-record-persistence` block + `data-draft-key` + `data-draft-reset-path` + `data-record-persistence-kind` so the existing `frontend/app.js` machinery autoloads / autosaves to `localStorage` per keystroke. Draft persists across page reloads until the user clicks Save (which clears the draft) or "Delete saved local draft" (which manually clears with an undo affordance per `entry_form.templ:55-100`).
- [ ] Researcher can search Person Records from a picker modal and insert `[Name](#person/ID)` tokens at the cursor; the rendered detail page resolves each token to a Person Record block.
- [ ] Researcher can hit "Save copy" on a live Article to snapshot it (read-only sibling row); Revisions tab lists snapshots with Restore + Delete.
- [ ] Unknown Display ID in a token renders as `⚠ [Unknown: D-00123]` on PDF / Static HTML output (fail-loud, not silent fallback).
- [ ] Researcher can export the Article to PDF (via Typst `article.typ`), Static HTML (self-contained), or raw `.md` (download).
- [ ] "Cited in" panel appears on Person Record detail page (when ≥1 Article references the Person); hidden when zero.
- [ ] Articles ship in the Shared (.ddshare) and Backup (.ddbak) archive bundles without a new toggle; Static archive gains `window.DIXIE_DATA.articles[]` next to `records`.

## Apply sites (v1 checklist, per AGENTS.md apply-sites law)

- [ ] Backend — `internal/records/article_service.go` (CRUD + ref resolution + token scan + snapshot lifecycle)
- [ ] Backend — `internal/appshell/articles_handlers.go` (every route in the slice-2 stub)
- [ ] Backend — `internal/appshell/routes.go` route registration (`/articles/*` family)
- [ ] Frontend — nav item "Articles" between Records and Export
- [ ] Frontend — `/articles` list view (card grid)
- [ ] Frontend — `/articles/{id}` detail view (split source/preview + refs panel)
- [ ] Frontend — `/articles/{id}/edit` editor (markdown source + sanitized preview + Soldier/new-style local-draft persistence)
- [ ] Frontend — Person Record picker modal (`/articles/{id}/refs/picker`)
- [ ] Frontend — Revisions tab on detail (snapshot list + Restore + Delete + "Save copy" button)
- [ ] Frontend — "Cited in" panel on Person Record detail (`soldier_card.templ`)
- [ ] Exports — `templates/article.typ` + Typst snapshot test (`internal/exportcontract/`)
- [ ] Exports — Static HTML renderer + `window.DIXIE_DATA.articles[]` bundle shape
- [ ] Exports — `/articles/{id}/raw` GET returns `text/markdown` body with Content-Disposition attachment header
- [ ] Archive — Shared + Backup bundles include `articles[]` + `article_refs[]` without a new toggle
- [ ] Migration — v61 schema + forward + partial-down test (`internal/db/v61_migration_test.go`)
- [ ] Glossary — `CONTEXT.md` Article + Article Reference + Article Snapshot entries
- [ ] Docs — `CHANGELOG.md` `[Unreleased]` ### Added bullet
- [ ] Smoke probes — `audit/smoke_articles.mjs` per UI apply site (response shape + `page.url()` after click)
- [ ] Entry-type-gate (Slice X, mirror #363) — server-side guard if any sibling entity's URL is mistakenly routed into `/articles/*`

The feature is not "shipped" until every box is checked.

## Slice plan

Per `feature-protocol.md` 5-section template + `tdd.md` forward-application rule (2026-07-04 TDD protocol anchors all new Tier-2 vertical slices). Slices 2-5 are stubs only; their detailed shape gets filled in by the session that ships them.

### Slice 0 (prefactor — fully detailed)

**Goal:** explore current shape for blockers that would force the Slices 1-5 to take workarounds. Per the "Prefactor before slicing" section of feature-protocol.md; worked example at the same line points to the v60/v61 `event_sources` table-shape bug that a prefactor pass would have caught.

- Run the architecture-improver skill in **lite mode** (no HTML report, no user-facing deliverable).
- Look specifically for: existing module shapes that force workarounds, facades / dispatchers / type unions that Articles will leak across, existing tests that Articles will silently break.
- **Confirm the persistence pattern is generic** at `frontend/app.js:1438-1500` (the Soldier's `data-record-persistence` block + helpers are not Soldier-specific — adding to a new form just needs the same `data-*` attribute set; cite lines 1446-1500 + the helper at 1452-1456 if the prefactor wants to confirm).
- **Files** (touch): unknown until the lite pass runs; 1-3 files typical.
- **Success criteria**: any prefactor identified, committed as its own atomic `### Maintenance` CHANGELOG bullet BEFORE Slice 1; OR explicit "no prefactor needed" finding documented in the issue's commit message (so #320's worked-example anti-pattern doesn't repeat).
- **Regression net**: `go test -short -count=1 ./...` green post-prefactor; orphan-handler probe exit 0.
- **Outcome**: a short blocker list (or empty) that locks the Slice 1 shape. Re-locks the 15 locked decisions at the bottom if the prefactor surfaces a conflict.

### Slice 0.5 (TDD RED — fully detailed)

**Goal:** write the failing test that pins the feature's headline acceptance criterion BEFORE any Slice 1 code lands. Per `tdd.md:111-156` (Step 1 RED). Anchors the rest of the work; the test name itself documents the slice's user-facing acceptance criterion.

- **One test, one assertion path.** Pick the criterion from "Acceptance criteria" that's most central: "Researcher can `/articles/new` to create a new Article; lands on `/articles/{id}` showing title + body + refs panel."
- **Test file**: `internal/appshell/articles_handlers_test.go::TestHandleArticleCRUD_RoundTrip` (table-driven; uses the existing `newStressApp` fixture from the #320 series).
- **Behavior**: create app, POST `/articles/new` with title+subtitle+markdown; assert 303 to `/articles/{id}` + GET that URL returns 200 with the title + body in HTML. Goes RED today (route not registered).
- **Regression net**: this test is the contract. Every Slice that touches the article CRUD path keeps this test green.
- **Do NOT write Slice 1 code yet.** Stop after Step 1 (RED). Step 2 (GREEN) lands in Slice 1's commit.

### Slice 1 (tracer bullet — fully detailed)

**Goal:** the smallest end-to-end vertical that proves the headline criterion works: schema + minimal service + minimum invoker (the `/articles` list page) so the user can SEE one Article exist.

- **Files**: `internal/db/migrations.go` (v61 blocks A + B + user_version bump); `internal/db/v61_migration_test.go` (forward + partial down); `internal/records/article_service.go` (`Create` + `GetByID` only); `internal/appshell/routes.go` (`GET /articles` list + `GET /articles/{id}` detail shell); `internal/templates/articles.templ` (list skeleton); `internal/templates/article_detail.templ` (detail shell — no editor yet, no picker yet); `CONTEXT.md` (glossary entries).
- **Success criteria**: Slice 0.5's test goes GREEN after this slice lands. `make test` green. `node audit/discover_orphan_handlers.mjs` exit 0 (or expected-orphan-route annotation if `/articles/new` POST isn't wired yet).
- **Regression net**: Slice 0.5's `TestHandleArticleCRUD_RoundTrip` stays green; new `internal/db/v61_migration_test.go` asserts `articles` + `article_refs` exist with all columns + indexes after forward, and drop cleanly on down; `internal/records/article_service_test.go::TestCreateArticleMintsARTDisplayID` pins the v180 namespace discipline.
- **Apply-site mapping**: this slice ships the Backend — `article_service.go` + routes + `/articles` + `/articles/{id}` shells + glossary + migration. Does NOT ship: editor, picker, Revisions, exports, archive integration, local-draft persistence. Local-draft persistence lands with the editor in Slice 3 (same commit).

### Slice 2 (stub — fill in at shipping time)

Backend service completion: full CRUD (List / GetByID / GetByDisplayID / Update / Delete), ref resolution (`ResolveRefs(markdown)` token scan), InsertRef. Full handler surface (every route in the apply-sites list). Handler tests (~15 cases per the spec's rough count).

### Slice 2.5 (stub)

Snapshot lifecycle: new service methods `Snapshot` / `Restore` / `DeleteSnapshot`. Routes for create / restore / delete. Snapshot-of-snapshot rejected. Edit-snapshot rejected. ~8 handler test cases. Mirrors #320 slot semantics for the snapshot rows.

### Slice 3 (stub — Tier-3 apply-site unit per route; editor lands as Tier-2 vertical)

UI surface ships one route per commit. Each commit pairs the templ invoker with the matching `audit/smoke_articles.mjs` assertion that verifies both the response shape AND `page.url()` after the click (per `feature-protocol.md:481` anti-pattern guard).

The markdown editor (`/articles/{id}/edit` + `/articles/new`) lands as a **Tier-2 vertical** because the editor + sanitized preview + local-draft persistence cross layers together (templ + JS dispatcher + helper reuse). Editor commit carries:

- `<form data-record-persistence data-draft-key="edit-article-{id}" data-record-persistence-kind="edit" data-draft-reset-path="/articles/{id}/edit" data-draft-record-version="{updated_at|id}">` (per `entry_form.templ:40-50` pattern — confirms the helper reuse path)
- `<div data-record-persistence ... class="..." data-clear-draft-trigger / data-cleared-draft-undo ...>` (the "Local draft only" / "Committed to database" banner + "Delete saved local draft" + undo affordance, per `entry_form.templ:55-105`)
- Markdown source + sanitized preview pane + debounce 250ms JS preview
- Reusable `draftKey(s, isEdit)` helper (or sibling `articleDraftKey` if the signature needs adapting)

### Slice 4 (stub)

Exports (PDF + Static HTML + Raw markdown). PDF: new `templates/article.typ` + `render-person-card(s)` helper extracted from `templates/common/record_card.typ` (single-sourced Person Record rendering). Static HTML: self-contained renderer. Raw: `text/markdown` with attachment header. Typst snapshot test pinned in `internal/exportcontract/`.

### Slice 5 (stub)

Archive integration: Shared (.ddshare) + Backup (.ddbak) bundles include `articles[]` + `article_refs[]`; Static archive emits `window.DIXIE_DATA.articles[]`. Migration reversibility classification in `docs/migrations/reversibility.md`.

### Slice X (stub — NEW, mirrors today's #363 server-gate pattern)

Entry-type gate: if the entry-type / URL-shape distinction ever lands a similar mismatch on Articles (e.g. a handler that should serve only Person Records mistakenly catches an Article id), apply the same catch-all 303-redirect pattern that #363 just shipped for `/soldiers/{id}*` on Event rows. The current slice plan doesn't predict where the boundary lives — Slice 0's prefactor + Slice 0.5's RED coverage may surface it, in which case this slice is added at that time with the #363 pattern as the template.

## Locked decisions (re-confirm after Slice 0 prefactor)

Per the prior spec (2026-07-04 draft). User signed off implicitly via "use the prior spec" — re-lock after Slice 0 surfaces any conflicts. Decision #12 clarified 2026-07-05 to disambiguate "server-side auto-save" (NO) from "local-draft persistence" (YES, Soldier/new pattern).

| # | Decision | Choice | Rationale |
|---|---|---|---|
| 1 | Name | **Article** | `Note` too small, `Essay` too literary, `Document` collides with Source Record. |
| 2 | Storage | **Same DB, new `articles` table** | Cross-table reverse-lookup via `article_refs` is trivial; one backup / restore flow. |
| 3 | Schema layout | **Two tables: `articles` + `article_refs`** | Splitting refs lets us add `position` / `kind` later without schema rewrite. |
| 4 | Display ID namespace | **`ART-NNNNN`** | v180 discipline. Sibling to `EVT-NNNNN` for #320. `NextArticleID` mirrors `NextEventID` at `internal/db/csaid.go`. |
| 5 | Reference syntax | **`[Display Text](#person/D-00123)`** | Markdown link with custom scheme. Picker inserts; renderer resolves. |
| 6 | Reference resolution | **Strict Display ID lookup, fail-loud** | Unresolved token renders `⚠ [Unknown: D-00123]` on PDF/HTML. |
| 7 | Editor | **Markdown source + sanitized HTML live preview** (no WYSIWYG in v1) | Matches repo Tailwind/htmx aesthetic; `bluemonday` sanitizer; avoids contenteditable WYSIWYG bugs (the modal-invoker failure mode #1 in `tdd.md:41`). |
| 8 | PDF pipeline | **Typst, new `templates/article.typ`** | Reuses Typst tooling; refactor `render-person-card(s)` from `record_card.typ` for single-sourced rendering. |
| 9 | Export formats v1 | **PDF, Static HTML, raw `.md`** | Three deliverables. No Shared Archive toggle in v1 — Articles ride existing bundle. |
| 10 | Static archive shape | **`window.DIXIE_DATA.articles: Article[]`** | Sibling to `records` (same pattern as #320 #335 for `events[]`). |
| 11 | Versioning | **Manual snapshots** ("Save copy" → read-only sibling row) | Researcher-driven; no per-save history bloat. |
| 11a | Snapshot lifecycle | **User-managed** (Revisions tab: list + Delete + Restore; snapshot stays after restore) | Mirrors archive-delete affordances. |
| 12 | Draft persistence | **Local-draft persistence per the Soldier/new pattern (entry_form.templ:40-105 + frontend/app.js:1438-1500)** — `localStorage` autoload/autosave per keystroke, "Local draft only" / "Committed to database" banner, "Delete saved local draft" undo affordance. **No server-side auto-save** — explicit Save commits to the DB and clears the local draft. | Matches Soldier/new UX exactly. The original spec conflated "no auto-save" with "no local persistence" — corrected 2026-07-05. |
| 13 | UI placement | **Top-level nav item "Articles" between Records and Export** | Same pattern as #320 Event Records nav. |
| 14 | Reference picker | **Person Record search modal (display name OR Display ID, filtered live)** | Reuses existing Person Record search service. |
| 15 | Reverse-lookup panel | **"Cited in" panel on Person Record page; hidden when 0 refs** | New templ component. |

## Out of scope (explicitly deferred, locked at v1)

- Article versioning / revision history (replaced by Slice 2.5 manual snapshots)
- Auto-revisions on every save (v2)
- Visual diff between revisions (v2)
- **Server-side auto-save** (the original "no auto-save" decision; clarified to mean this, not local-draft persistence) (v2 if requested)
- Auto-save / collaborative editing (v2)
- Source Record / Claim / Finding references (v2; v1 is Person-only per locked decision #6)
- Article → Article references (v2)
- Article-only filter on Browse (v2; v1 uses Articles nav)
- WYSIWYG (explicit non-goal; markdown only)
- Image upload into Article body (v2; markdown may reference external URLs)
- Article-specific Shared Archive toggle (Articles ride existing bundle unconditionally in v1)
- Templates / snippets (v2)

## Related

- `CONTEXT.md` Laws: Backend-First, Glossary, Schema-bump, Doc-comment, Apply-sites, Ticket close-out (per #320 sequence close-gate commit `1547fc8`)
- `docs/agents/feature-protocol.md` — 5-section feature template, Slice 0 prefactor rule, 3-tier commit rule
- `docs/agents/tdd.md` — Slice 0.5 RED test discipline
- `docs/agents/dialog-guard.md` — `/articles/{id}/pdf` handler must use `guardedSaveFileDialog` per `CONTEXT.md:144-160`
- `docs/agents/cli-plan.md` — headless CLI subcommands for Articles (later phase per 7-phase CLI roadmap)
- `docs/migrations/reversibility.md` — per-block classification for the v61 blocks
- Issue #320 — Event Records (sibling feature in v60); mirrors nav + display_id + apply-sites pattern
- Issue #363 — server-gate pattern (`/soldiers/{id}*` redirects to `/events/{id}*` on Event rows); cited as Slice X's template
- Issue #266 — schema version discipline; this PR advances `CurrentSchemaVersion` 60 → 61
- Issue #257 — "shipped but invisible" sweep; the apply-sites checklist is the explicit anti-pattern guard
- Issue #183 — worked example for apply-sites + slice plan + glossary
- `internal/db/csaid.go` — `NextDXDID` namespace discipline; `NextArticleID` is a sibling
- `templates/common/record_card.typ` — refactor candidate for `render-person-card(s)` helper
- `internal/archive/static_archive.go` — sibling `articles[]` next to `records[]` mirrors #320 #335
- **`internal/templates/entry_form.templ:40-105` — the local-draft persistence pattern that the Article editor mirrors**
- **`frontend/app.js:1438-1500` — the generic `draftKeyForForm` / `isDraftableField` / `recordPersistenceTarget` helpers that are not Soldier-specific; any form with the right `data-*` attributes gets persistence for free**
- **`internal/templates/entry_form_helpers.go:37-90` — `draftKey` / `draftMode` / `draftRecordVersion` / `draftResetPath` / `recordPersistenceClass` / `recordPersistenceHeading` / `recordPersistenceMessage` — these helpers are Soldier-typed; Slice 0 should determine whether a generic-typed extraction is the prefactor, or whether sibling Article-typed helpers are acceptable**
