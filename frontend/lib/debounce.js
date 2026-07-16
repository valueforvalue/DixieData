// Shared debounce helper.
//
// DixieData carries four debounce implementations that
// drifted apart (200ms vs 250ms vs 150ms vs a render-pulse
// 50ms; see issue #573). Consolidation landed in slice 1:
// this helper exists so future call sites all read the
// same code, and so fixing a bug in one place covers all
// sites.
//
// Usage (browser, UMD-style):
//
//   const debounce = window.__dixieDebounce;
//   const trigger = debounce(() => refreshPreview(), 250);
//   input.addEventListener("input", trigger);
//
// Behavior: trailing-edge only. Calls within the window
// collapse; the trailing call fires after the window has
// been quiet for `ms` milliseconds. The wrapped function
// returns whatever the underlying fn returned at the
// trailing fire; intermediate calls return `undefined`.
//
// Timer isolation: each `debounce(...)` instance owns one
// timer. The helper exposes `{ schedule, cancel }` for tests
// that need to wait for or reset the timer without firing.
//
// See frontend/lib/debounce.test.mjs for the pinned
// behavior contract and audit/smoke_articles.mjs +
// audit/smoke_browse_*.mjs for the regression net.

(function (root) {
  "use strict";

  /**
   * @template {(...args: any[]) => any} F
   * @param {F} fn
   * @param {number} ms
   * @returns {F & { schedule: () => void, cancel: () => void }}
   */
  function debounce(fn, ms) {
    /** @type {ReturnType<typeof setTimeout> | null} */
    let timer = null;

    const wrapped = function (/** @type {...any[]} */ ...args) {
      if (timer !== null) {
        clearTimeout(timer);
      }
      timer = setTimeout(() => {
        timer = null;
        return fn.apply(this, args);
      }, ms);
    };

    wrapped.schedule = function () {
      if (timer !== null) {
        clearTimeout(timer);
      }
      timer = setTimeout(() => {
        timer = null;
        fn();
      }, ms);
    };

    wrapped.cancel = function () {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
    };

    return /** @type {any} */ (wrapped);
  }

  /** @type {any} */ (root).__dixieDebounce = debounce;
  return root;
})(typeof window !== "undefined" ? window : globalThis);
