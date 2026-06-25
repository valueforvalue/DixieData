## Problem

The feedback log (`internal/appshell/app_feedback.go`) is a JSONL file
in the data dir. There is no retention policy and no settings toggle to
clear old entries. With the audit harness and longer-running desktop
sessions, the log can grow without bound.

**Source:** 2026-06-24 full audit; deferred from issue #107 (finding 7.13).

## Goal

Add a settings option that prunes feedback entries older than a
configurable retention window. Default to 365 days.

## Approach

1. Add an `AppSettings` struct (or extend the existing one) with a
   `FeedbackRetentionDays int` field defaulting to 365.
2. Persist settings to `appdata` (e.g. `appdata/settings.json`) via
   a small `LoadSettings` / `SaveSettings` helper.
3. On app startup, prune feedback entries older than the retention
   window. Run the prune once per process start so it stays cheap.
4. Add a settings page toggle that updates the retention days and
   saves the file.
5. Add a settings handler that exposes a `POST /settings/feedback-retention`
   endpoint.
6. Add tests that cover prune behaviour and settings round-trip.

## Files likely touched

- `internal/appdata/appdata.go` (settings persistence)
- `internal/appshell/settings_handlers.go` (new endpoint)
- `internal/appshell/app_feedback.go` (prune call on startup)
- `internal/templates/settings.templ` (UI toggle)
- `internal/appshell/settings_handlers_test.go` (regression)

## Out of scope

- Per-category retention policies.
- Cloud-synced settings (single-device only for the desktop app).