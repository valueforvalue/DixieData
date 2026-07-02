# Feature Add Protocol

The canonical procedure for adding a new feature to DixieData.
Pairs with [`rpci.md`](rpci.md) (procedural flow) and
[`issue-tracker.md`](issue-tracker.md) (issue filing); this doc owns
the feature-specific decisions, commit shape, and module discipline.

## Pre-flight checklist

Tick these before any recon. Five skills, ~30 seconds:

- [ ] **Glossary** — read [`CONTEXT.md`](../../CONTEXT.md) end-to-end.
       If the feature adds a domain concept, propose a glossary
       entry in the issue body.
- [ ] **Flow** — read [`docs/agents/rpci.md`](rpci.md). The four
       phases (Research → Plan → Critique → Implement) apply.
       Features default to **full RPCI** unless one-line bug-fix-shaped.
- [ ] **Bug patterns** — read
       [`docs/COMMON_BUGS.md`](../COMMON_BUGS.md) §layer for every
       layer the feature touches (HTMX wiring, templ markup,
       frontend JS, Go backend, Typst, accessibility, calendar/API,
       build/CI, database, debugging).
- [ ] **Cross-layer contract** — read
       [`docs/CODE_CHANGES.md`](../CODE_CHANGES.md) when the feature
       touches templ + htmx + JS + Go handler together. It's the
       worked example for one-system drift.
- [ ] **Index** — read [`docs/agents/INDEX.md`](INDEX.md) to know
       which tier-1/2 docs (CLI plan, Typst tips, UI map wireframes)
       to load for this task.

Add to the checklist when the feature touches:

- **Native dialogs** → [`docs/agents/dialog-guard.md`](dialog-guard.md)
- **CLI subcommand** → [`docs/agents/cli-plan.md`](cli-plan.md)
- **UI hunt / redesign** → [`docs/ui-map/README.md`](../ui-map/README.md)
  + the affected wireframe
- **Database schema** → [`docs/migrations/`](../../migrations/) (read
  the most recent two for the established pattern)
- **Audit regression net** → existing `audit/smoke_*.mjs` for
  parallel surfaces

## When to use

| Task shape | Flow |
|---|---|
| One-line bug fix, clear repro | **Skip the protocol.** Bug protocol in `issue-tracker.md` applies. |
| One-commit feature (e.g. a checkbox toggle on existing surface) | **Lightweight.** Issue body has Use case + Acceptance criteria + Regression net. No slice plan. |
| Multi-file feature, 2-3 layers | **Full protocol.** Use case + Locked decisions + Apply-sites checklist + Acceptance criteria + Slice plan in the issue body. |
| Subsystem feature, 4+ files across layers | **Full protocol + design artifact.** Issue body as above + `.rpiv/artifacts/designs/<slug>.md` for the architecture. |
| Cross-cutting feature, 6+ files, design questions | **Full protocol + 7-phase pipeline.** Add `docs/RESEARCH.md` → `docs/PRD.md` → `docs/TASKS.csv` for the heavy artifacts. Issue body always carries the slice plan; the artifact files are the durable record. |

## The 3-tier commit rule

One commit = one of:

### Tier 1 — user-visible outcome

The whole feature lands in one commit because no intermediate state
is shippable. The "feature flag + opt-in rollout" pattern doesn't
fit DixieData (single-binary, single-user), so this tier is rare.

### Tier 2 — vertical slice across one boundary

A backend vertical (schema + service + handler) OR a UI vertical
(templ + JS + audit probe) in one commit, because alone it doesn't
compile or doesn't surface to the user. **Always paired with at
least one matching UI apply-site in the same PR** (see
[Backend-First Law](../../CONTEXT.md) in `CONTEXT.md`).

Examples from issue #183:

- `9e96ea5 feat(db): tags + person_record_tags + archive_meta (v58)`
  — schema alone (compiles, but useless without service)
- `fb20b00 feat(records): TagService + ArchiveMetaService`
  — service alone (compiles, but unreachable without handlers)
- `d3e5141 feat(http): tagging HTTP handlers + route registration`
  — handlers alone (reaches from curl, but no UI)
- `582d4de feat(ui): /tags mgmt page + tag picker page + soldier integration`
  — three UI pages in one commit (coherent unit)

### Tier 3 — apply-site unit

One apply-site = one commit, when each apply-site is independently
significant and worth its own PR. Examples from #183:

- `f589dd9 feat(browse): add row tag chips + bulk-tag toolbar (Slice C)`
  — Browse row chips + bulk-tag toolbar = one user-visible surface
- `20381f2 feat(tags): wire the tag picker to the Person Record detail page`
  — Person Record detail page tag editor = one user-visible surface

**The rule:** if you can write a one-sentence commit subject that
doesn't need "and", it's atomic. If it needs "and" twice, decompose.

**Anti-pattern:** per-layer micro-commits (one for schema, one for
service, one for handler, one for UI). Three of #183's commits were
in this shape and each was independently uncompilable. Bundle into
a Tier 2 vertical instead.

**Anti-pattern:** mega-commit (everything for a feature in one
commit). Hides the apply-site unit; impossible to revert one slice
without losing the others; review has to read all layers at once.

## Module discipline (deep-module)

A feature is more than "make it work" — it's also "leave the
codebase deeper than you found it."

### Define the facade before the internals

In the PR description (or `.rpiv/artifacts/designs/<slug>.md` for
subsystem features), write the **public service interface** +
**DTO contracts** BEFORE any internal code. UI and tests cross this
seam; the implementation is invisible behind it.

```go
// Service signature (public seam) — write this first
type TagService interface {
    NormalizeName(raw string) string
    UpsertByName(raw string) (Tag, error)
    Attach(personID int64, tagID int64) error
    Detach(personID int64, tagID int64) error
    TagsForPerson(personID int64) ([]Tag, error)
    // ... only methods demanded by acceptance criteria
}

// DTO returned to UI (boundary mapping)
type TagDTO struct {
    ID             int64
    Name           string  // original casing
    NormalizedName string  // case-insensitive lookup key
    CreatedAt      time.Time
}
```

Internal helpers (`upsertTagRow`, `attachPersonTag`,
`loadTagsForPersonSQL`) are private to the package and not in the
interface. If a caller needs them, the seam is wrong.

### Service is the seam

- UI depends on the service's DTOs, never on the persistence struct.
- Tests cross the service's interface, not the persistence layer.
- If `internal/templates/foo.templ` imports `internal/db` to reach a
  `Soldier` struct directly, the seam is broken — add a DTO.

### Deletion test (at PR review time)

Would deleting this module break N callers?

- **N == 0** → it's a pass-through, refactor it into the caller.
- **N >= 2** → it's earning its keep, keep the module.
- **N == 1** → borderline; often an indicator of premature
  extraction. Roll it back unless the second caller is already
  scheduled.

### The two-adapter rule

Before adding a new public method to an existing service, ask:

> Does an acceptance criterion demand this method, OR is there a
> second adapter (test fake + prod impl, or two callers) that
> needs it?

If neither, the method is speculative. Drop it. If a future change
needs it, add it then. Speculative API surface is the YAGNI tax.

### Don't return persistence structs to UI

```go
// WRONG — templ imports db package
func (s *Soldiers) Get(id int64) db.Soldier { ... }

// RIGHT — boundary DTO, mapping at the seam
func (s *Soldiers) Get(id int64) SoldierDTO { ... }
```

The mapper is private. The DTO is the contract. Persistence
structs can grow columns without breaking UI.

## Pipeline phasing

Features scale the artifact weight with complexity:

| Complexity | Artifacts |
|---|---|
| One-commit | Issue body only |
| Multi-file, 2-3 layers | Issue body (Use case + Decisions + Apply-sites + Acceptance + Slice plan) |
| 4+ files across layers | Above + `.rpiv/artifacts/designs/<slug>.md` |
| 6+ files, design questions | Above + `docs/RESEARCH.md` + `docs/PRD.md` + `docs/TASKS.csv` |

**Always:** issue body carries the slice plan inline. Artifact files
are the durable record; the issue is the entry point for triage
and discussion.

See the [`ai-pipeline-7phase`](../../CONTEXT.md) skill in
`~/.agents/skills/ai-pipeline-7phase/SKILL.md` for the deterministic
phase transitions.

## Feature issue template

Use this when filing an enhancement. Mirrors the bug template in
[`docs/agents/issue-tracker.md`](issue-tracker.md) — every section
is required unless marked optional.

```markdown
## Summary
<One sentence: what the feature does and who it serves.>

## User story
<As a <role>, I want <capability>, so that <outcome>.>

## Locked decisions
<Numbered list of decisions already settled during recon /
interview. Each one cites the source: "Decided in <PR/issue/chat
on YYYY-MM-DD>". Locked decisions are NOT re-opened during the
Critique phase unless the user explicitly says so.>

## Proposed UX
<Per apply-site, what the user sees. References the screen by name
("the /soldiers/{id} detail page") and the surface by ID
(`page.detail`, `panel.summary`). For CLI work, write the command
surface: `dixiedata <cmd> --flag`.>

## Apply sites (v1 checklist)
- [ ] <Surface 1 — e.g. "Person Record detail page tag editor">
- [ ] <Surface 2 — e.g. "Browse row chip + bulk-tag toolbar">
- [ ] <Surface 3 — e.g. "/tags management page">
- [ ] <Surface 4 — e.g. "Share page 'Include Tags' checkbox">

The feature is not "shipped" until every box is checked. Backend-only
landings require a tracked follow-up issue for each missing UI
surface; see Backend-First Law in `CONTEXT.md`.

## Glossary changes (if any)
<Quote the new entry exactly as it should appear in `CONTEXT.md`
under "## Language" and "## Relationships". If the feature uses
existing terms only, write "None." — don't propose a glossary
change just because the feature exists.>

## Schema sketch (if any)
<For features that add tables/columns, paste the migration SQL
with the version bump + the seed data. Reference the most recent
migration file in `docs/migrations/` for shape.>

## Acceptance criteria
- [ ] <Observable: "User can apply N tags to a Person Record via
      the detail page; chips render with monogram + name + × remove.">
- [ ] <Observable: "...">

## Slice plan
### Slice 1: <name>
- Files: <paths>
- Success criteria: <observable, testable in 5 min>
- Regression net: <unit test, smoke probe, or manual step>

### Slice 2: <name>
- ...

## Test plan
- **Unit** — <file: TestXxx> (~N cases)
- **Handler** — <file: TestXxx> (~N cases)
- **Migration** — <file: TestXxx> (if schema changed)
- **Smoke probe** — `audit/smoke_<feature>.mjs` (per UI apply-site)

## Files
- <bulleted list of every file that will be touched>

## Regression net
- <unit test names + audit/smoke probe filenames>

## Related
- <issue numbers, ADR numbers, `docs/COMMON_BUGS.md` section refs>
```

### What goes in each section

**User story** is for the next maintainer, not for the agent. Write
it from the researcher's perspective, not the engineer's. "I want
to organise Person Records by ad-hoc categories" — not "add a tag
table with a many-to-many join."

**Locked decisions** is the durable record of what was settled
before implementation. Re-opening a locked decision during Plan or
Critique is a scope-creep red flag; capture the decision in a
comment on the issue, don't silently re-decide.

**Apply sites** is a checklist, not prose. Prose hides gaps;
checkboxes make them visible. Issue body must list every v1 surface
as a checked box. PR description must mirror the checklist and tick
boxes as commits land.

**Glossary changes** are required when the feature introduces a new
domain term, even if it feels obvious ("tags", "campaigns",
"battles"). The glossary is the contract — UI copy, ADRs, and
future features depend on it. Don't ship the feature without the
glossary update in the same PR.

**Acceptance criteria** are testable in 5 minutes by a human with
the build. "It works" is not a criterion; "User can apply 3 tags
to a Person Record and they render as chips on the detail page"
is.

**Slice plan** mirrors the [3-tier commit rule](#the-3-tier-commit-rule).
Each slice names its tier in the subject (`Slice C: row chips`)
and ships the whole tier in one commit. Backend-only slices MUST
list their matching UI apply-site in the acceptance criteria of
the same PR (or a linked follow-up issue with its own checklist).

## CHANGELOG law

Every user-visible feature change gets a bullet in
[`CHANGELOG.md`](../../CHANGELOG.md) `[Unreleased]` under the
right heading (`### Added`, `### Changed`, `### Removed`).
Internal refactors that don't change user-visible behavior live
under `### Maintenance`. The bullet references the issue number
and the regression net filename.

## Branch policy

- **One slice = one commit = one PR (or push to dev).** Per
  [`AGENTS.md`](../../AGENTS.md): direct commits to `dev` for
  slices that touch one logical change; feature branches only for
  multi-commit work, new surfaces, or user request.
- **`main` is always stable.** Promotion requires explicit user
  direction and a passing full suite. See `AGENTS.md`.

## When to load what

| If you're working on... | Read these (in order) |
|---|---|
| Any feature | This file (you are here), then `CONTEXT.md` |
| Backend handler / service | `docs/CODE_CHANGES.md`, `docs/COMMON_BUGS.md` §4 (Go backend) |
| Templ / htmx | `docs/COMMON_BUGS.md` §1 (HTMX wiring) + §2 (templ) |
| Frontend JS | `docs/COMMON_BUGS.md` §3 (Frontend JS) |
| Typst PDF surface | `docs/agents/tune-iteration.md`, `docs/agents/typst-layout-tips.md` |
| CLI subcommand | `docs/agents/cli-plan.md` |
| Database schema | `docs/migrations/` (latest two) |
| Audit smoke probe | `docs/agents/manual-audit-playbook.md` |
| Native dialog | `docs/agents/dialog-guard.md` (mandatory read) |

## Anti-patterns

- **Ship the backend first, UI later.** Violates the
  [Backend-First Law](../../CONTEXT.md). The 2026-06 "shipped but
  invisible" sweep (issue #257) surfaced four features that landed
  backend + handlers + routes with no UI to invoke them. Don't
  repeat.
- **One mega-commit for the whole feature.** Hides slice boundaries;
  impossible to revert one surface without losing the others;
  reviewer has to read all layers at once. Use the 3-tier rule.
- **Per-layer micro-commits.** Schema alone, service alone, handler
  alone. Each is uncompilable. Bundle into a Tier 2 vertical or
  stay with a single Tier 3 apply-site.
- **Speculative public API.** Adding methods to a service that no
  acceptance criterion demands and no caller uses. Apply the
  two-adapter rule.
- **Returning persistence structs to UI.** Breaks the seam; grows
  the coupling surface; makes future schema changes a UI nightmare.
  Map to a DTO at the boundary.
- **Glossary drift.** Adding a feature without updating
  `CONTEXT.md` when the feature introduces a new domain term. The
  glossary is the contract — drift compounds across features.
- **Smoke probe missing.** Every UI apply-site needs an
  `audit/smoke_<feature>.mjs` assertion that checks both the
  response shape AND `page.url()` after the click. Response-only
  assertions miss the htmx `hx-swap="none"` + 303 silent-swallow
  class (commits `70878ac` → `3612dab`).
- **Filing the design in the issue body.** Issue body is the
  research + acceptance + slice plan. Architecture decisions live
  in `.rpiv/artifacts/designs/<slug>.md`. Don't paste code into the
  issue.

## References

- [`CONTEXT.md`](../../CONTEXT.md) — glossary + Laws (read first)
- [`AGENTS.md`](../../AGENTS.md) — session protocol, branch policy,
  commit shape (read alongside this file)
- [`docs/agents/rpci.md`](rpci.md) — Research → Plan → Critique →
  Implement flow (procedural complement)
- [`docs/agents/issue-tracker.md`](issue-tracker.md) — bug protocol
  + label taxonomy (sister doc for bugs)
- [`docs/agents/INDEX.md`](INDEX.md) — 3-tier progressive disclosure
  of which docs to load per task
- [`docs/COMMON_BUGS.md`](../COMMON_BUGS.md) — recurring bug
  patterns by layer (read before touching each layer)
- [`docs/CODE_CHANGES.md`](../CODE_CHANGES.md) — cross-layer working
  contract (chi-router migration, hx-attr strip)
- [`docs/agents/dialog-guard.md`](dialog-guard.md) — native dialog
  re-entry race (mandatory for any export/import handler)
- [`docs/agents/cli-plan.md`](cli-plan.md) — CLI subcommand roadmap
- [`docs/agents/triage-labels.md`](triage-labels.md) — full 6-axis
  label taxonomy
- [`docs/ui-map/README.md`](../ui-map/README.md) — UI reference
  (screen × component matrix)
- [`docs/migrations/`](../../migrations/) — schema version history
- Issue #183 — worked example of the slice plan + apply-sites
  checklist + glossary changes shape
- Issue #257 — the "shipped but invisible" sweep that motivated
  the Backend-First Law