# Plan — issue #582: full runtime microcopy sweep + loading-second-sentence decision

This plan covers **follow-up issue #582**, opened at the close of
issue #581's session. Slices 1–4 all shipped (commits
`ed37efd` + `822b40b` + `1b7fa1b` + `e565a8c`); slice 4 was
thin-recon per the Plan-and-critique gate. #582 catalogues the
deferred work and resolves it in one commit.

## What this issue actually needs (recon)

A "full toast sweep" reads as audit-only probes, but recon shows
two real surface gaps slice 4 skipped. The plan covers all three
items from the issue checklist, not just the toast probe.

### 1. Server-side toast strings (`X-DixieData-Toast` header)

The user sees these via the `dispatchDixieDataForm` path
(frontend/app.js:4269 reads `toastMessage =
response.headers.get("X-DixieData-Toast")`). Slice-4 probe
pinning 9 `showToast(...)` literals in `frontend/app.js`
**did not scan** the Go-side producers, so any drift in those
strings bypasses the regression net.

Production sites (verified by `grep -rn 'X-DixieData-Toast'
internal/appshell/` excluding `_test.go`):

| File:line | Verbatim | Class |
|---|---|---|
| `articles_handlers.go:731` | `fmt.Sprintf("Article PDF saved to %s.", filepath.Base(path))` | R3 action-form — accept |
| `events_handlers.go:902` | `fmt.Sprintf("Event PDF saved to %s.", filepath.Base(path))` | R3 action-form — accept |
| `calendar_handlers.go:96` | `"Identity saved. Loading DixieData..."` | R5 two-sentence — keep (second clause acknowledges the redirect auto-fire) |
| `soldiers_handlers.go:708` | `fmt.Sprintf("Display ID recovered: %s", newID)` | R5 status-form ("recovered") → rephrase to `"Display ID set to %s"` (action-form) |
| `exports_handlers.go:654 + :665` | `sanitiseToastForHeader(message)` | helper + caller-passed — out of reach of the probe |
| `respond.go:138 + :299` | `message` (caller-passed) | helper + caller-passed — out of reach of the probe |

Net actionable: **1 microcopy edit** (`soldiers_handlers.go:708`
passive → action-form). 1 decision documented
(`calendar_handlers.go:96` two-sentence keeps per #561 R5
status-form variant).

### 2. Remaining 15 untested `frontend/app.js` toast literals

`grep -n 'showToast(' frontend/app.js` shows 24 call sites.
Slice-4 pins 9; 15 remain. Categorised:

| # | Line | Verbatim | Notes |
|---|---|---|---|
| 1 | 1354 | `"Preview content was not available."` | already pinned (slice 4) |
| 2 | 1750 | `"Open a record with a saved Record ID before launching the scratch pad."` | long body (~83 chars); passes 80-char toast rule by header — accept |
| 3 | 1781 | `message \|\| "Scratch pad opened."` | caller-passed fallback — pin the literal |
| 4 | 1783 | `message \|\| "Scratch pad failed to open."` | caller-passed fallback — pin the literal |
| 5 | 1786 | `"Scratch pad failed to open."` | pin |
| 6 | 2646 | `"Saved local draft restored."` | already pinned (slice 4) |
| 7 | 3706 | `"No path to copy."` | already pinned |
| 8 | 3717 | `"Path copied."` | already pinned |
| 9 | 3719 | `"Could not copy the path. Long-press to select."` | pin |
| 10 | 4269 | `toastMessage \|\| "Request failed."` | caller-passed fallback — pin the literal |
| 11-13 | 4292/4327/4351 | `showToast(toastMessage, toastKind)` | pure caller-passed — out of reach |
| 14 | 4645 | `"Could not load print options."` | already pinned |
| 15 | 4787 | `"Nothing to copy."` | already pinned |
| 16 | 4792 | `"Clipboard helper unavailable."` | already pinned |
| 17 | 4800 | `"Copied: " + preview` | dynamic — pin the literal prefix `"Copied: "` |
| 18 | 4802 | `"Could not copy. Long-press to select."` | pin |
| 19 | 5569 | `warnings[0]` | dynamic — out of reach |
| 20 | 5571 | `warnings.length + " stale filter values; click 'Show details' for the list."` | dynamic string template — pin the literal suffix |
| 21 | 6485 | `"Choose exactly two records to compare."` | already pinned |
| 22 | 6661 | `"Browse refresh failed."` | already pinned |

Net new literal pins (not in slice-4 probe today): **9 strings**
across 7 call sites.

### 3. Loading-screen second sentence

`internal/appshell/app.go:460`:
> "The local archive is still starting up. This screen will
> refresh automatically."

Slice-1 R5 reads "verbose body under heading" but slice-1 also
preserves status-form copy when the message conveys state the
user needs ("No Soldiers match your filter" precedent). The
second clause "this screen will refresh automatically" is the
user-relevant signal — without it, a user unsure what's
happening would manual-refresh. **Keep** with a one-line doc
amendment to `docs/agents/ux-microcopy.md` justifying the
keep (slice-1 R5 carveout for status revealing state the user
needs).

### 4. CLI help verb descriptions

`main.go::cliHelpText` 13 verb lines — read for verbosity /
duplication / self-explanatory headings. Spot-audit confirms:
no verbosity (each is a single noun + a tabular list), no
duplication (each verb maps to one CLI subcommand), no
self-explanatory headings (the section is empty in verb
descriptions). **No edits; freeze as-is per slice-3 pattern.**

## Slice decomposition (one commit = one reviewable unit)

### Slice 582A — Probe + tests + driver + RED fixtures

**Goal**: ship the slice-4 probe extension pinning the 9 new
literal strings + the 4 server-side toast strings, plus tests
+ Make + CI + doc amendment, BEFORE code edits land. The probe
exits 0 on current repo (GREEN-on-HEAD) for the new pins; the
slice-1 R5 fix to `soldiers_handlers.go:708` will flip one pin
to "edited, retarget required".

**Files added / changed**:

- `audit/smoke_runtime_microcopy.mjs` extended:
  - 9 new literal pins in `frontend/app.js` (toast rows 2, 3,
    4, 5, 9, 10, 17, 18, 20).
  - 4 new pins in the server-side toast surface
    (`internal/appshell/{articles,events,calendar,soldiers}_handlers.go`).
  - 1 new forbidden rule: passive "recovered" / "saved" / "set"
    patterns in `soldiers_handlers.go` toast strings (small
    R5 carveout for the keep-cases above).
- `audit/smoke_runtime_microcopy.test.mjs` extended:
  - 4 new GREEN-on-HEAD baseline assertions (server-side toast
    literals present).
  - 6 new synthetic positives (drop one literal each; assert
    probe exits 1).
  - 1 new synthetic positive for the `soldiers_handlers.go`
    R5 fix.
  - 1 new synthetic positive for the `calendar_handlers.go`
    R5 freeze (assert no forbidden rule fires).
- `Makefile`: no new target (existing `lint-runtime-microcopy`
  picks up the rule additions).
- `.github/workflows/test.yml`: no change (same probe name).
- `docs/agents/ux-microcopy.md`: amendment to the
  "Runtime coverage" section enumerating the 9 new toast
  literal pins + the 4 server-side toast strings + the
  R5-carveout note for the loading screen + the explicit
  freeze of the CLI help text.

**Per `docs/agents/tdd.md`**: the RED tests in
`smoke_runtime_microcopy.test.mjs` are the acceptance
criterion. The slice is GREEN when the test suite is green on
the post-fix repo with `--strict` exiting 0.

### Slice 582B — Fixes (the actual microcopy edits)

**Goal**: the actionable edits recon surfaced.

**Files changed** (1 edit in 1 file):

- `internal/appshell/soldiers_handlers.go:708`:
  `fmt.Sprintf("Display ID recovered: %s", newID)` →
  `fmt.Sprintf("Display ID set to %s", newID)`.

Per `docs/agents/ux-microcopy.md` R5 (status-form → action-form
where the action is known), the canonical phrasing is verb-led.

**No other code edits**: the loading-screen sentence is frozen,
the CLI help text is frozen, and the server-side toast strings
that pass the audit are accepted.

**Constraint**: no domain meaning changes; no glossary
violations; no toast contract changes (`X-DixieData-Toast` /
`X-DixieData-Toast-Type` shape unchanged); the
`sanitiseToastForHeader` Unicode-rewrite unchanged.

**Files updated to drop assertions on removed text**: grep
`internal/appshell/soldiers_handlers_test.go` for the literal
`"Display ID recovered"` substring before committing. If found,
retarget to `"Display ID set to"` to match the edit.

### Slice 582C — CHANGELOG + commit

**Goal**: close the slice with the same review-trail shape that
slices 1–4 used.

**Files changed**:

- `CHANGELOG.md`: `[Unreleased]` `### Fixed` bullet naming
  #582 + the actionable edit + the regression net; reference
  the [Unreleased] Maintenance slice-4 bullet
  ("#581 ... audit-only") that this issue extends.

**Commit subject**: `runtime: rephrase soldiers handler toast
action-form + extend runtime microcopy probe (#582)`.

**Commit body** mirrors slice 1 / slice 2:

- First bullet: fix class summary (1 R5 rephrase in
  `soldiers_handlers.go`).
- Second bullet: regression net — extended
  `audit/smoke_runtime_microcopy.mjs` + `…test.mjs` (9 new
  literal pins + 4 server-side toast pins), ux-microcopy.md
  amended with the carveout + CLI freeze notes.
- Third bullet: out-of-scope notes (caller-passed `showToast`
  variants, `respond.go` dynamic toast surface, CLI verb
  descriptions — all freeze as-is).

## Acceptance criteria for the whole slice

- [ ] `node audit/smoke_runtime_microcopy.mjs --strict` exits 0.
- [ ] `node audit/smoke_runtime_microcopy.test.mjs` all green.
- [ ] `make lint-runtime-microcopy` / `-strict` / `-test` exit
      0.
- [ ] All 5 microcopy probes (`smoke_microcopy.mjs` +
      `smoke_static_archive_microcopy.mjs` +
      `smoke_pdf_microcopy.mjs` +
      `smoke_icalendar_microcopy.mjs` +
      `smoke_runtime_microcopy.mjs`) exit 0 under `--strict`.
- [ ] `docs/agents/ux-microcopy.md` "Runtime coverage"
      paragraph enumerates the new pins + the carveout + the
      CLI freeze.
- [ ] `CHANGELOG.md` `[Unreleased]` `### Fixed` bullet
      references #582 + the regression net.
- [ ] `go test -short ./internal/appshell/...` green
      (assertion retarget on `"Display ID recovered"`).
- [ ] `npm run typecheck` clean; `npm run lint:js` clean.

## Verification commands

```text
make lint-runtime-microcopy-strict         # 0 findings (GREEN-on-HEAD)
make lint-runtime-microcopy-test           # all assertions green
make lint                                  # 4 microcopy probes + 1 lint aggregate all green
go test -short -count=1 ./internal/appshell/...   # no regressions
```

## Out of scope (deferred to future issues per #582 locked decisions)

- Caller-passed `showToast(toastMessage, toastKind)` variant
  sites (lines 4292 / 4327 / 4351 in `frontend/app.js`, plus
  `respond.go`'s dynamic `message`) — out of probe reach; a
  dynamic-fixture test would land in a separate slice if a real
  drift surfaces.
- Refactoring `showToast()` into a typed helper (per slice-1 /
  slice-2 + `669a761` precedent; helper-extraction lives in its
  own slice).
- Visual redesign or new export surfaces.
- Full audit pass over the `respond.go` error envelope surface
  (~30 callers; user-visible surface only via the toast
  `message` arg, which the probe cannot pin).

## Related

- #581 — slices 1–4 (Static Archive, PDF, iCalendar, runtime).
- `e565a8c` — slice 4 commit (audit-only; thin-recon).
- `docs/agents/ux-microcopy.md` — the rule spec the probe
  enforces; this slice extends its coverage map.
- `docs/agents/tdd.md` — RED-first discipline.
- `docs/agents/notes/slice3-decomposition.md` — the
  decomposition pattern.
