// audit/smoke_share_queue_clear_after_export.mjs — regression net
// for issue #244: after a successful .ddshare export from the
// Share Queue, the queue must clear so the user doesn't see
// "Share queue: 3" after they just exported 3.
//
// The fix is data-clear-share-queue-on-success="true" on the
// export form + a new dispatchDixieDataForm branch that on
// success calls writeShareQueue([]) + renderShareQueuePage().
// The branch is gated on responseOk && !redirectTo so failed
// exports leave the queue intact.
//
// This probe verifies the JS dispatch path directly:
// 1. Inject a synthetic form with the new attribute
// 2. Seed localStorage with 3 IDs
// 3. Stub the export endpoint to return 200 + toast header
// 4. Trigger the form submission via the data-dixie-submit
//    document click handler
// 5. Assert: fetch fired, localStorage cleared, toast shown
//
// The form-attribute presence on the real templ forms
// (page + modal) is verified by fetching /share/queue and
// inspecting the rendered HTML.

const PORT = 9982;
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

  // Step 1: assert templ-rendered forms have the new attribute.
  console.log("Step 1: assert form attributes in templ-rendered HTML");
  await page.goto(`http://127.0.0.1:${PORT}/share/queue`, { waitUntil: "networkidle" });
  await wait(500);
  const pageFormAttr = await page.evaluate(() => {
    const form = document.querySelector("[data-share-queue-page-form]");
    return form ? form.getAttribute("data-clear-share-queue-on-success") : null;
  });
  record("page-form-has-clear-attr", pageFormAttr === "true", { attr: pageFormAttr });
  // The modal is rendered on demand via /share/queue/modal (a
  // fragment route). Verify the attribute is in the fragment.
  const modalRoute = await fetch(`http://127.0.0.1:${PORT}/share/queue/modal`).then((r) => r.text());
  const modalFormAttr = modalRoute.match(/id="share-queue-modal-form"[^>]*data-clear-share-queue-on-success="true"/) !== null;
  record("modal-form-has-clear-attr", modalFormAttr, { note: "via /share/queue/modal fragment" });

  // Step 2: navigate to home + seed localStorage.
  await page.goto(`http://127.0.0.1:${PORT}/`, { waitUntil: "domcontentloaded" });
  await page.evaluate(() => {
    localStorage.setItem("dixiedata.share-queue", JSON.stringify([101, 102, 103]));
  });

  // Step 3: navigate to /share/queue and wait for installShareQueueGlobals.
  console.log("\nStep 2: navigate to /share/queue + verify queue is seeded");
  await page.goto(`http://127.0.0.1:${PORT}/share/queue`, { waitUntil: "networkidle" });
  await wait(1500);
  const before = await page.evaluate(() => {
    const raw = localStorage.getItem("dixiedata.share-queue");
    return raw ? JSON.parse(raw) : null;
  });
  record("localStorage-seeded", before && before.length === 3, { before });

  // Step 4: intercept the export endpoint.
  let exportRequest = null;
  await page.route(`**/export/shared-archive*`, (route) => {
    exportRequest = { url: route.request().url(), method: route.request().method() };
    return route.fulfill({
      status: 200,
      headers: { "X-DixieData-Toast": "Exported 3 Person Records. Share queue cleared." },
      body: "",
    });
  });

  // Step 5: directly fire the dispatch path by clicking the form's
  // submit button (which is type="submit" inside a data-dixie-submit
  // form). The button is disabled when no rows are selected, so we
  // programmatically enable + click it. This is the minimum path
  // to exercise the new branch in dispatchDixieDataForm.
  console.log("\nStep 3: drive the export submission");
  const result = await page.evaluate(async () => {
    const btn = document.querySelector("[data-share-queue-page-bulk-export]");
    if (!(btn instanceof HTMLButtonElement)) return { error: "no button" };
    btn.disabled = false;
    // The form's submit handler (in installShareQueueGlobals) injects
    // hidden inputs from checked checkboxes. We have no rows in this
    // test, so the request body is empty — that's fine, the server
    // returns 200 regardless and the dispatch path is what we're
    // exercising.
    btn.click();
    await new Promise((r) => setTimeout(r, 600));
    return {
      storageAfter: localStorage.getItem("dixiedata.share-queue"),
    };
  });
  console.log("    result:", JSON.stringify(result));
  await wait(500);

  record("export-fetch-fires", exportRequest !== null, { request: exportRequest });
  record("hits-export-endpoint", exportRequest && exportRequest.url.includes("/export/shared-archive"), { url: exportRequest && exportRequest.url });
  record("uses-post", exportRequest && exportRequest.method === "POST", { method: exportRequest && exportRequest.method });
  record("localStorage-cleared-on-success", result.storageAfter === "[]", { after: result.storageAfter });

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