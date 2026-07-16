// Shared clipboard helper (issue #576).
//
// DixieData ships two click-to-copy surfaces:
//   1. The /jobs/{id} copy-path buttons ([data-copy-path])
//   2. The Markdown syntax cheatsheet per-row copy
//      ([data-md-cheatsheet-copy-key/value])
//
// Both want the same behaviour: write a string to the
// clipboard, fall back gracefully when the async clipboard
// API is not available, show a toast on success/failure
// (toasts stay at the call site so each surface can name
// the right thing — the helper just does the write).
//
// This helper extracts the clipboard-write half so both
// surfaces call the same code (DRY §1.1). The secure-context
// fallback path mirrors the existing copy-path handler.
//
// Usage (browser, UMD-style):
//
//   const copyText = window.__dixieCopyText;
//   await copyText("hello world");
//
// Returns a promise that resolves when the text is on the
// clipboard (modern API success) or after the legacy
// fallback completes.
//
// See frontend/lib/clipboard.test.mjs for the pinned
// behavior contract.

(function (root) {
  "use strict";

  /**
   * @param {string} text
   * @returns {Promise<void>}
   */
  async function copyTextToClipboard(text) {
    const value = String(text ?? "");
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(value);
        return;
      } catch (error) {
        // Secure-context rejection or permission denial —
        // fall through to the legacy execCommand path so
        // the user still gets the text on their clipboard.
      }
    }
    // Legacy fallback for browsers without the async
    // clipboard API (plain HTTP, sandboxed iframes, older
    // WebViews). The textarea + select + execCommand('copy')
    // sequence works in every WebView/WebKit/Blink variant
    // DixieData has ever shipped to.
    if (typeof document === "undefined") return;
    const tmp = document.createElement("textarea");
    tmp.value = value;
    tmp.style.position = "fixed";
    tmp.style.opacity = "0";
    document.body.appendChild(tmp);
    tmp.select();
    try {
      document.execCommand("copy");
    } finally {
      document.body.removeChild(tmp);
    }
  }

  /** @type {any} */ (root).__dixieCopyText = copyTextToClipboard;
  return root;
})(typeof window !== "undefined" ? window : globalThis);
