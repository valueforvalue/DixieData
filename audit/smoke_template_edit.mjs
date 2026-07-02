// audit/smoke_template_edit.mjs — regression net for
// issue #258: the Save Changes button on the print-config
// modal's Saved Templates dropdown. The button was
// already wired in the templ (data-export-templates-update)
// and the JS handler (updateSelectedTemplate) was already
// implemented, but the chi router only registered
// r.Patch("/export/templates/{id}", ...). The JS comment
// in updateSelectedTemplate said "handler accepts POST as
// well as PATCH" — true at the handler level, but chi
// matches the exact verb, so the JS POST fetch landed on
// 405 Method Not Allowed and the button silently did
// nothing. This probe asserts the full user-visible
// flow end-to-end.
//
// Pre-fix: Save Changes button click returns 405, modal
// status remains stale, rename does not persist.
// Post-fix: button click returns 200, rename persists.
//
// Steps:
//   1. Seed a template via the Save endpoint
//   2. Open /share, open the print-config modal
//   3. Verify Save Changes button is hidden by default
//   4. Load the seeded template; verify Save Changes
//      button becomes visible
//   5. Click Save Changes; verify 200 response
//   6. Rename via the template name input; click Save
//      Changes again; verify the rename persisted
//   7. Cleanup: delete the test template

const PORT = 9993;
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

let Playwright = null;
try { Playwright = await import("playwright"); }
catch (e) { console.error("playwright import failed:", e.message); process.exit(2); }

let pass = 0, fail = 0;
function record(name, ok, details = {}) {
  if (ok) { pass++; console.log(`  ✓ ${name} (${JSON.stringify(details)})`); }
  else { fail++; console.log(`  ✗ ${name} (${JSON.stringify(details)})`); }
}

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

  // === Step 1: seed a template via the Save endpoint ===
  console.log("Step 1: seed a template via /export/templates POST");
  const seedResp = await fetch(`http://127.0.0.1:${PORT}/export/templates`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      template_name: "Smoke Edit Template",
      scope: "all",
      orientation: "L",
      sort_by: "last_name",
    }).toString(),
  });
  const seedData = await seedResp.json().catch(() => ({}));
  record("seed-template-created", seedResp.status === 200 && seedData.id, { status: seedResp.status, data: seedData });
  const templateId = seedData && seedData.id;

  // === Step 2: open /share + the print-config modal ===
  console.log("\nStep 2: open /share + the print-config modal");
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(500);
  await page.evaluate(() => {
    const btn = document.querySelector("[data-print-config-open]");
    if (btn) btn.click();
  });
  await wait(800);

  // === Step 3: Save Changes button is hidden by default ===
  const saveChangesInitial = await page.evaluate(() => {
    const btn = document.querySelector("[data-export-templates-update]");
    return btn ? { hidden: btn.classList.contains("hidden") } : null;
  });
  record("save-changes-hidden-on-modal-open", saveChangesInitial && saveChangesInitial.hidden === true, saveChangesInitial);

  // === Step 4: Load the template; Save Changes becomes visible ===
  console.log("\nStep 4: load template + verify Save Changes becomes visible");
  await page.evaluate((id) => {
    const sel = document.querySelector("[data-export-templates-select]");
    if (sel instanceof HTMLSelectElement) {
      sel.value = String(id);
      sel.dispatchEvent(new Event("change", { bubbles: true }));
    }
  }, templateId);
  await wait(300);
  await page.evaluate(() => {
    const btn = document.querySelector("[data-export-templates-load]");
    if (btn) btn.click();
  });
  await wait(1500);

  const saveChangesAfterLoad = await page.evaluate(() => {
    const btn = document.querySelector("[data-export-templates-update]");
    return btn ? { hidden: btn.classList.contains("hidden") } : null;
  });
  record("save-changes-shown-after-load", saveChangesAfterLoad && saveChangesAfterLoad.hidden === false, saveChangesAfterLoad);

  // === Step 5: click Save Changes (POST /export/templates/{id}) ===
  console.log("\nStep 5: click Save Changes; assert POST returns 200");
  const updateRespPromise = page.waitForResponse(
    (res) => /\/export\/templates\/\d+/.test(res.url()) && res.request().method() === "POST" && !/\/apply/.test(res.url()),
    { timeout: 5000 }
  ).catch(() => null);
  await page.evaluate(() => {
    const btn = document.querySelector("[data-export-templates-update]");
    if (btn) btn.click();
  });
  const updateResp = await updateRespPromise;
  if (updateResp) {
    const status = updateResp.status();
    const body = await updateResp.json().catch(() => ({}));
    record("save-changes-returns-200", status === 200, { status, body });
    record("response-name-roundtripped", body && body.name === "Smoke Edit Template", { name: body && body.name });
  } else {
    record("save-changes-response-captured", false, { reason: "no POST captured within 5s" });
  }

  // === Step 6: rename + Save Changes again; assert rename persisted ===
  console.log("\nStep 6: rename + Save Changes; assert rename persisted");
  await page.evaluate(() => {
    const input = document.querySelector("[data-export-template-name-input]");
    if (input instanceof HTMLInputElement) input.value = "Smoke Edit Renamed";
  });
  await wait(300);
  const renameRespPromise = page.waitForResponse(
    (res) => /\/export\/templates\/\d+/.test(res.url()) && res.request().method() === "POST" && !/\/apply/.test(res.url()),
    { timeout: 5000 }
  ).catch(() => null);
  await page.evaluate(() => {
    const btn = document.querySelector("[data-export-templates-update]");
    if (btn) btn.click();
  });
  const renameResp = await renameRespPromise;
  if (renameResp) {
    const body = await renameResp.json().catch(() => ({}));
    record("rename-persisted", body && body.name === "Smoke Edit Renamed", { name: body && body.name });
  } else {
    record("rename-response-captured", false, { reason: "no POST captured within 5s" });
  }

  // Verify the rename persisted in storage (GET /export/templates)
  const listResp = await fetch(`http://127.0.0.1:${PORT}/export/templates`);
  const listData = await listResp.json().catch(() => ({}));
  const renamed = (listData.templates || []).find((t) => t.id === templateId);
  record("rename-visible-in-list", renamed && renamed.name === "Smoke Edit Renamed", { found: renamed });

  // === Step 7: cleanup ===
  console.log("\nStep 7: cleanup");
  const delResp = await fetch(`http://127.0.0.1:${PORT}/export/templates/${templateId}`, { method: "DELETE" });
  record("cleanup-deleted-template", delResp.ok || delResp.status === 204, { status: delResp.status });

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