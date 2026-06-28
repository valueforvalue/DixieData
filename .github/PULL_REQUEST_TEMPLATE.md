# Pull Request

## Summary

<!-- One or two sentences: what does this PR change and why. -->

## Linked issue

<!-- Issue number, or "none" if the change has no tracked issue. -->

## Layer touched

<!-- Tick every layer that this PR modifies. -->

- [ ] Templ markup (`.templ`)
- [ ] HTMX wiring (`hx-*` attributes, handlers)
- [ ] Frontend JS (`frontend/app.js`, `frontend/app.css`)
- [ ] Go backend (`internal/appshell/...`)
- [ ] Typst PDF templates (`templates/*.typ`)
- [ ] Database schema / migrations (`internal/db/...`)
- [ ] Documentation (`docs/`, `CONTEXT.md`, `AGENTS.md`)
- [ ] Build / CI (`.github/workflows/`, `Makefile`, `scripts/`)

## Click-driven surfaces checklist

<!-- If this PR adds or modifies any button, form, or submit path,
     complete this section. The `appshell.TestAll303sWriteHXRedirect`
     test fails any new handler that ships a 303 without HX-Redirect,
     so verify the pattern is followed BEFORE pushing. Reference:
     internal/templates/components/conventions.md "Buttons that POST
     and expect navigation". -->

- [ ] Every new/modified POST-then-navigate handler writes BOTH
      `Location` and `HX-Redirect` response headers (or is on the
      `exemptFunctions` allow-list with a one-line reason).
- [ ] Every new export/import button has a matching
      `share-{path}-navigates-to-<dest>` assertion in
      `audit/smoke.mjs` that checks `page.url()` after click, not
      just response headers.
- [ ] No form has a `submit` listener that calls `event.preventDefault()`
      and bypasses htmx redirect (see `COMMON_BUGS.md` §3.4 for the
      printable-PDF modal pattern that this rule prevents from
      recurring).
- [ ] Every `<button>` declares `type="submit"` (or is a known
      JS-triggered button with `type="button"`).
- [ ] Every `hx-post`/`hx-get`/`hx-put`/`hx-delete` URL goes through
      `internal/routebuilder` (not a bare string literal) — caught by
      `internal/templates/hx_guard_test.go::TestHXURLsUseBuilders`.
- [ ] Every `hx-target="#..."` selector that resolves to a durable
      surface has been promoted to `internal/uiids.Registry`.

## Accessibility checklist

<!-- If this PR adds any interactive UI, complete this section.
     DixieData uses div overlays (not native <dialog>) for modals;
     the helpers in frontend/app.js own focus trap + Esc + restore. -->

- [ ] Every form input has a matching `<label for="...">` (or
      `aria-label` / `aria-labelledby`).
- [ ] Every modal/overlay uses `showOverlayModal` / `hideOverlayModal`
      helpers (not native `<dialog>` — reverted in commit 1548407).
- [ ] Every interactive element has an accessible name.
- [ ] Table headers declare `scope="col"` / `scope="row"` where
      appropriate.

## Tests

<!-- DixieData requires the following test layers for any new
     click-driven surface. Tick every box that applies; do NOT
     leave boxes blank if the PR adds behaviour. -->

- [ ] Unit test for the Go handler (asserts response status +
      headers + body, not just "doesn't panic").
- [ ] `audit/smoke.mjs` smoke assertion (asserts both response
      shape AND `page.url()` after click for POST-then-navigate).
- [ ] `internal/templates/page_snapshot_test.go` goquery assertion
      for any new top-level page or panel.
- [ ] `internal/appshell/redirect_headers_test.go` stays green
      (runs in `go test ./... -short`; no per-PR action needed).
- [ ] `make test` passes (`go test ./... -short`).
- [ ] `make tpl` produces no diff (only matters if `*.templ` changed).

## CHANGELOG

<!-- DixieData CHANGELOG.md follows Keep a Changelog. Every
     user-visible change lands in [Unreleased] under Added /
     Changed / Fixed / Maintenance in the SAME commit that
     lands the change. Internal refactors live under Maintenance. -->

- [ ] `CHANGELOG.md` `[Unreleased]` section updated with a bullet
      under the appropriate heading (Added / Changed / Fixed /
      Maintenance).
- [ ] No [Unreleased] bullet duplicates a bullet that already lives
      in a tagged release section.

## Commit hygiene

<!-- Per AGENTS.md "Commits and branches". One commit = one logical
     change. If the message splits cleanly in half and each half still
     stands alone, you have two commits. -->

- [ ] Commits follow `<area>: <imperative summary>` (≤72 chars).
- [ ] Subject + blank line + 1–3 bullets explaining WHY.
- [ ] Branch name follows `feature/<short-kebab>`,
      `fix/<short-kebab>`, or `chore/<short-kebab>`.
- [ ] No "agent-scratch", "temp", or "wip" in branch or commit names.
- [ ] No bundling of unrelated changes (templ + handler + audit +
      CHANGELOG across 4 commits is correct; one 200-line commit
      bundling all four is not).