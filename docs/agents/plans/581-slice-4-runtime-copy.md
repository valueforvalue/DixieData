# Plan — issue #581, Slice 4: runtime + server + startup + CLI copy audit

This plan covers **Slice 4 of issue #581** — the audit + remediation
of the runtime user-facing copy that the first three slices did not
cover: frontend JavaScript literals (toasts, alerts, preview states,
modal titles), server response strings (validation, not-found, success,
warning, error messages surfaced through pages or toasts), startup
resources (loading placeholder, Local Archive quote content), and
the CLI interface (help, version, usage, command results).

Slice 1 (Static Archive HTML) shipped in `ed37efd`. Slice 2 (Typst PDF)
in `822b40b`. Slice 3 (iCalendar, audit-only) in `1b7fa1b`. This
slice is the largest — it must be a single reviewable unit but spans
4 distinct surfaces.

## Why this is its own slice

**#581 locked decision 5**: "Existing #561 rules remain the
standard." Slices 1 + 3 already covered HTML-and-prompt-emitting
frontends; slice 2 covered PDF; this slice covers what they did
not — dynamic runtime copy produced by `frontend/app.js` +
server-rendered messages that aren't in the `.templ` files + the
loading screen + the CLI surface.

**#581 slice plan (from the issue body)**: "Close runtime copy
gaps in frontend JavaScript, server responses, startup resources,
and CLI interface text; correct coverage documentation."

## Surface inventory (verified by direct read)

### 4.A — frontend/app.js (~6,812 lines)

User-facing copy producers:

| Pattern | Approx count | Notes |
|---|---:|---|
| `showToast(message, kind)` | 30+ call sites | per-slice 1 audit + per-event ones; many ARE action-form already (#561 left) |
| `confirm(`, `prompt(`, `alert(` | 0 | codebase uses showToast only — verified by grep |
| `confirm(...)` legacy? | 0 | (verified above) |
| modal context-menu text | scattered in `flashes`, foldouts, popovers | a separate audit pass |

Existing precedent: `669a761` slice 16 fixed `SoldierCard` + others
in this file. The audit-class surface this slice targets is
**leftover** candidate strings the earlier sweep did not catch.

### 4.B — server response strings (`internal/appshell/app.go` + handlers)

User-visible server strings surface via:
- HTTP responses that contain pages or toasts
- JSON error envelopes consumed by `frontend/app.js`

There is **no central helper today**: each handler builds its own
error / success / warning message inline. A probe can scan the
handler packages for strings used in `fmt.Fprintln`, `http.Error`,
and templ-rendered `c.String` / `c.HTML` responses.

### 4.C — startup resources

Two surfaces:
1. `internal/appshell/app.go:456` — the Loading DixieData screen
   ("Loading DixieData..." + "The local archive is still starting
   up. This screen will refresh automatically.")
2. The Local Archive quote content — sourced from a fixture; needs
   recon to confirm where it lives and whether it has redundancy.

### 4.D — CLI help text (`main.go:cliHelpText`)

Hand-maintained help text covering 13 verbs. Per `docs/agents/cli-plan.md`,
the full roadmap covers 7 phases (smoke, doctor, list/show/search,
export, import, migrate/backup/restore point/logs/config, debug).
The first three slices did not touch CLI copy.

**Drift guard**: `main_test.go` asserts every verb the dispatcher
knows about appears in the help text. The microcopy audit looks
separately at the verb descriptions for verbosity / duplication /
self-explanatory headings.

## Recon catalog (audit-only candidates)

Because recon is partial here, the plan rows below are
**provisional**: the Plan-and-critique gate will reconfirm each
in the user-facing chat before slice work begins.

| # | Surface | Class | Disposition |
|---|---|---|---|
| 1 | `app.js` ~30 `showToast()` call sites | scan for verbose body / duplicated narration | TBD — likely audit-only with a small fix list |
| 2 | `app.go:456` loading placeholder | "Loading DixieData..." is acceptable per slice 1 patterns; the second sentence ("The local archive is still starting up. This screen will refresh automatically.") repeats the heading | drop OR keep per slice 1 patterns |
| 3 | `main.go:cliHelpText` (~13 verb lines) | scan for `Headless archive operations` eyebrow / verbose intro paragraph | likely keep (CLI help is structurally different from `.templ` headings) |
| 4 | `fmt.Fprintln(opts.Writer, ...)` error/warning lines in `cli_*.go` | scan for "no migration needed (already at current)"-shaped status messages | accept (CLI researcher-facing copy is precise by design) |
| 5 | `cli_debug.go:846` `usage: dixiedata debug request <path>` | single sub-sub-command usage line | accept |

The slice's job is to surface any actionable microcopy edits and
ship them per RPCI; the catalog freezes the rest.

## Slice decomposition (one commit = one reviewable unit)

### Slice 4A — Recon + probe + driver wiring + RED/GREEN fixtures

**Goal**: ship the probe(s) pinning the runtime copy verbatim,
**before** any drift happens. The slice may end up GREEN-on-HEAD
(audit-only) per the slice-3 path; recon determines which.

**Files added**:

- `audit/smoke_runtime_microcopy.mjs` — covers `frontend/app.js`,
  `internal/appshell/app.go`, `main.go:cliHelpText`,
  `internal/appshell/cli_*.go::fmt.Fprintln(opts.Writer, ...)`,
  and the startup-placeholder HTML. Asserts the canonical strings
  the recon catalog freezes.
  - Required: each repo-load copy that's a pinned user-facing
    string (loading screen title + body, CLI help opener + a few
    verb descriptions, the most-trafficked toast messages).
  - Forbidden: any redundant narration patterns the earlier
    audits missed (the slice may find 0 or a few).
  - Source override: `RUNTIME_SOURCE` env (file path; probe scans
    raw Go / JS source for substring matches).
  - Skips: JSON keys, route names, CSS, generated Tailwind output,
    service-worker plumbing, error chains not shown to researchers.
- `audit/smoke_runtime_microcopy.test.mjs` — 12+ assertions per
  the recon findings: baseline-on-HEAD + per-rule synthetic
  positives + strict-mode flip.
- `Makefile`: add `lint-runtime-microcopy`, `…-strict`, `…-test`.
- `.github/workflows/test.yml`: add all three to the audit step.
- `docs/agents/ux-microcopy.md`: new "Runtime coverage (issue
  #581)" section.

**Per `docs/agents/tdd.md`**: the GREEN-on-HEAD assertions ARE the
acceptance criterion. If a few actionable microcopy edits land
in slice 4B, those fixes + retargeted assertions land together.

### Slice 4B — Fixes (if any)

**Goal**: only if recon surfaced actionable edits (the slice-2A
status determines this). If yes, the slice lands them per RPCI:
- 1–3 microcopy edits in `frontend/app.js` (likely drop a duplicate
  toast narration across call sites);
- 0–1 edit in `internal/appshell/app.go:456` (loading screen
  second sentence);
- 0 edits in `main.go:cliHelpText` (likely freeze the text as-is,
  per slice 3 pattern).

**If recon returns 0 actionable edits**, slice 4B is skipped
altogether and the slice lands as audit-only — same shape as
slice 3.

### Slice 4C — CHANGELOG + commit

**Goal**: close the slice with the same review-trail shape that
slices 1, 2, and 3 used.

**Files changed**:

- `CHANGELOG.md`: `[Unreleased]` `### Fixed` or `### Maintenance`
  bullet (depending on whether 4B landed any user-visible edits)
  referencing #581 + the regression net.

**Commit subject**: `runtime: tighten runtime + server + startup +
CLI microcopy (#581 slice 4)` (or `runtime: pin runtime + server +
startup + CLI copy via format-aware microcopy probe (...)` for the
audit-only shape, matching slice 3).

## Acceptance criteria for the whole slice

- [ ] `node audit/smoke_runtime_microcopy.mjs --strict` exits 0
      (whether audit-only or post-fix).
- [ ] `node audit/smoke_runtime_microcopy.test.mjs` all green.
- [ ] `make lint-runtime-microcopy` / `-strict` / `-test`
      all exit 0.
- [ ] CI audit step runs all five probes alongside
      `smoke_microcopy.mjs` + `smoke_static_archive_microcopy.mjs`
      + `smoke_pdf_microcopy.mjs` + `smoke_icalendar_microcopy.mjs`.
- [ ] `docs/agents/ux-microcopy.md` carries the "Runtime coverage"
      paragraph mirroring the prose of slices 1 + 2 + 3.
- [ ] `CHANGELOG.md` `[Unreleased]` bullet references #581 + the
      regression net (Fixed if 4B landed edits; Maintenance if
      audit-only).
- [ ] `go test -short ./...` stays green.
- [ ] `npm run typecheck` clean; `npm run lint:js` clean.

## Open questions for the user before 4A ships

1. **Recon depth** — should the probe cover ~30 `showToast()` sites
   in `frontend/app.js` (high signal; high effort) or just the
   top-traffic ~10 (lower signal; slice-1 precedent of catching
   duplicate narration across call sites)?
2. **Start-screen second sentence** — drop or keep? The sentence
   "The local archive is still starting up. This screen will
   refresh automatically." repeats the heading semantically.
3. **CLI help verb descriptions** — the 13 hand-maintained verb
   lines; any obvious verbosity or duplication, or accept as-is?

## Principle warnings (`docs/agents/pragmatic-principles.md`)

- **YAGNI (§1) — verified.** Recon will likely surface 0–3 small
  edits; the rest is audit-only. Probe scope stays bounded; we
  do NOT generalise into a JS-linter.
- **DRY (§2) — not violated.** The probe is its own file; no copy
  dedup needed at this layer.
- **Orthogonality (§4) — justified.** Format-aware probe follows
  slices 1 + 2 + 3 + (this slice) precedent. Slice-4 is the
  final probe family before the whole #581 audit closes.

## Verification commands

```text
node audit/smoke_runtime_microcopy.mjs --strict         # 0 findings
node audit/smoke_runtime_microcopy.test.mjs             # all assertions green
node audit/smoke_icalendar_microcopy.mjs --strict       # slice-3 still green
node audit/smoke_pdf_microcopy.mjs --strict             # slice-2 still green
node audit/smoke_static_archive_microcopy.mjs --strict  # slice-1 still green
node audit/smoke_microcopy.mjs --strict                 # slice-1 still green
go test -short -count=1 ./...                            # no regressions
npm run typecheck                                         # clean
npm run lint:js                                           # clean
```

## Out of scope (deferred to future issues per #581 locked decisions)

- Refactoring the 30+ `showToast()` call sites into a typed
  helper (would touch every page; separate concern).
- Rewriting the CLI roadmap past `--smoke` (per
  `docs/agents/cli-plan.md`).
- New export surfaces; visual redesign.
- Full-coverage JS linter; the probe is microcopy-class only.

## Related

- `1b7fa1b` — slice 3 (iCalendar, audit-only).
- `822b40b` — slice 2 (Typst PDF).
- `ed37efd` — slice 1 (Static Archive HTML).
- `docs/agents/ux-microcopy.md` — the rule spec the probe
  enforces.
- `docs/agents/cli-plan.md` — the CLI subcommand roadmap; CLI
  copy must align per that doc.
- `docs/agents/tdd.md` — RED-first discipline.
- `docs/agents/notes/slice3-decomposition.md` — the
  decomposition pattern.
