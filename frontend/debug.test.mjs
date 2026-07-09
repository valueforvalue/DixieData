// Regression test for the debug logger never-throw contract.
//
// The logger wraps every `console.*` call so an unhandled throw
// inside the logger would break application code. The catch
// sites in frontend/debug.js carry a `// intentional: never-throw
// logger` marker — this test proves the marker is honest.
//
// Run with: `node --test frontend/debug.test.mjs`
//
// See docs/agents/error-handling.md "JS catches (client-side)" and
// issue #436 Slice D.

import { test, before } from "node:test";
import { strict as assert } from "node:assert";

// --- Stubs ---------------------------------------------------------------
//
// The debug.js IIFE touches: console, fetch, navigator.sendBeacon,
// Blob, setTimeout/clearTimeout, window. We install stubs once,
// then mutate fetch/navigator per test by reassigning those
// properties on the stubs.

let debug; // window.__dixieDebug
let fetchImpl;
let beaconImpl;
let consoleStub;

before(async () => {
  // console stub: track calls; never throw.
  consoleStub = {};
  for (const m of ["log", "info", "warn", "error", "debug"]) {
    consoleStub[m] = function (...args) {
      consoleStub[`_${m}_calls`] = (consoleStub[`_${m}_calls`] || []).concat([args]);
    };
  }
  fetchImpl = () => Promise.resolve({ ok: true });
  beaconImpl = () => true;

  // Assign BEFORE importing debug.js.
  globalThis.console = consoleStub;
  globalThis.fetch = fetchImpl;
  Object.defineProperty(globalThis, "navigator", {
    value: { sendBeacon: beaconImpl },
    configurable: true,
    writable: true,
  });
  globalThis.Blob = class Blob {
    constructor(parts, opts) {
      this.parts = parts;
      this.type = (opts && opts.type) || "";
    }
  };
  globalThis.window = {
    addEventListener: () => {},
    location: { pathname: "/test", search: "" },
    __dixieDebugDisabled: false,
  };
  globalThis.setTimeout = setTimeout;
  globalThis.clearTimeout = clearTimeout;

  // Single import — the IIFE bails on subsequent loads via the
  // `if (window.__dixieDebug) return;` guard, so re-importing
  // wouldn't pick up the new stubs anyway.
  const url = new URL("./debug.js", import.meta.url);
  await import(url.href);
  debug = globalThis.window.__dixieDebug;
  assert.ok(debug, "debug.js did not install window.__dixieDebug");
});

// --- Tests ---------------------------------------------------------------

test("logger does not throw when JSON.stringify rejects an arg", () => {
  const realStringify = JSON.stringify;
  JSON.stringify = () => {
    throw new TypeError("circular");
  };
  try {
    assert.doesNotThrow(() => debug.push("info", [{ a: 1 }]));
  } finally {
    JSON.stringify = realStringify;
  }
});

test("logger does not throw on hostile toString arg", () => {
  const hostile = {
    toString() {
      throw new Error("boom");
    },
  };
  assert.doesNotThrow(() => debug.push("info", [hostile]));
});

test("console.log routes through push without throwing when push throws", () => {
  // installConsoleHook wraps console.log in a try/catch. Even if
  // push() throws, the user's console.log call must return
  // normally (and consoleStub.log below never sees the throw).
  const realPush = debug.push;
  debug.push = () => {
    throw new Error("push exploded");
  };
  let callReturned = false;
  try {
    // The HOOKED console.log (the one debug.js installed) absorbs
    // the throw. We call through it directly via globalThis.console
    // because consoleStub was the original target. To exercise the
    // hook, we need to call globalThis.console.log AFTER debug.js
    // installed it. globalThis.console is the same object — so the
    // installed hook IS consoleStub.log now.
    globalThis.console.log("hello");
    callReturned = true;
  } finally {
    debug.push = realPush;
  }
  assert.equal(callReturned, true);
});

test("flush() does not throw when fetch rejects", async () => {
  const realFetch = globalThis.fetch;
  globalThis.fetch = () => Promise.reject(new Error("network down"));
  try {
    debug.push("info", ["trigger flush"]);
    assert.doesNotThrow(() => debug.flush());
    // Let the rejected promise settle.
    await new Promise((r) => setTimeout(r, 20));
  } finally {
    globalThis.fetch = realFetch;
  }
});

test("flush() does not throw when fetch throws synchronously", () => {
  const realFetch = globalThis.fetch;
  globalThis.fetch = () => {
    throw new TypeError("fetch unavailable");
  };
  try {
    debug.push("info", ["trigger sync throw"]);
    assert.doesNotThrow(() => debug.flush());
  } finally {
    globalThis.fetch = realFetch;
  }
});
