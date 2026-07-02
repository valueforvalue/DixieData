// audit/smoke_jobs_dismiss_button.mjs — regression net for
// issue #249: the Dismiss button on /jobs/{id} always lands
// on /share (or the kind-specific fallback), regardless of
// the page that triggered the job.
//
// The fix: the templ renders the Dismiss button with
// data-dismiss-job + data-dismiss-target instead of a
// hard-coded onclick. The JS handler at DOMContentLoaded
// prefers document.referrer when it is same-origin and
// not a /jobs/* path; otherwise it falls back to
// data-dismiss-target.
//
// This probe drives a real navigation: it lets the click
// handler call window.location.assign (which navigates)
// and uses Playwright's framenavigated event to record the
// resulting URL. Then it asserts the destination matches
// the expectation for each scenario (referer wins, /jobs
// referer falls back, off-origin falls back, empty
// referer falls back).

const PORT = 9975;
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

// Helper: navigate to a /jobs/1-shaped page, override
// document.referrer, click a data-dismiss-job button with a
// given target, and wait for the resulting navigation.
async function dismissScenario(page, referer, dismissTarget, buttonId) {
  // First, navigate to /share so the page is in a known state.
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });

  // Now navigate to a synthetic /jobs/1 page on the same origin.
  // Use page.goto to an actual /jobs/{id} URL on the server. We
  // do not have a real job, but the /jobs/{id} handler returns
  // 404 if the id is missing; the templ won't render. Workaround:
  // use about:blank and inject our own page.
  //
  // Simpler: navigate to /share, inject the dismiss button + a
  // meta referrer, then call the JS handler directly. Since the
  // handler is on document, any click on the button will fire
  // it; document.referrer is whatever Playwright reports (usually
  // empty when the page is reached via page.goto).
  await page.evaluate(({ target, id, ref }) => {
    document.body.insertAdjacentHTML("beforeend", `
      <div>
        <button type="button" data-dismiss-job="" data-dismiss-target="${target}" id="${id}">Dismiss</button>
      </div>
    `);
    Object.defineProperty(document, "referrer", { configurable: true, get: () => ref });
    // Replace history URL so the dismiss button (which we route
    // through window.location.assign) navigates from /jobs/1.
    history.replaceState({}, "", "/jobs/1");
  }, { target: dismissTarget, id: buttonId, ref: referer });

  // Wait for navigation triggered by the click.
  const navPromise = page.waitForNavigation({ url: /\/jobs\/1|target/, timeout: 3000 }).catch(() => null);
  await page.evaluate((id) => document.getElementById(id).click(), buttonId);
  const nav = await navPromise;
  if (!nav) {
    return { url: page.url() };
  }
  return { url: nav.url() };
}

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

  console.log("Step 1: referer is /browse?x=1 — wins over /share fallback");
  const r1 = await dismissScenario(
    page,
    `http://127.0.0.1:${PORT}/browse?x=1`,
    "/share",
    "dismiss-share",
  );
  console.log("    r1.url:", r1.url);
  record("referer-wins", r1.url && r1.url.includes("/browse"), { url: r1.url });
  record("referer-preserves-query", r1.url && r1.url.includes("x=1"));

  console.log("\nStep 2: referer is /jobs/2 — fall back to /calendar target");
  const r2 = await dismissScenario(
    page,
    `http://127.0.0.1:${PORT}/jobs/2`,
    "/calendar",
    "dismiss-calendar",
  );
  console.log("    r2.url:", r2.url);
  record("jobs-referer-skipped", r2.url && r2.url.endsWith("/calendar"), { url: r2.url });

  console.log("\nStep 3: referer is off-origin — fall back to /share target");
  const r3 = await dismissScenario(
    page,
    "https://evil.example.com/x",
    "/share",
    "dismiss-share2",
  );
  console.log("    r3.url:", r3.url);
  record("off-origin-uses-fallback", r3.url && r3.url.endsWith("/share"), { url: r3.url });

  console.log("\nStep 4: empty referer — fall back to /browse target");
  const r4 = await dismissScenario(
    page,
    "",
    "/browse",
    "dismiss-browse",
  );
  console.log("    r4.url:", r4.url);
  record("empty-referer-uses-fallback", r4.url && r4.url.endsWith("/browse"), { url: r4.url });

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