// audit/smoke_foldout_nav.mjs — regression net for
// issue #264: the Share top-nav foldout. The pattern
// (first consumer: Share) is generic — any future nav
// item that adopts the same trigger + panel + ARIA shape
// (data-foldout-trigger / data-foldout-panel, role="menu")
// will be picked up by the same installFoldouts() init
// in app.js and the same accessibility contract.
//
// Pre-fix: top-nav had two flat links (Share + Share Queue).
// Post-fix: a single Share trigger with a foldout panel
// containing 4 menu items (Export / Import / Share Queue /
// Build Share Archive).
//
// This probe verifies:
//   1. The Share trigger button is a <button> (not <a>)
//      with aria-haspopup="menu", aria-expanded="false"
//      initially, and aria-controls pointing at the panel.
//   2. The panel is hidden initially with role="menu"
//      and 4 menuitems inside.
//   3. Clicking the trigger opens the panel, sets
//      aria-expanded="true", and moves focus to the first
//      menuitem (per WAI-ARIA menu pattern).
//   4. ESC closes the panel and returns focus to the
//      trigger (keyboard accessibility).
//   5. Clicking outside the panel closes it.
//   6. ArrowDown from the trigger opens + focuses the
//      first menuitem.
//   7. ArrowUp from the first menuitem wraps to the last;
//      ArrowDown from the last wraps to the first
//      (WAI-ARIA menu wrap convention).
//   8. On /share (or any /share/* sub-page), the trigger
//      has aria-current="page" (per the issue's locked
//      decision: only the trigger, never sub-items).
//   9. On other pages (/calendar), the trigger does NOT
//      have aria-current="page".

const PORT = 9992;
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

  // === Step 1: load /calendar (Share is NOT the active page here) ===
  console.log("Step 1: load /calendar + verify trigger ARIA contract");
  await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
  await wait(1000);

  const trigger = await page.evaluate(() => {
    const t = document.querySelector("[data-foldout-trigger='layout.share.menu']");
    if (!(t instanceof HTMLElement)) return null;
    return { tag: t.tagName, label: t.textContent.trim(), ariaExpanded: t.getAttribute("aria-expanded"), ariaHasPopup: t.getAttribute("aria-haspopup"), ariaControls: t.getAttribute("aria-controls") };
  });
  record("trigger-is-button", trigger && trigger.tag === "BUTTON", trigger);
  record("trigger-aria-haspopup-menu", trigger && trigger.ariaHasPopup === "menu", { ariaHasPopup: trigger && trigger.ariaHasPopup });
  record("trigger-aria-expanded-false-initially", trigger && trigger.ariaExpanded === "false", { ariaExpanded: trigger && trigger.ariaExpanded });
  record("trigger-aria-controls-points-at-panel", trigger && trigger.ariaControls === "layout.share.menu", { ariaControls: trigger && trigger.ariaControls });

  const triggerAriaCurrentOnCalendar = await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.getAttribute("aria-current"));
  record("trigger-not-aria-current-on-calendar", triggerAriaCurrentOnCalendar === null || triggerAriaCurrentOnCalendar === "false", { ariaCurrent: triggerAriaCurrentOnCalendar });

  // === Step 2: panel structure ===
  console.log("\nStep 2: panel structure");
  const panel = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    if (!(p instanceof HTMLElement)) return null;
    return { hidden: p.classList.contains("hidden"), role: p.getAttribute("role"), tag: p.tagName, itemCount: p.querySelectorAll('[role="menuitem"]').length, items: Array.from(p.querySelectorAll('[role="menuitem"]')).map((a) => ({ label: a.textContent.trim(), href: a.getAttribute("href"), tag: a.tagName })) };
  });
  record("panel-hidden-initially", panel && panel.hidden === true, panel);
  record("panel-is-ul-with-role-menu", panel && panel.tag === "UL" && panel.role === "menu", { tag: panel && panel.tag, role: panel && panel.role });
  record("panel-has-4-menuitems", panel && panel.itemCount === 4, { itemCount: panel && panel.itemCount });
  record("menuitems-are-anchors", panel && panel.items.every((i) => i.tag === "A"), { items: panel && panel.items.map((i) => i.tag) });
  record("menuitems-have-distinct-hrefs", panel && new Set(panel.items.map((i) => i.href)).size === panel.items.length, { hrefs: panel && panel.items.map((i) => i.href) });
  record("menuitems-include-expected-labels", panel && panel.items.map((i) => i.label).join("|") === "Export|Import|Share Queue|Build Share Archive", { labels: panel && panel.items.map((i) => i.label) });

  // === Step 3: click opens + focuses first menuitem ===
  console.log("\nStep 3: click trigger + verify open + focus");
  await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.click());
  await wait(300);
  const afterClick = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    const t = document.querySelector("[data-foldout-trigger='layout.share.menu']");
    return { hidden: p.classList.contains("hidden"), ariaExpanded: t.getAttribute("aria-expanded"), activeRole: document.activeElement?.getAttribute("role") };
  });
  record("panel-opens-on-click", afterClick.hidden === false, afterClick);
  record("aria-expanded-true-after-open", afterClick.ariaExpanded === "true", { ariaExpanded: afterClick.ariaExpanded });
  record("focus-moved-to-first-menuitem", afterClick.activeRole === "menuitem", { activeRole: afterClick.activeRole });

  // === Step 4: ESC closes + returns focus ===
  console.log("\nStep 4: ESC closes + returns focus");
  await page.keyboard.press("Escape");
  await wait(300);
  const afterEsc = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    const t = document.querySelector("[data-foldout-trigger='layout.share.menu']");
    return { hidden: p.classList.contains("hidden"), ariaExpanded: t.getAttribute("aria-expanded"), focusBackOnTrigger: document.activeElement === t };
  });
  record("panel-closes-on-esc", afterEsc.hidden === true, afterEsc);
  record("aria-expanded-false-after-esc", afterEsc.ariaExpanded === "false", afterEsc);
  record("focus-returns-to-trigger-on-esc", afterEsc.focusBackOnTrigger === true, afterEsc);

  // === Step 5: outside click closes ===
  console.log("\nStep 5: outside click closes");
  await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.click());
  await wait(300);
  await page.evaluate(() => document.body.click());
  await wait(300);
  const afterOutside = await page.evaluate(() => document.querySelector("[data-foldout-panel='layout.share.menu']")?.classList.contains("hidden"));
  record("panel-closes-on-outside-click", afterOutside === true, { hidden: afterOutside });

  // === Step 6: keyboard nav ===
  console.log("\nStep 6: keyboard navigation");
  await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.focus());
  await page.keyboard.press("ArrowDown");
  await wait(200);
  const afterArrowDown = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    const items = Array.from(p.querySelectorAll('[role="menuitem"]'));
    return { hidden: p.classList.contains("hidden"), focusedIndex: items.indexOf(document.activeElement) };
  });
  record("arrow-down-opens-and-focuses-first-item", afterArrowDown.hidden === false && afterArrowDown.focusedIndex === 0, afterArrowDown);

  await page.keyboard.press("ArrowUp");
  await wait(200);
  const afterArrowUp = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    const items = Array.from(p.querySelectorAll('[role="menuitem"]'));
    return { focusedIndex: items.indexOf(document.activeElement), itemCount: items.length };
  });
  record("arrow-up-wraps-to-last-item", afterArrowUp.focusedIndex === afterArrowUp.itemCount - 1, afterArrowUp);

  await page.keyboard.press("ArrowDown");
  await wait(200);
  const afterWrap = await page.evaluate(() => {
    const p = document.querySelector("[data-foldout-panel='layout.share.menu']");
    const items = Array.from(p.querySelectorAll('[role="menuitem"]'));
    return { focusedIndex: items.indexOf(document.activeElement) };
  });
  record("arrow-down-wraps-to-first", afterWrap.focusedIndex === 0, afterWrap);

  await page.keyboard.press("Escape");
  await wait(200);

  // === Step 7: aria-current=page on /share ===
  console.log("\nStep 7: aria-current=page on /share");
  await page.goto(`http://127.0.0.1:${PORT}/share`, { waitUntil: "networkidle" });
  await wait(1000);
  const onShare = await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.getAttribute("aria-current"));
  record("aria-current-page-set-on-share", onShare === "page", { ariaCurrent: onShare });

  // === Step 8: deep-link to anchor from menu item ===
  console.log("\nStep 8: menu item deep-link to /share#export-section");
  await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
  await wait(800);
  await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.click());
  await wait(200);
  // Click Export (first menuitem)
  await page.evaluate(() => {
    const item = document.querySelector("[data-foldout-panel='layout.share.menu'] [role='menuitem']");
    if (item instanceof HTMLElement) item.click();
  });
  await page.waitForURL(/\/share#export-section$/, { timeout: 5000 }).catch(() => null);
  const url = page.url();
  record("export-menuitem-navigates-to-anchor", /\/share#export-section$/.test(url), { url });

  // === Step 9: build-share-archive anchor present ===
  console.log("\nStep 9: anchor #build-share-archive exists on /share");
  const buildAnchor = await page.evaluate(() => document.getElementById("build-share-archive") !== null);
  record("build-share-archive-anchor-present", buildAnchor === true, { present: buildAnchor });

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