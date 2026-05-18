# AGENT_ARCHITECTURE_MAP

## Agent-Friendliness Grade

**Grade: C**

The system has meaningful domain depth (archive lifecycle, merge workflows, analytics, exports), but discoverability and boundary control are weakened by a very large app orchestrator, a broad `internal/services` package with concrete cross-coupling, and UI templates coupled to service-layer types.

## Audit Method & Verification

- Mapped directories, package-level imports, and major file-size/function-count hotspots.
- Verified dependency shape using `go list` package imports.
- Ran existing test suite with `go test ./...`.
- Ran build verification with `go build ./...`.

### Test Verification Findings

- **Build status:** pass (`go build ./...`).
- **Test status:** fail (`go test ./...`) with multiple path/OS-separator failures across:
  - `/home/runner/work/DixieData/DixieData/app_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/appdata/appdata_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/services/backup_service_test.go`
  - `/home/runner/work/DixieData/DixieData/internal/services/google_service_test.go`
- Tests heavily assert implementation details (exact markup strings and file path shape) rather than stable public contracts.
- Most tests are in-package (`package main`, `package services`, `package templates`, etc.), which reduces pressure on strict public interfaces and allows internals to be tested directly.

## True Deep Module Boundaries (Current State)

Despite current friction, the architecture naturally clusters into these deeper modules:

1. **Archive Data Kernel**
   - Scope: schema/migrations, identity/versioning, indexing, path conventions, core models.
   - Core paths:
     - `/home/runner/work/DixieData/DixieData/internal/db`
     - `/home/runner/work/DixieData/DixieData/internal/models`
     - `/home/runner/work/DixieData/DixieData/internal/appdata`
     - `/home/runner/work/DixieData/DixieData/internal/dates`
   - Value: highest domain density with low folder fan-out.

2. **Domain Workflows Layer**
   - Scope: soldier lifecycle, backups/shared merge, export pipelines, diagnostics, audit, analytics, integrations.
   - Core path:
     - `/home/runner/work/DixieData/DixieData/internal/services`
   - Value: rich behavior; should be primary “business API surface.”

3. **Delivery Surface (Wails + HTTP + Templates)**
   - Scope: route handling, request parsing, response rendering, UI composition.
   - Core paths:
     - `/home/runner/work/DixieData/DixieData/app.go`
     - `/home/runner/work/DixieData/DixieData/internal/templates`
     - `/home/runner/work/DixieData/DixieData/frontend`
   - Value: user-facing interaction layer, but currently too entangled with workflow internals.

## Top 3 Critical Shallow Spots

## 1) `/home/runner/work/DixieData/DixieData/app.go` (Monolithic Orchestrator)

**Why it is shallow/leaky**
- Combines bootstrapping, route registry, controller logic, filesystem behavior, and workflow orchestration in one file.
- Very high surface area (3k+ lines, 140+ funcs) makes mental mapping expensive for a fresh agent.
- Direct DB calls and cross-service coordination in handlers leak domain boundaries up into the delivery layer.

**Impact**
- Harder progressive disclosure.
- High change-collision risk.
- Public module interface is implicit (many handler funcs) instead of explicit.

## 2) `/home/runner/work/DixieData/DixieData/internal/services` (Overloaded Single Package)

**Why it is shallow/leaky**
- Multiple large “god services” with broad responsibility overlap (`soldier_service`, `backup_service`, `export_service`).
- Concrete service-to-service dependencies (`*SoldierService` passed directly into other services) instead of narrow capability interfaces.
- A single package namespace masks subdomain boundaries (archive import/export vs audit vs analytics vs Google integration).

**Impact**
- Internal complexity is high but interface control is weak.
- Refactors tend to cascade.
- Dependency graph creates hidden coupling instead of clear seams.

## 3) `/home/runner/work/DixieData/DixieData/internal/templates` + Tests (UI Coupled to Service Types)

**Why it is shallow/leaky**
- Template signatures depend on `internal/services` structs (e.g., analytics/review/research view types), coupling rendering to workflow internals.
- Source `.templ`, generated `*_templ.go`, and tests are co-located, creating directory noise and weak progressive disclosure.
- Template tests assert literal content fragments, which tends to lock styling/string details more than UI contracts.

**Impact**
- UI boundary is not a clean “Grey Box” facade.
- Service-internal type changes ripple into view layer.
- Fresh-agent onboarding cost is elevated by generated-file volume.

## Refactor Blueprint (Step-by-Step)

1. **Establish explicit architectural layers in folder structure**
   - Split runtime code into `internal/core`, `internal/workflows`, and `internal/delivery` (or equivalent naming).
   - Move generated template artifacts behind a clearly marked subfolder to reduce map noise.

2. **Extract delivery composition root from `app.go`**
   - Create a small composition module for startup/wiring only.
   - Move route registration into feature-focused route groups.
   - Move handler logic into delivery adapters with narrow dependency contracts.

3. **Define stable workflow facades per domain capability**
   - Partition `internal/services` into subdomains (records, merge, export, analytics, integrations).
   - Introduce narrow interfaces for cross-subdomain collaboration; remove direct concrete service references.
   - Keep high-complexity internals private inside each subdomain package.

4. **Create a UI view-model boundary**
   - Introduce delivery-specific DTO/view-model packages consumed by templates.
   - Stop passing service-layer structs directly to templates.
   - Normalize route-to-view mapping so templates receive minimal, stable payload shapes.

5. **Harden public interface test guardrails**
   - Add black-box tests against package boundaries (use external test packages where practical).
   - Prioritize contract tests for route outputs, archive manifest formats, and import/export compatibility.
   - Reduce over-reliance on fragile exact-string assertions except where format guarantees are intentional.

6. **Add architecture fitness checks**
   - Enforce allowed dependency directions (delivery -> workflows -> core).
   - Add lightweight checks that block forbidden imports (e.g., templates importing workflow internals directly).

7. **Migrate incrementally by vertical slice**
   - Start with one high-value slice (e.g., review queue or backup import/export).
   - For each slice: extract contracts, move code, preserve behavior with compatibility tests, then prune old wiring.
   - Repeat until `app.go` is reduced to thin composition + transport adapters.

8. **Re-baseline cross-platform tests**
   - Fix path separator assumptions and OS-specific expectations.
   - Ensure tests validate behavior-level contracts across environments.

## Target End-State (Deep Modules + Grey Box)

- Fresh agent can understand the system from directory names and package boundaries in under 10 minutes.
- Each major domain exposes one small public facade while hiding high internal complexity.
- Delivery layer only depends on stable workflow contracts and view models.
- Tests primarily lock public behavior and compatibility guarantees, not incidental implementation details.
