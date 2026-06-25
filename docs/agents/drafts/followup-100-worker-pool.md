## Problem

`internal/appshell/jobs_handlers.go` starts each job in a fresh
goroutine via `jobs.Registry.Start`. With one goroutine per concurrent
export, two simultaneous exports work fine but a third is queued on
the Go runtime scheduler. The audit issue #100 spec calls for a
configurable worker pool (default 2) to cap concurrent work and
protect memory.

**Source:** 2026-06-24 full audit; deferred from issue #100.

## Goal

Run background exports through a worker pool with a configurable
concurrency limit (default 2).

## Approach

1. Wrap the Registry with a semaphore channel of size N (default 2).
2. `Start()` acquires the semaphore before launching the goroutine;
   the goroutine releases on exit.
3. If the pool is saturated when `Start()` is called, queue the job
   (status='queued') and let it pick up the semaphore when a worker
   frees.
4. Add a config knob: env var `DIXIEDATA_JOBS_CONCURRENCY` (or a
   settings field) to override the pool size at startup.
5. Add a test that fires N+1 jobs back-to-back and asserts the N+1th
   starts only after one of the first N completes.

## Files likely touched

- `internal/jobs/jobs.go` (semaphore)
- `internal/jobs/jobs_test.go` (concurrency test)
- `internal/appshell/app.go` (config knob wiring)

## Out of scope

- Distributed job queue (single-process is fine for desktop).