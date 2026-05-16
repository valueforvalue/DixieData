# DixieData
A tool for keeping track of our Confederate dead. 

UI surface IDs for pages, panels, tabs, and overlays are documented in `docs\ui-ids.md`.

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
