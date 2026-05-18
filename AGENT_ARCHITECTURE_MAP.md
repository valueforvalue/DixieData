# AGENT_ARCHITECTURE_MAP

## Agent-Friendliness Grade

**Grade: A**

The architecture now follows deep-module and grey-box boundaries closely enough that a fresh agent can navigate the system through package names and facade seams instead of reverse-engineering one large file and one overloaded package. Runtime complexity is concentrated inside domain packages (`internal/records`, `internal/archive`, `internal/integrations`) while the delivery layer consumes stable DTOs and facade contracts.

## Audit Method & Verification

- Mapped the refactor around runtime package boundaries, template imports, and `app.go` dependencies.
- Split the old `internal/services` runtime code into domain packages and left `internal/services` as a compatibility shim only.
- Added delivery-only DTOs in `internal/viewmodel` and presentation adapters in `internal/presentation`.
- Replaced `app.go` concrete service fields with facade interfaces.
- Ran `go build ./...` after the refactor.
- Ran `go test ./...` after the refactor.

### Verification Findings

- **Build status:** pass (`go build ./...`).
- **Test status:** fail (`go test ./...`) due pre-existing cross-platform path and Windows-path expectation tests, not due compile/boundary regressions. Remaining failures are in:
  - `/home/runner/work/DixieData/DixieData/app_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/appdata/appdata_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/archive/backup_service_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/integrations/google_service_test.go`

## Current Deep Modules

1. **Delivery Surface**
   - `/home/runner/work/DixieData/DixieData/app.go`
   - `/home/runner/work/DixieData/DixieData/app_facades.go`
   - `/home/runner/work/DixieData/DixieData/internal/presentation`
   - `/home/runner/work/DixieData/DixieData/internal/templates`
   - `/home/runner/work/DixieData/DixieData/internal/viewmodel`
   - Responsibility: HTTP/Wails routing, request parsing, dialog orchestration, DTO mapping, and rendering.

2. **Records Domain**
   - `/home/runner/work/DixieData/DixieData/internal/records`
   - Responsibility: record lifecycle, search, review workflow, analytics, camaraderie, timelines, research logs, collections, and duplicate comparison logic.

3. **Archive Domain**
   - `/home/runner/work/DixieData/DixieData/internal/archive`
   - Responsibility: backup/shared archive import-export, printable/export artifacts, diagnostics bundles, image storage/orphan management, and archive zip mechanics.

4. **Integration Domain**
   - `/home/runner/work/DixieData/DixieData/internal/integrations`
   - Responsibility: Google auth, drive upload, sheets export, and calendar sync.

5. **Core Data Kernel**
   - `/home/runner/work/DixieData/DixieData/internal/db`
   - `/home/runner/work/DixieData/DixieData/internal/models`
   - `/home/runner/work/DixieData/DixieData/internal/appdata`
   - `/home/runner/work/DixieData/DixieData/internal/dates`
   - Responsibility: persistence, schema, model storage contracts, app data paths, and date parsing.

## Before & After: Top 3 Shallow Spots

### 1) `app.go` Monolith

**Before**
- Held concrete service dependencies.
- Built review queue DTOs inline.
- Mixed routing with delivery shaping and domain wiring.

**After**
- Uses strict facade interfaces from `/home/runner/work/DixieData/DixieData/app_facades.go`.
- Routes requests to deep modules and presentation adapters only.
- Delivery shaping moved into `/home/runner/work/DixieData/DixieData/internal/presentation` and `/home/runner/work/DixieData/DixieData/internal/viewmodel`.

### 2) Overloaded `internal/services`

**Before**
- Soldier, backup, export, audit, analytics, diagnostics, image, and Google logic shared one namespace.
- Cross-domain helpers were effectively global inside a shallow package.

**After**
- Runtime code is split across:
  - `/home/runner/work/DixieData/DixieData/internal/records`
  - `/home/runner/work/DixieData/DixieData/internal/archive`
  - `/home/runner/work/DixieData/DixieData/internal/integrations`
- `internal/services` is reduced to a compatibility shim rather than the architectural center.
- Internal helpers stay package-private inside the new domain packages unless a narrow compatibility alias is required.

### 3) UI-to-Service Coupling

**Before**
- Templates imported `internal/services` and `internal/models` directly.
- Service/domain structs leaked straight into `.templ` signatures.

**After**
- Templates consume only DTOs from `/home/runner/work/DixieData/DixieData/internal/viewmodel`.
- `/home/runner/work/DixieData/DixieData/internal/presentation/views.go` is now the grey-box adapter between domain objects and rendered views.
- Service/domain changes are buffered behind DTO mappers instead of rippling directly into template signatures.

## Delivery Facades Used by `app.go`

- `soldiersFacade`
- `anniversaryFacade`
- `analyticsFacade`
- `reviewFacade`
- `imageFacade`
- `exportFacade`
- `backupFacade`
- `diagnosticsFacade`
- `integrationFacade`

These interfaces keep `app.go` focused on transport concerns while the deep modules own the behavioral complexity.

## Why the Grade Is Now A

- **Progressive disclosure is strong:** directory names now reveal the architecture immediately.
- **Grey-box delivery boundary exists:** templates only see DTOs, not storage or workflow structs.
- **Deep modules hide complexity:** archive mechanics, record workflows, and integrations each expose a compact public surface over large internal behavior.
- **`app.go` is constrained by contracts:** orchestration depends on interfaces rather than concrete god-services.
- **Refactorability improved:** package seams now align with domain responsibilities, so future work can target one module without touching the entire old `internal/services` namespace.
