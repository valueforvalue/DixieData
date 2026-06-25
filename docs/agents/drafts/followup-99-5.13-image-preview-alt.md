## Problem

The image preview modal that opens from the soldier detail page
(`data-image-preview` button) renders the caption verbatim into an
alt attribute. If the caption was pasted from another source and
contains HTML markup, the alt attribute on the preview `<img>` includes
raw HTML that screen readers may interpret or skip inconsistently.

**Source:** 2026-06-24 full audit; deferred from issue #99.

## Goal

Sanitise the alt text on the image preview thumbnail so it always
matches the `imageAltText` helper used in the soldier detail page.

## Approach

1. Update the preview thumbnail alt to use
   `imageAltText(img.Caption, s.DisplayID)` — same helper as the
   soldier detail page.
2. If the preview modal renders its own `<img>` separate from the
   thumbnail, apply the same helper there.
3. Add a regression test that passes a caption with HTML markup and
   asserts the rendered preview `<img>` alt attribute is sanitised.

## Files likely touched

- `internal/templates/soldier_card.templ` (the preview thumbnail)
- `internal/templates/soldier_card_test.go` (regression)

## Out of scope

- General HTML sanitisation in user-supplied text fields. That is a
  broader security concern tracked elsewhere.