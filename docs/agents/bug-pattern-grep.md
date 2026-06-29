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
