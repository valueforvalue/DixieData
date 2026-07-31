// audit/_lib/smoke_form.mjs (issue #715)
//
// submitAndWait — the canonical "click a form submit and read
// post-navigation DOM state" pattern for DixieData audit
// probes. Encapsulates the fix for the race shape that
// surfaces #709, #713, and #714 all fixed independently:
//
//   await Promise.all([
//     page.waitForURL(/\/some\/path/, ...).catch(() => null), // resolves immediately if URL unchanged
//     page.locator('form[...]').click(),                      // triggers POST via dispatcher
//   ]);
//   await page.goto(`${BASE}/some-path`, { waitUntil: 'networkidle' }); // races with dispatcher's programmatic nav
//
// Two bugs in that shape:
//   1. `waitForURL` resolves IMMEDIATELY when the current URL
//      already matches the regex — it does not wait for any
//      pending navigation to complete. The .catch(() => null)
//      silences timeouts, so a slow network silently degrades
//      to "no wait".
//   2. The defensive second `goto` fires BEFORE the
//      dispatcher's own programmatic navigation (the one
//      driven by X-DixieData-Redirect) completes. The second
//      goto's GET fetches the page before the server has
//      committed the previous POST, so the rendered DOM
//      reflects the PREVIOUS state, not the just-saved one.
//
// submitAndWait fixes both:
//   - It uses `page.waitForResponse` (filtered by HTTP method
//     + URL predicate) BEFORE the click. The promise resolves
//     when the POST response headers are received, which means
//     the server has already committed the change in its
//     handler. The handler's atomic.Store / DB write happens
//     BEFORE the response is sent.
//   - It then awaits `waitForLoadState('networkidle')` so the
//     dispatcher's programmatic navigation completes (the
//     network goes idle when the GET that follows the POST
//     redirect finishes).
//
// The probe then reads DOM state directly — no second goto,
// no racing with the dispatcher's navigation.
//
// Usage:
//   const submit = page.locator('form[action="/settings/export-surface"] button[type="submit"]').first();
//   await submitAndWait(page, {
//     submit,
//     urlPredicate: (u) => u.includes('/settings/export-surface'),
//     method: 'POST',
//   });
//   const exportChecked = await page.locator('input[name="export_surface"][value="toast-only"]').isChecked();
//
// Options:
//   - submit (required): a Playwright Locator resolving to the
//     submit button (or any element that fires a form submit).
//   - urlPredicate (required): function (url: string) => boolean.
//     Used to filter `page.waitForResponse`. Should match the
//     exact endpoint URL (or a URL fragment of it).
//   - method (optional, default 'POST'): HTTP method to match.
//     The waitForResponse predicate also requires the URL to
//     match; method is the second filter.
//   - timeout (optional, default 15_000): waitForResponse timeout
//     in ms. On timeout, the call resolves with `{ response: null }`
//     rather than throwing — callers can decide whether a missing
//     response is a fatal error or a tolerated no-op (e.g. when
//     the form action is dispatched via a fetch the probe does
//     not want to assert on).
//   - networkidleTimeout (optional, default 10_000): timeout for
//     `waitForLoadState('networkidle')` after the response.
//   - extraClickOptions (optional): forwarded to the click() call.
//
// Returns:
//   { response: Response | null, page: Page }
//
// The caller does NOT need to navigate afterwards — the
// dispatcher's programmatic navigation (driven by the form's
// X-DixieData-Redirect response header) has already settled by
// the time submitAndWait returns. If the caller wants a fresh
// page load (rare), they can `await page.goto(...)` themselves.

/**
 * @typedef {object} SubmitAndWaitOptions
 * @property {import('playwright').Locator} submit Locator for the submit button
 * @property {(url: string) => boolean} urlPredicate URL match function
 * @property {'POST'|'PATCH'|'PUT'|'DELETE'|'GET'} [method='POST']
 * @property {number} [timeout=15000]
 * @property {number} [networkidleTimeout=10000]
 * @property {import('playwright').LocatorClickOptions} [extraClickOptions]
 */

/**
 * @typedef {object} SubmitAndWaitResult
 * @property {import('playwright').Response | null} response
 * @property {import('playwright').Page} page
 */

/**
 * Click a submit button and wait for the in-flight HTTP
 * response + the dispatcher's programmatic navigation to
 * settle. See file header for the race this avoids.
 *
 * @param {import('playwright').Page} page
 * @param {SubmitAndWaitOptions} opts
 * @returns {Promise<SubmitAndWaitResult>}
 */
export async function submitAndWait(page, opts) {
  if (!page) throw new Error('submitAndWait: page is required');
  if (!opts || !opts.submit) throw new Error('submitAndWait: opts.submit (Locator) is required');
  if (typeof opts.urlPredicate !== 'function') throw new Error('submitAndWait: opts.urlPredicate (function) is required');

  const method = opts.method || 'POST';
  const timeout = opts.timeout ?? 15_000;
  const networkidleTimeout = opts.networkidleTimeout ?? 10_000;

  // Subscribe to the response BEFORE the click so the listener
  // is in place when the dispatcher's fetch goes out. We use
  // Promise.all so the click is not blocked by the wait.
  const responsePromise = page
    .waitForResponse(
      (r) => opts.urlPredicate(r.url()) && r.request().method() === method,
      { timeout },
    )
    .catch(() => null);

  await opts.submit.click({ timeout: 5_000, ...(opts.extraClickOptions || {}) });

  const response = await responsePromise;

  // Wait for the dispatcher's programmatic navigation (driven
  // by X-DixieData-Redirect) to settle. networkidle is the
  // load state when no network connections are active for
  // at least 500ms.
  await page.waitForLoadState('networkidle', { timeout: networkidleTimeout }).catch(() => null);

  return { response, page };
}
