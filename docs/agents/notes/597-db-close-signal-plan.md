# Issue #597 — DB close on exit: signal handler for CLI subcommands

Plan recorded before implementation per `docs/agents/feature-protocol.md`
§Tracer-bullet rule + `docs/agents/tdd.md`. Sign-off gate: this note +
the `## Slice plan` section of the issue body. No code lands before the
user approves the plan.

## Goal
Install a `signal.NotifyContext` handler in every CLI subcommand runner
in `main.go` and in `RunSmoke` (internal/appshell/smoke.go) so that
SIGINT / SIGTERM on POSIX cancels the lifecycle ctx and lets the
deferred `a.Shutdown(ctx)` (or `smokeShutdown`) actually fire — which
in turn drains jobs (5s budget) and closes the SQLite DB.

The Wails desktop path is **not touched**. Wails has its own signal
handling that already invokes `OnShutdown` on graceful close.

## Root cause (locked)
- `grep -rn "signal\.Notify\|NotifyContext\|SIGINT\|SIGTERM\|os/signal" main.go internal/appshell/` returns **zero matches**.
- Every CLI subcommand runner uses `context.Background()` for the
  lifecycle ctx, so the ctx cannot be cancelled even if a handler
  existed.
- `(*App).shutdown` (internal/appshell/lifecycle.go:288-302) does the
  right thing (drain jobs with timeout → `a.database.Close()`), but
  defers do not fire when the process is killed by a signal that
  bypasses the Go runtime's signal trampoline — i.e. SIGINT/SIGTERM
  with no handler installed.

## User-facing acceptance criterion
On Linux/macOS, `Ctrl+C` during any `dixiedata <subcommand> ...`
invocation (or during `dixiedata --smoke`) terminates the process
within 6 seconds, with no `dixiedata.db-wal` or `dixiedata.db-shm`
sidecar files remaining in the data directory.

On Windows the equivalent console-close path is best-effort (Go maps
it to a signal that may or may not arrive before process termination);
this slice does NOT regress that path and adds no Windows code. The
Wails desktop binary remains the recommended Windows entry point.

## Slice plan

### Slice 1 (landed as `774eff9`) — RED test on a blocking verb + flip all 6 subcommand runners to `signal.NotifyContext`
- **Subcommand shape:** the test fixture is `dixiedata logs --follow --data-dir <tmp>` (`runAdminSubcommand`). It opens the DB then blocks on a `select { case <-ctx.Done(): ... }` (`cli_admin.go:1008-1010`). It is the only existing verb that stays alive long enough for the test to inject a signal between "DB opened" and "deferred Shutdown fired." The blocking verb lives in admin, which means the per-runner test fixture is shared across runners — the test cannot pin a single runner in isolation. **Revised scope (vs. earlier draft):** Slice 1 flips **all 6** runners in one commit. Six identical 4-line edits across one file is one reviewable unit per `AGENTS.md` §Commits ("one commit = one reviewable unit; six near-identical edits ARE one reviewable unit"). Slice 2 adds defence-in-depth per-runner coverage.
- **Files:**
  - `main.go` — flip all 6 subcommand runners (lines 287, 314, 340, 368, 392, 424) from `ctx := context.Background()` to `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM); defer stop()`. Add `os/signal`, `syscall` imports as needed.
  - `internal/appshell/cli_signal_test.go` (new) — POSIX-only test (`t.Skip` if `runtime.GOOS == "windows"`). Builds `bin/dixiedata-test` via `go build -o <t.TempDir()>/dixiedata-test .` inside the test, `t.Cleanup` removes it. Spawns it with `dixiedata-test logs --follow --data-dir <t.TempDir()>`, polls the data dir for `dixiedata.db-wal` existence (2s budget) to confirm DB is open, sends `syscall.SIGTERM`, asserts:
    1. Process exits within 6s.
    2. Exit code is 0 (Shutdown completed normally; ctx cancellation flows to `cli_admin.go:1009` returning 0, then deferred Shutdown closes the DB).
    3. No `dixiedata.db-wal` or `dixiedata.db-shm` remains in the data dir (poll briefly to avoid OS cleanup lag).
  - `internal/appshell/cli_signal_test.go` — also include a shorter-form test using `dixiedata list soldiers --data-dir <tmp>` that asserts the same exit-code + no-WAL invariants, to cover the "fast-verb" path where Shutdown runs on the post-runner return.
- **Contract touch:** `(*App).Shutdown(ctx)` is already on the contract. No signature change. The implicit new public seam is: "the lifecycle ctx passed to `Startup` and `Shutdown` is signal-aware on POSIX; SIGINT/SIGTERM cancel it before the deferred Shutdown fires." Documented as a comment on each of the 6 runners ("lifecycle ctx is signal-aware; SIGINT/SIGTERM invoke Shutdown before exit").
- **Regression net:**
  - RED test fails BEFORE Slice 1 lands (subprocess leaves WAL/SHM behind on SIGTERM because no handler installed; the `logs --follow` goroutine continues until SIGKILL).
  - RED test goes green AFTER Slice 1 lands (handler installed; ctx cancels; `logs --follow` returns 0; deferred Shutdown closes DB).
  - Existing CLI tests keep passing — ctx change is additive; the deferred Shutdown was already running on normal exit.
- **Failure modes this slice prevents:** the EXACT bug class the issue reports. Before: defer never fires, WAL lingers. After: defer fires, WAL cleaned.

### Slice 2 — defence-in-depth per-runner test coverage
- **Files:** `internal/appshell/cli_signal_test.go` — extend with one POSIX subprocess test per remaining runner that has a meaningful blocking verb or a verb that opens + writes to the DB (so a missing Shutdown would leak something observable). Candidates by runner:
  - `runQuerySubcommand` — `dixiedata search` is fast but writes the audit ring; the test asserts the audit ring is closed (file handle not held) after SIGTERM. Use `lsof` if available OR just rely on the no-WAL invariant.
  - `runMutateSubcommand` — `dixiedata soldier create` opens DB + writes; assert no WAL after SIGTERM (already covered by Slice 1 fixture shape — keep this light).
  - `runExportSubcommand` — exports open a writer + DB. The blocking path is `--out <tmpfile>`; the test sends SIGTERM mid-export and asserts the writer's temp file is closed (no `*.partial` straggler).
  - `runImportSubcommand` — imports open a reader + DB. Same shape.
  - `runDebugSubcommand` — read-only, fast. Skip per-runner test; the Slice 1 fixture covers it transitively.
- **Pragmatic call:** if any runner does NOT have an observable side effect that distinguishes "Shutdown fired" from "Shutdown skipped," skip its dedicated test. The Slice 1 fixture (`logs --follow` in admin) is the contract pin. Defence-in-depth is nice-to-have, not load-bearing. **Revised Slice 2 scope:** add ONE test on `runExportSubcommand` (the writer-close signal is the most observable) and skip the rest.
- **Regression net:** Slice 1 RED test still passes; new export test passes.
- **Atomicity:** Slice 2 is one commit.

### Slice 3 — wire the same ctx treatment into `RunSmoke`
- **Files:** `internal/appshell/smoke.go:113` (`defer smokeShutdown(app)`) and the surrounding `RunSmoke` function. The smoke path uses its own internal ctx; the fix is to derive that ctx from `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` so Ctrl+C during `--smoke` also triggers `smokeShutdown` (which mirrors `(*App).shutdown` inline).
- **RED test:** extend `internal/appshell/smoke_test.go` (POSIX-only) with a SIGTERM-during-smoke assertion. Same shape as Slice 1 but targeting the smoke verb.
- **Contract touch:** `smokeShutdown(a *App)` already exists and is the correct body. No signature change.
- **Regression net:** existing `smoke_test.go` tests keep passing; new SIGTERM test passes.

### Slice 4 — comment update + CHANGELOG bullet + close issue
- **Files:**
  - `internal/appshell/cli_mutate_test.go:445-446` — update the comment that says "production code calls os.Exit after Shutdown so the dangling handle is never observed outside tests." After this slice, the production flow is: signal → ctx cancel → deferred Shutdown runs → process exits cleanly. The comment is now misleading; replace with the accurate contract.
  - `CHANGELOG.md` `[Unreleased]` → `### Fixed` → "CLI subcommands now drain jobs and close the DB cleanly on SIGINT/SIGTERM (POSIX) (issue #597, `cli_*_test.go`)."
  - `gh issue close 597 --comment "Closed — landed in <sha>: <subject>"` per `AGENTS.md` §Issue-tracker close-out law.

## Files touched (full list)
- `main.go` — 6 subcommand runners (all in Slice 1)
- `internal/appshell/cli_signal_test.go` — new file, Slice 1 RED + Slice 2 defence-in-depth
- `internal/appshell/smoke.go` — Slice 3
- `internal/appshell/smoke_test.go` — Slice 3 RED
- `internal/appshell/cli_mutate_test.go:445-446` — Slice 4 comment
- `CHANGELOG.md` — Slice 4 bullet

7 files. No new public surface, no schema, no templ, no JS.

## Decisions locked
1. **First-signal semantics** — `signal.NotifyContext` cancels on first SIGINT/SIGTERM. A long `--import` mid-flight aborts. This matches Unix conventions for graceful tools and is simpler than "drain on first, hard-exit on second" (which would need a manual `signal.Notify` + goroutine).
2. **POSIX-only tests** — Windows console-close maps to a best-effort signal that may not arrive. Skip with `t.Skip` on Windows; do NOT add a Windows-specific workaround in v1.
3. **No logging change in v1** — `a.Shutdown` does NOT log a "signal-driven exit" line in this slice. Open Question 1 (logging) deferred — not worth the slice weight. If support asks for it later, a one-line `slog.Info` lands as a `### Maintenance` CHANGELOG bullet.
4. **Wails path untouched** — Wails has its own signal handling. Adding `signal.NotifyContext` in `main()` BEFORE `wails.Run` would interfere with Wails' handler (double-cancel on the same signal). Slice 1 starts inside the subcommand runners, not at `main()`.
5. **Test harness shape** — build `bin/dixiedata-test` once per test via `go build -o`, `t.Cleanup` removes it. No Makefile target needed; the test is hermetic. If the build time makes the test slow, gate behind `-short` (skip on `-short`).

## Principle warnings
None. The slice is mechanical (replace `context.Background()` with `signal.NotifyContext(...)` in 6 spots, add a `defer stop()`, write a test). No DRY/orthogonality/Demeter violations; no new abstractions.

## What assumptions does this PR make?
- **Assumption:** `(*App).Shutdown(ctx)` honours ctx cancellation for the job-drain phase. **Pinned by:** reading `internal/appshell/lifecycle.go:289-296` (5s timeout via `context.WithTimeout(ctx, ...)` — true cancel propagation). No new test needed; existing test coverage on Shutdown is sufficient (the test in Slice 1 fails the same way if Shutdown did not respect ctx).
- **Assumption:** `signal.NotifyContext` is available (Go 1.16+). **Pinned by:** `go.mod` declares go 1.21+ (verified at slice time).
- **Assumption:** the `modernc.org/sqlite` driver flushes its WAL writes when `(*sql.DB).Close()` is called normally (no torn writes on a clean close). **Pinned by:** existing `db.Close()` tests pass on every PR; no historical evidence of WAL corruption on clean close. Out of scope to re-test.
- **Assumption:** the test subprocess can be killed with `syscall.SIGTERM` from `t.Cleanup` without race against the test's own PID. **Pinned by:** standard `os/exec` pattern; the subprocess's PID is captured from `cmd.Process.Pid` before sending the signal.

## Open questions for the user
1. **Logging on signal** — should `a.Shutdown` log a `slog.Info` line noting signal-driven exit? Recommended **no** for v1 (slice stays small; support can grep the ring buffer for `ctx.Err() == context.Canceled` if needed). Confirm or override.
2. **First-signal vs. second-signal** — confirmed first-signal per the issue body. Override if you want "drain on first, hard-exit on second" (would add ~15 lines of manual signal goroutine).
3. **Test build time** — building `bin/dixiedata-test` inside the test takes ~5-10s. Acceptable, or do you want the test gated behind `-short` (CI runs it, `go test -short` skips)? Recommended **run always** — the bug is high-priority and a 10s test is cheap insurance.

## References
- Issue #597 — body has the symptom + repro + root-cause analysis that this plan operationalises
- `internal/appshell/lifecycle.go:288-302` — `(*App).shutdown`, the closure this plan makes actually fire
- `internal/db/db.go:100-103` — `(*DB).Close`, the operation whose guarantee this plan enforces
- `main.go:240-243` — Wails desktop path, **explicitly out of scope**
- `docs/agents/feature-protocol.md` §Ticket close-out law — `gh issue close` in Slice 4
- `docs/agents/tdd.md` §Per-layer recipes — Slice 1 = RED test BEFORE the fix lands