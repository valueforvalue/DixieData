## Problem

`internal/jobs/jobs.go` Registry stores jobs in an in-memory map. When
the Wails webview reloads (or the user restarts the app), all jobs
vanish — including any in-flight export. The audit issue #100 spec
explicitly accepts in-memory only for v1 but the deferral note says
persistence is a follow-up.

**Source:** 2026-06-24 full audit; deferred from issue #100.

## Goal

Persist job state to a JSON file in the data dir so a webview reload
or app restart can re-attach to running jobs.

## Approach

1. On every job state change (queued -> running -> done/error/cancelled),
   append the snapshot to `dataDir/jobs.jsonl`.
2. On Registry startup, replay the JSONL into in-memory state,
   marking any job that was 'running' as 'interrupted' (since the
   worker goroutine is gone).
3. Add tests that simulate a process restart and assert jobs
   rehydrate with correct status.
4. Bound the JSONL size: keep only the last 1000 entries (or rotate
   to `dataDir/jobs-YYYY-MM-DD.jsonl`) to avoid unbounded growth.

## Files likely touched

- `internal/jobs/jobs.go` (persistence hooks)
- `internal/jobs/jobs_test.go` (rehydrate test)
- `internal/appshell/app.go` (Registry initialisation with persistence
  path)

## Out of scope

- Migrating in-flight jobs to a new worker after restart. The
  `interrupted` status is honest about that.