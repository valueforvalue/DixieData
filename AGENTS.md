# AGENTS.md

Working notes for AI coding agents operating in this repository.

## Domain vocabulary (ubiquitous language)

DixieData has a strict, evolving glossary. Every feature, doc, and commit message should
match. **Before writing any user-facing copy or schema-touching code, read
[`CONTEXT.md`](CONTEXT.md) end-to-end.** The canonical terms:

- **Person Record** — primary archive entry for one person (soldier, wife, widow)
- **Display ID** — canonical user-facing identifier for a Person Record
- **Local Archive** — live working collection on one machine
- **Shared Archive** — merge-oriented archive package exchanged between users
- **Backup Archive** — full replacement snapshot for restore and safekeeping
- **Restore Point** — automatic pre-update recovery bundle
- **Static Archive** — read-only browser-viewable export
- **Source Record** — attached evidence item (pension, application, etc.)
- **Claim** — assertion extracted from a Source Record
- **Finding** — verified Claim that has cleared review

See `CONTEXT.md` for the full glossary and anti-patterns.

## Wails runtime hazards (READ BEFORE TOUCHING frontend/app.js form-dispatch)

The Wails v2.12.0 desktop binary embeds the frontend in a custom
WebView2 host that exposes the Go HTTP server as
`http://wails.localhost`. That custom-protocol server has two
quirks every frontend change must respect. Issue #428 / commit
`77290ca` documented them; this section is the working-memory
reminder so the next agent doesn't re-discover them by
debugging "Position must be a positive integer" toasts.

**Quirk 1 — PATCH/PUT/DELETE bodies are stripped.** The
asset server delivers the body of GET and POST requests to
the Go handler, but a real PATCH/PUT/DELETE sent from the
WebView2 reaches the handler with an empty `r.Body`. The
method is correct; only the body is missing. Repro: click a
Source Record ▲/▼ button on `/soldiers/{id}`. Symptom: server
returns 400 "Position must be a positive integer" because
`r.ParseForm()` is empty.

**Workaround:** the dispatcher in `frontend/app.js`
(`dispatchDixieDataForm`) rewrites PATCH/PUT/DELETE to POST
and sets the `X-HTTP-Method-Override` header when the target
URL contains `wails.localhost`. The server's
`requestMethodOverride` middleware
(`internal/appshell/app.go:487`) already accepts the header
and rewrites the method back to PATCH before the chi router
dispatches, so handlers stay PATCH-only. Plain Chromium (the
audit harness, which talks to `dixiedata-web` over plain
HTTP) keeps the real PATCH so Playwright assertions on the
genuine method still pass.

**Quirk 2 — multipart/form-data bodies are stripped too.** Even
with the override, the FormData body itself was being dropped
by the asset server. JSON bodies (used by `/debug/client-logs`)
DO survive, and urlencoded bodies (`Content-Type:
application/x-www-form-urlencoded`) survive per the asset
server's contract.

**Workaround:** the same dispatcher branch converts
`FormData` to `URLSearchParams.toString()` when the target
URL contains `wails.localhost`. The form fields are delivered
verbatim to `r.ParseForm()`. Vanilla Chromium keeps the
FormData body so file uploads + large bodies still work in
the audit harness.

**Regression net:** `audit/dispatcher_patch_method.test.mjs`
has 7 assertions that pin both workarounds. If a future
refactor removes either, the test fails with a clear
message. The `Wails-PATCH` and `Wails-FormData` comments in
`frontend/app.js` are the source-of-truth breadcrumbs.

**TL;DR for future agents:** any new form on a templ page
that uses `method="patch"` (or PUT/DELETE) and a
`data-dixie-submit` form will hit these workarounds
automatically. The dispatcher catches it. You do NOT need to
add a per-form workaround. If a fetch in some other path
*doesn't* go through `dispatchDixieDataForm`, replicate the
wails.localhost gate there too — see the comments for the
exact checks.

## Form-attribute mutation hazard (issue #689, class 8)

**A new failure mode surfaced in the 2026-07-28 #676/#682
diagnostic session.** Any JS code that runs on form
initialization and assigns to `form.action`, `form.method`,
or `form.enctype` mutates the server-rendered URL and breaks
the form's submit flow. The single canonical exception is the
synthetic-form branch in `dispatchDixieDataForm` that builds a
form from a `data-action` URL. Any other location is a bug.

The canonical offender at time of writing: `syncEntryTypeFields`
at `frontend/app.js:3946-3949` unconditionally sets
`form.action = "/soldiers"` on every `initializeDynamicContent`
pass, clobbering the edit URL `/soldiers/{id}` on the Person
Record edit form. The fix lands as #689. The detection gate
(`scripts/lint-no-form-mutation.js`, added by #687 extended scope)
walks `frontend/**/*.js` and fails CI on any non-allowed mutation.

**TL;DR for future agents:** never assign to `form.action`,
`form.method`, or `form.enctype` outside `dispatchDixieDataForm`'s
synthetic-form branch. The form's `action` is set by the server
(`/soldiers/{id}` for edit, `/soldiers` for new). If you need to
change the URL dynamically, change the URL on the button's
`data-action` attribute and let the dispatcher build a synthetic
form. See [`docs/COMMON_BUGS.md` §3.9](docs/COMMON_BUGS.md#39-js-side-formaction-mutation--js-clobbers-the-server-rendered-url-after-init-689-priorityhigh-targetrc)
for the canonical bug class entry.

## Nested-form hazard (issue #682, class 4, release-blocker)

**HTML5 forbids `<form>` inside `<form>`.** The browser parser
silently closes the outer form when it encounters an inner one
([WHATWG §13.2.6.4.3, "in body" insertion mode](https://html.spec.whatwg.org/multipage/parsing.html#parsing-main-inbody)).
Anything between the inner form's open and the outer form's close
is reparented. Submit buttons in the loop end up with no
`<form>` ancestor in the rendered DOM.

Two live instances at time of writing:
- `internal/templates/entry_form.templ` — outer form at L55
  contains inner image-upload form at L392. Save Changes button
  at L432 is orphaned.
- `internal/templates/soldier_card.templ` — outer images-download
  form at L567 contains inner image-upload form at L574.
  Download Selected Images button is orphaned.

The detection gate (`scripts/lint-no-nested-forms.py`, added by
#682) walks every `.templ` and fails CI on any nested form. The
fix is structural: move the inner form out of the outer form's
scope. The `data-results-target` on the inner form already points
to a sibling-scoped ID, so the conceptual model was always
sibling. See [`docs/COMMON_BUGS.md` §2.7](docs/COMMON_BUGS.md#27-nested-form-rendering-defect--html5-parser-silently-closes-the-outer-form-682-release-blocker-for-rcv11)
for the canonical bug class entry.

**TL;DR for future agents:** never author a `<form>` inside another
`<form>` in a `.templ` file. If a section needs a form, lift it
to a sibling of the outer form. The `entry_form.templ` and
`soldier_card.templ` instances are fixed by #682.

## Empty-body dispatch hazard (issue #691, class 9, release-blocker)

**A new failure mode surfaced in the 2026-07-28 #676 diagnostic
session (after #682 and #689 landed).** The body-construction
branch in `dispatchDixieDataForm` (frontend/app.js:5206) checks
`button.closest("form")` to decide whether to build a FormData
body. When the Save button is reparented (because of the nested
form auto-close, or any other invalid HTML structure), the
button has no form ancestor in the DOM. `button.closest("form")`
returns `null`. The else branch fires:

```js
if (button instanceof HTMLElement && button.closest("form")) {
  const fd = isSubmitButton ? new FormData(form, button) : new FormData(form);
  fetchOptions.body = fd;
} else {
  fetchOptions.body = new FormData(); // ← empty body
}
```

The form-finding branch above (line 5110) correctly handles this
with `button.closest("form") || button.form`. The
body-construction branch doesn't apply the same fallback. The
body is empty, the server sees an empty form, `parseSoldierForm`
returns all-empty fields, and `Update` wipes the record.

**The fix:** use the already-resolved `form` variable instead of
re-checking `closest()`:

```diff
-        if (button instanceof HTMLElement && button.closest("form")) {
+        if (form instanceof HTMLFormElement) {
```

**TL;DR for future agents:** any code path that needs a form
reference must use the resolved `form` variable (which has the
`closest() || button.form` fallback applied) or apply the same
fallback. Don't re-check `button.closest("form")` — it returns
`null` for reparented buttons. See [`docs/COMMON_BUGS.md` §3.10](docs/COMMON_BUGS.md#310-empty-body-dispatch--body-construction-uses-raw-closest-instead-of-the-resolved-form-691-release-blocker-targetrc)
for the canonical bug class entry. The detection gate
(`scripts/lint-no-form-mutation.js`, scope extended by #687) fails
CI on any `button.closest("form")` outside the form-finding branch.

**The lesson:** A band-aid that fixes one branch can mask a
sibling branch. The form-finding branch's `button.form` fallback
correctly resolved the form, which made the body-construction
branch's missing fallback visible: the form was found, but the
body was empty. **Every place that needs a form reference must
apply the same fallback**, not just the first one.

## Embed-tree skip hazard (issue #686, class 5, target:rc)

**JS file 404 in Wails, on disk at expected path, `window.__<x>`
undefined.** The file is silently dropped from the Wails binary.
The cause is in Go's `embed` package: files and directories whose
names begin with `.` or `_` are excluded from `//go:embed`. The
DixieData convention has been to put shared JS helpers in
`frontend/lib/` (after the rename from `_lib/` in `12f1834a`).
The article Preview button bug was the canonical instance:
`frontend/_lib/debounce.js` was never embedded.

The detection gate (`make verify-embed-tree`, added by #686)
walks `frontend/**` and the assets referenced by `index.html`
and the rendered runtime HTML, fails if any reference is to a
file that does not exist, and reports any `_`-prefixed or
`.`-prefixed top-level dir under `frontend/` as informational
R1 warnings. The `--strict` flag flips the missing-reference
failures to CI-blocking. Sibling to `make lint-bake-bootstrap`
and `make lint-htmx-guard`.

**TL;DR for future agents:** never place a frontend helper
under a `_`-prefixed or `.`-prefixed top-level dir. The
DixieData convention is `frontend/lib/` for shared helpers. If
you need a new shared helper, place it at `frontend/lib/foo.js`
or `frontend/lib/foo/index.js` and update the `<script src>`
in `index.html`. Run `make verify-embed-tree` before opening
the PR. See [`docs/COMMON_BUGS.md` §8.6](docs/COMMON_BUGS.md#86-embed-tree-skip--goembed-silently-drops-_-prefixed-files--the-686-regression-net)
for the canonical bug class entry.

**The HANDLER_ALLOWLIST caveat:** the `verify-embed-tree` probe
excludes URLs that Go handlers synthesize (e.g. `/boot-theme.js`
is served by `internal/appshell/boot_theme.go`, not by the
embedded frontend). If you add a new Go-handler-served URL that
the templates reference, add it to the allowlist in
`audit/verify_embed_tree.mjs`.

## File map (entry points)

| Path | Role |
|---|---|
| `main.go` | Wails app entry point (also handles `--smoke` headless boot) |
| `internal/appshell/` | App bootstrap + request handlers + stress tests |
| `internal/debug/` | Structured slog harness — `Configure`, `SetDebugMode`, `IsEnabled`, `FromContext`, `GetRingBuffer`, sink registry (file + ring + stderr mirror). Read this when touching anything that calls `slog.Debug` or the Debug Console |
| `internal/debug/trace/` | Build-tag-gated zero-cost instrumentation (`//go:build debug` + no-op stub). Use `trace.Log()` for entry/exit/branch markers; reach for ADR 0006 for the slog-vs-trace rule |
| `internal/db/` | SQLite schema, migrations, `GetAppVersion()` |
| `internal/htmxattr/` | Typed `htmxattr.Mux` builder — use instead of raw `hx-*` strings |
| `internal/routebuilder/` | Typed URL builders — use instead of string route literals |
| `internal/records/` | Person Record, Source Record, Claim, Finding model logic |
| `internal/templates/` | Templ HTML templates (regenerate with `make tpl`) |
| `internal/uiids/` | Canonical DOM ID constants (used by goquery invariant tests) |
| `internal/exportcontract/` | Shared types between Go export pipeline and JS frontend |
| `frontend/` | Static assets, Tailwind input, `app.js`, `app.css` output |
| `audit/` | UI/UX audit harness (round 1 → 4), `npm run audit` |
| `tools/tune/` | Iteration harness for design polish |
| `templates/` | Typst source templates (PDF rendering, NOT Go `text/template`) |
| `docs/` | User manual, ADRs, audit narrative, release docs |
| `docs/ui-map/` | **UI reference** — screen × component matrix, per-screen ASCII wireframes, route/surface lookup, gaps. Read first for any UI bug hunt or redesign. |
| `scripts/` | PowerShell + bash build/test helpers (`build-common.ps1`, `run-crash-dump.ps1`, `debug-crash.dlv`) |
| `Makefile` | Top-level DX (run `make help` for all targets) |
| `CONTEXT.md` | **Glossary + Laws source of truth** — read first |

## Commits and branches

- **One commit = one reviewable unit.** Default: one user-visible
  capability or one well-scoped bug fix, including its tests +
  docs + slice-internal refactors. A multi-layer feature can be
  a single commit when the layers only make sense together — the
  v60 slice-3 foundation commit `d09e852` (18 files, 1491
  insertions) is the canonical example; the slice decomposition
  doc at `docs/agents/notes/slice3-decomposition.md` exists as
  archaeology for that case. For features that decompose into
  independently reviewable pieces (e.g. v61's `event_sources`
  table migration across db / service / viewmodel / UI / docs),
  prefer one commit per piece — but the test is **"can a
  reviewer understand the complete shape of this change from the
  diff alone?"** not **"does the message split cleanly in
  half?"** Over-splitting (the previous rule) produced 6 commits
  for one bug fix in v61, each with multi-paragraph commit
  messages describing what the *next* slice would do — the
  message became a prediction the agent had to write before
  having the final answer, and the sequence was hard to review
  in isolation. Under-splitting is a smaller risk: a single
  reviewable commit beats four commits where each one is
  unreadable without the others.
- **The actual recurring failure** the repo has seen is
  **decomposition without a plan** — large commits that bundle
  templ + handler + audit test + regression net + CHANGELOG
  because no one wrote the slice plan first. The fix for that
  is **slice the plan first, ship the slices** — see
  `docs/agents/notes/v61-event-sources-decomposition.md` for a
  worked example (6 slices, each independently reviewable, each
  preceded by a RED test per `docs/agents/tdd.md`).
- **Commit message shape:** `<area>: <imperative summary>` for the
  subject (≤72 chars), blank line, then 1–3 bullets explaining
  *why* and what the regression net is. Reference the issue
  number if one exists (`issue #130`). Look at recent commits
  with `git log --oneline -20` for the in-repo house style.
- **Closing the issue is part of the slice, not a follow-up.**
  This repo lands via direct commits to `dev`, not via merged PRs,
  so GitHub's auto-close keywords (`fixes #N`, `Closes #N`) never
  fire — they only parse PR bodies. The agent that lands the final
  slice must `gh issue close <N> --comment "Closed — landed in <sha>: <subject>"`
  in the same turn. Wrap the issue number in parens at the end of
  the subject for human-readable grep (`(#N)` is a breadcrumb, not
  a closing keyword). See [`docs/agents/issue-tracker.md`](docs/agents/issue-tracker.md)
  §"Closing issues when the work ships" for the full discipline,
  including the sweep fallback for issues that slipped through.

### Branch policy

The default workflow is to **commit and push directly to `dev`**.
Feature branches are reserved for work that meets at least
one of these criteria:

- The work will land across **multiple commits** and benefits
  from review or rollback granularity that doesn't fit on dev
- The user **explicitly asks** for a branch
- The work is a **significant new surface** (a new screen, a
  new sub-system, a new external integration) that warrants
  a PR for review

**Examples:**

| Work shape | Where it lands |
|---|---|
| One-line bug fix on an existing handler | `dev` directly |
| Small templ + JS tweak (button label, copy, CSS) | `dev` directly |
| New smoke probe + small backend fix | `dev` directly |
| New audit/ probing probe on its own | `dev` directly |
| Multi-commit feature (e.g. #253 split /share) | `feature/<short-kebab>` |
| User explicitly says "branch this" | whatever the user says |
| New screen or sub-system | `feature/<short-kebab>` + PR |

### Four-branch model (dev / rc/v* / stable / main)

As of 2026-07-26 (ADR 0009 + ADR 0011), the repo carries four
named branches with distinct roles:

- **`dev`** — integration. Agents and humans commit here
  directly per the default flow below; PRs target `dev`.
  No branch protection.
- **`rc/v<MAJOR>.<MINOR>`** — release-candidate stabilization
  branch. Created from `dev` when RC1 cuts; receives only
  bug fixes, docs, tests, CI fixes, and regression nets
  (ADR 0011). Long-lived: stays around after the release as
  the patch-release maintenance line (`v1.1.1`, `v1.1.2`...).
  Protected by the commit-message + diff-size gate in
  `.github/workflows/rc-lint.yml`.
- **`stable`** — released-code home. Promotion destination
  for `make promote` (per ADR 0008). Future releases tag
  and ship from here.
- **`main`** — frozen legacy production record. Sits at
  HEAD `56e31f0` at the moment this ADR was written (2026-07-03)
  and accepts no new commits, ever. Preserved as the audit
  anchor for "what users had on 2026-07-03" so the
  production state is recoverable even after multiple
  releases have shipped from `stable`.

### Branch protection (main + stable + rc/v*)

Per ADR 0009, both `main` and `stable` get the **same**
standard GitHub branch protection rules:

- No direct pushes (require a PR).
- No force-pushes.
- No branch deletion.
- Require CI green (`build`, `test`, `audit` workflows)
  before merge.

Per ADR 0011, `rc/v*` is stricter than `stable` + `main`:
the commit-message + diff-size gate (`lint-rc-commits`
workflow) must also pass, and the `release-blocker` label
is required on every PR. The full ruleset is documented in
[`.github/BRANCH_PROTECTION.md`](.github/BRANCH_PROTECTION.md).

`dev` is **not** protected — direct commits are the default
flow per §Branch policy below. The rules are documented in
[`.github/BRANCH_PROTECTION.md`](.github/BRANCH_PROTECTION.md)
so future agents have a checklist to apply them via `gh api`
or the GitHub UI.

### RC branch policy (ADR 0011, Zephyr-style feature freeze)

The `rc/v*` branch is in **feature freeze** the moment it is
cut from `dev`. Only stabilization changes are allowed:

| Commit type | Allowed on `rc/v*`? |
|---|---|
| `fix:` | ✓ (the entire point) |
| `docs:` | ✓ |
| `test:` | ✓ (regression net for existing features) |
| `ci:` | ✓ (build / CI fixes) |
| `chore:` | ✓ (must reference the fix it supports) |
| `feat:` | ✗ — open on `dev` for the next release |
| `refactor:` | ✗ — open on `dev` |
| `perf:` | ✗ — open on `dev` |
| `build:` | ✗ — open on `dev` |

A commit that touches ≥ 50 files is **always** rejected,
regardless of type — large diffs are the canonical signal
of a refactor or feature masquerading as a fix.

**Sync direction: RC → dev (VisIt pattern).** Fixes land
on `rc/v*` first, then get merged into `dev` as a
follow-up commit. The follow-up commit's message
references the RC commit by SHA:

```
fix(tags): add data-method=DELETE to tag delete form (#664)

Backport of rc/v1.1 commit 8d7989c4 to dev.
```

The opposite direction (dev → RC) is allowed for the
special case where a fix is developed on `dev` first and
backported. The PR description must say "backport from dev"
and link the original `dev` commit SHA.

**Enforcement.** The CI gate at
`.github/workflows/rc-lint.yml` parses every commit's
conventional-commit type prefix and fails the build on
disallowed types. The script at
`scripts/ci/lint-rc-commits.mjs` also has a `--local
<base>..<head>` mode for pre-push checks. The companion
test at `scripts/ci/lint-rc-commits.test.mjs` (run in CI
on every PR to dev + stable via the test workflow) pins
the classifier behavior on each allowed + disallowed type.

### Promotion: dev → rc/v* → stable → tag

**`stable` is always releasable.** No direct commits to
`stable`. No merge into `stable` that does not first pass
the full test suite + visual sweep on `rc/v*`, per the
gate chain in ADR 0008.

The promote flow (PR via GitHub UI) is:

1. **When RC1 cuts: create `rc/v<MAJOR>.<MINOR>` from
   `dev` HEAD.** This is the moment the v1.1 line enters
   feature freeze per ADR 0011. New features keep going
   to `dev` for the next release; `rc/v*` accepts only
   fixes. The commit cut + the manifest's first RC entry
   happen in the same slice.
2. **Operator runs `make promote-dry-run`** — runs the
   gate chain (gates 1-9 from ADR 0008) against
   `rc/v*` HEAD and prints the result. No push, no tag,
   no PR.
3. **Operator runs `make promote`** — runs the gate chain
   (halts on failure). On success, it opens a PR
   `rc/v* → stable` via `gh pr create` with the gate-chain
   output in the PR body. No code is merged yet.
4. **Operator reviews the PR in the GitHub UI** — reads
   the diff, the gate-chain output, the commit log, and
   any CI annotations. The operator is the merge
   authority; no auto-merge is configured.
5. **Operator merges the PR via the GitHub UI** — CI
   re-runs as part of the merge branch protection rules.
6. **Operator runs `scripts/release-github.ps1`** — tags
   the merge commit on `stable`, pushes the tag, opens a
   draft GitHub release.

The `rc/v*` branch is **not** deleted after step 6. It
stays around as the patch-release maintenance line:
`v1.1.1`, `v1.1.2`, etc. cherry-picks come from
`rc/v<MAJOR>.<MINOR>`. New development for `v1.2` continues
on `dev`.

If `rc/v*` has commits `stable` doesn't have, `make
promote` aborts and instructs the operator to run
`make promote-prep` to sync. See ADR 0009 §"Conflict
policy" for the resolution flow. Until `make promote` is
implemented, do not promote `rc/v*` to `stable` without
explicit user direction. If you think a promotion is
needed, ask first.

### `main` is frozen

`main` is the legacy production-history anchor. It exists
so future agents (human or LLM) can reference "what users
had on 2026-07-03" without grepping the commit log. Do not
commit to `main`, do not force-push, do not delete it. If
a future refactor needs to clean up the branch list, that
is a separate ADR; this ADR explicitly preserves `main`.

### Branch hygiene

- **No dangling branches.** A `feature/<short-kebab>`
  branch exists to land ONE feature. Once the PR is merged
  into dev, **delete the branch in the same merge step** —
  on GitHub: "Delete branch" is a checkbox in the merge
  dialog. Locally: `git branch -d <name>` (or `-D` if the
  merge cleanup left a stray). A branch that has been
  merged but not deleted is a code-smell: it suggests the
  author either forgot or the work was abandoned silently.
- **Large feature branches need a PR.** When a feature
  branch is used (per the criteria above), the user
  expects a PR against dev. Open the PR **the moment the
  first green commit lands** — even if the work is
  WIP — so the user can see progress, comment, and request
  changes. Do not let a branch accumulate commits in
  isolation for days before opening a PR.
- **No `wip`, no `temp`, no `agent-scratch`.** Branch
  names that signal indecision are noise. If a branch is
  not worth naming, the work belongs on dev directly (per
  the default above).
- **The five dead-branch sweep** (June 2026) cleaned up 5
  abandoned branches from the same date whose underlying
  work had been re-implemented and shipped via other paths.
  The cost was ~30 minutes of triage + the loss of context
  the abandoned branches carried. The new policy is the
  lesson: don't create a branch you aren't going to merge
  + delete within the same session.

### Pushing and verification

- **Before pushing:** `make test` (runs `go test ./...
  -short`) and `make tpl` (regenerates
  `internal/templates/*_templ.go` from the `.templ` sources
  — these generated files are **gitignored**, so the diff
  after `make tpl` only shows up in your working tree,
  never in the PR. CI regenerates them in the workflow
  step before tests run). If you touched htmx or templ
  markup, run `node audit/smoke.mjs` against a live
  `dixiedata-web` server. `make audit` runs the full
  visual sweep.
- **Bake-style generator discipline (issue #589 / #591):**
  any PR that touches a script under `scripts/bake-*/` or
  a package whose directory contains a gitignored
  `baked*.go` MUST run `make verify-fresh-bake` before
  `git commit`. That target deletes every gitignored
  generated artifact (`internal/templates/*_templ.go`,
  `internal/releasehistory/baked.go`,
  `internal/activityhistory/baked.go`), regenerates them
  via `make tpl`, then runs `make test`. Issue #588's
  bootstrap-ordering bug class was missed by the standard
  pre-push discipline because stale gitignored files on the
  contributor's machine masked the real compile failure —
  the clean-tree gate closes that gap. The deletion list
  lives inline in the Makefile target; update it when adding
  a new bake-style generator so the gate stays accurate.
  The `make lint-bake-bootstrap` probe is the structural
  counterpart: it walks every `scripts/bake-*/main.go` and
  asserts no script imports the package it is responsible
  for generating (the exact bug shape from #588). CI runs
  the probe under `--strict` on every PR; a future PR that
  reintroduces the direct import will fail the gate.
- **CHANGELOG:** every user-visible change gets a bullet in `CHANGELOG.md` `[Unreleased]` under `### Added`, `### Changed`, `### Fixed`, or `### Maintenance` in the same commit that lands the change. Internal refactors that don't change user-visible behavior live under `### Maintenance`.
- **Click-driven surfaces:** any new templ button that POSTs and expects navigation must follow the recipe in `internal/templates/components/conventions.md` ("Buttons that POST and expect navigation") AND grow a matching `audit/smoke.mjs` assertion that verifies both the response shape AND `page.url()` after the click. The response-only assertion is insufficient — that is how the htmx `hx-swap="none"` + 303 silent-swallow bug shipped (commit `70878ac` → caught in `3612dab`).

### Feature apply sites — the checklist rule

When a feature issue lists **v1 apply sites** (e.g. issue
#183: "Browse row chip + Person Record detail page tag
editor + `/tags` management page"), the apply sites are a
**checklist, not a paragraph**. The feature is not "shipped"
until every checkbox is checked.

- **Issue body format:** list v1 apply sites as a markdown
  checklist (`- [ ] ...`), not a prose sentence. Prose
  hides gaps; checkboxes make gaps visible.
- **PR description format:** mirror the issue's checklist
  in the PR body. Each landed commit covers one or more
  boxes; check them off in the PR description as the
  commits land.
- **Implementation discipline:** if a commit lands only
  the backend (service + handler + route) for an apply
  site, the PR must NOT be marked ready-for-review until
  the matching templ/JS UI is in the same PR (or in a
  linked follow-up PR with its own checklist). The "ship
  backend first, UI later" anti-pattern is the root cause
  of the 4 "shipped but invisible" bugs surfaced in
  issue #257's sweep.
- **Orphan detection:** run
  `node audit/discover_orphan_handlers.mjs` in CI. The
  probe greps `internal/appshell/routes.go` for registered
  handlers and `internal/templates/*.templ` for any
  invoker (button/form/data-action referencing the
  route). Handlers with no invoker are flagged. This
  catches the pattern before the user does.

## Agent skills

### REPL-driven reasoning (Python scratch)

[`docs/agents/repl.md`](docs/agents/repl.md) covers the boundary rule
for the vendored `repl` skill (`.agents/skills/repl/`, sourced from
`valueforvalue/my-skill-framework`). Python is a scratch tool for
deterministic investigation — parsing, numeric sweeps, format
validation. NEVER imported by Go code, NEVER in CI. If a Python script
proves useful enough to keep, port it to Go.

### Working guides (read before touching the layer)

These two files are the high-leverage pre-commit reads. They document
recurring bug patterns extracted from 79 `fix:` commits across the
history. Read the section that matches the layer you're about to
touch:

- [`docs/COMMON_BUGS.md`](docs/COMMON_BUGS.md) — bug-pattern catalog
  by layer (HTMX wiring, templ markup, frontend JS, Go backend,
  Typst, accessibility, calendar/API, build/CI, database,
  debugging). Includes `Find it:` greps for each pattern.
- [`docs/CODE_CHANGES.md`](docs/CODE_CHANGES.md) — cross-layer
  working contract. Read this when your change touches templ +
  htmx + JS + Go handler together (the chi-router migration and
  the hx-attr strip both shipped as one-system drift).

### UI map (read before any UI bug hunt or redesign)

[`docs/ui-map/README.md`](docs/ui-map/README.md) is the single
entry point for the UI reference. It contains:

- [`INDEX.md`](docs/ui-map/INDEX.md) — screen × component matrix.
- [`routes.md`](docs/ui-map/routes.md) — every route → owning
  screen + handler.
- [`surfaces.md`](docs/ui-map/surfaces.md) — canonical DOM IDs
  (`page.*`, `panel.*`, `tab.*`, `overlay.*`).
- [`components.md`](docs/ui-map/components.md) — atomic components
  (button, card, empty_state, field, pill, toast).
- [`glossary.md`](docs/ui-map/glossary.md) — region vocabulary
  (drawer/modal/panel/section).
- [`states.md`](docs/ui-map/states.md) — cross-cutting states
  (loading / empty / error / unauthorized).
- [`gaps.md`](docs/ui-map/gaps.md) — orphaned routes, unrouted UI,
  redundancy findings.
- [`wireframes/`](docs/ui-map/wireframes/) — one ASCII wireframe
  per screen, with HTMX wiring tables and per-screen footguns.

When hunting a UI bug, start at the wireframe for the affected
screen, then follow the HTMX wiring table to the owning handler.
  htmx + JS + Go handler together (the chi-router migration and
  the hx-attr strip both shipped as one-system drift).

### Adding a feature

[`docs/agents/feature-protocol.md`](docs/agents/feature-protocol.md)
is the canonical procedure for adding a new feature. Read it
end-to-end before any feature work. The protocol covers the 3-tier
commit rule, module discipline (define the facade before the
internals; service is the seam; two-adapter rule), pipeline phasing
(lightweight issue body by default; `docs/RESEARCH.md` →
`docs/PRD.md` → `docs/TASKS.csv` only for 6+ file cross-layer
features), and the per-layer "when to load what" table. The
protocol references RPCI; you don't need to re-read it separately.

### Docs index (progressive disclosure)

[`docs/agents/INDEX.md`](docs/agents/INDEX.md) is the 3-tier
progressive-disclosure table for `docs/`. **Read it at session
start** to know which tier-1/2 docs to load for your task. Tier 0
is always loaded (already cross-referenced from this file); Tier 1
loads by matching the task to a role; Tier 2 (under
`docs/historical/`) loads only on explicit reference.

### Issue tracker

Issues are tracked in GitHub Issues for this repository using the `gh` CLI.
See `docs/agents/issue-tracker.md`. Labels follow the 6-axis taxonomy
in [`docs/agents/triage-labels.md`](docs/agents/triage-labels.md):
**Type × Status × Area × Priority × Cohort × Meta**. Use
`./scripts/sync-labels.sh` (idempotent) to apply the label set and
`./scripts/backfill-labels.sh` to apply area/priority labels to
existing open issues.

### Triage labels

The triage label vocabulary uses the canonical labels: `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, and `wontfix`. See
[`docs/agents/triage-labels.md`](docs/agents/triage-labels.md) for the full
6-axis taxonomy (Type × Status × Area × Priority × Cohort × Meta) and
the `scripts/sync-labels.sh` + `scripts/backfill-labels.sh` helpers.

### Domain docs

This repo uses a single-context domain-doc layout centered on the root `CONTEXT.md`;
ADRs live under `docs/adr/` when present. See `docs/agents/domain.md`.

### Native dialog guard law (read before adding any export or import)

Wails v2.12.0 on Windows crashes the frontend process if two native `SaveFileDialog`
or `OpenFileDialog` calls land on the UI thread at the same time. Every export and
import handler MUST guard its native dialog call with `a.inFlight.LoadOrStore` +
`defer a.inFlight.Delete`. **Before adding or modifying a handler that opens a
native dialog, read [`docs/agents/dialog-guard.md`](docs/agents/dialog-guard.md)
end-to-end.** The pattern (helper, inline, or sentinel-error) and the regression
tests are documented there. The rule is also encoded at glossary level in
`CONTEXT.md` under "Laws (non-negotiable)" so domain work can't drift past it.

### CLI / headless mode (read before adding non-GUI entry points)

`dixiedata --smoke` boots without the GUI for CI and user support.
The full subcommand roadmap (`doctor`, `list`, `show`, `search`,
`export`, `import`, `migrate`, `backup`, `restore point`, `logs`,
`config`, `debug`) is staged across 7 phases in
[`docs/agents/cli-plan.md`](docs/agents/cli-plan.md). Phase 1
(`smoke`) is shipping now. **Before adding any new subcommand, read
the phase layout in that doc** — every CLI command dispatches to
existing `*App` methods, never duplicates handler logic.

## LLM session protocol

These rules govern how an LLM agent behaves in a session on this repo.
They are the procedural complement to the architectural boundaries in
[`CONTEXT.md`](CONTEXT.md) and [`AGENT_ARCHITECTURE_MAP.md`](AGENT_ARCHITECTURE_MAP.md):
those define what shape the code must take; this defines how the agent
works until the code ships.

- **No implementation before direction approved.** Recon (read, grep,
  find, list) is always fine. Implementation (edit, write, scaffolding
  new files, running `make` targets that mutate state) waits for
  explicit approval of a proposed direction. LLM agents default to
  writing more than necessary; the gate keeps that in check.
- **Batched discovery.** Up to 3 focused questions per turn, batched
  in a single `ask_user_question` call. One-at-a-time questioning
  breaks flow. Skip discovery entirely if the request is already clear.
- **YAGNI — features only, not interfaces.** Do not introduce
  *features* (methods nobody calls, branches nobody exercises,
  parameters nobody passes) for requirements that do not exist
  yet. Speculative behavior creates more problems than it solves.
  Interface shape is a separate question. Apply the
  net-complexity-gain test in `docs/agents/complexity.md` §1: a
  slightly broader interface that costs the implementer an hour but
  saves every caller from relearning the module earns its keep.
  The two-adapter rule in `feature-protocol.md` §Module discipline
  and the deep-module rule are the operational form.
- **Bias toward action.** When two options are close in quality, pick
  one and go. Movement creates clarity. The cost of "wrong choice
  easily reversible" is lower than the cost of a long deliberation.
- **Proportional depth.** Match the weight of the process to the
  weight of the task. A small bug fix may need zero questions; a new
  subsystem deserves a more thorough exploration. Let task complexity
  guide conversation complexity.
- **Pragmatic principles as guides, not laws.** When a slice is
  about to violate a principle documented in
  [`docs/agents/pragmatic-principles.md`](docs/agents/pragmatic-principles.md)
  (DRY, orthogonality, Law of Demeter, etc.), the agent's job is
  to **warn + cite** in the slice Plan, not to silently violate
  or silently refuse. The Plan's "Principle warnings" block names
  the principle, the operational form, the rationale for the
  temporary violation, and the cleanup plan. The user signs off
  as part of the Plan approval gate. An undocumented violation is
  a bug; a documented violation is a known and intentional
  exception. See the doc's §"The warn + cite protocol" for the
  full shape.

### Capturing decisions

- **Default:** capture in the conversation. The user is the chat.
- **Default flow for non-trivial work:** the user invokes RPCI
  (Research → Plan → Critique → Implement). The full procedure
  is in `docs/agents/rpci.md`; the short form is: research enough
  to write a plan, write the plan with surfaced decisions,
  critique with the user, implement slice by slice with
  regression tests. The Critique phase is the gate — no
  implementation before the user signs off.
- **Promote to ADR:** if a decision is durable enough that the next
  LLM session (or a human six months from now) needs to know it
  without reading the chat log, write `docs/adr/000N-<slug>.md`.
  Match the existing ADR shape in that directory.
- **Never inline in source.** Source-file comments are reserved for
  non-obvious code, not session breadcrumbs. Inline comments about
  "we discussed this" rot.
