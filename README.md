# DixieData
A tool for keeping track of our Confederate dead. 

UI surface IDs for pages, panels, tabs, and overlays are documented in `docs\ui-ids.md`.

## Release line

The current production line is **v1.1.17** and is derived directly from `internal\db\schema.go`.

Key v1.1 features include:

- Smart Back navigation that preserves the researcher’s place when returning from detail and edit views
- quick search backed by FTS5 with scratch-pad indexing and recent-record defaults in the browse view
- advanced search filters for entry type, review status, burial data, and other research fields
- sharded image storage under `.dixiedata\images\<A>\<B>\<display-id>\...` for better large-archive scaling
- merge-oriented `.ddshare` archives and full replacement `.ddbak` backups

## Documentation

- `docs\ai-handoff.md` - comprehensive project handoff for another AI or engineer
- `docs\implementation-and-features.md` - architecture, implementation details, and feature reference
- `docs\user-manual.md` - end-user operating guide
- `docs\ui-ids.md` - UI surface ID reference used for testing and debugging

## Build scripts

- `.\build-release.ps1` builds the standard production executable in `build\bin\DixieData.exe`.
- `.\build-release.ps1 -Archive` also packages `build\bin\*` into `release\DixieData-release-YYYY-MM-DD.zip`.
- `.\build-debug.ps1` builds the app and writes `build\bin\Run-DixieData-Debug.ps1`.
- `.\build-debug.ps1 -Run` builds and immediately launches the debug build with visible UI IDs.
- `.\run-debug.ps1` launches the current build with `--debug-ui-ids`, and `-Rebuild` forces a fresh debug build first.

These scripts preserve `build\bin\google-oauth-defaults.json` across `wails build -clean` so local shared Google OAuth defaults are not lost on rebuild.

The Wails window title is driven from `db.GetAppVersion()` in `main.go`, and runtime metadata uses the same dynamic version through `internal\buildinfo`.
