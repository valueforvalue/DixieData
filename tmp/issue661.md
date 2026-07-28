## Problem

The "Download and Apply Latest Update" button on `/settings/updates` gives no feedback during the download phase. The user sees a toast ("Applying DixieData v1.1.11-rc1. The app will restart shortly.") only AFTER the entire zip is downloaded, verified, extracted, and the restore point is created. For a 40MB zip on a slow connection, the user sits staring at a button with no indication of progress — they don't know if the update is working, hung, or if they should click again.

The current flow in `handleApplyLatestUpdate` (`internal/appshell/app_update.go`):
1. `PrepareLatest()` runs — downloads zip, verifies checksum, extracts, creates restore point, writes apply script. All blocking.
2. `Presentation.SettingsUpdateApplyStarted(version)` renders — shows "Downloading and applying..." toast.
3. PowerShell script starts.
4. `a.Quit()` after 750ms.

The user sees NOTHING between step 1 and step 2. The "Downloading and applying..." toast only appears after the download is complete.

## Proposed solution

Replace the blocking `PrepareLatest()` with a streaming pipeline that reports progress:

1. **Download phase** — stream the zip with a progress callback. `internal/update/updater.go` already has `downloadFile()` — extend it to accept a progress callback (bytes received, total bytes from Content-Length).
2. **Verify phase** — stream-hash the file as it's written, no need to re-read.
3. **Extract phase** — already streaming, just report file count.
4. **Restore point phase** — quick, just show "Creating restore point...".
5. **Apply phase** — current toast + quit.

The progress bar lives in `#settings-update-status` (the div we just added for the Check for Updates results). It renders:
- "Downloading update... 12.3 MB / 40.4 MB (30%)" with a visual progress bar
- "Verifying checksum..."
- "Extracting..."
- "Creating restore point..."
- "Applying..." (existing toast)

## Apply sites

- [ ] `internal/update/updater.go` — add progress callback to `downloadFile()`, add `PrepareLatestWithProgress(callback)` variant
- [ ] `internal/appshell/app_update.go` — call the new variant, pass a callback that updates a per-request progress struct
- [ ] `internal/presentation/progress.go` (new) — `UpdateProgressBar(downloaded, total int)` templ component
- [ ] `internal/templates/entry_form.templ` — add `#settings-update-progress` div in the SettingsUpdatePanel, targeted by the apply form via `data-results-target`
- [ ] `frontend/app.js` — extend the dispatcher to poll a `/settings/updates/progress` endpoint during the apply, or use server-sent events for real-time updates
- [ ] `internal/records/LocalSettings` or new `internal/update/Progress` type — thread-safe progress state that the `/settings/updates/progress` endpoint reads

## Acceptance criteria

- [ ] User sees "Downloading update..." with bytes received / total bytes within 200ms of clicking the button
- [ ] Progress bar updates at least every 500ms during the download
- [ ] User sees "Verifying checksum...", "Extracting...", "Creating restore point...", "Applying..." as each phase completes
- [ ] If the download fails mid-stream, the user sees the error inline (not just a toast that disappears)
- [ ] Restore point rollback still works if the user cancels mid-download (the existing restore-point flow already handles this since it's created BEFORE the apply, not during the download)

## Out of scope

- Pause/resume download
- Bandwidth throttling
- Multiple simultaneous downloads
- Checksum progress (the file is small enough that hashing after download is fine)

## Related

- Issue #658 — the RC1 cohort is exercising the in-place update flow and reporting UX issues
- ADR 0001 — restore points as the rollback mechanism for failed updates
