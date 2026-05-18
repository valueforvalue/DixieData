# Refactor Plan

## 1. Decouple the UI (Templates)
- Create `/home/runner/work/DixieData/DixieData/internal/viewmodel` as the delivery-only DTO package.
- Move all template-facing shapes behind Plain Old Data DTOs:
  - record/search/form DTOs for soldier list/detail/search/calendar/entry screens
  - review DTOs for review queue, comparisons, orphan images, and merge conflicts
  - analytics DTOs for insights and research packs
  - workflow DTOs for camaraderie, timelines, research logs, collections, and conflict ledgers
  - integration DTOs for Google status/share screens
- Add mapper functions in `internal/viewmodel` so `app.go` converts domain results into DTOs before calling templates.
- Update `.templ` files and template tests so templates import `internal/viewmodel` only.

## 2. Shatter `internal/services`
- Extract runtime code into deeper domain packages:
  - `/home/runner/work/DixieData/DixieData/internal/records`: `soldier_service.go`, `anniversary_service.go`
  - `/home/runner/work/DixieData/DixieData/internal/review`: `audit_service.go`
  - `/home/runner/work/DixieData/DixieData/internal/insights`: `analytics_service.go`
  - `/home/runner/work/DixieData/DixieData/internal/archive`: `backup_service.go`, `export_service.go`, `diagnostics_service.go`, `image_service.go`, `archive_writer.go`
  - `/home/runner/work/DixieData/DixieData/internal/integrations`: `google_service.go`
- Keep `internal/services` as a compatibility shim only if required for older tests/imports; app/template runtime code must stop depending on it.
- Internal helper functions to unexport/lowercase behind the new package boundaries:
  - diagnostics bundle naming helper
  - archive zip writer helper usage remains package-private in `internal/archive`
  - Google upload-name and calendar event helpers stay package-private in `internal/integrations`
  - review-entry assembly moves out of `app.go` into DTO mapping helpers
- Update tests to live with their owning deep module or to import the new package paths directly.

## 3. De-monolith `app.go`
- Replace concrete service fields with strict facade interfaces grouped by domain:
  - `recordsFacade`
  - `reviewFacade`
  - `insightsFacade`
  - `archiveFacade`
  - `integrationFacade`
  - `anniversaryFacade`
- Limit `app.go` to HTTP/Wails request parsing, dialog orchestration, redirects, and rendering.
- Push view assembly and queue/comparison composition behind domain + DTO mapper functions.
- Update `reloadServices` to construct the new deep-module implementations and assign them through the facade interfaces.

## 4. Verification & Regrade
- Rebuild templ output after template signature changes.
- Run `go build ./...` and `go test ./...`.
- Remove `REFACTOR_PLAN.md` after execution.
- Rewrite `AGENT_ARCHITECTURE_MAP.md` with the new module map, before/after shallow-spot summary, and upgraded grade justification.
