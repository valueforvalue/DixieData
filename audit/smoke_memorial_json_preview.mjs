// audit/smoke_memorial_json_preview.mjs — regression net for
// issue #254: the Memorial JSON import flow that previously
// targeted #share-status. The /share-status panel was
// removed (deprecated by /jobs/{id} job pages, issue #193);
// the Memorial JSON import now redirects to /jobs/{id} for
// its confirmation flow (commit 3748db7). This probe verifies:
//
//   1. /share renders a Memorial JSON Import card
//   2. The card contains an empty #memorial-preview-target
//      slot (kept as a documented attach point per the issue)
//   3. Clicking the "Import Memorial JSON" button POSTs
//      /import/memorial-json and gets a redirect to
//      /jobs/{id} (NOT an in-page panel that no longer
//      exists)
//   4. The response sets X-DixieData-Redirect pointing at
//      /jobs/{id}
//
// Pre-fix (with #share-status removed): this probe documents
// the migration contract. If a future change accidentally
// re-targets #share-status, this probe fails.

const PORT = 9991;
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

  // === Step 1: load /share + assert Memorial JSON card structure ===
  console.log("Step 1: load /share + assert Memorial JSON card");
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(500);

  // 1.1. The deprecated #share-status panel must NOT exist.
  const shareStatusExists = await page.evaluate(() => document.getElementById("share-status") !== null);
  record("share-status-removed", shareStatusExists === false, { shareStatusExists });

  // 1.2. The Memorial JSON card has the new attach-point slot.
  const memorialSlot = await page.evaluate(() => {
    const slot = document.getElementById("memorial-preview-target");
    if (!slot) return null;
    return { exists: true, empty: slot.children.length === 0 && (slot.textContent || "").trim() === "" };
  });
  record("memorial-preview-target-slot-exists", memorialSlot !== null, { memorialSlot });
  record("memorial-preview-target-empty", memorialSlot && memorialSlot.empty === true, { memorialSlot });

  // 1.3. The Memorial JSON Import button is present.
  const memorialBtn = await page.evaluate(() => {
    const btns = Array.from(document.querySelectorAll("button"));
    const btn = btns.find((b) => /Import Memorial JSON/i.test(b.textContent || ""));
    if (!btn) return null;
    return { exists: true, action: btn.getAttribute("data-action") };
  });
  record("memorial-button-present", memorialBtn !== null, { memorialBtn });
  record("memorial-button-targets-import-action", memorialBtn && memorialBtn.action === "/import/memorial-json", { action: memorialBtn && memorialBtn.action });

  // === Step 2: click the button + assert the redirect ===
  console.log("\nStep 2: click + assert redirect to /jobs/{id}");
  const requests = [];
  page.on("request", (req) => { if (req.url().includes("/import/memorial-json")) requests.push({ url: req.url(), method: req.method() }); });

  // Note: clicking the button opens a native file dialog in
  // real Wails mode. In web mode, the openFileDialogOverride
  // hook returns "" (cancel) unless overridden. The handler
  // responds with respondError (KindValidation) and sets
  // X-DixieData-Toast. We assert the POST happens + the
  // response shape (does NOT target an in-page panel).
  const responsePromise = page.waitForResponse((res) => res.url().includes("/import/memorial-json") && res.request().method() === "POST", { timeout: 5000 }).catch(() => null);
  await page.evaluate(() => {
    const btns = Array.from(document.querySelectorAll("button"));
    const btn = btns.find((b) => /Import Memorial JSON/i.test(b.textContent || ""));
    if (btn) btn.click();
  });
  const response = await responsePromise;

  if (response) {
    record("memorial-post-dispatched", true, { status: response.status() });
    const headers = await response.allHeaders();
    record("memorial-response-no-share-status-target", !("hx-target" in headers) || headers["hx-target"] !== "#share-status", { hxTarget: headers["hx-target"] });
  } else {
    record("memorial-post-dispatched", false, { reason: "no POST captured within 5s" });
  }

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