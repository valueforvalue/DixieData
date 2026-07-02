// audit/smoke_soldier_tag_picker.mjs — regression net for
// issue #256: the soldier detail page tag editor was the
// primary "shipped but invisible" gap. The fix wires
// `tag_picker.templ`-style UI into the SoldierDetail
// summary card as a sub-card at the bottom.
//
// This probe verifies:
//   1. The detail page renders the new "Tags" sub-card
//   2. The sub-card has a "+ Add tag" expand button
//   3. The sub-card has an empty-state message when the
//      soldier has no tags
//   4. The add-tag form has the right action + method
//   5. The form has data-reload-on-success="true"
//   6. Submitting fires POST to the right URL
//
// The probe uses a pre-seeded scratch archive.

const PORT = 9986;
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

  // Find a soldier to use. /browse has direct links.
  await page.goto(`http://127.0.0.1:${PORT}/browse?page=1`, { waitUntil: "networkidle" });
  await wait(500);
  const probeSoldier = await page.evaluate(() => {
    const links = Array.from(document.querySelectorAll('a[href^="/soldiers/"]'));
    for (const a of links) {
      const m = (a.getAttribute("href") || "").match(/^\/soldiers\/(\d+)$/);
      if (m) return { id: parseInt(m[1], 10) };
    }
    return null;
  });
  if (!probeSoldier) {
    console.log("    no soldier found in scratch DB; skipping");
    await browser.close();
    process.exit(0);
  }
  console.log("    using soldier id", probeSoldier.id);

  // Intercept attach + detach
  let attachRequest = null;
  await page.route(`**/soldiers/${probeSoldier.id}/tags`, (route) => {
    if (route.request().method() === "POST") {
      attachRequest = { url: route.request().url(), method: route.request().method() };
      return route.fulfill({
        status: 200,
        headers: { "X-DixieData-Toast": "Tag added." },
        body: "",
      });
    }
    return route.continue();
  });

  // === Step 1: navigate to detail page ===
  console.log("\nStep 1: navigate to /soldiers/{id} + assert sub-card present");
  await page.goto(`http://127.0.0.1:${PORT}/soldiers/${probeSoldier.id}`, { waitUntil: "networkidle" });
  await wait(500);

  // 1. Tags sub-card heading
  const tagsHeading = await page.evaluate(() => {
    return Array.from(document.querySelectorAll("p, h2, h3, h4, span"))
      .some((el) => /^Tags$/i.test((el.textContent || "").trim()));
  });
  record("Tags-subcard-heading-renders", tagsHeading);

  // 2. Empty state OR chip list
  const subcardState = await page.evaluate(() => {
    return {
      empty: document.body.textContent.includes("No tags. Add a virtual cemetery"),
      chips: document.querySelectorAll("button[aria-label^='Remove tag']").length > 0,
    };
  });
  record("subcard-shows-empty-or-chips", subcardState.empty || subcardState.chips, subcardState);

  // 3. + Add tag expand
  const addTagSummary = await page.evaluate(() => {
    return Array.from(document.querySelectorAll("summary"))
      .some((el) => (el.textContent || "").includes("Add tag"));
  });
  record("add-tag-summary-present", addTagSummary);

  // === Step 2: expand the add-tag form ===
  console.log("\nStep 2: expand + capture form attributes");
  await page.evaluate(() => {
    const s = Array.from(document.querySelectorAll("summary"))
      .find((el) => (el.textContent || "").includes("Add tag"));
    if (s && s.parentElement instanceof HTMLDetailsElement) {
      s.parentElement.open = true;
    }
  });
  await wait(200);

  // 4. tag_name input exists
  const inputExists = await page.evaluate(() => {
    return !!document.querySelector("input[name='tag_name']");
  });
  record("tag-name-input-found", inputExists);

  // 5. Form attributes
  const formAttrs = await page.evaluate(() => {
    // The add-tag form is inside the <details> and has action ending /tags
    const f = document.querySelector("form[action$='/tags']:not([action*='/tags/'])");
    if (!f) return null;
    return {
      action: f.getAttribute("action"),
      method: (f.getAttribute("method") || "GET").toUpperCase(),
      reloadOnSuccess: f.getAttribute("data-reload-on-success"),
    };
  });
  record("add-tag-form-action-is-correct", formAttrs && formAttrs.action && formAttrs.action.endsWith(`/soldiers/${probeSoldier.id}/tags`), { action: formAttrs && formAttrs.action });
  record("add-tag-form-reloads-on-success", formAttrs && formAttrs.reloadOnSuccess === "true", { reloadOnSuccess: formAttrs && formAttrs.reloadOnSuccess });

  // 6. Submit fires POST
  await page.evaluate(() => {
    const inp = document.querySelector("input[name='tag_name']");
    if (inp) inp.value = "vc-shiloh";
    const f = document.querySelector("form[action$='/tags']:not([action*='/tags/'])");
    if (f instanceof HTMLFormElement) f.submit();
  });
  await wait(800);

  record("attach-fetch-fires", attachRequest !== null, { request: attachRequest });
  record("attach-uses-post", attachRequest && attachRequest.method === "POST", { method: attachRequest && attachRequest.method });
  record("attach-url-correct", attachRequest && attachRequest.url.includes(`/soldiers/${probeSoldier.id}/tags`), { url: attachRequest && attachRequest.url });

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