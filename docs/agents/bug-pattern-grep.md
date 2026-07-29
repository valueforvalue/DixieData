# Bug Pattern Grep Cookbook

A flat, copy-paste-ready grep reference for the 8 recurring UI
bug patterns documented in `docs/COMMON_BUGS.md`. Use this when
reviewing a change, hunting a regression, or before merging a
PR that touches `internal/appshell/`, `internal/templates/`, or
`frontend/app.js`.

Each section gives:

1. **What you find** — the bug class name and the symptom
2. **The grep** — copy-paste; each block is independent
3. **What the result means** — false-positive filtering notes

If a grep returns any non-empty result, open the file and read
the surrounding 20 lines. If the pattern matches without the
guard idiom, the change is a regression.

---

## 1. `redirect-contract-drift` (§1.10)

**Symptom:** Handler returns 200 + `X-DixieData-Toast` but
no `X-DixieData-Redirect`. The side effect happens, the toast
displays, the user stays on the form page.

```bash
# 303 writers that bypass writeExportRedirect
grep -rn 'StatusSeeOther\|http\.Redirect' internal/appshell/*.go \
  | grep -v _test.go | grep -v writeExportRedirect

# 200 handlers with X-DixieData-Toast but no X-DixieData-Redirect
# in the same file (likely a missing redirect)
for f in $(grep -l 'X-DixieData-Toast' internal/appshell/*.go | grep -v _test.go); do
  if ! grep -q 'X-DixieData-Redirect\|writeExportRedirect' "$f"; then
    echo "$f"
  fi
done

# Pre-Option-C residue: hx-post + hx-swap="none" (htmx-only path
# drops the X-DixieData-* headers)
grep -rln 'hx-swap="none"' internal/templates/ | xargs grep -l 'hx-post='
```

**False-positive filter:** A handler can be toast-only (no
redirect) if the user stays on the same page on purpose. The
[1.10 entry](COMMON_BUGS.md#110-redirect-contract-drift--handlers-keep-forgetting-x-dixie-data-redirect)
lists the cases where this is legitimate.

---

## 2. `dialog-guard-incomplete` (§4.10)

**Symptom:** A new export/import handler calls a native dialog
(`SaveFileDialog`, `OpenFileDialog`, `OpenDirectoryDialog`,
`OpenMultipleFilesDialog`) without the `inFlight.LoadOrStore` +
`defer a.inFlight.Delete` guard. Concurrent calls crash the
WebView2 process.

```bash
# All native dialog call sites in appshell
grep -rn 'a\.SaveFileDialog\|a\.OpenFileDialog\|a\.OpenDirectoryDialog\|a\.OpenMultipleFilesDialog\|runtime\.SaveFileDialog\|runtime\.OpenFileDialog' \
  internal/appshell/ --include="*.go" \
  | grep -v _test.go

# Sites that appear to miss the inFlight guard
grep -B1 -A4 'SaveFileDialog\|OpenFileDialog\|OpenDirectoryDialog\|OpenMultipleFilesDialog' \
  internal/appshell/exports_handlers.go internal/appshell/imports_handlers.go internal/appshell/app.go \
  | grep -L 'inFlight.LoadOrStore\|guardedSaveFileDialog\|errExportInFlight'
```

**False-positive filter:** The `runtime.SaveFileDialog` /
`runtime.OpenFileDialog` matches inside `internal/appshell/runtime.go`
are the **wrapper layer** and are guarded. Only matches in
`*_handlers.go` are call sites that need the guard. Always
re-read the surrounding 20 lines.

**Law:** every new export/import handler **must** guard its
native dialog call. See
[`docs/agents/dialog-guard.md`](dialog-guard.md) for the
canonical pattern (helper, inline, or sentinel-error).

---

## 3. `stale-status-panel-after-submit` (§3.5)

**Symptom:** Click button. Side effect happens. Toast displays.
**Target panel** (status panel, list, scan results) does not
refresh. User has to navigate away and back to see the result.

```bash
# Forms with hx-swap="none" that should swap into a panel
grep -rn 'hx-swap="none"' internal/templates/*.templ

# Handlers writing X-DixieData-Toast but no X-DixieData-Redirect
# in the same file (likely a 2 sub-pattern above)
for f in $(grep -l 'X-DixieData-Toast' internal/appshell/*.go | grep -v _test.go); do
  if ! grep -q 'X-DixieData-Redirect\|writeExportRedirect' "$f"; then
    echo "$f"
  fi
done

# data-results-target / data-status-target without matching target id
for d in $(grep -rho 'data-results-target="[^"]*"' internal/templates/ | sort -u); do
  target_id=$(echo "$d" | sed 's/data-results-target="//;s/"//')
  if ! grep -rq "id=\"$target_id\"" internal/templates/; then
    echo "missing target id: $target_id"
  fi
done
```

---

## 4. `htmx-attr-strip-by-boot-js` / `data-dixie-submit` missing (§1.11)

**Symptom:** Form submits. Network fires. Server runs. The
custom dispatcher (`dispatchDixieDataForm`) never reads the
response headers, so the toast doesn't render, the modal
doesn't close, the form doesn't clear. The handler side is
correct; the JS post-response path is bypassed entirely.

```bash
# Templ forms with hx-post / hx-get but no data-dixie-submit
# (pre-Option-C residue or forgotten opt-in)
for f in $(grep -rl 'hx-post=\|hx-get=' internal/templates/ --include="*.templ"); do
  if ! grep -q 'data-dixie-submit' "$f"; then
    echo "$f"
  fi
done

# The single boot-JS submit listener + the strip loop
grep -n 'addEventListener.*"submit"\|matches.*data-dixie-submit\|removeAttribute.*hx-' frontend/app.js
```

**False-positive filter:** Forms inside `htmxattr.Mux{}.Attrs()...`
without `data-dixie-submit` are **always** wrong. Forms with
just `hx-get=` for polling fragments (e.g.
`hx-get="/jobs/active" hx-trigger="every 3s"`) are correct
without `data-dixie-submit` (no submit event fires).

---

## 5. `duplicate-job-handling` (§4.11)

**Symptom:** User double-clicks "Export Database PDF". Expected:
one job, second click redirects to the existing `/jobs/{id}`.
Actual: second job is enqueued, OR the second click returns an
error body.

```bash
# The dedup map / in-flight tracking
grep -rn 'inFlight\|inFlightEntry\|alreadyInFlight' internal/appshell/*.go \
  | grep -v _test.go

# Handlers that should redirect to existing /jobs/{id} on duplicate
grep -rn 'errExportInFlight\|Export already in progress' internal/appshell/

# The jobs registry re-allocation site
grep -rn 'jobs\.NewWithConcurrency\|jobs\.New(' internal/appshell/

# Reload sites that could clobber the registry
grep -rn 'reloadServices\|a\.jobs' internal/appshell/*.go | grep -v _test.go
```

---

## 6. `toast-encoding-mojibake` (§4.12)

**Symptom:** A toast displays mojibake (`â€¦` for `…`,
`â€"` for `—`, `Ã—` for `×`, `Â ` for non-breaking space).
Most often visible when the toast appears via the
`X-DixieData-Toast` HTTP header.

```bash
# All toast header writers
grep -rn 'X-DixieData-Toast\|setToastHeader' internal/appshell/*.go \
  | grep -v _test.go

# Tests that guard against reintroduction
grep -rn 'TestInProgressToastStringsContainActualEllipsis\|TestSetInfoToastHeaderWritesInfoKind\|TestSanitiseToastForHeader' \
  internal/appshell/
```

**Law:** every `X-DixieData-Toast` write goes through
`sanitiseToastForHeader`. If a new character is needed, add it
to `toastHeaderASCIIReplacements` in
`internal/appshell/exports_handlers.go` + add a test in
`internal/appshell/toast_header_sanitise_test.go`.

---

## 7. `jobs-progress-page-fragment` (regression: `/jobs/{id}`)

**Symptom:** `/jobs/{id}` page fails one of:
- doesn't auto-poll while the job runs (c06349c)
- shows the popup overlay card for static-archive artifacts
  (c77ab9b)
- serves viewable artifacts as `attachment` (forces download,
  not inline view) (2f4d587, 34cc06f)

```bash
# The full-page JobStatusView (must auto-poll, must branch on artifact kind)
grep -n 'JobStatusView\|JobStatusFragment' internal/templates/jobs.templ

# Artifact handler disposition
grep -rn 'Content-Disposition' internal/appshell/jobs_handlers.go internal/appshell/exports_handlers.go

# Pop-up card surface for jobs
grep -rn 'overlay\.jobsProgress\|data-jobs-popup' internal/templates/ frontend/app.js
```

---

## 8. `route-misregistered-or-wrong-verb` (§4.13)

**Symptom:** A clickable button or form posts and the server
returns 405 (Method Not Allowed) with no error in the UI. Or
the button looks like it does nothing because the route is
registered with the wrong HTTP method.

```bash
# Every chi route registration
grep -rn 'r\.Get\|r\.Post\|r\.Put\|r\.Delete\|chi\.Route\|chi\.HandleFunc' \
  internal/appshell/routes.go internal/appshell/*.go \
  | grep -v _test.go

# Cross-check verb against handler
grep -rn 'http\.MethodPost\|http\.MethodGet\|http\.MethodDelete' \
  internal/appshell/*_handlers.go | grep -v _test.go

# Templ URLs that don't have a matching route
for url in $(grep -rho 'hx-get="[^"]*"\|hx-post="[^"]*"\|action="[^"]*"' \
              internal/templates/ | grep -oE '"[^"]+"' | tr -d '"' | sort -u); do
  if ! grep -rq "\"$url\"" internal/appshell/routes.go internal/appshell/*.go; then
    echo "templ uses $url but no route matches"
  fi
done
```

---

## 9. `foldout-trigger-click-race` (§3.6, FUTURE-NAV-AVOID)

**Symptom:** First click on a top-nav foldout trigger does
nothing. Workaround: click another nav link first, then
the trigger — the panel opens. The pattern is the
**bubble-phase race** between the trigger's own click
handler and a document-level outside-click handler that
closes any open panel that doesn't contain the click
target.

**Why this grep matters:** The top-nav foldout pattern
(issue #264) is going to be revamped. Any new top-nav
re-implementing the "trigger button + dropdown panel"
pattern MUST guard against the race. The grep finds
new outside-click handlers that lack the guard.

```bash
# Every document-level click handler that closes a
# dropdown / popover / panel
grep -rn 'addEventListener.*"click"' frontend/app.js \
  | grep -v _test

# Pair each with its body to check for the guard
# The guard idiom (after the fix in d8f73b7):
#   if (trigger === target || trigger.contains(target)) continue;
# A handler that iterates a list of (trigger, panel)
# pairs and calls something like
#   if (panel.classList.contains("hidden")) continue;
#   panel.classList.add("hidden");
# WITHOUT the target-equals-trigger guard is a bug.
grep -rnB 2 -A 5 'classList\.add("hidden")' frontend/app.js \
  | grep -v _test
```

**What the result means:** A new outside-click handler
that iterates `trigger`/`panel` pairs and conditionally
adds `hidden` to the panel is a foldout-trigger-click-
race waiting to happen. Add the guard before merging.
The guard must compare against the click target, not
the click target's ancestor (`trigger.contains(target)` is
NOT enough because a click on the trigger itself has
`target === trigger`, not `target` inside the trigger).

**Real example:**
- `d8f73b7 fix(foldout): outside-click handler closed the panel the trigger just opened (issue #283 followup)`

---

## 10. `foldout-install-on-htmx-swap` (§3.7, FUTURE-NAV-AVOID)

**Pattern under audit:** any new top-nav foldout trigger
that uses the `data-foldout-trigger` / `data-foldout-panel`
ARIA shape. The trigger's click listener is wired by
`installFoldouts()`, which is called from TWO places:
`document.addEventListener("DOMContentLoaded", ...)` (cold
boot) and `initializeDynamicContent` (every `htmx:load`).
A new foldout that relies on the init must verify both
hooks run for its trigger — the §3.7 cold-start bug
appears when the trigger is rendered by an htmx swap
that happens AFTER `DOMContentLoaded` fires on the
initial page.

**Grep 1 — confirm the install function is wired into
both hooks:**

```bash
grep -nE 'installFoldouts\(\)|DOMContentLoaded|initializeDynamicContent' frontend/app.js
```

**What the result means:**
- The function should appear at minimum 2 times:
  once inside the `DOMContentLoaded` handler and once
  inside `initializeDynamicContent`.
- A new init pattern (e.g. an event-driven component)
  that mirrors `installFoldouts` must follow the same
  two-hook rule. See the fix in commit
  `<pending: fix(foldout): re-init foldouts on htmx
  swap (issue #285)>` for the established shape.

**Grep 2 — find any new `addEventListener` inside a
function that lives outside the DOMContentLoaded +
initializeDynamicContent pair, paired with a trigger
selector that only appears after an htmx swap:**

```bash
grep -nE 'data-foldout-trigger|role="menuitem"' frontend/app.js internal/templates/**/*.templ
```

**What the result means:**
- Every new `[data-foldout-trigger]` must be in the
  DOM at the time `installFoldouts` runs, OR the
  function must be re-runnable on `htmx:load` (which
  is the established pattern in §3.7).
- A trigger that's added to the layout but rendered
  by an htmx swap will be missing its listener on
  cold start, which is the bug.

**Audit step:** if you add a new foldout to
`internal/templates/layout.templ`, run the
`audit/smoke_foldout_nav.mjs` probe in a fresh
`build/bin/dixiedata.exe` boot. The probe forces a
re-install via `window.__foldoutProbeReinit` (Step
10) and asserts the click toggles. If the new
foldout's trigger isn't reachable from the smoke
probe, the cold-start bug is reproducing for it.

**Real example:**
- (pending) `fix(foldout): re-init foldouts on htmx swap (issue #285)`

---

## 11. `nested-form-rendering-defect` (§2.7, release-blocker for rc/v1.1)

**Symptom:** A submit button inside a templ form appears to work
but its `closest('form')` returns `null` in the rendered DOM. The
button has no `<form>` ancestor. The dispatcher silently bails
(Layer 2 of the #676 diagnostic session).

**Why this grep matters:** HTML5 forbids `<form>` inside `<form>`.
The browser parser silently closes the outer form when it hits
the inner one. Any new `.templ` that adds a form inside an
existing form section is a regression-class bug.

```bash
# Walk every .templ for nested <form> patterns. The lint target
# scripts/lint-no-nested-forms.py (issue #682) runs in `make lint`.
# Manual fallback:
python3 -c "
import re, glob
for path in sorted(glob.glob('internal/templates/**/*.templ', recursive=True)):
    stack = []
    for i, line in enumerate(open(path, encoding='utf-8'), 1):
        m = re.match(r'^(\s*)<form(\s|>|/>)', line)
        cm = re.match(r'^(\s*)</form>', line)
        if m and not line.rstrip().endswith('/>'):
            ind = len(m.group(1))
            for s in stack:
                if s[1] < ind:
                    print(f'{path}:{i}: NESTED <form> inside <form> opened at L{s[0]}')
            stack.append((i, ind))
        if cm:
            for k in range(len(stack)-1, -1, -1):
                if stack[k][1] >= len(cm.group(1)):
                    del stack[k:]
                    break
"

# Manual confirmation in the rendered DOM:
# document.querySelector('button[type="submit"]').closest('form')
# Returns null when the bug is present.
```

**What the result means:** Any `<form>` whose opening tag is more
indented than another `<form>` still on the stack at the same
line is a violation. The fix is mechanical: lift the inner form
to a sibling of the outer form, or use the synthetic-form path
in `dispatchDixieDataForm` via `data-action`.

**Audit step:** if a PR adds a new form inside a templ section,
run `make lint-no-nested-forms` (added by #682) before opening
the PR. The lint fails on any nested form. The smoke probe
`audit/smoke_no_nested_forms.mjs` (also added by #682) walks
every page and asserts every submit button's
`closest('[data-dixie-submit="true"]')` is non-null.

**Real example:**
- `internal/templates/entry_form.templ` — outer form at L55 contains
  inner image-upload form at L392. Save Changes button at L432
  is orphaned.
- `internal/templates/soldier_card.templ` — outer images-download
  form at L567 contains inner image-upload form at L574.
  Download Selected Images button is orphaned.

---

## 12. `js-side-form-action-mutation` (§3.9, target:rc)

**Symptom:** On `/soldiers/{id}/edit`, the Save Changes button
submits to `/soldiers` (create URL) instead of `/soldiers/{id}`
(edit URL). The server returns 400; the user sees no save. The
form template renders the correct URL — the mutation happens
client-side after the page loads.

**Why this grep matters:** Any JS code that runs on form
initialization and assigns to `form.action`, `form.method`, or
`form.enctype` mutates the server-rendered values. The single
canonical exception is the synthetic-form branch in
`dispatchDixieDataForm` that builds a form from a `data-action`
URL. Any other location is a bug class (see #689).

```bash
# Any assignment to form.action / form.method / form.enctype in
# frontend/. The only allowed location is the synthetic-form
# branch in dispatchDixieDataForm.
grep -rn 'form\.action\s*=\|form\.method\s*=\|form\.enctype\s*=' frontend/ \
  | grep -v 'synthetic\|dispatchDixieDataForm'

# Verify the rendered DOM, not just the templ source:
# document.querySelector('form').action
# document.querySelector('form').dataset.entryTypeFormAction
# The second should be null/absent on the edit page. If it's
# "/soldiers", the JS-side mutation has run.
```

**What the result means:** Any mutation outside the synthetic-form
branch of `dispatchDixieDataForm` is a class-8 bug. The fix is
to remove the mutation. The lint target `scripts/lint-no-form-mutation.js`
(added by #687 extended scope) walks `frontend/**/*.js` and fails
CI on any non-allowed mutation.

**Audit step:** if a PR adds a new JS initializer that touches a
form, run `make lint-no-form-mutation` (added by #687) before
opening the PR. The lint fails on any form.action/method/enctype
assignment outside the synthetic-form branch.

**Real example:**
- `frontend/app.js:3946` — the `syncEntryTypeFields` mutation
  (still present at time of writing; fix lands as part of #689).
- The diagnostic session for #676 / #682 surfaced this bug after
  the nested-form band-aids accidentally let the dispatch fire.

---

## 13. `empty-body-dispatch` (§3.10, release-blocker for rc/v1.1)

**Symptom:** Save Changes submits an empty body. The server logs
show `raw body len=0 body=""` and `parseSoldierForm result
firstName="" lastName="" displayID="" err=<nil>`. The handler
runs, the form parses, but every field is empty. The `Update`
writes empty values to every column, wiping the record. The user
sees the original record after save with everything blank.

**Why this grep matters:** the body-construction branch in
`dispatchDixieDataForm` (frontend/app.js:5206) checks
`button.closest("form")` to decide whether to build a FormData
body. The Save button is in the truncated outer form (via the
`form="entry-edit-form"` HTML5 attribute), but the button is NOT
a DOM descendant of the truncated form — the HTML5 parser
reparented it. `button.closest("form")` returns `null`. The else
branch fires, sending an empty body.

The form-finding branch (line 5110) already handles this correctly
with `button.closest("form") || button.form`. The
body-construction branch doesn't apply the same fallback. Any
new code path that needs a form reference must apply the same
fallback.

```bash
# Walk frontend/ for body-construction branches that use
# button.closest("form") without the button.form fallback. The
# only allowed location is the form-finding branch (which
# applies the closest() || button.form fallback first) and the
# button.form branch of the synthetic-form construction.
grep -rn 'button\.closest("form")' frontend/ \
  | grep -v 'form\\s*=\\|synthetic\\|replace-by-test-results-target'

# Verify the server-side body is non-empty:
grep -n '\[DD DEBUG\] raw body' internal/appshell/soldiers_handlers.go
# [DD DEBUG] raw body len=0 body=""  ← bug present
# [DD DEBUG] raw body len=823 body="display_id=DXD-00004&first_name=..."  ← bug fixed
```

**What the result means:** Any `button.closest("form")` outside
the form-finding branch is a class-9 bug. The fix is to use the
already-resolved `form` variable (which has the fallback applied)
instead of re-checking `closest()`. The lint target
`scripts/lint-no-form-mutation.js` (added by #687 extended scope)
walks `frontend/**/*.js` and fails CI on any non-allowed
`button.closest("form")` site.

**Audit step:** if a PR adds a new code path that needs a form
reference, run `make lint-no-form-mutation` (added by #687
extended scope) before opening the PR. The lint fails on any
branch that uses `button.closest("form")` without the
`button.form` fallback.

**Real example (planned fix):**

- `frontend/app.js:5206` — the body-construction branch (still
  present at time of writing; fix lands as part of #691).
- The diagnostic session for #690/#691 surfaced this bug after
  the user added a `[DD DEBUG]` log to `handleUpdateSoldier` that
  showed `raw body len=0 body=""`.

**Related:**

- #691 (release-blocker fix)
- #682 (nested form — the root cause of the reparenting)
- #689 (JS-side form.action mutation — sibling bug class)
- `docs/COMMON_BUGS.md` §3.10 (canonical class entry)
- `docs/COMMON_BUGS.md` §3.9 (sibling class entry — JS-side form.action mutation)

---

## How to use this cookbook

1. Before merging a PR, run the greps relevant to the changed
   file. Skip greps for layers the change doesn't touch.
2. If a grep returns a non-empty result, open the file and
   read the surrounding 20 lines. The match is the candidate
   site; the read decides if it's a real bug.
3. If a real bug is found, the
   [canonical recipe](COMMON_BUGS.md) tells you the fix.
4. If a real bug is found and a test doesn't exist for it,
   **add a test in the same commit** (or a follow-up commit if
   the fix is too large for a single PR). The test becomes the
   regression net for the next sweep.

## When to grow this cookbook

- When a new `fix(*):` commit lands, add the failure pattern
  to `docs/COMMON_BUGS.md` (matching the existing style) +
  add the grep to this cookbook if it's a recurring class.
- When a new layer is added (e.g. a new templ subpackage or a
  new JS module), add a grep section for that layer's bugs.
- When a grep returns >30 false-positives on a clean tree,
  refine the grep. The goal is "first match is the bug" or
  "no matches is clean".
