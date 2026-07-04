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
   `addEventListener("submit", ...)` sites. The codebase has two
   semantically distinct kinds of `<form>` submit:

   | Kind | Signal | Canonical dispatcher | Template attribute |
   |---|---|---|---|
   | **Navigation/data submit** | the form carries `data-dixie-submit` | `dispatchDixieDataForm` | `data-dixie-submit="true"` |
   | **Utility submit** | the form does NOT carry `data-dixie-submit` (drafts, prefs, presets) | bespoke handler with `// htmx-guard: utility-submit` marker | none |

   Drift in either direction is the bug class this probe catches.

## The marker

```js
// htmx-guard: utility-submit
form.addEventListener("submit", (ev) => { ... });
```

The `// htmx-guard: utility-submit` comment sits on the line immediately
preceding any `form.addEventListener("submit", ...)` site that is NOT
a navigation/data submit (i.e., the listener does not funnel through
`dispatchDixieDataForm`).

### Rules

1. The marker MUST be on the line directly preceding the `addEventListener("submit"` call (no blank line between).
2. The marker applies to the immediately-following `addEventListener("submit"` site ONLY — a marker 10 lines above does not count.
3. Doc-level handlers (`document.addEventListener("submit", ...)`) MUST
   either branch on `data-dixie-submit` inside the listener body
   (delivering to `dispatchDixieDataForm`) OR carry the
   `// htmx-guard: utility-submit` marker.
4. The probe treats a marker as authoritative — there is no per-marker
   allowlist of WHICH utility submits are legitimate. Future utility
   submits require the marker too.

### What the probe does NOT do

- It does not refactor existing utilities into `dispatchDixieDataForm`.
  That is the follow-up issue (see #316 close comment).
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

You MUST add the `// htmx-guard: utility-submit` marker on the
preceding line. The CI probe `make lint-htmx-guard` will fail
otherwise.

When you add a `form.addEventListener("submit", ...)` on a
`<form data-dixie-submit>` form that does NOT route through
`dispatchDixieDataForm`, the probe also fails. That is a
**navigation/data submit handler** and must use
`dispatchDixieDataForm`. See `#248` for the dispatcher contract.

## Probe source

`audit/discover_htmx_guard.mjs` — `--strict` flag exits 1 on any
violation. Run `make lint-htmx-guard` to invoke the probe locally.

## Related

- Issue #316 — the canonical issue for this probe
- `docs/COMMON_BUGS.md` §1.10, §3.4 — the bug classes this catches
- `frontend/app.js:5225` — the canonical `data-dixie-submit` doc-level delegate (no marker needed; the listener itself gates on `data-dixie-submit`)
- `frontend/app.js:3117` — `dispatchDixieDataForm` definition
- `internal/htmxattr/htmxattr.go:155-167` — the Go-side `validateTarget`
  helper that already encodes the same # vs non-# selector rule;
  slice 4 of #316 will enable its dev-build panic.
