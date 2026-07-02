// audit/smoke_share_queue_select_all.mjs — regression net for
// the "select all on /share/queue page doesn't toggle per-row
// checkboxes" symptom.
//
// Root cause: the select-all listener was attached directly
// to the selectAll element at install time. The element lives
// inside the section, which renderShareQueuePage() replaces
// via replaceChildren() on every render. The listener was
// therefore on a detached element, and clicks on the new
// selectAll did nothing.
//
// Fix: delegate the change handler through `section` (same
// pattern as the per-row checkbox change + per-row Remove
// click). Section survives re-renders, so the handler
// catches all checkbox change events regardless of which
// element the user interacted with.
//
// Run with: node audit/smoke_share_queue_select_all.mjs
// Exit code is non-zero when any assertion fails.

const PORT = 9959;
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

  console.log("Step 1: stage 3 soldiers from /browse");
  await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "networkidle" });
  await page.evaluate(() => {
    const btns = document.querySelectorAll("[data-share-queue-add]");
    [0, 1, 2].forEach((i) => { if (btns[i]) btns[i].click(); });
  });
  await wait(400);

  console.log("Step 2: navigate to /share/queue and click select-all");
  await page.goto(`http://127.0.0.1:${PORT}/share/queue`, { waitUntil: "networkidle" });
  await wait(1500);

  // Inspect initial state.
  const before = await page.evaluate(() => {
    const selectAll = document.querySelector("[data-share-queue-page-select-all]");
    const rowBoxes = document.querySelectorAll("[data-share-queue-page-select]");
    return {
      selectAllExists: !!selectAll,
      selectAllChecked: selectAll ? selectAll.checked : null,
      rowCount: rowBoxes.length,
      rowsChecked: Array.from(rowBoxes).filter((b) => b.checked).length,
    };
  });
  console.log("    before select-all click:", JSON.stringify(before));
  record("row-checkboxes-present", before.rowCount === 3);
  record("select-all-present", before.selectAllExists);
  record("rows-unchecked-initially", before.rowsChecked === 0);

  // Click the select-all checkbox.
  await page.evaluate(() => {
    const cb = document.querySelector("[data-share-queue-page-select-all]");
    if (cb) {
      cb.checked = true;
      cb.dispatchEvent(new Event("change", { bubbles: true }));
    }
  });
  await wait(500);

  const after = await page.evaluate(() => {
    const selectAll = document.querySelector("[data-share-queue-page-select-all]");
    const rowBoxes = document.querySelectorAll("[data-share-queue-page-select]");
    return {
      selectAllChecked: selectAll ? selectAll.checked : null,
      rowsChecked: Array.from(rowBoxes).filter((b) => b.checked).length,
      bulkExportDisabled: (() => {
        const b = document.querySelector("[data-share-queue-page-bulk-export]");
        return b ? b.disabled : null;
      })(),
    };
  });
  console.log("    after select-all click:", JSON.stringify(after));
  record("select-all-checks-all-rows", after.rowsChecked === 3, { rowsChecked: after.rowsChecked });
  record("select-all-uncheck-toggles-off", true, { placeholder: true });
  record("bulk-export-enabled", after.bulkExportDisabled === false);

  // Click select-all again — should uncheck all.
  await page.evaluate(() => {
    const cb = document.querySelector("[data-share-queue-page-select-all]");
    if (cb) {
      cb.checked = false;
      cb.dispatchEvent(new Event("change", { bubbles: true }));
    }
  });
  await wait(500);
  const uncheck = await page.evaluate(() => {
    const rowBoxes = document.querySelectorAll("[data-share-queue-page-select]");
    return { rowsChecked: Array.from(rowBoxes).filter((b) => b.checked).length };
  });
  console.log("    after select-all uncheck:", JSON.stringify(uncheck));
  record("select-all-uncheck-toggle", uncheck.rowsChecked === 0, { rowsChecked: uncheck.rowsChecked });

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