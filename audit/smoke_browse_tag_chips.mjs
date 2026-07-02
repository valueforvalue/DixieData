// audit/smoke_browse_tag_chips.mjs — regression net for
// issue #183 Slice C: browse row tag chips + bulk-tag toolbar.

const PORT = 9989;
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

function post(url, body) {
  return fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams(body).toString(),
    redirect: "manual",
  });
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

  // === Step 1: tag a soldier ===
  console.log("Step 1: find a soldier + tag via API");
  await page.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "networkidle" });
  await wait(500);
  const sid = await page.evaluate(() => {
    const row = document.querySelector("table tbody tr");
    if (!row) return null;
    const href = row.getAttribute("data-browse-row-href");
    const m = href && href.match(/\/soldiers\/(\d+)/);
    return m ? parseInt(m[1], 10) : null;
  });
  record("soldier-id-extracted", sid !== null, { sid });
  if (sid) {
    const resp = await post(`http://127.0.0.1:${PORT}/soldiers/${sid}/tags`, { tag_name: "vc-shiloh" });
    record("tag-attached", resp.ok, { status: resp.status });
  }
  await wait(300);

  // === Step 2: reload browse, check Tags column + chips ===
  console.log("\nStep 2: browse → Tags column + chips");
  await page.goto(`http://127.0.0.1:${PORT}/browse?scope=all&page_size=200`, { waitUntil: "networkidle" });
  await wait(1000);

  const colHeader = await page.evaluate(() => {
    const th = document.querySelector('th[data-browse-column="tags"]');
    return th !== null && (th.textContent || "").includes("Tags");
  });
  record("tags-column-header", colHeader);

  // At least one row has a tag chip or em-dash in the Tags column
  const tagCells = await page.evaluate(() => {
    const cells = document.querySelectorAll('td[data-browse-column="tags"]');
    let withChips = 0, withDash = 0;
    for (const cell of cells) {
      const chip = cell.querySelector(".rounded-full");
      if (chip) withChips++;
      else if (cell.textContent.includes("—")) withDash++;
    }
    return { withChips, withDash, total: cells.length };
  });
  record("tag-cells-exist", tagCells.total > 0, tagCells);
  record("rows-with-tag-chips", tagCells.withChips > 0, tagCells);
  record("rows-with-empty-dash", tagCells.withDash > 0, tagCells);

  // === Step 3: bulk-tag toolbar ===
  console.log("\nStep 3: bulk-tag toolbar behavior");
  const toolbarExists = await page.locator("#browse-bulk-toolbar").count() > 0;
  record("toolbar-exists", toolbarExists);

  const toolbarHidden = await page.evaluate(() => {
    const t = document.querySelector("#browse-bulk-toolbar");
    return t !== null && t.classList.contains("hidden");
  });
  record("toolbar-hidden-by-default", toolbarHidden);

  // Click a desktop table checkbox
  await page.locator("table tr [data-browse-select]").first().click({ force: true });
  await wait(400);

  const toolbarVisible = await page.evaluate(() => {
    const t = document.querySelector("#browse-bulk-toolbar");
    return t !== null && !t.classList.contains("hidden");
  });
  record("toolbar-visible-after-select", toolbarVisible);

  const idsPopulated = await page.evaluate(() => {
    const input = document.querySelector("[data-browse-bulk-ids]");
    if (!(input instanceof HTMLInputElement)) return null;
    return { value: input.value, valid: /^\d+(,\d+)*$/.test(input.value.trim()) };
  });
  record("selected-ids-populated", idsPopulated && idsPopulated.valid, idsPopulated);

  const form = await page.evaluate(() => {
    const f = document.querySelector("#browse-bulk-toolbar form");
    if (!f) return null;
    return {
      action: f.action.endsWith("/browse/bulk-tag"),
      hasTagInput: !!f.querySelector("input[name='tag_name']"),
      hasApply: (f.querySelector("button[type='submit']")?.textContent || "").includes("Apply"),
    };
  });
  record("bulk-form-action", form && form.action, form);
  record("bulk-form-has-tag-input", form && form.hasTagInput);
  record("bulk-form-has-apply", form && form.hasApply);

  // Intercept + submit
  let postFired = false;
  await page.route("**/browse/bulk-tag", (route) => {
    postFired = true;
    return route.fulfill({ status: 303, headers: { "X-DixieData-Redirect": "/browse" }, body: "" });
  });
  await page.locator("#browse-bulk-toolbar button[type='submit']").click({ force: true });
  await wait(600);
  record("bulk-tag-post-fires", postFired);

  // Column toggle includes Tags
  const colToggle = await page.evaluate(() => {
    const inputs = document.querySelectorAll("input[data-browse-column-toggle]");
    for (const inp of inputs) { if (inp.value === "tags") return true; }
    return false;
  });
  record("tags-column-toggle-exists", colToggle);

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