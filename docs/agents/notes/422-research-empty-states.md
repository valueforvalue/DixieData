# #422 — Empty-state 500 on Research sub-pages: fix plan

Issue: https://github.com/valueforvalue/DixieData/issues/422
Branch: dev
Commit: 1818d2e (slice 3 landed)

## Decision

**Both** (picker intelligence + handler empty states), shipped as two slices.

## Slice 1 — Handler empty states (stop the bleeding)

All 8 handlers render a 200 (not 500) for empty-soldier scenarios. This fixes both the picker flow AND direct URL access (e.g. `/soldiers/3/camaraderie` typed directly).

### Changes

| File | What |
|---|---|
| `internal/records/soldier_service.go` | Export sentinel errors: `ErrNoUnitInfo`, `ErrNoResearchPackState`, `ErrNoResearchPackCounty` (or wrap existing errors so handlers can `errors.Is`) |
| `internal/appshell/research_handlers.go` | Replace `respondInternal` with empty-state render for "not enough data" errors; normalize error dispatch across all 8 handlers |
| `internal/templates/research_empty_states.templ` | New file: empty-state components for camaraderie, research-pack/state, research-pack/county. Each takes the soldier's name/id and a message like "No unit information recorded. Edit the soldier record to add a unit, regiment, or company." |
| `internal/presentation/views.go` | New wrappers: `UnitCamaraderieEmpty`, `ResearchPackEmptyState`, `ResearchPackEmptyCounty` |
| `internal/appshell/research_context_handlers_test.go` | RED-first tests: `TestHandleUnitCamaraderieRendersEmptyStateForUnitlessSoldier`, `TestHandleResearchPackStateRendersEmptyState`, `TestHandleResearchPackCountyRendersEmptyState`, `TestHandleAddResearchTaskRejectsBlankTitleWith400`, `TestHandleResolveResearchTaskReturns404ForMissingTask` |
| `CHANGELOG.md` | `### Fixed` bullet under `[Unreleased]` |

### Error dispatch normalisation

| Handler | Current | Fixed |
|---|---|---|
| camaraderie | "not found" gate (dead) → 500 | `errors.Is(err, ErrNoUnitInfo)` → empty-state 200; `errors.Is(err, sql.ErrNoRows)` → 404 |
| timeline | "not found" gate (dead) → 500 | `errors.Is(err, sql.ErrNoRows)` → 404; everything else → 500 |
| research-pack | "not found" gate (dead) → 500 | `errors.Is(err, ErrNoResearchPack*)` → empty-state 200; `errors.Is(err, sql.ErrNoRows)` → 404 |
| research-log (view) | all errors → 404 | `errors.Is(err, sql.ErrNoRows)` → 404; everything else → 500 |
| research-log (add task) | all errors → 500 | Validation errors → 400; `sql.ErrNoRows` → 404; everything else → 500 |
| research-log (resolve) | all errors → 500 | "task not found" → 404; everything else → 500 |
| conflict-ledger | all errors → 404 | `sql.ErrNoRows` → 404; everything else → 500 |

Commit shape: `fix(research): empty-state 200 for camaraderie + research-pack missing-data paths; normalize error dispatch (issue #422 slice 1)`

## Slice 2 — Picker intelligence

Hide unsupported sub-pages from the picker so users never click into an empty page. Layer 1 already renders graceful empty states, so this is defense-in-depth.

### Changes

The picker page (`/research`) reads the current person from cookie, checks which 5 soldier-scoped sub-pages are supported:

| Sub-page | Supported when |
|---|---|
| Camaraderie | `unit != ""` |
| Timeline | always (works with empty soldier) |
| Research Log | always |
| Conflict Ledger | always |
| Research Pack / State | `pension_state != ""` OR `birth_info` has state |
| Research Pack / County | `birth_info` has county |

Unsupported sub-pages are still shown in the foldout (changing layout.templ is out of scope) but dimmed/greyed with a tooltip "No unit information available yet." Or hidden entirely. Decision TBD.

### Files

| File | What |
|---|---|
| `internal/appshell/research_picker_handlers.go` | `handleResearchPicker` reads current person, computes `supportedNextActions` map, passes to viewmodel |
| `internal/viewmodel/types.go` | `ResearchPickerView` gains `SupportedActions map[string]bool` |
| `internal/templates/research_picker.templ` | Continue shortcut + search result forms conditionally render or dim unsupported sub-pages |
| CHANGELOG | `### Changed` bullet |

Commit shape: `research(picker): hide unsupported sub-pages when soldier data is missing (issue #422 slice 2)`

## Ship check

- [ ] `just test-lint && go test -short ./...` → all GREEN (including new RED-first tests)
- [ ] `audit/discover_orphan_handlers.mjs` → no new orphans
- [ ] Manual: create soldier with empty unit + empty birth_info → picker opens → click Camaraderie → empty-state page (not 500)
- [ ] Manual: same soldier → Research Pack → County → empty-state page (not 500)
- [ ] Manual: same soldier → Research Pack → State → empty-state page (not 500)
