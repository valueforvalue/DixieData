# Error handling in DixieData

House style for surfacing errors to users and operators. The
canonical anti-pattern is **swallowed errors**: a `catch` that
returns `[]`/`null`/`false`, a Go `defer x.Close()` that drops the
error, an `if err != nil` that logs and returns nil without telling
the user. Every error path in DixieData should reach one of three
surfaces:

1. **Toast** — transient, page-level, fire-and-forget.
2. **Inline message** — embedded in the swapped region (htmx target
   or a templ fragment), persistent until the next action.
3. **Full-page error** — wraps the Layout chrome, used for routes
   that can't degrade gracefully.

This doc covers the helper to reach for, the layer it lives in, and
the regression-net probe that pins the pattern.

## The locked decision (issue #384)

> Errors are surfaced, not swallowed.

Encoded at glossary level in `CONTEXT.md` under "Laws
(non-negotiable)". The pattern below is the operational form of
that law. When you write a new handler, fetch, or catch, walk this
checklist before committing:

1. Did I write a `catch` that returns `[]`, `null`, `false`, or `{}`?
   If yes, add `console.warn` (toolbox) or `showToast` (app.js).
2. Did I write `if err := X.Render(...); err != nil { ... }` without
   calling a respondError family helper? If yes, switch to
   `respondErrorFragment` (or `respondError` if a toast is enough).
3. Did I write `defer x.Close()` without capturing the error? If
   yes, switch to:
   ```go
   defer func() {
       if cerr := x.Close(); cerr != nil {
           slog.Warn("close failed", "component", "io", "err", cerr)
       }
   }()
   ```
4. Does the user see a message when something fails? If no, fix it
   before merging.

## Go helpers (server-side)

All live in `internal/appshell/respond.go`.

| Helper | Body | Toast | When to use |
|---|---|---|---|
| `respondError` | plain text message | yes | htmx fragment handler where the toast region handles the user signal. Default for failure paths. |
| `respondErrorFragment` | `EmptyStateError` HTML | yes | htmx fragment handler whose swap target IS the whole region (not a toast region), so the user would otherwise stare at an unchanged/empty panel. Use when the page-level toast isn't enough. |
| `respondErrorPage` | full Layout error page | (page chrome) | Full-page routes. Use for handlers that serve entire pages, not fragments. |
| `respondValidation` / `respondNotFound` / `respondConflict` / `respondUnavailable` / `respondInternal` | shorthand for `respondError` with the matching kind | yes | Pick the kind that matches the failure; the shorthand avoids hand-mapping to HTTP status. |

### Decision flow

```
full-page route?           → respondErrorPage
htmx fragment, toast OK?   → respondError (or shorthand)
htmx fragment, no toast?   → respondErrorFragment
Render itself failed?      → respondErrorFragment
```

`respondErrorFragment` is the right answer when the failure is in
the templ Render call itself — there's nothing else to swap into
the target, so the error IS the response body.

### Logging

`respondError*` all log via `debug.FromContext(r.Context())` so the
`request_id` (set by `debug.Middleware`) is attached to the audit
line. The `audit` field is a stable token (`respond-error`,
`respond-error-fragment`, `respond-error-page`) so the audit harness
can grep for these without parsing log lines. Don't change the
audit field without updating the regression net.

## JS catches (client-side)

### App-level (`frontend/app.js`)

Use `showToast(message, kind)` from `app.js`. It writes into the
toast region declared in the Layout, supports `success`/`info`/
`warning`/`error` kinds, and is dismissible.

```js
try {
  await fetch("/api/thing");
} catch (err) {
  console.warn("fetch /api/thing failed", err);
  if (typeof showToast === "function") {
    showToast("Could not reach the server.", "error");
  }
}
```

If the failure happens inside a modal body that lost its toast
plumbing (e.g. a fragment fetch that targets the modal content
itself, not the page toast region), inline the error in the modal:

```js
modal.innerHTML = `
  <div class="empty-state empty-state-error" role="alert"
       data-empty-state-kind="error">
    <p class="text-sm font-semibold text-red-800">
      <span aria-hidden="true" class="mr-1">⚠</span>Could not load X.
    </p>
    <p class="mt-1 text-sm text-red-700">
      Try opening the dialog again.
    </p>
  </div>`;
```

This mirrors what `respondErrorFragment` does server-side. Keep the
two in lock-step so the audit probe stays green.

### Toolbox (`frontend/debug-toolbox.js`)

The toolbox is standalone-loadable — it runs before `app.js`
installs `showToast`. Use `console.warn` with the `[dixie:toolbox]`
prefix so debug-console dumps are greppable:

```js
try {
  // ...
} catch (err) {
  if (typeof console !== "undefined") {
    console.warn("[dixie:toolbox] operation failed", err);
  }
  return fallback;
}
```

The toolbox is devtools-only, so the user is the operator — a log
line in the devtools console IS the user signal.

## Templ surfaces

Two flavors of `EmptyState` live in
`internal/templates/components/empty_state.templ`:

- `EmptyState(title, body, extraClass)` — neutral sepia card for
  empty lists (info kind). Existing call sites.
- `EmptyStateError(title, body, extraClass)` — red border, warning
  icon, `role="alert"`, `data-empty-state-kind="error"`. Used by
  `respondErrorFragment` and any inline JS fallback.

Both expose `data-empty-state="true"` (the audit harness hook) and
the `data-empty-state-kind` attribute that distinguishes the two.
CSS rules in `frontend/tailwind.css`:

- `.empty-state` — sepia border + parchment background.
- `.empty-state-error` — red border + soft red background.

If you add a new visual flavor, add the kind constant to
`empty_state.templ` and the CSS rule to `tailwind.css`. The audit
probe pins the attribute set so a refactor that strips the marker
fails the regression net.

## Regression net

`audit/smoke_swallowed_errors.mjs` is a source-scan probe that
asserts:

1. The three `debug-toolbox.js` LS wrappers (`readShareQueueFromLocalStorage`,
   `writeShareQueueToLocalStorage`, `readPresetsFromLocalStorage`)
   each call `console.warn` on the failure path.
2. The `print-records` fragment fetch in `app.js` calls both
   `console.warn` AND `showToast` AND inlines an
   `empty-state-error` block in the modal body.
3. The three wrapped Go sites (`app.go:555` ShareView, `app.go:618`
   ResearchCollectionsHubView, `events_handlers.go:76` EventList)
   each call `respondErrorFragment` on the Render failure path.
4. `EmptyStateError` is exported from `internal/templates/components/`
   and its templ renders `data-empty-state-kind="error"`.

Run with:

```
node audit/smoke_swallowed_errors.mjs
```

Exit code is non-zero when any assertion fails. The probe is
deliberately source-scan (not JSDOM) because the regressions it
catches are introduced at the editor level — same rationale as
`audit/dispatcher_patch_method.test.mjs`.

## Out of scope for slice 1

The remaining ~30 bare-Render sites from issue #384 are converted in
later slices. Don't grow this slice past the locked scope. New sites
should follow this doc and add their own probe entries; bulk
migration is a separate task.