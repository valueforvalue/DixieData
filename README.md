# DixieData

DixieData is a Windows-first Wails desktop archive for Civil War research records. It keeps people records, notes, source entries, images, exports, backups, and merge workflows in a local-first SQLite-backed application.

## What the app does

- manages soldier, wife, and widow records in one archive
- supports search, review queues, analytics, and research workflows
- stores scratch pads and images alongside the local archive data
- exports printable reports, JSON/Excel/iCalendar output, `.ddbak` backups, and `.ddshare` merge archives
- includes optional Google Drive, Sheets, and Calendar integrations

## How to get oriented

- `AGENT_ARCHITECTURE_MAP.md` - the structural map of the current Deep Modules, Grey Box boundary, Facades, and automation entrypoints
- `docs\ai-handoff.md` - engineer/agent handoff focused on working context and inspection order
- `docs\implementation-and-features.md` - implementation reference for major workflows and features
- `docs\user-manual.md` - end-user operating guide
- `docs\ui-ids.md` - UI surface ID reference used for testing and debugging

## Build and validation

- `go test ./...` runs the full Go test suite
- `go build ./...` runs the baseline compile check
- `.\scripts\build-release.ps1` builds the production executable in `build\bin\DixieData.exe`
- `.\scripts\build-debug.ps1` builds the debug executable and launcher
- `.\scripts\run-debug.ps1` launches the current debug build with UI IDs enabled

## Current release line

The current production line is derived from `internal\db\schema.go` via `db.GetAppVersion()`.
