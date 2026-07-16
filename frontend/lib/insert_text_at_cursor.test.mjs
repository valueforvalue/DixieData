// Regression test for the shared insert-at-cursor helper
// (frontend/lib/insert_text_at_cursor.js, issue #610 slice 2).
//
// The helper must satisfy four invariants:
//   1. The helper exposes itself as `window.__dixieInsertTextAtCursor`
//      (or `globalThis.__dixieInsertTextAtCursor` in Node).
//   2. Calling the helper with a non-textarea first arg returns
//      false and does not throw.
//   3. Calling the helper with a non-string second arg returns
//      false and does not throw.
//   4. Calling the helper with a valid textarea + string inserts
//      the text at the current cursor position (selectionStart /
//      selectionEnd), the textarea's value reflects the insertion,
//      an 'input' event is dispatched, and the textarea is re-
//      focused.
//
// Run with: `node --test frontend/lib/insert_text_at_cursor.test.mjs`

import { test, before } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(here, "insert_text_at_cursor.js");
const helperSrc = readFileSync(helperPath, "utf8");

// Minimal HTMLTextAreaElement mock. Implements just enough of the
// DOM contract for the helper to drive it: value, selectionStart,
// selectionEnd, setRangeText, focus, dispatchEvent. setRangeText
// is the heart of the helper — it splices the text at the
// selection range and shifts the caret to the end of the inserted
// run. We mirror the Web IDL semantics: any selected text is
// replaced; the caret lands at (start + text.length) after.
class StubTextarea {
  constructor(initialValue = "") {
    this.value = initialValue;
    this.selectionStart = initialValue.length;
    this.selectionEnd = initialValue.length;
    this.focusCalls = 0;
    this.events = [];
  }
  focus() {
    this.focusCalls += 1;
  }
  setRangeText(text, start, end, selectionMode) {
    const before = this.value.slice(0, start);
    const after = this.value.slice(end);
    this.value = before + text + after;
    // Mirrors HTMLTextAreaElement.setRangeText selectionMode="end":
    // caret lands at (start + text.length).
    this.selectionStart = start + text.length;
    this.selectionEnd = start + text.length;
    // selectionMode is consumed; not asserted in the test surface.
    void selectionMode;
  }
  dispatchEvent(event) {
    this.events.push({ type: event.type, bubbles: event.bubbles });
    return true;
  }
}

// Install the stub globally before loading the helper. The helper
// type-checks `instanceof HTMLTextAreaElement`, so we register the
// stub as the global HTMLTextAreaElement for the test runtime.
before(() => {
  /** @type {any} */ (globalThis).HTMLTextAreaElement = StubTextarea;
});

// Load the helper.
globalThis.__dixieInsertTextAtCursor = undefined;
new Function(helperSrc)();
const insertTextAtCursor = globalThis.__dixieInsertTextAtCursor;

test("helper exposes itself on globalThis when no window is present", () => {
  assert.equal(typeof insertTextAtCursor, "function", "helper must attach __dixieInsertTextAtCursor to globalThis");
});

test("non-textarea first arg: returns false, does not throw", () => {
  assert.doesNotThrow(() => {
    const result = insertTextAtCursor({ value: "fake" }, "x");
    assert.equal(result, false, "non-textarea input must return false");
  });
});

test("non-string second arg: returns false, does not throw", () => {
  const ta = new StubTextarea();
  assert.doesNotThrow(() => {
    const result = insertTextAtCursor(ta, 42);
    assert.equal(result, false, "non-string text must return false");
  });
  assert.equal(ta.value, "", "textarea value must not be mutated on non-string input");
});

test("valid input: inserts at cursor, focuses, dispatches input event", () => {
  const ta = new StubTextarea("hello world");
  ta.selectionStart = 5;
  ta.selectionEnd = 5;
  const result = insertTextAtCursor(ta, " there");
  assert.equal(result, true, "valid input must return true");
  assert.equal(ta.value, "hello there world", "text must be inserted at cursor position");
  assert.equal(ta.focusCalls, 1, "textarea must be re-focused after insert");
  assert.equal(ta.events.length, 1, "exactly one event must be dispatched");
  assert.equal(ta.events[0].type, "input");
  assert.equal(ta.events[0].bubbles, true, "input event must bubble (lets htmx + draft listeners see it)");
});

test("replaces selection: selected text is overwritten by the insert", () => {
  const ta = new StubTextarea("hello world");
  // Select "world" (indices 6..11).
  ta.selectionStart = 6;
  ta.selectionEnd = 11;
  insertTextAtCursor(ta, "team");
  assert.equal(ta.value, "hello team", "selected range must be replaced by the inserted text");
  assert.equal(ta.selectionStart, 10, "caret must land at start + inserted-text length");
});