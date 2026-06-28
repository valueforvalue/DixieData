# Job artifact Content-Disposition bug

**Filed:** 2026-06-28
**Reporter:** Jeremy Morris
**Severity:** UX-breaking (user stranded on blank tab)
**Affected:** every export that produces an artifact the
"Open" link is wired to. Confirmed for ddbak exports; same
code path hits PDFs, JPGs, CSVs, ICSs.

## Symptom

When a background export job completes, the user lands on
`/jobs/{id}` (the status page). At the bottom of the page is
a green "Export complete" card with an "Open {label}" link
that points at `/jobs/{id}/artifact` and uses
`target="_blank"`.

Clicking "Open" on a `.ddbak` export opened a new tab that
immediately went blank. The browser had begun a download
(the artifact was served with `Content-Disposition:
attachment`) and the new tab had nothing to render. The user
had to find the download in their browser's download tray
or the "save as" dialog. Same for PDFs and JPGs: the user
expected to *see* the rendered file in the new tab, but
got a download instead.

## Root cause

`internal/appshell/jobs_handlers.go::streamJobArtifact` was
unconditionally setting:

```go
w.Header().Set("Content-Type", "application/octet-stream")
w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(path)+"\"")
```

for every artifact, regardless of file type. The link in
`internal/templates/jobs.templ` is `target="_blank"`, so
the new tab had no document to render and the browser
displayed a blank / "this file is being downloaded" page.

## Fix

Split artifacts into two categories based on file extension:

- **Viewable** (PDF, JPG, JPEG, PNG, GIF, WEBP, SVG, HTML,
  HTM, TXT, JSON): serve inline with a real `Content-Type`
  so the browser renders them in the new tab.
- **Download** (DDBak, DDShare, ZIP, CSV, ICS, anything
  else): keep `Content-Disposition: attachment` so the
  browser saves them to disk.

Implemented in `jobArtifactHeaders(path)` (new helper, in
`internal/appshell/jobs_handlers.go`) called from
`streamJobArtifact`. The viewable-vs-download decision is
a small `map[string]string` (`jobArtifactMimeByExt`).
Adding a new viewable type is one line; adding a new
download type is zero (the default is `attachment`).

## Tests

`internal/appshell/jobs_handlers_test.go` adds three
table-driven tests that lock the new behavior:

- `TestHandleJobArtifactInlineForViewableTypes` — every
  viewable extension is served with
  `Content-Disposition: inline` and the matching
  `Content-Type`. Reaches the wire path through a real
  `app.jobs.Start` + `app.jobs.SetResultPath`.
- `TestHandleJobArtifactAttachmentForDownloadTypes` —
  every download extension stays at
  `Content-Disposition: attachment` and octet-stream.
  Includes DDBak, DDShare, ZIP, CSV, ICS. JSON is
  intentionally NOT in this list because browsers render
  JSON natively.
- `TestJobArtifactHeaders_Unit` — fast check on the
  `jobArtifactHeaders` helper that doesn't stand up a job.
  Covers case-insensitivity (`.JPG`), spaces in the
  filename, and the no-extension fallthrough.

## Verification

- `go test ./internal/appshell/ -short -count=1` — all
  pass.
- `go test ./... -short -count=1` — 26/26 packages pass.
- Wails build: `make debug` succeeds (27s).
- Manual: in the GUI, kick off a soldier PDF export; the
  status page's "Open Printable archive PDF" link should
  now open a new tab with the PDF rendered, not a blank
  tab with a download prompt. Kick off a ddbak export;
  the "Open Static web archive" link should still
  trigger a download (ddbak isn't viewable).

## Related

- `docs/agents/cli-plan.md` — the CLI work doesn't hit
  this bug because `dixiedata export` writes directly to
  the user-supplied `--out` path and never goes through
  the artifact stream. The bug is GUI-only.
- The job status page is the same one wired to the
  shared-archive import's pre-merge restore-point
  snapshot (commit `35ab425`) but the snapshot ID is
  printed in the import response, not surfaced via an
  artifact link, so the bug doesn't affect it.
