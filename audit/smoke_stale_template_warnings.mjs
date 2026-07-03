// Regression net for issue #259 — "Show details" toggle for stale-template
// warnings in the print-config modal. The JS click handler has been wired
// for a long time (`frontend/app.js:4308-4315` — installs a click listener
// on `[data-export-templates-warnings-toggle]` that toggles the
// `hidden` class on `[data-export-templates-warnings-wrap]` and flips
// `aria-expanded` + the button label). The templ half (`internal/templates/
// partials/print_config_modal.templ:49-53`) renders the toggle button,
// the wrap div, and the live `<ul data-export-templates-warnings>` list.
// Before #259 the JS was dead because no element had the selector.
// Pre-fix: the toggle button never appears in the rendered HTML.
// Post-fix: clicking the toggle reveals the list with each stale-filter
// value as an `<li>`; clicking again hides it.
const PORT = 9987;
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
    try { const r = await fetch(`http://127.0.0.1:${PORT}/calendar`); if (r.ok) return; } catch (_) { /* not yet */ }
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

  // Step 1: confirm the toggle is rendered on both pages that include the
  // print-config modal (since #265 the modal lives on /browse AND
  // /share/exports; /share redirects elsewhere).
  console.log("Step 1: confirm the modal fragment renders the toggle + wrap");
  for (const route of ["/browse", "/share/exports"]) {
    const html = await (await fetch(`http://127.0.0.1:${PORT}${route}`)).text();
    record(`modal-rendered-${route}`, html.includes('data-export-templates-warnings-toggle'), { len: html.length });
    record(`wrap-present-${route}`, html.includes('data-export-templates-warnings-wrap'), {});
    record(`list-present-${route}`, html.includes('data-export-templates-warnings'), {});
    record(`toggle-show-label-${route}`, html.includes("Show details"), {});
  }

  const browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();
  page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

  // Step 2: open the modal in a real browser, verify the toggle is wired
  // (click on the live button changes aria-expanded + label), and that
  // the wrap reveals/collapses.
  console.log("\nStep 2: open modal + click Show/Hide details");
  await page.goto(`http://127.0.0.1:${PORT}/browse`);
  await wait(2500);

  // Trigger the real open path: openPrintConfigModal() runs
  // installExportTemplates() which wires the click listener. The
  // `[data-print-config-open]` button click handler is at
  // frontend/app.js:5023-5027. So we click the real button
  // ("Print/Export Selected") rather than naked classList surgery.
  await page.evaluate(() => {
    const btn = document.querySelector("[data-print-config-open]");
    if (btn instanceof HTMLElement) (btn).click();
  });
  await wait(800);

  const initialState = await page.evaluate(() => {
    const toggle = document.querySelector("[data-export-templates-warnings-toggle]");
    const wrap = document.querySelector("[data-export-templates-warnings-wrap]");
    if (!(toggle instanceof HTMLElement) || !(wrap instanceof HTMLElement)) return { ok: false };
    return {
      ok: true,
      label: toggle.textContent.trim(),
      expanded: toggle.getAttribute("aria-expanded"),
      wrapHidden: wrap.classList.contains("hidden"),
    };
  });
  record("toggle-initial-state", initialState.ok && initialState.label === "Show details" && initialState.expanded === "false" && initialState.wrapHidden, initialState);

  // Click the toggle once. Verify revealed.
  let revealed = null;
  for (let i = 0; i < 10; i++) {
    await page.evaluate(() => {
      const toggle = document.querySelector("[data-export-templates-warnings-toggle]");
      if (toggle instanceof HTMLElement) (toggle).click();
    });
    await wait(120);
    const snapshot = await page.evaluate(() => {
      const toggle = document.querySelector("[data-export-templates-warnings-toggle]");
      const wrap = document.querySelector("[data-export-templates-warnings-wrap]");
      if (!(toggle instanceof HTMLElement) || !(wrap instanceof HTMLElement)) return { ok: false };
      return {
        ok: true,
        label: toggle.textContent.trim(),
        expanded: toggle.getAttribute("aria-expanded"),
        wrapHidden: wrap.classList.contains("hidden"),
      };
    });
    if (snapshot.ok && !snapshot.wrapHidden) {
      revealed = snapshot;
      break;
    }
  }
  record("toggle-reveals-wrap", revealed !== null && revealed.label === "Hide details" && revealed.expanded === "true" && !revealed.wrapHidden, revealed ?? { ok: false, retries: 10 });

  // Click again — should re-collapse.
  let recollapsed = null;
  for (let i = 0; i < 10; i++) {
    await page.evaluate(() => {
      const toggle = document.querySelector("[data-export-templates-warnings-toggle]");
      if (toggle instanceof HTMLElement) (toggle).click();
    });
    await wait(120);
    const snapshot = await page.evaluate(() => {
      const toggle = document.querySelector("[data-export-templates-warnings-toggle]");
      const wrap = document.querySelector("[data-export-templates-warnings-wrap]");
      if (!(toggle instanceof HTMLElement) || !(wrap instanceof HTMLElement)) return { ok: false };
      return {
        ok: true,
        label: toggle.textContent.trim(),
        expanded: toggle.getAttribute("aria-expanded"),
        wrapHidden: wrap.classList.contains("hidden"),
      };
    });
    if (snapshot.ok && snapshot.wrapHidden && snapshot.label === "Show details") {
      recollapsed = snapshot;
      break;
    }
  }
  record("toggle-recollapses-wrap", recollapsed !== null && recollapsed.label === "Show details" && recollapsed.expanded === "false" && recollapsed.wrapHidden, recollapsed ?? { ok: false, retries: 10 });

  await browser.close();
} catch (err) {
  fail++; console.error("probe threw:", err.message);
} finally {
  try { server.kill(); } catch (_) { /* noop */ }
  setTimeout(() => {
    console.log(`\n  ${pass} passed / ${fail} failed`);
    process.exit(fail === 0 ? 0 : 1);
  }, 200);
}
