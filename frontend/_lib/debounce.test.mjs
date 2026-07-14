// Regression test for the shared debounce helper
// (frontend/_lib/debounce.js, issue #573).
//
// The helper must satisfy three invariants:
//   1. Trailing-edge only. A burst of calls collapses to one
//      invocation of the wrapped fn, fired after the window
//      has been quiet for `ms`.
//   2. The trailing call sees the LATEST args, not the first.
//      (The picker search re-runs with the latest query, etc.)
//   3. cancel() drops the pending invocation entirely.
//
// Plus a self-shielding sanity test that `cancel` then a fresh
// `schedule` does fire — guards against an over-aggressive
// implementation that nulls out the closure permanently.
//
// Run with: `node --test frontend/_lib/debounce.test.mjs`

import { test } from "node:test";
import { strict as assert } from "node:assert";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const helperPath = resolve(here, "debounce.js");
const helperSrc = readFileSync(helperPath, "utf8");

// Load the helper. The script is a UMD-style IIFE that attaches
// `__dixieDebounce` to `window` (browser) or `globalThis` (Node).
// In Node we drive it via globalThis; in the browser Wails picks
// it up via a `<script>` tag in index.html.
globalThis.__dixieDebounce = undefined;
new Function(helperSrc)();
const debounce = globalThis.__dixieDebounce;
assert.equal(typeof debounce, "function", "helper must attach __dixieDebounce to globalThis when no window is present");

test("trailing-edge: burst collapses to one fire", async () => {
  let calls = 0;
  const trigger = debounce(() => {
    calls += 1;
  }, 30);

  trigger();
  trigger();
  trigger();
  // Hasn't fired yet — burst is mid-window.
  assert.equal(calls, 0, "trailing-edge must NOT fire during the burst");
  await new Promise((r) => setTimeout(r, 60));
  assert.equal(calls, 1, "exactly one trailing invocation fires after the window");
});

test("trailing-edge: latest args reach the wrapped fn", async () => {
  let seen = null;
  const trigger = debounce((value) => {
    seen = value;
  }, 30);

  trigger("a");
  trigger("b");
  trigger("c");
  await new Promise((r) => setTimeout(r, 60));
  assert.equal(seen, "c", "wrapped fn must receive the LAST call's args, not the first");
});

test("cancel: drops pending invocation", async () => {
  let calls = 0;
  const trigger = debounce(() => {
    calls += 1;
  }, 30);

  trigger();
  trigger.cancel();
  await new Promise((r) => setTimeout(r, 60));
  assert.equal(calls, 0, "cancel must prevent the pending fire");
});

test("cancel + schedule: a fresh schedule still fires", async () => {
  let calls = 0;
  const trigger = debounce(() => {
    calls += 1;
  }, 30);

  trigger();
  trigger.cancel();
  trigger.schedule();
  await new Promise((r) => setTimeout(r, 60));
  assert.equal(calls, 1, "after cancel, a new schedule must still fire");
});

test("two debounce instances are independent", async () => {
  const aCalls = [];
  const bCalls = [];
  const a = debounce((v) => aCalls.push(v), 30);
  const b = debounce((v) => bCalls.push(v), 30);

  a("first");
  b("second");
  await new Promise((r) => setTimeout(r, 60));
  assert.deepEqual(aCalls, ["first"]);
  assert.deepEqual(bCalls, ["second"]);
});
