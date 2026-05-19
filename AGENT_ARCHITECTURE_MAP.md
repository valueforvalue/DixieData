# AGENT_ARCHITECTURE_MAP

## Agent-Friendliness Grade

**Grade: A**

The repository now presents the architecture progressively: the repo root is a thin entry surface, the delivery shell lives under `internal/appshell`, and the behavioral complexity is concentrated inside the **Deep Modules** in `internal/records`, `internal/archive`, and `internal/integrations`. Templates remain behind the **Grey Box** `internal/presentation` -> `internal/viewmodel` boundary, so agents can follow the package layout instead of reverse-engineering cross-layer coupling.

## Audit Method & Verification

- Re-audited the root directory after the deep-module refactor to identify remaining delivery-shell noise.
- Moved the runtime delivery shell from the repo root into `internal/appshell`.
- Moved root-level delivery tests alongside that package so the root now contains only the actual executable entrypoint for runtime Go code.
- Moved the reusable build/run PowerShell entry scripts from the repo root into `scripts`.
- Moved the Find a Grave HTML fixture from the root into `tests/testdata`.
- Updated `main.go` to bind the internal app shell.
- Updated stress tests to target `internal/records` and `internal/archive` directly instead of the `internal/services` compatibility shim.
- Refactored the kept PowerShell scripts to resolve the repo root from `scripts\` and updated downstream test harness scripts that invoked moved delivery-shell tests.
- Re-ran `go build ./...`.
- Re-ran `go test ./...`.
- Executed `.\scripts\build-release.ps1` successfully from its new location.

### Verification Findings

- **Build status:** pass (`go build ./...`).
- **Test status:** pass (`go test ./...`).

## Current Layout

### Root entry surface

- `main.go`
- `README.md`
- `AGENT_ARCHITECTURE_MAP.md`
- `scripts`
- frontend assets and repo metadata

The root no longer carries the delivery runtime implementation, its tests, or PowerShell build/run entry scripts. `main.go` is the only runtime Go entrypoint at the repository root.

### Automation entrypoints

- `scripts/build-common.ps1`
- `scripts/build-debug.ps1`
- `scripts/build-demo-release.ps1`
- `scripts/build-release.ps1`
- `scripts/run-debug.ps1`
- `scripts/run-stress-tests.ps1`

Responsibility: repo-level build, packaging, debug launch, and stress-workflow automation through the standard Go/Wails toolchain.

### Delivery surface

- `internal/appshell/app.go`
- `internal/appshell/app_facades.go`
- `internal/appshell/stress_logging.go`
- `internal/presentation`
- `internal/templates`
- `internal/viewmodel`

Responsibility: Wails/HTTP delivery, request parsing, route orchestration, **Facades**, Grey Box presentation adapters, and rendering.

### Records domain

- `internal/records`

Responsibility: record lifecycle, search, review workflow, analytics, camaraderie, timelines, research logs, collections, and duplicate comparison logic.

### Archive domain

- `internal/archive`

Responsibility: backup/shared archive import-export, printable/export artifacts, diagnostics bundles, image storage/orphan management, and archive zip mechanics.

### Integration domain

- `internal/integrations`

Responsibility: Google auth, Drive upload, Sheets export, and calendar sync.

### Core data kernel

- `internal/db`
- `internal/models`
- `internal/appdata`
- `internal/dates`

Responsibility: persistence, schema, storage contracts, app-data paths, and date parsing.

### Compatibility shim

- `internal/services`

Responsibility: compatibility aliases only. It is no longer the architectural center and should not be the default target for new work.

## Suggested inspection order

1. `README.md`
2. `AGENT_ARCHITECTURE_MAP.md`
3. `internal/appshell/app.go`
4. `internal/appshell/app_facades.go`
5. `internal/presentation/views.go`
6. `internal/viewmodel`
7. `internal/records`
8. `internal/archive`
9. `internal/integrations`
10. `scripts`

## Final Boundary Notes

### Delivery shell

- `internal/appshell/app.go` remains transport/orchestration code only.
- Frontend-facing **Facades** are declared in `internal/appshell/app_facades.go`.
- `main.go` now only constructs the shell, binds it to Wails, and hosts embedded frontend assets.

### Grey Box boundary

- Templates consume `internal/viewmodel` DTOs/ViewModels only.
- `internal/presentation/views.go` is the adapter layer that converts domain objects into view-ready DTOs/ViewModels and rendered components.
- Template tests remain in `internal/templates` and stay on DTO/ViewModel-facing inputs instead of domain/database fixtures.

### Deep Modules

- `internal/records`, `internal/archive`, and `internal/integrations` own the behavioral complexity.
- Internal helpers stay package-private unless a narrow public contract requires exposure.

## Test Guardrails Status

The suite now acts as a reliable firewall for the architecture:

- `internal/appshell` contains delivery-shell and end-to-end HTTP smoke coverage next to the shell it verifies.
- `internal/templates` tests remain DTO-oriented and do not reach into raw database/domain services.
- `tests/stress` now exercises deep-module seams through `internal/records` and `internal/archive` directly rather than through the `internal/services` compatibility shim.
- Shared fixtures that support multiple packages now live under `tests/testdata` instead of the repo root.
- Full validation currently passes with `go build ./...` and `go test ./...`.

## Script Audit Log

### Kept and relocated

- `build-common.ps1` - kept as the shared script helper; now resolves the repo root from `scripts\`.
- `build-debug.ps1` - kept for debug builds and launcher generation.
- `build-demo-release.ps1` - kept for seeded demo packages.
- `build-release.ps1` - kept for production packaging; verified from `scripts\`.
- `run-debug.ps1` - kept for launching debug builds with UI IDs enabled.
- `run-stress-tests.ps1` - kept for the stress workflow and updated to target `internal\appshell` tests from the new script location.

### Updated supporting scripts outside `scripts`

- `tests\goldmaster\run-suite.ps1` - updated to resolve the repo root and run moved delivery-shell tests from `internal\appshell`.
- `tests\stress\filesystem-chaos.ps1` - updated to resolve the repo root and run the moved filesystem-chaos stress test from `internal\appshell`.

### Deleted

- None. Every root PowerShell script still provides distinct build, packaging, debug, or stress utility that is not replaced by static documentation alone.
