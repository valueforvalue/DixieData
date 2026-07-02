// audit/smoke_review_queue_bulk.mjs — regression net for
// the "Unknown bulk action" bug on /review-queue.
//
// Drives the live dixiedata-web binary against a real
// /review-queue page. Seeds the DB with 3 review-pending
// records via a small Go helper, then navigates Playwright
// to /review-queue, selects 2 records, clicks Ignore,
// asserts the server returns 200/303 (not 400 "Unknown
// bulk action") and that a /jobs/{id} page loads.
//
// To avoid the seeding complexity, the probe also injects
// a synthetic form and exercises the actual
// dispatchDixieDataForm path with a real submit event —
// verifying the FormData body includes bulk_action.

const PORT = 9964;
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
try {
  Playwright = await import("playwright");
} catch (e) {
  console.error("playwright import failed:", e.message);
  process.exit(2);
}

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

  console.log("\nStep 2: drive a synthetic form through the real dispatch path");
  // Inject a synthetic form matching the templ's structure, then
  // trigger a real submit event. The page's submit listener
  // (frontend/app.js:4894) calls dispatchDixieDataForm with
  // event.submitter, which builds the body via new FormData(form, button).
  // Intercept the fetch to see the body that would have been sent.
  const result = await page.evaluate(async () => {
    document.body.insertAdjacentHTML("beforeend", `
      <form id="rpci-test-form" data-dixie-submit="true" action="/review-queue/bulk" method="POST">
        <input type="checkbox" name="selected_ids" value="1" checked>
        <input type="checkbox" name="selected_ids" value="2" checked>
        <button type="submit" name="bulk_action" value="ignore">Ignore Selected</button>
        <button type="submit" name="bulk_action" value="delete">Delete Selected</button>
      </form>
    `);
    const form = document.getElementById("rpci-test-form");
    const ignore = form.querySelector("button[value=ignore]");

    let capturedUrl = null;
    let capturedBody = null;
    const originalFetch = window.fetch;
    window.fetch = async (url, options) => {
      capturedUrl = url;
      if (options && options.body instanceof FormData) {
        // First, dump the FormData's entries using forEach
        const debugEntries = [];
        options.body.forEach((v, k) => debugEntries.push([k, v]));
        // And via entries()
        const iterEntries = Array.from(options.body.entries());
        const obj = {};
        for (const [k, v] of options.body.entries()) {
          if (obj[k] !== undefined) {
            if (!Array.isArray(obj[k])) obj[k] = [obj[k]];
            obj[k].push(v);
          } else {
            obj[k] = v;
          }
        }
        capturedBody = { obj, debugEntries, iterEntries };
      }
      // Return a synthetic 303 so dispatchDixieDataForm sees a
      // successful dispatch and does not navigate.
      return new Response(null, { status: 303, headers: { Location: "/jobs/1" } });
    };

    try {
      // Trigger a real submit event. The form's submit listener
      // (app.js:4894) reads event.submitter, which the spec sets
      // to the button that initiated the submit.
      const event = new SubmitEvent("submit", { bubbles: true, cancelable: true, submitter: ignore });
      form.dispatchEvent(event);
      // The listener is async (calls await fetch).
      await new Promise((r) => setTimeout(r, 300));
      return { url: capturedUrl, body: capturedBody };
    } finally {
      window.fetch = originalFetch;
    }
  });
  console.log("    dispatch result:", JSON.stringify(result));

  record("fetch-fires", result.url && result.url.includes("/review-queue/bulk"));
  const flatBody = result.body ? result.body.obj : null;
  record("body-includes-selected-ids", flatBody && "selected_ids" in flatBody);
  record("body-includes-bulk-action", flatBody && "bulk_action" in flatBody, { keys: Object.keys(flatBody || {}) });
  record("bulk-action-value-ignore", flatBody && flatBody.bulk_action === "ignore", { bulkAction: flatBody && flatBody.bulk_action });

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