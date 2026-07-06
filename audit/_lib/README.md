# `audit/_lib/` — Playwright harness helpers

Shared modules for `audit/smoke_*.mjs` probes. Each helper owns
one concern so probes can import only what they need. The
`index.mjs` barrel re-exports the multi-probe helpers for
convenience; single-file helpers (`manual-walk.mjs`,
`pension-state-save-probe.mjs`, `pension-state-widow-probe.mjs`)
are intentionally NOT re-exported because they're one-shot
probes, not reusable helpers.

| File | Exports | Purpose |
|---|---|---|
| `cleanup.mjs` | `registerCleanup`, `runWithCleanup` | Kill spawned `dixiedata-web` process trees on Ctrl-C, fatal, or successful exit. Windows `taskkill /F /T` + POSIX `pkill -9`. |
| `filechooser.mjs` | `setFileChooserFixture` | Drive Wails native file-picker dialogs (`OpenFileDialog`, `OpenMultipleFilesDialog`) from smoke probes. |
| `index.mjs` | (barrel) | Re-exports `registerCleanup`, `runWithCleanup`, `setFileChooserFixture`. |

## `setFileChooserFixture` — driving native file pickers

The Wails desktop runtime pops a native OS dialog
(`runtime.OpenFileDialog` / `runtime.OpenMultipleFilesDialog`)
when the user clicks a button wired to one of those handlers.
The dialog blocks on a user gesture, so any smoke probe that
clicks such a button without driving the dialog hangs forever.

`setFileChooserFixture` attaches a `page.on('filechooser', ...)`
handler that calls `fileChooser.setFiles(paths)` for every
chooser the page emits while the handler is installed. Wails'
WebView2 forwards its native picker through Playwright's
`filechooser` event, so the standard Playwright API works
without any monkey-patching.

### Signature

```js
import { setFileChooserFixture } from './_lib/filechooser.mjs';

const unsubscribe = setFileChooserFixture(page, paths);
```

- `page` — a Playwright `Page` instance.
- `paths` — `string | string[]`. A single path for
  `OpenFileDialog`, a list for `OpenMultipleFilesDialog`. The
  helper normalizes to a `string[]` internally.
- Returns: an `unsubscribe` function that detaches the handler.
  Optional — probes that end soon can ignore it.

### Contract

- **Idempotent.** Calling `setFileChooserFixture` twice on the
  same page replaces the previous handler instead of stacking.
  The `unsubscribe` returned by the first call becomes a no-op.
- **Per-call fixture.** The v1 scope is per-call (no registry,
  no global state). The caller passes the paths each time.
- **Scope is per-page.** Two pages in the same probe script do
  not trip over each other; the helper keeps handler slots on a
  `WeakMap`.
- **Safe to install before the click.** Playwright queues
  listeners attached before the chooser event, so the helper
  can be set up before triggering the dialog.

### Example — upload one image to an Event Record gallery

```js
import { setFileChooserFixture } from './_lib/filechooser.mjs';

const FIXTURE = path.join(__dirname, 'fixtures', 'event-image.png');

// Navigate to the event detail page first.
await page.goto(`${BASE}/events/${id}`);

// Wire the file chooser BEFORE clicking the import button.
const off = setFileChooserFixture(page, [FIXTURE]);

try {
  await page.click('button:has-text("Add Images From Computer")');
  // Wait for the import job to complete + the gallery to render.
  await page.waitForSelector('[data-image-card]', { timeout: 30_000 });
  const cardCount = await page.locator('[data-image-card]').count();
  if (cardCount < 1) throw new Error('no image card rendered');
} finally {
  off();
}
```

### Limits (out of v1 scope)

- `SaveFileDialog` (export-side native save picker) is NOT
  covered. The export probes (`smoke_articles.mjs`,
  `probe-json-export.mjs`) sidestep the picker via the
  server-side "scratch-dir" seam and don't need this helper.
  A future `setSaveDialogFixture` could land if the export
  surface ever moves to the picker.
- `MessageDialog` (native confirm / alert) is NOT covered.
  DixieData uses in-DOM confirmation flows for destructive
  actions; the helper would not add value there.
- Per-probe fixture registry is intentionally NOT built. v1
  scope is per-call; a registry can land when a second probe
  needs the same fixture list across multiple steps.

### Related

- Issue #385 — original scope + locked decisions.
- Issue #348b — first consumer (populated-gallery smoke for
  `/events/{id}/images`).
- `internal/appshell/app.go:1106` — `handleImportSoldierImages`
  call site that this helper drives (and `events_handlers.go:1259`
  for the events equivalent).