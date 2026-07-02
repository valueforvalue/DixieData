// audit/smoke_review_queue_resolve_reload.mjs — regression net
// for issue #250: clicking Mark as Resolved on /review-queue
// did not refresh the page, top-nav badge, or confirmation
// toast. The fix is data-reload-on-success="true" on the
// button + a dispatchDixieDataForm branch that shows the
// toast immediately and calls window.location.reload().
//
// This probe uses Playwright's framenavigated event to
// detect the reload, plus page.route to intercept the
// resolve fetch and respond with a stub 200 + toast header.
// The button is injected via page.evaluate, but the click
// is via page.click (page-aware, survives navigation).

const PORT = 9979;
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

  // Intercept the resolve endpoint with a stub 200 + toast.
  let resolveRequested = null;
  await page.route(`**/soldiers/*/review/resolve**`, (route) => {
    resolveRequested = { url: route.request().url(), method: route.request().method() };
    return route.fulfill({
      status: 200,
      headers: { "X-DixieData-Toast": "Success: review item resolved." },
      body: "",
    });
  });

  // Listen for navigations (the reload).
  const navigations = [];
  page.on("framenavigated", (frame) => {
    if (frame === page.mainFrame()) {
      navigations.push({ url: frame.url(), when: Date.now() });
    }
  });

  console.log("Step 1: navigate to /review-queue + inject button");
  await page.goto(`http://127.0.0.1:${PORT}/review-queue`, { waitUntil: "networkidle" });
  await wait(500);
  await page.evaluate(() => {
    document.body.insertAdjacentHTML("beforeend", `
      <form id="rpci-reload-form" data-dixie-submit="true">
        <button type="button" id="rpci-resolve-btn"
                data-action="/soldiers/42/review/resolve?context=queue"
                data-dixie-submit="true"
                data-reload-on-success="true">Mark as Resolved</button>
      </form>
    `);
  });
  navigations.length = 0; // reset after the initial nav

  console.log("\nStep 2: click the button");
  const beforeClick = Date.now();
  // Use force:true since the synthetic form/button may not be
  // visible in the viewport.
  await page.locator("#rpci-resolve-btn").click({ force: true });
  // Wait for the navigation that reload triggers.
  await page.waitForFunction((before) => performance.now() > before + 500, beforeClick, { timeout: 3000 }).catch(() => null);
  await wait(1500);

  console.log("\nStep 3: verify");
  console.log("    resolve request:", resolveRequested);
  console.log("    navigations:", navigations.length, navigations.map((n) => n.url));

  record("fetch-fires", resolveRequested !== null);
  record("hits-resolve-endpoint", resolveRequested && resolveRequested.url.includes("/soldiers/42/review/resolve"), { url: resolveRequested && resolveRequested.url });
  record("preserves-context-queue", resolveRequested && resolveRequested.url.includes("context=queue"));
  record("uses-post", resolveRequested && resolveRequested.method === "POST", { method: resolveRequested && resolveRequested.method });
  record("reload-fired", navigations.length > 0, { navigations: navigations.map((n) => n.url) });
  // After reload, the page should still be /review-queue.
  const finalUrl = page.url();
  record("page-reloaded-to-review-queue", finalUrl.includes("/review-queue"), { finalUrl });

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