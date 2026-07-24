# htmx-guard conventions

This doc describes the marker convention the `audit/discover_htmx_guard.mjs`
probe enforces on `frontend/app.js`. The probe is the executor; this doc
is the spec. Keep them in sync.

## Why this exists

Issue #316 ships a CI-failing lint probe that catches two recurring
attribute-drift classes:

1. **Toast-no-redirect** — a Go handler calls `setInfoToastHeader(w, ...)`
   but does not set `X-DixieData-Redirect` (or one of its siblings:
   `writeExportRedirect`, `enqueueExport(`, `respondDuplicateInFlight(`)
   in the same function body. Symptom: user clicks a button, sees a
   "Saved" toast, but stays on the same page instead of being navigated
   to the result. The 70978ac → 3612dab cycle in the repo history is the
   canonical example.

2. **JS submit coexistence** — `frontend/app.js` has multiple
   `addEventListener("submit", ...)` sites. The codebase has three
   semantically distinct kinds of `<form>` submit:

   | Kind | Signal | Canonical dispatcher | Template attribute |
   |---|---|---|---|
   | **Navigation/data submit** | the form carries `data-dixie-submit` | `dispatchDixieDataForm` | `data-dixie-submit="true"` |
   | **Utility submit** (preventDefault) | the form does NOT carry `data-dixie-submit` and the submit is fully owned by the JS | `dispatchUtilitySubmit(form, callback)` | none |
   | **Submit-prep** (allow default to bubble) | the form has its own submit semantics downstream and the JS only stages data | `dispatchSubmitPrep(form, callback)` | none (often also `data-dixie-submit="true"`) |

   Drift in either direction is the bug class this probe catches.

## The helpers

```js
// Utility submit — preventDefault + run callback.
dispatchUtilitySubmit(form, (form) => saveCurrentQueueAsPresetPage(panel, form));

// Submit-prep — run callback, then let the submit continue
// (form is typically data-dixie-submit="true" and the dispatcher's
// own submit will fire next).
dispatchSubmitPrep(exportForm, (form) => stageHiddenFieldsBeforeSubmit(form));
```

Both helpers are the **canonical, recognized** submit shapes for
utility-form submits in `frontend/app.js`. The walker accepts a
listener without ceremony when it sees a call to either helper
inside its body.

### When to use which

- **`dispatchUtilitySubmit`** — when your submit handler fully owns
  the submit (prevents the default, runs a local side-effect, no
  network IO). Example: the share-queue preset save (line 3995).

- **`dispatchSubmitPrep`** — when your submit handler is a
  side-effect that runs BEFORE another submit flow (the form is
  `data-dixie-submit="true"` and a downstream dispatcher will
  fetch). Example: the share-queue export form (line 4067) which
  stages hidden `selected_ids` inputs before the
  `dispatchDixieDataForm` delegate runs. Example: the PDF
  preferences persistence (line 5316) on
  `form[data-pdf-pref-scope]` which writes localStorage before
  the PDF-export dispatcher fires.

### The marker convention (deprecated, retained as fallback)

```js
// htmx-guard: utility-submit
form.addEventListener("submit", (ev) => { ... });
```

The `// htmx-guard: utility-submit` comment was the slice-1 marker
that the walker accepted before the helpers existed. All three
historically-marked sites migrated to helpers (issue #317 closed
that debt), but the probe still accepts the marker as a fallback
so that legacy reader code or future contributors who don't know
about the helpers are flagged less aggressively.

**New code should use the helpers**, not the marker. The marker
is documented here only so future readers recognize what
`// htmx-guard: utility-submit` means in a comment search.

### What the probe does NOT do

- It does not refactor existing utilities into `dispatchDixieDataForm`.
  That is the follow-up issue (issue #317) — closed in commit `c2f59df`.
- It does not lint TypeScript types or actual runtime behavior — it is
  a static source scan. Use `audit/smoke_*.mjs` for runtime contract
  tests.

## Target selector rule

The probe's third walker (`/audit/discover_htmx_guard.mjs →
findTemplOrphanTargets`) scans every `.templ` file under
`internal/templates/**` for `hx-target`, `data-results-target`, and
`data-status-target` attributes. The rule is straightforward:

- `#X` selectors MUST have a matching `id="X"` somewhere in the
  templ tree. htmx and `dispatchDixieDataForm` both write into
  the resolved element; a `#X` with no matching `id` is a silent
  no-op and a UX bug (no error, no feedback).
- `this` is always allowed (htmx self-targeting pseudo).
- Any other selector (`[data-bar]`, `body`, `.cls`, `:nth(...)`) is
  allowed without validation. htmx accepts any valid CSS selector.

Example violation (no `id="nonexistent-target"` anywhere):

```html
<!-- ❌ violation -->
<div hx-get="/x" hx-target="#nonexistent-target">...</div>

<!-- ✅ valid -->
<div id="results" hx-get="/x" hx-target="#results">...</div>
<div hx-get="/y" hx-target="this">...</div>
<div hx-get="/z" hx-target="body">...</div>
```

If an exotic non-`#` target needs documentation (e.g. a runtime-
created element), add a `// htmx-guard: known-target` comment on
the preceding templ line and extend the probe to honor it. Today
no marker is required because all current targets fall within the
three valid forms above.

## Author checklist

When you add any of the following to `frontend/app.js`:

- `form.addEventListener("submit", ...)` on a `<form>` that does not
  carry `data-dixie-submit`.
- `document.addEventListener("submit", ...)` that handles a form
  other than `data-dixie-submit`.

You MUST route the submit through one of the canonical helpers:

```js
dispatchUtilitySubmit(form, callback);  // preventDefault + cb
dispatchSubmitPrep(form, callback);     // cb, allow default to bubble
```

The CI probe `just lint-htmx-guard` will fail otherwise. The
deprecated `// htmx-guard: utility-submit` marker is retained as
a fallback but new code should use the helpers.

When you add a `form.addEventListener("submit", ...)` on a
`<form data-dixie-submit>` form that does NOT route through
`dispatchDixieDataForm`, the probe also fails. That is a
**navigation/data submit handler** and must use
`dispatchDixieDataForm`. See `#248` for the dispatcher contract.

## Probe source

`audit/discover_htmx_guard.mjs` — `--strict` flag exits 1 on any
violation. Run `just lint-htmx-guard` to invoke the probe locally.

## Related

- Issue #316 — the canonical issue for this probe
- `docs/COMMON_BUGS.md` §1.10, §3.4 — the bug classes this catches
- `frontend/app.js:5225` — the canonical `data-dixie-submit` doc-level delegate (no marker needed; the listener itself gates on `data-dixie-submit`)
- `frontend/app.js:3117` — `dispatchDixieDataForm` definition
- `internal/htmxattr/htmxattr.go:155-167` — the Go-side `validateTarget`
  helper that already encodes the same # vs non-# selector rule;
  slice 4 of #316 will enable its dev-build panic.
