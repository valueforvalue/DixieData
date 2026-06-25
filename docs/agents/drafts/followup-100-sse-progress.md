## Problem

`internal/jobs/jobs.go` workers update progress via
`Progress.Set(percent, message)`, but the UI on `/jobs/{id}` only
re-pulls every 2 seconds. For a 10-minute export on a slow connection,
the user sees coarse-grained progress. The audit issue #100 spec calls
for SSE or polling; current implementation is polling.

**Source:** 2026-06-24 full audit; deferred from issue #100.

## Goal

Push progress updates to the browser in real time so the progress bar
moves smoothly.

## Approach

1. Add a `/jobs/{id}/stream` endpoint that returns
   `text/event-stream` and writes a Job snapshot whenever
   `Progress.Set` is called.
2. Add a per-job subscriber channel in the Registry; `Progress.Set`
   writes to the channel; the handler multiplexes subscribers.
3. The `/jobs/{id}` page swaps htmx polling for an `EventSource`
   pointing at `/jobs/{id}/stream`; the page re-renders the progress
   fragment on each event.
4. Keep the existing `/jobs/{id}/status` polling endpoint as a
   fallback for browsers without `EventSource`.
5. Add a regression test that fires `Progress.Set` several times and
   asserts the SSE handler emits one event per call.

## Files likely touched

- `internal/jobs/jobs.go` (subscriber channel)
- `internal/appshell/jobs_handlers.go` (SSE handler)
- `internal/templates/jobs.templ` (EventSource wiring)

## Out of scope

- WebSocket transport (EventSource is sufficient for one-way
  server-to-client push).