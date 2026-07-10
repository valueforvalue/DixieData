# HTMX Split-Brain Architecture: Root-Cause Diagnosis

**Date:** 2026-06-28
**Branch:** dev (e5a8378)
**Status:** Diagnosis complete. Fix options presented.

---

## Executive Summary

DixieData's UI has been plagued by recurring bugs where buttons silently fail to
navigate, responses disappear, and the same "fix" (adding `HX-Redirect` headers
to server handlers) gets applied repeatedly without resolving the underlying
issue. The root cause is an architectural conflict: **the app runs two competing
network layers simultaneously — real htmx.js and a custom JS reimplementation —
and neither owns the full request/response cycle.**

The recent bug-fix commits (`3612dab`, `11f1c01`, `a6f7fa2`, `a77f52d`) all
target symptoms of this split-brain architecture. The fixes are correct in
isolation but futile in aggregate: every new handler that uses a different htmx
attribute pattern risks breaking because the custom JS layer doesn't support it,
and every `HX-Redirect` header written by the server is dead code that the
custom JS never reads.

---

## Architecture: What Actually Runs

### Two network layers, one DOM

```
 ┌─────────────────────────────────────────────────┐
 │                Browser (WebView2)                 │
 │                                                   │
 │  ┌──────────────┐       ┌──────────────────────┐ │
 │  │  htmx.js 2.x  │       │  app.js request()    │ │
 │  │  (loaded but   │       │  (custom fetch-based │ │
 │  │   neutered)    │       │   htmx clone)        │ │
 │  └──────┬─────────┘       └──────────┬───────────┘ │
 │         │                            │             │
 │         │  htmx attributes           │  data-*     │
 │         │  STRIPPED at boot          │  copies     │
 │         │  (7 of them)               │  (7 cached) │
 │         ▼                            ▼             │
 │  ┌──────────────────────────────────────────────┐ │
 │  │              DOM (HTML elements)              │ │
 │  │  <button hx-post="/export/json"              │ │
 │  │          hx-target="this"        ← survives  │ │
 │  │          hx-swap="none">         ← survives  │ │
 │  └──────────────────────────────────────────────┘ │
 └─────────────────────────────────────────────────┘
```

### Layer 1: htmx.js 2.x (neutered)

- Loaded via `<script defer src="/htmx.min.js">` in `layout.templ:54`
- Introduced Jun 24, 2026 (commit `3f75356`) alongside the stripping mechanism
- Registers event delegation on `document` for click/submit
- **Never fires** because app.js strips `hx-get`, `hx-post`, `hx-put`, `hx-delete`,
  `hx-trigger`, `hx-confirm`, `hx-include` before htmx can process them
- Reads `HX-Redirect` response header natively — but never receives a response
  to process

### Layer 2: app.js `request()` (the custom clone)

- Original `request()` function created Apr 28, 2026 (commit `d4dbbab`), ~2
  months before htmx.js was loaded
- Designed for a world WITHOUT htmx.js — reads `hx-*` attributes directly from
  the DOM
- Intercepts ALL clicks/submits on elements matching `[hx-get],[hx-post],...`
  via `preventDefault()` + `event.target.closest()`
- Sends `fetch()` with `X-Requested-With: DixieData` header
- Reads **only** `X-DixieData-Redirect` response header (line 3062)
- Has a fallback redirect path: `response.redirected && response.url !== requestUrl`
  (line 3087) — catches 303 redirects that `fetch()` follows
- Reimplements `applyResponse()` with swap modes (innerHTML, outerHTML,
  beforebegin, afterend, beforeend, none)

### The Attribute Lifecycle

At `DOMContentLoaded` (line 3492-3509):

```javascript
const cachedAttrs = ["hx-get", "hx-post", "hx-put", "hx-delete",
                     "hx-trigger", "hx-confirm", "hx-include"];
// ^ NOTE: hx-target and hx-swap are NOT in this list

document.querySelectorAll("[hx-get], [hx-post], [hx-put], [hx-delete], [hx-trigger]")
  .forEach((el) => {
    cachedAttrs.forEach((attr) => {
      const value = el.getAttribute(attr);
      if (value !== null) el.setAttribute("data-" + attr, value);
      el.removeAttribute(attr);  // ← original destroyed
    });
  });
```

After stripping:
- `hx-post` → gone, `data-hx-post` holds the cached value
- `hx-target` → **survives** (not in `cachedAttrs` list)
- `hx-swap` → **survives** (not in `cachedAttrs` list)

Functions that use `hxAttr()` (reads both original + `data-*` fallback) work
correctly for cached attributes. Functions that use raw `getAttribute()` on
cached attributes return `null`. Functions that read non-cached attributes
(`hx-target`, `hx-swap`) accidentally work because those attributes survive.

---

## Timeline: How We Got Here

| Date | Commit | Event |
|---|---|---|
| Apr 28 | `d4dbbab` | `request()` function created (235 LOC). No htmx.js loaded. |
| Jun 24 | `3f75356` | htmx.js 2.x loaded. Stripping introduced to prevent double-firing. Stripped **9 attributes** including `hx-target` and `hx-swap`. |
| Jun 27 | `98b5acd` | Fix: cache attrs to `data-*` before stripping. Added `hxAttr()`/`hxHas()` helpers. **Reduced cachedAttrs from 9 to 7** (dropped `hx-target` and `hx-swap`). Updated "all call sites" — **but missed `getTarget()` and `getSwap()`** which still use raw `getAttribute()`. |
| Jun 27 | `eafaee7` | `templ.SafeURL` silently dropped htmx attrs from rendered HTML. Fix: change to plain string. |
| Jun 27 | `b185f0e` | 3 bugs from live testing: 303 redirect handling, outerHTML swap destroying target, missing swap modes. |
| Jun 28 | `3612dab` | First HX-Redirect sweep: added `HX-Redirect` to export handlers → `/jobs/{id}`. |
| Jun 28 | `11f1c01` | Second HX-Redirect sweep: 5 more handlers + global test `TestAll303sWriteHXRedirect`. |
| Jun 28 | `a6f7fa2` | Dropped printable-PDF JS shim that bypassed htmx. |
| Jun 28 | `a77f52d` | Documented the JS submit interceptor bug class in `COMMON_BUGS.md` §3.4. |
| Jun 28 | `e5a8378` | Ad-hoc debug probes for /setup issues. |

**The UI was more stable before Jun 24** because:
1. No htmx.js was loaded → no stripping needed → no `data-*` caching complexity
2. `request()` read `hx-*` attributes directly from the DOM (they were never
   stripped)
3. Every function used raw `getAttribute()` — and it worked because attributes
   were always present

The Jun 24 htmx.js loading + stripping introduced the split-brain. The Jun 27
fix (caching to `data-*`) papered over the worst symptom (dead buttons) but
didn't resolve the underlying architecture.

---

## Confirmed Bugs in Current Code

### Bug 1: `HX-Redirect` is dead code in the JS layer (CRITICAL)

**File:** `frontend/app.js:3062`

```javascript
const redirectTo = response.headers.get("X-DixieData-Redirect");
// NEVER reads "HX-Redirect"
```

**Server side:** 13 handlers write `HX-Redirect` alongside 303. 9 handlers write
`X-DixieData-Redirect` on 200. The JS only reads the latter.

**What actually makes redirects work:** The `response.redirected` fallback at
line 3087:

```javascript
if (response.redirected && response.url !== requestUrl) {
    window.location.assign(response.url);
    return;
}
```

`fetch()` follows 303 redirects by default. After a 303, `response.redirected`
is `true` and `response.url` is the final URL (e.g., `/jobs/123`). The check
`response.url !== requestUrl` triggers navigation.

**This is fragile** because:
- `HX-Redirect` headers on 303 responses are INVISIBLE to `fetch()` — the
  browser follows the redirect and `response.headers` comes from the final 200
  response, not the intermediate 303
- If `response.url` ever normalizes to equal `requestUrl` (e.g., both become
  absolute URLs pointing to the same path), the redirect is silently swallowed
- Any handler that returns 200 with inline markup (not 303, not
  X-DixieData-Redirect) falls through to `applyResponse()` with broken target
  resolution

### Bug 2: `getTarget()` doesn't handle `hx-target="this"`

**File:** `frontend/app.js:647-654`

```javascript
function getTarget(el) {
    const selector = el.getAttribute("hx-target");  // "this" survives stripping
    if (!selector) {
        const form = closestParentForm(el);
        return form ? getTarget(form) : document.body;
    }
    if (selector === "body") return document.body;
    return document.querySelector(selector);  // document.querySelector("this") → null
}
```

`document.querySelector("this")` returns `null`. Real htmx interprets `this` as
"the element itself." The custom JS has no such special-case.

**Impact:** 37 template sites use `hx-target="this"` (all paired with
`hx-swap="none"`). For 303-returning handlers and 200+X-DixieData-Redirect
handlers, `applyResponse()` is never reached, so this bug is masked. For error
cases (handler returns 200 error markup without a redirect header), the response
is silently dropped — `getTarget()` returns `null`, `applyResponse()` returns
early, no error is shown to the user.

### Bug 3: `getSwap()` and `getTarget()` use raw `getAttribute()` (FRAGILITY)

**File:** `frontend/app.js:660`

```javascript
function getSwap(el) {
    const form = closestParentForm(el);
    return el.getAttribute("hx-swap") || (form ? getSwap(form) : "innerHTML");
}
```

These are the only two functions in app.js that read htmx attributes WITHOUT
using `hxAttr()`. They accidentally work because `hx-target` and `hx-swap` were
**dropped from the `cachedAttrs` list** in commit `98b5acd` (reduced from 9 to 7
attributes). If anyone adds `hx-target` or `hx-swap` back to `cachedAttrs`,
these functions break instantly.

### Bug 4: `hx-target` and `hx-swap` excluded from `cachedAttrs` without documentation

The comment at line 3510 says `hx-swap` is "intentionally preserved," but this
decision was a side effect of reducing the cached list, not a deliberate design
choice. The original commit (`3f75356`) stripped 9 attributes including
`hx-target` and `hx-swap`. When `98b5acd` added caching, it dropped them —
likely because `getTarget()` and `getSwap()` weren't updated to use `hxAttr()`,
so keeping them unstripped was the path of least resistance. There is no
documentation explaining WHY these two attributes are preserved while 7 others
are stripped.

---

## Redirect Flow: What Happens When You Click "Export JSON"

```
User clicks Export JSON button on /share

    1. Click handler: event.target.closest("[data-hx-post]") matches
       → request(el)

    2. getMethod(el): hxHas(el, "hx-post") → data-hx-post exists → "POST"

    3. getUrl(el): hxAttr(el, "hx-post") → data-hx-post → "/export/json"

    4. fetch("/export/json", { method: "POST", headers: {...} })

    5. Server: enqueueExportWithResult(...)
       → w.Header().Set("HX-Redirect", "/jobs/abc123")  ← DEAD CODE from JS view
       → w.Header().Set("Location", "/jobs/abc123")
       → w.WriteHeader(303)

    6. Browser: fetch follows 303 → GET /jobs/abc123

    7. Server: /jobs/abc123 handler returns 200 HTML page

    8. response.redirected = true       ← this is what saves us
       response.url = "http://localhost:9879/jobs/abc123"
       requestUrl = "/export/json"

    9. Check: response.url !== requestUrl → TRUE → navigation!
       window.location.assign(response.url)

    10. Browser loads /jobs/abc123 with full page refresh
        → htmx.js loads fresh, app.js strips fresh
        → Page works correctly
```

**The `HX-Redirect` header at step 5 is never read.** It was written for the
benefit of htmx.js (step 1-3 bypass htmx entirely via `preventDefault()`). The
actual mechanism keeping the redirect alive is `fetch()`'s native 303-following
behavior at step 8-9.

---

## Why Bug Fixes Keep Failing: The Tail-Chase Cycle

```
New feature adds buttons with hx-post + hx-swap="none"
         │
         ▼
Buttons don't navigate (user stays on originating page)
         │
         ▼
Dev: "Must be missing HX-Redirect header"
    Adds HX-Redirect to handler ──────────────────────┐
         │                                             │
         ▼                                             │
Sometimes works, sometimes doesn't                     │
(depends on whether response.redirected catches it)    │
         │                                             │
         ▼                                             │
New handler added, same pattern, same break            │
         │                                             │
         └────── tail chase continues ─────────────────┘
```

The `response.redirected` fallback is the silent hero that makes most things
work. When it doesn't work (edge cases, error paths, double redirects,
certain Wails WebView2 behaviors), the failure is silent and confusing.

---

## Fix Options

### Option A: Surgical JS Fix (Low Risk, Addresses Symptoms)

Fix the three confirmed bugs in `frontend/app.js`:

1. **Add `HX-Redirect` reading to `request()`:**
   ```javascript
   const redirectTo = response.headers.get("X-DixieData-Redirect")
                    || response.headers.get("HX-Redirect");
   ```

2. **Add `"this"` handling to `getTarget()`:**
   ```javascript
   function getTarget(el) {
       const selector = hxAttr(el, "hx-target");  // use hxAttr
       if (!selector) {
           const form = closestParentForm(el);
           return form ? getTarget(form) : document.body;
       }
       if (selector === "body") return document.body;
       if (selector === "this") return el;  // NEW
       return document.querySelector(selector);
   }
   ```

3. **Migrate `getSwap()` to `hxAttr()`:**
   ```javascript
   function getSwap(el) {
       const form = closestParentForm(el);
       return hxAttr(el, "hx-swap") || (form ? getSwap(form) : "innerHTML");
   }
   ```

4. **Add `hx-target` and `hx-swap` to `cachedAttrs`** (so they follow the same
   lifecycle as other htmx attributes — this makes the `hxAttr()` migration in
   steps 2-3 necessary rather than cosmetic).

**Lines changed:** ~10 lines in `frontend/app.js`
**Risk:** Low. Fixes are targeted at known gaps.
**Does not fix:** The architectural split-brain. Future htmx attribute patterns
will still need to be manually supported in the custom JS.

---

### Option B: Make app.js an htmx Extension (Medium Risk, Architectural Fix)

Convert app.js from an htmx *replacement* to an htmx *extension*:

1. **Remove the DOMContentLoaded stripping** — let htmx attributes survive on
   the DOM
2. **Remove the click/submit `preventDefault()` interceptors** — let htmx.js
   handle request dispatch
3. **Hook into htmx events** instead of DOM events:
   - `htmx:beforeRequest` → show progress, set busy state
   - `htmx:afterRequest` → read `X-DixieData-*` custom headers, show toasts,
     handle calendar refresh, handle close-feedback
   - `htmx:beforeSwap` → apply custom swap logic if needed
   - `htmx:afterSettle` → reinitialize dynamic content
4. **Preserve the custom header system** (`X-DixieData-Redirect`,
   `X-DixieData-Toast`, etc.) as htmx response header handlers
5. **Delete `request()`, `applyResponse()`, `getTarget()`, `getSwap()`,
   `hxAttr()`, `hxHas()`, and the stripping loop** (~500 lines removed)

```javascript
// After migration, app.js becomes lean:
document.body.addEventListener("htmx:beforeRequest", (event) => {
    showProgress(event.detail.elt);
    setBusyState(event.detail.elt, true);
});

document.body.addEventListener("htmx:afterRequest", (event) => {
    const headers = event.detail.xhr.getAllResponseHeaders();
    // Handle X-DixieData-Redirect, X-DixieData-Toast, etc.
    setBusyState(event.detail.elt, false);
});
```

**Lines changed:** ~500 removed, ~200 added in `frontend/app.js`
**Risk:** Medium. Need to verify that no existing JS behavior depends on the
current request lifecycle. The double-firing problem that led to stripping in
the first place must be re-tested (but htmx event hooks vs DOM event handlers
shouldn't conflict because htmx fires its events on the element, not on
`document`).
**Fixes:** All htmx behaviors (including future ones) work natively. `HX-Redirect`
works. `hx-target="this"` works. `hx-swap="none"` works. No more `hxAttr()`/
`hxHas()` contract to maintain.

---

### Option C: Full htmx.js Ownership (High Risk, Cleanest Architecture)

Remove the custom network layer entirely. Let htmx.js own 100% of request/
response handling. Move all custom behavior (toasts, redirect state, calendar
refresh, feedback modal) to server-side `HX-Trigger` response headers + small
JS event listeners.

1. **Delete `request()`, `applyResponse()`, all DOM event interceptors, the
   stripping loop, `hxAttr()`, `hxHas()`** (~800 lines removed)
2. **Replace `X-DixieData-Redirect` with `HX-Redirect`** on all 9 handlers
   that currently use the custom header
3. **Replace `X-DixieData-Toast` with `HX-Trigger`** events:
   ```go
   w.Header().Set("HX-Trigger", `{"showToast": {"message": "...", "kind": "success"}}`)
   ```
4. **Small JS listeners** for custom HX-Trigger events:
   ```javascript
   document.body.addEventListener("showToast", (event) => {
       showToast(event.detail.message, event.detail.kind);
   });
   ```

**Lines changed:** ~800 removed in `frontend/app.js`, ~18 lines changed in
`internal/appshell/` (header name replacement), ~100 lines added for JS event
listeners
**Risk:** High. Requires changing 9 Go handlers and retesting all htmx-dependent
flows. The htmx.js integration may surface edge cases not covered by the custom
JS (different swap timing, different error handling).
**Fixes:** Architecture becomes standard htmx. No custom network layer to
maintain. `HX-Redirect` works natively. All htmx features work out of the box.

---

## Recommendation

**Implement Option B now, plan Option C for next release cycle.**

**Option B** (app.js as htmx extension) gives immediate relief:
- Eliminates the `hxAttr()`/`hxHas()` contract that every future JS edit must
  remember
- Makes `HX-Redirect` work natively through htmx.js — no more server-side
  header churn
- Preserves existing custom behavior (toasts, redirect state, calendar refresh)
  via `htmx:afterRequest` hooks
- Reduces app.js by ~500 lines
- The migration is surgical: remove stripping + interceptors, add htmx event
  hooks

**Option C** is the long-term target but needs more testing. The
`X-DixieData-Redirect` → `HX-Redirect` migration on 9 handlers requires
coordinated Go + JS changes and a full smoke-test pass.

---

## Verification Checklist

Before closing this issue, verify:

- [ ] `HX-Redirect` headers from Go handlers are processed by the JS layer
  (not silently dropped)
- [ ] `hx-target="this"` buttons work for both redirect (303) and non-redirect
  (200) responses
- [ ] `getSwap()` and `getTarget()` use `hxAttr()` (consistent with rest of
  app.js)
- [ ] `hx-target` and `hx-swap` are either added to `cachedAttrs` (with
  `hxAttr()` migration) OR explicitly excluded with an inline comment
- [ ] New `audit/smoke.mjs` assertion: click any `hx-target="this"` button,
  verify navigation OR response injection (not silent drop)
- [ ] `go test ./internal/appshell/ -run TestAll303sWriteHXRedirect -v` —
  still passes (all 303s have sibling HX-Redirect)
- [ ] No new raw `getAttribute("hx-*")` calls introduced in app.js (enforced
  by grep in CI)

---

## Sources

- `frontend/app.js` — custom JS network layer (3000+ lines)
- `internal/appshell/exports_handlers.go` — `enqueueExport`/`enqueueExportWithResult`
- `internal/appshell/redirect_headers_test.go` — `TestAll303sWriteHXRedirect`
- `docs/COMMON_BUGS.md` — §1.9 (htmx 303 trap), §3.4 (JS submit interceptor)
- `docs/CODE_CHANGES.md` — "When you change app.js"
- Git history: `3f75356` (stripping introduced), `98b5acd` (caching fix),
  `3612dab` (first HX-Redirect sweep), `11f1c01` (second sweep), `a6f7fa2`
  (JS shim removal)
