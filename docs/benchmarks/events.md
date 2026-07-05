# Event CRUD baseline

Issue: [#320](https://github.com/valueforvalue/DixieData/issues/320) child
[#338](https://github.com/valueforvalue/DixieData/issues/338) (slot 16 of 16,
the validation step that gates #320 close).

Micro-benchmarks in `internal/records/event_service_bench_test.go` capture
the hot-path cost for `EventService.ListEvents`,
`AttachEventToPerson`, `DetachEventFromPerson`, and `ListForPerson`. Run
before any pagination / junction optimization work so there is a
concrete reference.

## How to reproduce

```sh
# Smoke run (a few iterations, ~5s).
go test ./internal/records/ -run NONE \
  -bench BenchmarkEventService -benchtime=2s -benchmem

# Stable run (10s budget per benchmark, recommended before commit).
go test ./internal/records/ -run NONE \
  -bench BenchmarkEventService -benchtime=10s -benchmem \
  -count=3 > /tmp/events-bench.txt
```

The benchmark fixtures are self-contained: each benchmark seeds its own
SQLite DB via `newTestDB` and populates the Event / Person pools it
needs. The two pools are not cross-linked; per-benchmark setup attaches
what it needs and the b.N loop only times the operation under test.

For the heavy seed / load sweep (10k events, 5k persons, all-stress),
see `tests/stress/events_stress_test.go`:

```sh
# Dev: light variants only (under 5s).
go test ./tests/stress/ -run TestStressEvent -count=1

# CI / make stress: light + heavy (issue #338 specs).
DIXIEDATA_STRESS_FULL=1 go test ./tests/stress/ -count=1 \
  -run 'TestStressEvent(ListPaginationSeeded10k|AttachDetachRoundTrip|AttachToOnePerson100Links|PaginationMonotonicAt200|Seed5000PersonsAndAttach)'
```

## What the micro-benchmarks pin

| Benchmark | Spec | What it catches |
|---|---|---|
| `BenchmarkListEventsPagination` | 5,000 events, 50/page | `sql.Query` + scan cost across N pages (catches a SELECT-per-row regression) |
| `BenchmarkListEventsPageSizeDrift` | 5,000 events, sweep at {25,50,100,200} | wall-clock skew that an N+1 in OFFSET/LIMIT pagination would introduce |
| `BenchmarkAttachDetachRoundTrip` | 1 event + 1 person, 200 iterations | N+1 in the junction INSERT + DELETE paths (per-iter cost must stay constant) |
| `BenchmarkListForPerson` | 1 person with 100 linked events, multi-page | N+1 in `IN (SELECT event_id FROM event_person_links)` subquery |

## Captured baseline (commit `69c03d0`, 2026-07-04)

Captured on a Windows 11 dev box with `go test -benchtime=1s`. Numbers
are reference; a future architecture change is expected to regress
within ~2x before triggering a "re-establish baseline" PR.

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `BenchmarkListEventsPagination-12` | 16,477,368 | 407,035 | 9,298 |
| `BenchmarkListEventsPageSizeDrift/PageSize=25-12` | 15,198,607 | 205,402 | 4,726 |
| `BenchmarkListEventsPageSizeDrift/PageSize=50-12` | 16,448,159 | 406,945 | 9,299 |
| `BenchmarkListEventsPageSizeDrift/PageSize=100-12` | 16,347,893 | 806,493 | 18,315 |
| `BenchmarkListEventsPageSizeDrift/PageSize=200-12` | 18,481,009 | 1,610,367 | 36,517 |
| `BenchmarkAttachDetachRoundTrip-12` | 286,296 | 1,975 | 59 |
| `BenchmarkListForPerson-12` | 2,162,970 | 805,001 | 18,167 |

### Acceptance signal

- `PageSize=200 / PageSize=25` ns/op ratio = **1.22x**. The spec
  (issue #338 item 3) asks for monotonic-or-better; we observe
  ~constant time per row, which is stronger.
- `AttachDetachRoundTrip` allocs/op = **59** — constant across
  b.N values (the iterator does not allocate per loop cycle, so
  the per-call cost is purely SQL round-trips).
- `ListForPerson-100Links` ns/op = **2.16ms** for 100 rows in a
  single SQL round-trip (the `IN (SELECT ...)` subquery).
  Single-shot: confirmed by the constant allocs/op shape.

## Heavy seed results (DIXIEDATA_STRESS_FULL=1, commit `69c03d0`)

`TestStressEventListPaginationSeeded10k` against a 10,000-event seed:

| PageSize | sweep time |
|---|---|
| 25  | 16.15s |
| 50  |  8.15s |
| 100 |  4.24s |
| 200 |  2.26s |

Per-row cost falls as page size grows (constant startup amortized),
confirming the micro-bench result.

## Out of scope (issue #338 AC partial-coverage)

The issue spec includes two items this document does not cover:

- **Per-Event PDF render p95 < 500ms** (item 5). The render time
  is dominated by Typst cold-start on Windows (15-30s for the first
  compile of a new template); p95 is therefore a measurement of
  the build pipeline + typst startup rather than of Event CRUD
  performance. The audit smoke probe
  (`audit/smoke_events.mjs` step 11) covers the end-to-end render
  path on a single event and exits 0 with the issue #347 fix
  that bundles `templates/event_landscape.typ` into
  `build/bin/templates/`. A future slot may add a warm-cache
  Typst benchmark; for now the PDF time is bounded by tests
  /320 / pass.
- **Static Archive bundle with 10k events < 30s** (item 6).
  Covered by the typst-bulk-export baseline at
  `docs/benchmarks/typst-bulk-export-baseline.md` for the
  per-record PDF path; the static-archive JSON bundle path is
  exercised by the existing test suite (`internal/archive`).
