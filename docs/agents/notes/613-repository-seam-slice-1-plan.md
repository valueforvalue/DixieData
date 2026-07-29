# #613 Slice 1 — Repository seam for Person Record (read paths)

## Why this slice

Pragmatic-Programmer diagnostic (Orthogonality row 1) scored 2/10: 13 service files in `internal/records/` import `database/sql` and write inline `s.db.Conn().Exec(...)` strings. Postgres swap = 3-6 wk rewrite because no seam exists.

Slice 1 ships the seam + proves the pattern on the smallest meaningful surface (2 read methods) before expanding.

## Scope (locked per ask_user_question)

- **Methods migrated**: `SoldierService.GetByID(id)` (single-row fetch only — no Records/Images/Spouse joins in this slice) + `SoldierService.List(page, pageSize)`.
- **Interface location**: `internal/db/repo/` (interface) + `internal/db/repo/sqlite/` (impl). Service imports `repo`; repo imports nothing from `records/`.
- **State**: `SoldierService` keeps `*db.DB` field; gains `personRepo repo.PersonRecordRepo` field. Old `*db.DB` removed in slice N when last caller migrates.
- **Test strategy**: contract test (interface methods against in-memory SQLite) + parity test (new adapter returns identical results to legacy inline SQL on same fixture).

## Out of scope (slice 2+)

- Cross-table joins in `GetByID` (Records, Images, Spouse lookup stay inline; their own repos land in later slices).
- Write paths (`Create`, `Update`, `Delete`).
- `GetByDisplayID`, `ListByEntryTypes`, `RecentByIDs`, `ByIDs`, `GetImageByID`.
- Other repos (`EventRepo`, `ArticleRepo`, `TagRepo`).

## Repo API (locked)

```go
// internal/db/repo/person_record_repo.go
package repo

type PersonRecordRepo interface {
    // GetByID returns the raw soldier row + a Scanner that the caller
    // uses with scanSoldier (or any helper that uses the repo-package
    // internal scan helper). Returns sql.ErrNoRows if not found.
    GetByID(ctx context.Context, id int64) (*sql.Row, error)

    // List returns paginated soldiers + total count.
    List(ctx context.Context, page, pageSize int) (*sql.Rows, int, error)
}
```

**Why `*sql.Row` / `*sql.Rows` (option A from design)**: repo owns SQL, service owns domain normalization (hydrateLegacyDeathParts, pensionstate.Normalize, normalizeConfederateHomeFields). Keeps domain logic out of the repo. Scan helpers (`scanSoldier`, `scanListSoldiers`) stay in `internal/records/soldier_service.go` because they import domain packages (`pensionstate`, `confederatehomestatus`, `dates`).

The repo package exports the **column lists** as constants (`SoldierSelectColumns`, `SoldierListSelectColumns`) since the service scan helpers need them.

## File map (8 files)

| File | Change | LOC |
|---|---|---|
| `internal/db/repo/person_record_repo.go` | new — interface + column consts | ~40 |
| `internal/db/repo/sqlite/person_record_repo.go` | new — SQLite impl with `WithBusyRetry` parity | ~120 |
| `internal/db/repo/sqlite/person_record_repo_test.go` | new — contract test against in-memory SQLite | ~150 |
| `internal/records/soldier_service.go` | `GetByID` + `List` delegate to `s.personRepo`; keep scan helpers + cross-table joins inline | ~30 edited |
| `internal/records/person_record_repo_parity_test.go` | new — asserts new repo returns same rows as legacy inline SQL on same fixture | ~100 |
| `cmd/dixiedata-web/main.go` OR `internal/appshell/app.go` | wire `repo.NewSQLitePersonRecordRepo(s.DB())` into `SoldierService` | ~3 |
| `internal/records/soldier_service.go` (struct) | add `personRepo repo.PersonRecordRepo` field; update `NewSoldierService` | ~5 |
| `CHANGELOG.md` | [Unreleased] Maintenance bullet | ~5 |

## Slice 1 tests (RED first, then GREEN)

**Test 1 — Contract test** (`internal/db/repo/sqlite/person_record_repo_test.go`):

```go
func TestPersonRecordRepo_GetByID(t *testing.T) {
    db := newInMemoryDB(t)
    seedSoldiers(t, db, sampleSoldiers)
    r := sqlite.NewPersonRecordRepo(db)
    row, err := r.GetByID(context.Background(), 1)
    // ...scan row + assert fields match seed
}

func TestPersonRecordRepo_List(t *testing.T) {
    db := newInMemoryDB(t)
    seedSoldiers(t, db, 25Soldiers)
    r := sqlite.NewPersonRecordRepo(db)
    rows, total, err := r.List(ctx, 1, 10)
    // ...iterate rows + assert count + first/last ordering
}

func TestPersonRecordRepo_GetByID_NotFound(t *testing.T) {
    // assert sql.ErrNoRows
}

func TestPersonRecordRepo_WithBusyRetry(t *testing.T) {
    // assert the SQLite impl uses db.WithBusyRetry like the legacy code
}
```

**Test 2 — Parity test** (`internal/records/person_record_repo_parity_test.go`):

```go
// Seed the same fixture into two *db.DB instances: one for legacy
// SoldierService (using s.db.Conn() inline SQL) and one for the
// repo-backed SoldierService. Assert GetByID returns identical
// models.Soldier for the same input ID, and List returns identical
// paginated slices for the same page/pageSize.
```

## Implementation order

1. RED: write `TestPersonRecordRepo_GetByID` + `TestPersonRecordRepo_List` (compile fails — package doesn't exist).
2. GREEN: write `internal/db/repo/person_record_repo.go` (interface + column consts).
3. GREEN: write `internal/db/repo/sqlite/person_record_repo.go` (impl).
4. GREEN: compile passes; tests pass.
5. Edit `soldier_service.go`: add `personRepo` field, update `NewSoldierService`, refactor `GetByID` + `List` to delegate base row fetch to repo.
6. RED: write parity test (compares new path to legacy inline path on same fixture).
7. GREEN: parity test passes (both paths produce identical results).
8. Wire repo construction in app startup.
9. `just verify-fresh-bake` + `just test-lint && go test -short ./...` clean.
10. CHANGELOG bullet.
11. Commit + push.

## ADR

No new ADR. The seam decision is documented in #613 (the issue body). If slice 4+ reveals a deeper principle (e.g. "repos own SQL, services own domain normalization"), promote to ADR 0011.

## Risk + rollback

- **Risk**: scan helpers stay in soldier_service.go but use repo-package column consts. If a future repo (Postgres) uses different column names, scan helpers break. Mitigation: column consts are exported from repo and names match `soldiers` schema verbatim; Postgres-portable column names can be re-defined in the Postgres impl.
- **Rollback**: revert the slice's single commit. `SoldierService` field add + delegation is a small diff. Pre-slice code path is `s.db.Conn().QueryRow(...)` inline; post-slice code path is `s.personRepo.GetByID(ctx, id)`. Both produce the same scan dest shape.

## Estimate (PERT)

- O = 1 day
- A = 2 days
- N = 4 days
- P = (1 + 8 + 4) / 6 = 2.2 days

## Verification

- `go test -short -count=1 ./internal/db/repo/... ./internal/records/...` — new tests pass; existing tests unchanged.
- `just verify-fresh-bake` clean.
- Manual smoke: `dixiedata-web` boots, browse page loads (exercises List), soldier detail page loads (exercises GetByID).