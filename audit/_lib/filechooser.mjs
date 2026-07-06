// audit/_lib/filechooser.mjs — Playwright harness helper that drives
// the Wails native file-picker dialog (runtime.OpenFileDialog /
// runtime.OpenMultipleFilesDialog) from smoke probes.
//
// Background: Wails' native dialogs block on a UI-thread user
// gesture. Playwright emits a `filechooser` event for any <input
// type=file> click, and the Wails-bundled webview wires the same
// event for its native picker (so a smoke probe can install a
// handler and call `setFiles(paths)` to feed the import without a
// real human at the keyboard). Without this helper, any smoke
// step that touches "Add Images From Computer" or a CSV / file
// import hangs forever.
//
// Usage:
//
//   import { setFileChooserFixture } from './_lib/filechooser.mjs';
//
//   const unsubscribe = setFileChooserFixture(page, ['./fixture.png']);
//   await page.click('button:has-text("Add Images From Computer")');
//   // ... assertion ...
//   unsubscribe(); // optional; safe to skip if probe ends soon
//
// API:
//
//   setFileChooserFixture(page, paths) -> () => void
//     page   : Playwright Page
//     paths  : string | string[]   (single path or list)
//     returns: unsubscribe function that removes the handler
//
// Behavior:
//   - Normalizes `paths` to a string[] internally.
//   - Registers a `page.on('filechooser', ...)` handler that calls
//     `fileChooser.setFiles(paths)` for every chooser the page
//     emits while the handler is registered.
//   - Idempotent: if called twice on the same page, the second
//     call REPLACES the previous handler (does not stack). The
//     unsubscribe returned by the first call becomes a no-op.
//   - Safe to call before the dialog is opened; Playwright queues
//     listeners attached before the click that triggered the
//     chooser event.

/**
 * @typedef {import('playwright').Page} Page
 */

/**
 * Attach a one-shot file-chooser handler that feeds `paths` to
 * every native dialog the page emits while installed.
 *
 * @param {Page} page   Playwright page instance.
 * @param {string | string[]} paths  One path or a list of paths.
 * @returns {() => void} Unsubscribe — call to detach the handler.
 */
export function setFileChooserFixture(page, paths) {
  const list = Array.isArray(paths) ? paths.slice() : [paths];

  // Per-page state lives on a WeakMap so two pages in the same
  // probe script don't trip over each other's handlers.
  if (!setFileChooserFixture._slots) {
    setFileChooserFixture._slots = new WeakMap();
  }
  const slots = setFileChooserFixture._slots;

  // Replace any prior handler on this page — idempotent contract.
  const prior = slots.get(page);
  if (prior && typeof prior.detach === 'function') {
    prior.detach();
  }

  const handler = async (fileChooser) => {
    try {
      await fileChooser.setFiles(list);
    } catch (err) {
      // setFiles can reject if the chooser closed before our
      // handler ran, or if the OS dialog rejected one of the
      // paths. Surface the error to the probe via the page's
      // stderr channel without crashing the harness.
      console.error(
        `[filechooser] setFiles failed: ${err && err.message ? err.message : err}`,
      );
    }
  };

  page.on('filechooser', handler);

  let detached = false;
  const unsubscribe = () => {
    if (detached) return;
    detached = true;
    try {
      page.off('filechooser', handler);
    } catch (_) {
      // page may already be closed in the teardown path; swallow.
    }
    if (slots.get(page) === entry) {
      slots.delete(page);
    }
  };

  const entry = { detach: unsubscribe };
  slots.set(page, entry);
  return unsubscribe;
}