## Problem

The `/jobs/{id}` status page (`internal/templates/jobs.templ`) shows
the saved file path as `<code>{ResultPath}</code>` text when the job is
done. The path is rendered for display but the user has no way to open
the file or copy a usable link from the page itself. The audit issue
#100 acceptance criteria call for a 'completion triggers a download'.

**Source:** 2026-06-24 full audit; deferred from issue #100.

## Goal

Surface a one-click action on the job-status page that opens the
produced file in the OS default app (or downloads it).

## Approach

1. Add a `GET /jobs/{id}/artifact` endpoint that streams the
   `ResultPath` with the right `Content-Disposition` header.
2. Render the result block as
   `<a href='/jobs/{id}/artifact' target='_blank' rel='noopener'>Open {kind} file</a>`
   when status is done and ResultPath is set.
3. Update the Job snapshot to expose the saved basename + display
   label for the link text.
4. Add a regression test that completes a fake job with a
   ResultPath and asserts the artifact link is present.

## Files likely touched

- `internal/appshell/jobs_handlers.go` (new endpoint)
- `internal/templates/jobs.templ` (link markup)
- `internal/appshell/jobs_handlers_test.go`

## Out of scope

- File-system file manager integration (e.g. Reveal in Explorer).