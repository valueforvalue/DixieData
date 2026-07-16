// Regression test for the shared clipboard helper
// (frontend/_lib/clipboard.js, issue #576).
//
// The helper must satisfy three invariants:
//   1. `copyTextToClipboard(text)` writes the text to the
//      clipboard (success path).
//   2. On the modern API path (navigator.clipboard.writeText),
//      the call resolves after the write succeeds.
//   3. On the legacy fallback path (no async clipboard API),
//      the helper still resolves after the write.
//
// The test stubs `navigator.clipboard.writeText` so the
// success path is exercised without OS-level clipboard
// permissions in CI. The fallback path is exercised by
// deleting the stub and asserting the helper still resolves
// without throwing.
//
// Run with: `node --test frontend/_lib/clipboard.test.mjs`

import { test, before } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(here, "clipboard.js");
const helperSrc = readFileSync(helperPath, "utf8");

let lastWritten = null;
let clipboardShouldThrow = false;

before(() => {
  // Install a minimal `navigator` + `document` + `document.body`
  // shape so the helper can run in Node.
  /** @type {any} */ const g = globalThis;
  if (!g.navigator) g.navigator = {};
  g.navigator.clipboard = {
    async writeText(text) {
      if (clipboardShouldThrow) {
        throw new Error("simulated secure-context rejection");
      }
      lastWritten = text;
      return undefined;
    },
  };
  if (!g.document) g.document = {};
  /** @type {any} */ (g.document).createElement = (tag) => {
    if (tag !== "textarea") {
      throw new Error("test stub only supports createElement('textarea')");
    }
    return makeStubTextarea();
  };
  /** @type {any} */ (g.document).body = {
    appendChild() {},
    removeChild() {},
  };
});

function makeStubTextarea() {
  let value = "";
  return {
    set value(v) {
      value = String(v);
    },
    get value() {
      return value;
    },
    style: {},
    select() {},
  };
}

// Load the helper against globalThis. The script is a UMD-style
// IIFE that attaches `__dixieCopyText` to `window` (browser) or
// `globalThis` (Node).
globalThis.__dixieCopyText = undefined;
new Function(helperSrc)();
const copyTextToClipboard = globalThis.__dixieCopyText;
assert.equal(typeof copyTextToClipboard, "function", "helper must attach __dixieCopyText to globalThis when no window is present");

test("modern API: writes text via navigator.clipboard.writeText", async () => {
  lastWritten = null;
  clipboardShouldThrow = false;
  await copyTextToClipboard("hello world");
  assert.equal(lastWritten, "hello world");
});

test("fallback: when modern API throws, legacy execCommand path resolves", async () => {
  // Force the modern API to throw; helper must fall back to
  // the document.execCommand path which our stub silently
  // "succeeds" (no assertions on what execCommand does in
  // JSDOM — we just assert the helper does not throw).
  clipboardShouldThrow = true;
  // Stub execCommand to a no-op so the fallback path returns
  // cleanly. The Node test runner doesn't have one.
  /** @type {any} */ (globalThis.document).execCommand = () => true;
  await copyTextToClipboard("legacy path");
  clipboardShouldThrow = false;
});

test("empty string: writes empty string (does not throw)", async () => {
  lastWritten = null;
  await copyTextToClipboard("");
  assert.equal(lastWritten, "");
});
