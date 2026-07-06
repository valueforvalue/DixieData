# Docs Index — Progressive Disclosure

The `docs/` tree has ~80 markdown files. An agent does not need
to load all of them. This index tells you which tier each doc
belongs to and when to load it.

## Tier 0 — Always loaded

These are cross-referenced from [`CONTEXT.md`](../../CONTEXT.md)
or [`AGENTS.md`](../../AGENTS.md). Load at session start.

| Doc | Why |
|---|---|
| [`CONTEXT.md`](../../CONTEXT.md) | Glossary + Laws (non-negotiable). Read first. |
| [`AGENTS.md`](../../AGENTS.md) | Session protocol, branch policy, commit shape. |
| [`CHANGELOG.md`](../../CHANGELOG.md) | What's shipped; what's `[Unreleased]`. |
| [`docs/agents/feature-protocol.md`](feature-protocol.md) | The feature add protocol. Load for any feature work. |
| [`docs/agents/tdd.md`](tdd.md) | TDD discipline — RED test pins slice acceptance criterion BEFORE code lands. Cross-cuts every feature and every bug fix. Load alongside `feature-protocol.md`. |
| [`docs/agents/rpci.md`](rpci.md) | Research → Plan → Critique → Implement flow. |
| [`docs/agents/issue-tracker.md`](issue-tracker.md) | Bug + feature issue templates, label taxonomy. |
| [`docs/agents/triage-labels.md`](triage-labels.md) | The 6-axis label spec. |
| [`docs/agents/dialog-guard.md`](dialog-guard.md) | Mandatory read for any export/import handler. |
| [`docs/COMMON_BUGS.md`](../COMMON_BUGS.md) | Bug patterns by layer (HTMX, templ, JS, Go, Typst, a11y). |
| [`docs/CODE_CHANGES.md`](../CODE_CHANGES.md) | Cross-layer working contract (templ + htmx + JS + Go). |

## Tier 1 — Task-role loaded

These are loaded when the task matches the role. Don't pre-load
all of them; load only the ones that match your task.

### Bug work

| Doc | When |
|---|---|
| [`docs/agents/bug-pattern-grep.md`](bug-pattern-grep.md) | Hunting a bug class by grep pattern. |
| [`docs/agents/manual-audit-playbook.md`](manual-audit-playbook.md) | Manual UI walk. |
| [`docs/agents/audit-notes-TEMPLATE.md`](audit-notes-TEMPLATE.md) | Template for audit notes. |
| [`docs/agents/jobs-artifact-content-disposition-bug.md`](jobs-artifact-content-disposition-bug.md) | Working in the jobs area. |

### Feature work

| Doc | When |
|---|---|
| [`docs/agents/cli-plan.md`](cli-plan.md) | Adding or modifying a CLI subcommand. |
| [`docs/agents/doctor-impl-notes.md`](doctor-impl-notes.md) | Adding a doctor check. |
| [`docs/agents/tune-iteration.md`](tune-iteration.md) | Iterating on a Typst PDF surface. |
| [`docs/agents/typst-layout-tips.md`](typst-layout-tips.md) | Typst layout work (alongside tune-iteration.md). |
| [`docs/agents/domain.md`](domain.md) | Skills reading domain docs. |
| [`docs/agents/repl.md`](repl.md) | Python REPL scratch discipline — when to use `.agents/skills/repl/` for deterministic investigation, boundary rule (Python NEVER in Go build). |

### UI hunt / redesign

| Doc | When |
|---|---|
| [`docs/ui-map/README.md`](../ui-map/README.md) | UI reference entry point. |
| [`docs/ui-map/INDEX.md`](../ui-map/INDEX.md) | Screen × component matrix. |
| [`docs/ui-map/routes.md`](../ui-map/routes.md) | Every route → handler mapping. |
| [`docs/ui-map/surfaces.md`](../ui-map/surfaces.md) | Canonical DOM IDs. |
| [`docs/ui-map/wireframes/<screen>.md`](../ui-map/wireframes/) | One wireframe per screen — load only the affected one. |

### Schema / database

| Doc | When |
|---|---|
| [`docs/migrations/`](../../migrations/) | Read the latest two when touching schema. |
| [`docs/RELEASING.md`](../RELEASING.md) | Release procedure (when bumping versions). |

### Process / meta

| Doc | When |
|---|---|
| [`docs/PRD.md`](../PRD.md) | The product requirements doc. |
| [`docs/RESEARCH.md`](../RESEARCH.md) | Research artifact for cross-layer features. |
| [`docs/TASKS.csv`](../TASKS.csv) | CSV-driven task backlog for 7-phase pipeline. |
| [`docs/SERVICES.md`](../SERVICES.md) | Service inventory. |
| [`docs/THIRDPARTY.md`](../THIRDPARTY.md) | Third-party dependencies. |
| [`docs/agents/model-scope.md`](model-scope.md) | Subagent model allowlist + project shadow files. Load when configuring or debugging subagent models. |

## Tier 2 — On-demand

These live under [`docs/historical/`](../../historical/) and are
NOT loaded by default. Load only when the issue, PR, ADR, or
explicit user direction names them.

| Doc | When |
|---|---|
| `docs/historical/handoffs/*.md` | Historical session handoffs. Reference only when investigating a specific past decision. |
| `docs/historical/audit-resolved/*.md` | Resolved audits with stale path citations. Reference only when tracing why a current code shape exists. |
| `docs/historical/audit-runs/round-N/` | Old audit rounds (past the latest 3). Reference only when comparing current findings to a previous round. |
| `docs/historical/renderings/<surface>/round-N.md` | Old per-surface rendering reviews. Reference only when working on the same surface. |
| [`docs/audit/static-web-archive-audit-2026-06.md`](../audit/static-web-archive-audit-2026-06.md) | Resolved before the resolved/ convention existed. Still at the audit/ root for visibility. |
| [`docs/renderings/README.md`](../renderings/README.md) | The PDF rendering iteration workflow (Tier 1; the per-surface `review.md` files are Tier 2 once past the latest round). |

## Retention

The `docs/historical/` tree holds files retained for traceability
but not loaded by default. See [`docs/historical/README.md`](../../historical/README.md)
for the retention rule (latest 3 rounds in the working tree;
older rounds under `docs/historical/`).

## How to use this index

1. Read Tier 0 at session start. Stop. Do not load Tier 1 yet.
2. Read the task. Identify which Tier 1 doc(s) match.
3. Load only those Tier 1 docs.
4. If the task references a specific historical artifact by name
   or number, load it from `docs/historical/`. Otherwise, ignore
   the Tier 2 docs entirely.

If a doc you'd expect to find isn't listed here, it's either in
the wrong tier (open an issue) or it doesn't exist yet (open an
issue with a "missing doc" label).

## References

- [`CONTEXT.md`](../../CONTEXT.md) §Historical Artifact — glossary
  entry for the retention pattern
- [`docs/agents/feature-protocol.md`](feature-protocol.md) §When to
  load what — the same idea, scoped to feature work
- [`docs/agents/INDEX.md`](INDEX.md) (this file) — the canonical
  index for all of `docs/agents/`