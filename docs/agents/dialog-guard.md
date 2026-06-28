# Native dialog guard law

This file is the canonical reference for the **native dialog
re-entry race** that crashed every export in DixieData at least
once. If you are adding or modifying code that opens a native
`SaveFileDialog`, `OpenFileDialog`, `OpenDirectoryDialog`, or
`OpenMultipleFilesDialog`, **read this first**.

## Why this file exists

Wails v2.12.0 on Windows hosts native dialogs on the UI thread.
The frontend runs in WebView2. When a native dialog opens, it
takes focus from the WebView2 host. Wails' Windows frontend
listens for `onFocus` on the host window and calls
`Chromium.Focus()` → `controller.MoveFocus(...)` in response.

If **two** native dialogs land on the message loop at the same
time — the second one queued by a fast double-click, a parallel
JS bridge call, or a htmx race — both block the UI thread, and
the focus event chain fires while the WebView2 control is still
in an unstable state. The COM call returns
`The parameter is incorrect.`, the
`go-webview2` global error callback fires, and that callback is
hard-coded to call `os.Exit(1)`:

```go
// github.com/wailsapp/go-webview2@v1.0.22/pkg/edge/chromium.go:151
func (e *Chromium) errorCallback(err error) {
    e.globalErrorCallback(err)
    os.Exit(1)
}
```

Result: the entire DixieData process dies. Stack trace ends in
`Failed to unregister class Chrome_WidgetWin_0. Error = 1412`.
Upstream tracking: [wailsapp/wails#2807](https://github.com/wailsapp/wails/issues/2807).

## The contract

**Every call to `a.SaveFileDialog`, `a.OpenFileDialog`,
`a.OpenDirectoryDialog`, or `a.OpenMultipleFilesDialog` MUST be
guarded against re-entry.** A second call arriving while the
first dialog is still on screen MUST be rejected before it
reaches Wails.

### Pattern A — route through `guardedSaveFileDialog`

The helper at
[`internal/appshell/exports_handlers.go`](../../internal/appshell/exports_handlers.go)
already does the guard for you:

```go
func (a *App) guardedSaveFileDialog(kind string, opts runtime.SaveDialogOptions) (string, bool) {
    dupKey := fmt.Sprintf("export|%s|%s|%v", kind, opts.DefaultFilename, opts.Filters)
    if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
        return "", false
    }
    defer a.inFlight.Delete(dupKey)
    path, err := a.SaveFileDialog(opts)
    if err != nil || path == "" {
        return "", false
    }
    return path, true
}
```

Use this when you can express the dedup key as
`kind + filename + filters`. The kind prefix keeps two
different exports independent.

### Pattern B — inline `inFlight` guard

For call sites that need a more specific key (the soldier-record
PDF / JPG handlers key by `id + orientation + filename` so two
clicks on different soldiers don't block each other), copy the
inline shape used by `handleCalendarPDF` and the soldier PDF /
JPG / screenshot handlers in
[`internal/appshell/app.go`](../../internal/appshell/app.go):

```go
dupKey := fmt.Sprintf("kind|%s", uniqueIdentifier)
if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
    debug.FromContext(r.Context()).Debug("handlerName duplicate request rejected")
    respondError(w, r, KindUnavailable,
        "Export already in progress; please wait for the save dialog.", nil)
    return
}
defer a.inFlight.Delete(dupKey)

path, err := a.SaveFileDialog(runtime.SaveDialogOptions{ /* ... */ })
```

### Pattern C — guard helper that returns a sentinel error

For helpers like `exportFullDatabasePDFPath` that are called from
both an HTTP handler and a Wails binding (so neither caller has a
`*http.Request` in scope), return a sentinel error and map it at
the call site:

```go
var errExportInFlight = errors.New("export already in progress; please wait for the save dialog")

func (a *App) exportFullDatabasePDFPath(settings archive.PrintSettings) (string, error) {
    dupKey := fmt.Sprintf("db-pdf|%s", printableArchivePDFName(settings))
    if _, loaded := a.inFlight.LoadOrStore(dupKey, struct{}{}); loaded {
        return "", errExportInFlight
    }
    defer a.inFlight.Delete(dupKey)
    // ... call a.SaveFileDialog ...
}
```

Then in each caller:

```go
path, err := a.exportFullDatabasePDFPath(settings)
if errors.Is(err, errExportInFlight) {
    respondError(w, r, KindUnavailable, "...", err)
    return
}
```

## Things you must NOT do

- **Do not call `a.SaveFileDialog` directly from a handler.**
  Even single-call handlers crash because htmx can fire the
  request twice on a double-click. Always go through a guard.
- **Do not release the in-flight slot before the dialog
  returns.** Releasing early (e.g., calling `a.inFlight.Delete`
  before `a.SaveFileDialog`) re-opens the exact race the guard
  is meant to prevent. Use `defer`.
- **Do not collapse different exports onto the same key.** If
  the user exports JSON then immediately exports CSV, both
  should proceed. The key must include a kind prefix.
- **Do not share a key across users / archives / data dirs.**
  The `inFlight` map is on `*App`, which is a single instance
  per process — so the key only needs to disambiguate within
  one running app. Do not include absolute paths or PII.
- **Do not regress to native `<dialog>` for in-app modals.**
  Issue #117 tried that and shipped a transient WebView2
  focus-event reentry. See `CONTEXT.md` "Laws" for the rule.

## Where the guards live today

| Handler / binding                                       | File                                                | Pattern |
| ------------------------------------------------------- | --------------------------------------------------- | ------- |
| `handleSoldierPDF`                                      | `internal/appshell/app.go`                          | B       |
| `handleSoldierPDFNoImages`                              | `internal/appshell/app.go`                          | B       |
| `handleSoldierJPG`                                      | `internal/appshell/app.go`                          | B       |
| `handleCalendarPDF`                                     | `internal/appshell/app.go`                          | B       |
| `handleImageScreenshot`                                 | `internal/appshell/app.go`                          | B       |
| `exportFullDatabasePDFPath` (HTTP + Wails binding)      | `internal/appshell/exports_handlers.go`             | C       |
| `handleExportJSON` / `handleExportCSV` / `handleExportICalendar` / `handleExportStaticArchive` / `handleExportBackup` / `handleExportSharedArchive` / `handleExportBugReport` / `handleExportFeedbackLog` / `handleExportInsightsPDF` / `handleExportExcel` | `internal/appshell/exports_handlers.go` | A |
| `handleExportDatabasePDF` (HTTP fallback path)          | `internal/appshell/exports_handlers.go`             | C (via `exportFullDatabasePDFPath`) |
| Feedback log export                                     | `internal/appshell/app_feedback.go`                 | A       |

If you add a new export / import handler, **update this table**.

## Regression net

The test file
[`internal/appshell/save_dialog_guard_test.go`](../../internal/appshell/save_dialog_guard_test.go)
locks in the contract. Every guard variant must have:

- A duplicate-call test that asserts the second call returns
  `ok=false` (or `errExportInFlight`) without invoking the
  dialog a second time.
- A completion-then-retry test that asserts the slot is
  released after the dialog returns.
- A different-kind test that asserts two distinct kinds do not
  collide on the key.
- A cancel test that asserts cancelling the dialog releases
  the slot.

`TestExportFullDatabasePDFPathGuardKeys` covers the database
PDF path. New guards need new tests.

## How to extend coverage

1. Pick the call site and read its current handler signature.
2. Add the `dupKey` + `LoadOrStore` + `defer Delete` block
   (Pattern B), or return a sentinel error from a helper
   (Pattern C).
3. Make sure the kind prefix in `dupKey` distinguishes this
   export from every other export that shares the same
   filename template.
4. Map the duplicate case to either:
   - `respondError(w, r, KindUnavailable, "...", nil)` for HTTP
     handlers, or
   - a friendly toast / 429-equivalent for Wails bindings.
5. Add the regression test(s) listed above.
6. Update the table in this file.
7. Update `CHANGELOG.md` under `### Fixed`.

## History

- **2026-06-25** — `handleCalendarPDF` got the first inline
  guard (commit `f55bba0`) after a manual reproduction showed
  monthly PDF exports crashing on second click.
- **2026-06-27** — commit `162c353` introduced
  `guardedSaveFileDialog` and routed 9 export handlers through
  it, but missed the 5 call sites in `app.go` and
  `exportFullDatabasePDFPath`. The crash survived.
- **2026-06-27** — the round documented in this file added
  inline guards to the 5 missing sites and a sentinel-error
  pattern to the database PDF helper. Native `<dialog>` modals
  were reverted to div overlays as a defensive measure.
- **Open question** — should `OpenFileDialog` /
  `OpenDirectoryDialog` / `OpenMultipleFilesDialog` also go
  through a guarded helper? They can race the same way. Today
  each call site carries its own ad-hoc guard (or none). Worth
  a follow-up issue.

## Related

- `CONTEXT.md` — Laws section includes the same rule at
  glossary level so domain work can't drift past it.
- `CHANGELOG.md` — Unreleased > Fixed has the user-facing
  description.
- `docs/COMMON_BUGS.md` — pattern catalog entry for
  bug-hunter scans.
- `docs/CODE_CHANGES.md` — pre-mortem entry on this incident.