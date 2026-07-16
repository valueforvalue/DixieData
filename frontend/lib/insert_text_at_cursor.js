// Shared insert-at-cursor helper (issue #610 slice 2).
//
// DixieData's Article editor needs to insert Markdown template
// fragments at the user's current cursor position in the
// `<textarea id="article-body">` — from cheatsheet rows
// (slice 3), the editor toolbar buttons (slice 4), and the
// table-builder modal (slice 5). Without a shared utility,
// every consumer rolls its own setRangeText dance + selection
// bookkeeping; the helpers drift apart like debounce.js did
// (issue #573).
//
// The helper uses the modern HTMLTextAreaElement.setRangeText
// API (Edge / Chrome / Firefox / Safari since ~2016) which:
//   - Preserves the textarea's native undo stack (unlike
//     `textarea.value = textarea.value + text`, which clobbers
//     it).
//   - Dispatches a single 'input' event so the existing
//     `initializeDraftForms()` local-draft-persistence wiring
//     fires without extra JS.
//   - Honors the current selection — selected text is replaced,
//     and the caret lands at end-of-inserted-text.
//
// DixieData Test Run-time only ever uses one insertion target
// (the article body textarea on /articles/new and
// /articles/{id}/edit). Future consumers (event-source
// descriptions, settings notes) can reuse this helper; until a
// second call site exists the deep-module test stays focused
// on the type-check + selection-replace + focus + event-dispatch
// contract.
//
// Behavior:
//   - input1 not an HTMLTextAreaElement -> return false.
//   - text not a string -> return false.
//   - otherwise -> setRangeText(text, start, end, "end") +
//     dispatchEvent(new Event("input", {bubbles: true})) +
//     focus(), return true.
//
// See frontend/lib/insert_text_at_cursor.test.mjs for the
// pinned behavior contract.

(function (root) {
  "use strict";

  /**
   * @param {HTMLTextAreaElement} textarea
   * @param {string} text
   * @returns {boolean}
   */
  function insertTextAtCursor(textarea, text) {
    if (typeof HTMLTextAreaElement === "undefined" || !(textarea instanceof HTMLTextAreaElement)) {
      return false;
    }
    if (typeof text !== "string") {
      return false;
    }
    var start = textarea.selectionStart;
    var end = textarea.selectionEnd;
    textarea.setRangeText(text, start, end, "end");
    /** @type {any} */ (textarea).dispatchEvent(new Event("input", { bubbles: true }));
    textarea.focus();
    return true;
  }

  /** @type {any} */ (root).__dixieInsertTextAtCursor = insertTextAtCursor;
  return root;
})(typeof window !== "undefined" ? window : globalThis);