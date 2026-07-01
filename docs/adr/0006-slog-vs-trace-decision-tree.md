# Slog vs trace: a decision tree for DixieData instrumentation

## Status

Accepted 2026-07-01. Supersedes the ad-hoc convention where handlers reached for `slog.Debug` whenever they wanted to emit a debug-level line.

## Context

DixieData's logging harness (`internal/debug/log.go`) is a slog handler that:

- Installs a default JSONL handler at `~/.dixiedata-logs/app.log.jsonl` via `debug.Configure(cfg)`.
- Pipes every entry through a tee handler to a file, an in-memory ring buffer (500 entries), and an optional stderr mirror.
- Floors its slog level at `Info` until `SetDebugMode(true)` or `DIXIEDATA_DEBUG=1` flips it to `Debug`.
- Decorates each entry with a `request_id` when the call is made through `debug.FromContext(ctx)`.

The Debug Console panel (🐞 footer button, `panel.debug-console`) reads from `GetRingBuffer().Snapshot()` and filters by level. Frontend `console.log` / `console.error` / `window.onerror` events are batched and posted back to Go under `source=frontend`. **Everything debug-visible flows through this one harness.**

Issue #218 added `internal/debug/trace` — a two-file build-tag pair that emits `slog.Debug` calls when compiled with `-tags debug` and is a literal no-op otherwise. The pattern (`//go:build debug` + `//go:build !debug` stub) was already established in the codebase by `pdfium_windows.go` / `pdfium_nonwindows.go`, `launcher_windows.go` / `launcher_other.go`, and `renderers_windows.go` / `renderers_nonwindows.go`.

Before #218, every debug-level call in production code was `slog.Debug(...)` (often via `debug.FromContext(r.Context()).Debug(...)`). After #218, devs had two near-identical primitives and no documented rule for which to reach for. Inevitable outcome: the codebase would drift toward whichever the first author preferred, and the trace harness would silently rot or be sprinkled everywhere indiscriminately.

## Decision

Two distinct primitives, one decision rule:

| Primitive | Compiled in release? | Compiled with `-tags debug`? | Output level | Use for |
|---|---|---|---|---|
| `slog.Debug` / `slog.Info` / `slog.Error` | Always | Always | Always emitted at the configured level | Curated events that tell a story at the active log level. Errors always use `slog.Error`/`slog.Warn`. |
| `debug.FromContext(ctx).Debug(...)` | Always | Always | As above, plus `request_id` decoration | Curated per-request events. Default in handler bodies that already have `ctx`. |
| `trace.Log(msg, attrs...)` | **No-op** | `slog.Debug` → JSONL + ring + Debug Console | Debug level only, when `DIXIEDATA_DEBUG=1` or `SetDebugMode(true)` is on | High-volume instrumentation: entry/exit markers, branch decisions, dup-rejection, dispatch routing. |

**Rule of thumb:**

- If removing the call would lose diagnostic value when `-tags debug` is on, use `trace.Log`.
- If the line should appear in a normal INFO+ log trail (operator greps for it), use `slog.Debug` (or `debug.FromContext(ctx).Debug` when inside a handler with `ctx`).
- If it is an error or warning, always use `slog.Error` / `slog.Warn` — those are never stripped.

### When to migrate an existing `slog.Debug`

A call is a candidate for `trace.Log` migration when **all three** apply:

1. The message is structural (ENTER, EXIT, branch, dup-reject) rather than narrative ("step 1 → step 2 → step 3").
2. The attrs are debug-shaped (boolean flag, integer count, short id) rather than user-facing strings.
3. The call has zero value in an INFO+ log trail — it would be noise if a user ran the app with debug mode off.

If any criterion fails, keep `slog.Debug`. Migration is a deliberate, per-call decision; do not bulk-convert.

### What trace does **not** do

- Does not pass `ctx`. `trace.Log` is a free function; request_id propagates automatically via `slog.Default()` because `debug.Configure` calls `slog.SetDefault`. If a call site needs to assert or attach a request_id explicitly, it should reach for `debug.FromContext(ctx).With(...)` instead.
- Does not write to a separate sink. All trace entries flow through the existing `teeHandler` to JSONL + ring buffer + (when on) stderr mirror.
- Does not introduce a separate UI surface. The Debug Console already ingests from the ring buffer; trace entries surface there at the DEBUG level filter.

## Alternatives considered

- **Replace `slog.Debug` with `trace.Log` everywhere in handlers.** Would lose the always-on INFO trail for ops staff who enable `DIXIEDATA_DEBUG=1` only intermittently. Operators grep `.dixiedata-logs/app.log.jsonl` for the narrative steps of a long operation; removing those entries would degrade support workflows. Rejected.
- **Keep `slog.Debug` everywhere; add `trace.Log` only for new instrumentation.** Two near-identical primitives, no migration path, drift over time. Rejected.
- **One trace primitive with a runtime `Enabled()` check.** Would incur a function-call + atomic-load cost in every release build for zero diagnostic value. Defeats the point of compile-time elimination. Rejected.
- **Trace-specific UI tab in the Debug Console.** Premature; the level filter already distinguishes DEBUG entries. Rejected (YAGNI).

## Consequences

- New code that adds instrumentation picks `trace.Log` by default unless the line has narrative value at INFO+. Reviewers flag drift from the rule.
- Existing `slog.Debug` calls in handler bodies (8 in `handleCalendarPDF`, plus dup-rejection calls in `handleSoldierPDF`, `handleSoldierPDFNoImages`, `handleSoldierJPG`, `handleImageScreenshot`, `handleImportBackup`, `exportFullDatabasePDFPath`) are migrated one at a time, per the candidate criteria above. Each migration is its own commit with a smoke-test pass; see `internal/appshell/app.go:3612dab` for the per-commit + regression-net precedent.
- CI (`.github/workflows/test.yml`) gains a `go test -tags debug ./internal/debug/...` step to catch build-tag drift. Without it the `trace_nodebug.go` stub could silently rot and release builds could carry dead code from any future `//go:build debug` file.
- The Makefile (`web`, `seed`, `gold`, `tune-bin` targets) builds these debug-only binaries with `-tags debug` so audit-harness runs actually exercise trace calls. Release builds go through `wails build`, which adds `-tags debug` only when `-DebugBuild` is set in `scripts/build-common.ps1`.
- Documentation drift is the highest residual risk. Mitigation: this ADR + the user-manual "Debug logging + Debug Console" section cross-references the decision rule. Code review catches new files that introduce `slog.Debug` without checking the rule.

## Implementation notes

- `trace.Log` signature: `func Log(msg string, attrs ...any)`. The variadic tail accepts the standard `slog` attribute shape: `"key", value` pairs. No `context.Context` parameter.
- The `//go:build !debug` stub file uses the literal empty function body — Go's compiler dead-code-eliminates every call site at the `call` instruction, not at a runtime guard. Verified with `go test -tags debug ./internal/debug/trace/...` (3 tests pass) and `go test ./internal/debug/trace/...` (3 tests pass).
- Trace entries count toward the 500-entry ring buffer cap. A debug-mode session that triggers many entry/exit events per request could roll older entries off the console; use the JSONL file at `~/.dixiedata-logs/app.log.jsonl` for full fidelity. If a future debug session finds ring-buffer pressure from trace calls, the cap is a single constant in `internal/debug/log.go` (`RingSize`).