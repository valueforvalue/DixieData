// audit/smoke_review_queue_resolve.mjs — regression net for
// issue #248: the "Mark as Resolved" button on /review-queue
// hit /review-queue/bulk with no bulk_action, returning
// "Unknown bulk action. Use ignore or delete."
//
// Root cause: dispatchDixieDataForm only honored data-action
// when the button had no parent form. The "Mark as Resolved"
// button lives INSIDE the bulk-action form (so its checkbox
// and the data-select-all wiring stay co-located) but carries
// its own data-action="/soldiers/{id}/review/resolve?context=queue".
// The form's action won and the per-entry handler was never
// called.
//
// Fix: dispatchDixieDataForm now honors data-action when
// present, regardless of parent form. The handler builds a
// synthetic form from the button's data-action + data-method.
//
// This probe injects a synthetic form matching the templ's
// review-queue structure: 2 checkboxes + 1 bulk-action
// submit (Ignore) + 1 data-action per-row button (Mark as
// Resolved). It clicks Mark as Resolved and asserts the
// captured fetch goes to /soldiers/{id}/review/resolve,
// NOT /review-queue/bulk.

const PORT = 9972;
const SCRATCH = "C:/Development/DixieData/.scratch/webmode";
const WEB_BIN = "C:/Development/DixieData/build/bin/dixiedata-web.exe";

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

if (!existsSync(WEB_BIN)) { console.error("missing", WEB_BIN); process.exit(2); }
if (!existsSync(SCRATCH)) { console.error("missing", SCRATCH); process.exit(2); }

const server = spawn(WEB_BIN, ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH], { stdio: ["ignore", "pipe", "pipe"] });
server.stderr.on("data", () => {});
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function ready() {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`http://127.0.0.1:${PORT}/`); if (r.status >= 200 && r.status < 500) return; } catch {}
    await wait(500);
  }
  throw new Error("server never came up");
}

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

let Playwright = null;
try { Playwright = await import("playwright"); }
catch (e) { console.error("playwright import failed:", e.message); process.exit(2); }

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

  console.log("Step 1: navigate to /review-queue");
  await page.goto(`http://127.0.0.1:${PORT}/review-queue`, { waitUntil: "networkidle" });
  await wait(1000);

  console.log("\nStep 2: inject synthetic form + click Mark as Resolved");
  const result = await page.evaluate(async () => {
    document.body.insertAdjacentHTML("beforeend", `
      <form id="rpci-mark-form" data-dixie-submit="true" action="/review-queue/bulk" method="POST">
        <input type="checkbox" name="selected_ids" value="1" checked>
        <input type="checkbox" name="selected_ids" value="2" checked>
        <button type="submit" name="bulk_action" value="ignore">Ignore Selected</button>
        <button type="button" data-action="/soldiers/42/review/resolve?context=queue" data-dixie-submit="true">Mark as Resolved</button>
      </form>
    `);
    const form = document.getElementById("rpci-mark-form");
    const markBtn = form.querySelector("[data-action]");

    let capturedUrl = null;
    let capturedMethod = null;
    const originalFetch = window.fetch;
    window.fetch = async (url, options) => {
      capturedUrl = url;
      capturedMethod = options && options.method;
      // Return 200 with no X-DixieData-Redirect so dispatchDixieDataForm
      // does not navigate. We're testing the URL, not the redirect logic.
      return new Response(null, { status: 200, headers: { "X-DixieData-Toast": "Review item resolved." } });
    };
    try {
      markBtn.click();
      await new Promise((r) => setTimeout(r, 300));
      return { url: capturedUrl, method: capturedMethod };
    } finally {
      window.fetch = originalFetch;
    }
  });
  console.log("    mark-as-resolved result:", JSON.stringify(result));
  record("fetch-fires", result.url !== null);
  record("hits-resolve-endpoint", result.url && result.url.includes("/soldiers/42/review/resolve"), { url: result.url });
  record("does-not-hit-bulk", result.url && !result.url.includes("/review-queue/bulk"));
  record("preserves-context-queue", result.url && result.url.includes("context=queue"));
  record("uses-post", result.method === "POST", { method: result.method });

  await browser.close();
  console.log(`\n${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
} catch (e) {
  console.error("FATAL", e);
  process.exit(2);
} finally {
  server.kill();
  await wait(500);
}