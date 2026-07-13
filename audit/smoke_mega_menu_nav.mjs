// audit/smoke_mega_menu_nav.mjs — regression net for the post-#380 mega-menu
// pattern. Issue #380 (slice 3) collapsed the 2 prior top-nav foldouts
// (Research & Review + Share) + the standalone Insights pill into a
// single 2D Share & Review mega-menu. The probe here verifies the same
// ARIA + keyboard + navigation contract that smoke_foldout_nav.mjs
// enforced for the pre-#380 Share foldout, but on the post-#380 mega-menu
// primitive.
//
// What it asserts:
//   1. The Share & Review trigger is a <button> with aria-haspopup="menu",
//      aria-expanded="false" initially, and aria-controls pointing at the
//      panel's id.
//   2. The panel is hidden initially with role="menu" and is a <div>
//      (post-#380 mega-menus are <div>-rooted so the layout columns
//      can render inside, vs the pre-#380 foldout which was a <ul>).
//   3. Panel contains 12 menuitems organized in 2 sections
//      (Review & Research: 7 items, Share: 5 items). Issue #491
//      added "Archive Inventory" to the Review column (between
//      Open Review Queue and Open Timeline).
//   4. Each menuitem is a real <a href> (not a div with click handler).
//   5. Clicking the trigger opens the panel, sets aria-expanded="true",
//      and moves focus to the first menuitem.
//   6. ESC closes the panel and returns focus to the trigger.
//   7. Clicking outside the panel closes it.
//   8. ESC closes the panel (mega-menu design uses Tab + Enter on
//      menuitems themselves; the pre-#380 ArrowDown/ArrowUp nav was
//      intentionally dropped in slice 3 — see docs/agents/wcag.md
//      for the mega-menu vs menu pattern distinction).
//   9. The first menuitem inside the panel routes the user to a real
//      subpage (the Open Review Queue item).
//   10. First-click regression: on /calendar, clicking the trigger opens
//       the panel and it stays open (the issue #283 outside-click race
//       fix is preserved under the new mega-menu handler).
//
// Pre-#380 (issue #264) the foldout trigger selector was
// `[data-foldout-trigger='layout.share.menu']`. Post-#380 it became
// `[data-mega-menu-trigger='layout.share-review.menu']`. The probe
// uses the new selector verbatim.
//
// Migration from smoke_foldout_nav.mjs:
//   - selectors: data-foldout-trigger → data-mega-menu-trigger,
//     data-foldout-panel → data-mega-menu-panel
//   - trigger id: layout.share.menu → layout.share-review.menu
//   - expected menuitem count: 4 → 11 → 12 (issue #491 Archive
//     Inventory bump) → 11 (issue #549 Change Person removal)
//   - expected panel tag: UL → DIV (mega-menu panel is div-rooted)
//   - expected menuitem labels:Export|Import|Share Queue|Sync → 11 names
//     spread across Review & Research + Share sections
//   - kb nav behavior, ARIA contract, click-outside detection,
//     first-click regression, contrast checks all preserved.

// Issue #456 follow-up: BASE_URL set (CI mode) skips the spawn.
// Local-dev mode auto-spawns WEB_BIN against SCRATCH on PORT.
const PORT = 9993;
const SCRATCH = "C:/Development/DixieData/.scratch/webmode";
const WEB_BIN = "C:/Development/DixieData/build/bin/dixiedata-web.exe";
const BASE_URL = process.env.BASE_URL || "";

const BASE = BASE_URL || `http://127.0.0.1:${PORT}`;

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";

const OWN_SERVER = !BASE_URL;
if (OWN_SERVER) {
  if (!existsSync(WEB_BIN)) { console.error("missing", WEB_BIN); process.exit(2); }
  if (!existsSync(SCRATCH)) { console.error("missing", SCRATCH); process.exit(2); }

  spawn(WEB_BIN, ["-addr", `127.0.0.1:${PORT}`, "-scratch-dir", SCRATCH], { stdio: ["ignore", "pipe", "pipe"] }).stderr.on("data", () => {});
}
const wait = (ms) => new Promise((r) => setTimeout(r, ms));

async function ready() {
  for (let i = 0; i < 60; i++) {
    try { const r = await fetch(`${BASE}/`); if (r.status >= 200 && r.status < 500) return; } catch {}
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

let server;
let browser;
try {
  await ready();
  await wait(2000);

  browser = await Playwright.chromium.launch({ headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const page = await ctx.newPage();

  // === Step 1: trigger ARIA contract on /calendar ===
  console.log("\nStep 1: load /calendar + verify trigger ARIA contract");
  await page.goto(`${BASE}/calendar`, { waitUntil: "networkidle" });
  await wait(1000);

  const TRIGGER_SELECTOR = "[data-mega-menu-trigger='layout.share-review.menu']";
  const PANEL_SELECTOR = "[data-mega-menu-panel='layout.share-review.menu']";

  const trigger = await page.evaluate((sel) => {
    const t = document.querySelector(sel);
    if (!(t instanceof HTMLElement)) return null;
    return {
      tag: t.tagName,
      label: t.textContent.trim(),
      ariaExpanded: t.getAttribute("aria-expanded"),
      ariaHasPopup: t.getAttribute("aria-haspopup"),
      ariaControls: t.getAttribute("aria-controls"),
    };
  }, TRIGGER_SELECTOR);
  record("trigger-is-button", trigger && trigger.tag === "BUTTON", trigger);
  record("trigger-aria-haspopup-menu", trigger && trigger.ariaHasPopup === "menu", { ariaHasPopup: trigger && trigger.ariaHasPopup });
  record("trigger-aria-expanded-false-initially", trigger && trigger.ariaExpanded === "false", { ariaExpanded: trigger && trigger.ariaExpanded });
  record("trigger-aria-controls-points-at-panel", trigger && trigger.ariaControls === "layout.share-review.menu", { ariaControls: trigger && trigger.ariaControls });

  const triggerAriaCurrentOnCalendar = await page.evaluate((sel) => document.querySelector(sel)?.getAttribute("aria-current"), TRIGGER_SELECTOR);
  record("trigger-not-aria-current-on-calendar", triggerAriaCurrentOnCalendar === null || triggerAriaCurrentOnCalendar === "false", { ariaCurrent: triggerAriaCurrentOnCalendar });

  // === Step 2: panel structure ===
  console.log("\nStep 2: panel structure");
  const panel = await page.evaluate((sel) => {
    const p = document.querySelector(sel);
    if (!(p instanceof HTMLElement)) return null;
    return {
      hidden: p.classList.contains("hidden"),
      role: p.getAttribute("role"),
      tag: p.tagName,
      itemCount: p.querySelectorAll('[role="menuitem"]').length,
      items: Array.from(p.querySelectorAll('[role="menuitem"]')).map((a) => ({ label: a.textContent.trim(), href: a.getAttribute("href"), tag: a.tagName })),
    };
  }, PANEL_SELECTOR);
  record("panel-hidden-initially", panel && panel.hidden === true, panel);
  record("panel-is-div-with-role-menu", panel && panel.tag === "DIV" && panel.role === "menu", { tag: panel && panel.tag, role: panel && panel.role });
  record("panel-has-11-menuitems", panel && panel.itemCount === 11, { itemCount: panel && panel.itemCount });
  record("menuitems-are-anchors", panel && panel.items.every((i) => i.tag === "A"), { items: panel && panel.items.map((i) => i.tag) });
  record("menuitems-have-distinct-hrefs", panel && new Set(panel.items.map((i) => i.href)).size === panel.items.length, { hrefs: panel && panel.items.map((i) => i.href) });
  // 7 review-and-research items (issue #491 added "Archive Inventory"
  // to the column; the post-#380 R&R foldout had 6) + 5 share items
  // (issue #380 slice 3 added "Share landing" as the new first item
  // of the Share column to close the /share vs /share/exports
  // asymmetry footgun) = 12 total. The previous expected list of
  // 11 missed Archive Inventory AND predated the Share landing
  // rename; both stale items were caught in the 2026-07-13 CI run.
  const expectedLabels = [
    "Open Review Queue",
    "Archive Inventory",
    "Open Timeline",
    "Open Research Log",
    "Research Collections",
    "Insights",
    "Share landing",
    "Export",
    "Import",
    "Share Queue",
    "Sync",
  ];
  record("menuitems-include-expected-labels", panel && panel.items.map((i) => i.label).join("|") === expectedLabels.join("|"), { labels: panel && panel.items.map((i) => i.label) });

  // === Step 3: click opens + focuses first menuitem ===
  console.log("\nStep 3: click trigger + verify open + focus");
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(300);
  const afterClick = await page.evaluate(([tSel, pSel]) => {
    const p = document.querySelector(pSel);
    const t = document.querySelector(tSel);
    return { hidden: p.classList.contains("hidden"), ariaExpanded: t.getAttribute("aria-expanded"), activeRole: document.activeElement?.getAttribute("role") };
  }, [TRIGGER_SELECTOR, PANEL_SELECTOR]);
  record("panel-opens-on-click", afterClick.hidden === false, afterClick);
  record("aria-expanded-true-after-open", afterClick.ariaExpanded === "true", { ariaExpanded: afterClick.ariaExpanded });
  record("focus-moved-to-first-menuitem", afterClick.activeRole === "menuitem", { activeRole: afterClick.activeRole });

  // === Step 4: ESC closes + returns focus ===
  console.log("\nStep 4: ESC closes + returns focus");
  await page.keyboard.press("Escape");
  await wait(300);
  const afterEsc = await page.evaluate(([tSel, pSel]) => {
    const p = document.querySelector(pSel);
    const t = document.querySelector(tSel);
    return { hidden: p.classList.contains("hidden"), ariaExpanded: t.getAttribute("aria-expanded"), focusBackOnTrigger: document.activeElement === t };
  }, [TRIGGER_SELECTOR, PANEL_SELECTOR]);
  record("panel-closes-on-esc", afterEsc.hidden === true, afterEsc);
  record("aria-expanded-false-after-esc", afterEsc.ariaExpanded === "false", afterEsc);
  record("focus-returns-to-trigger-on-esc", afterEsc.focusBackOnTrigger === true, afterEsc);

  // === Step 5: outside click closes ===
  console.log("\nStep 5: outside click closes");
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(300);
  await page.evaluate(() => document.body.click());
  await wait(300);
  const afterOutside = await page.evaluate((sel) => document.querySelector(sel)?.classList.contains("hidden"), PANEL_SELECTOR);
  record("panel-closes-on-outside-click", afterOutside === true, { hidden: afterOutside });

  // === Step 6: keyboard ESC closes (mega-menu design) ===
  // Post-#380 (issue #380 slice 3) the mega-menu primitive does NOT
  // implement ArrowDown/ArrowUp navigation — those tests were valid
  // for the pre-#380 foldout (issue #264) and are dropped on migration.
  // The mega-menu is mouse-click + Escape-to-close; keyboard nav
  // assumes the user navigates via Tab + Enter on the menuitems
  // themselves (each menuitem is a real <a href>). If a future slice
  // adds arrow-key nav to the mega-menu, re-introduce the
  // arrow-up-wraps / arrow-down-opens assertions here.
  console.log("\nStep 6: ESC closes the mega-menu (no arrow-key nav)");
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(300);
  await page.keyboard.press("Escape");
  await wait(300);
  const afterEscFromMenu = await page.evaluate((sel) => document.querySelector(sel)?.classList.contains("hidden"), PANEL_SELECTOR);
  record("esc-from-mega-menu-closes-panel", afterEscFromMenu === true, { hidden: afterEscFromMenu });

  // === Step 7: menu items navigate to real subpages (issue #284) ===
  console.log("\nStep 7: menu items navigate to /share/{exports,imports,sync} subpages");
  await page.goto(`${BASE}/calendar`, { waitUntil: "networkidle" });
  await wait(800);
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(200);
  // Click the Share landing item (first in the Share column = 7th
  // overall now that Archive Inventory added an item to the
  // Review & Research column; was 6th before issue #491).
  await page.evaluate((sel) => {
    const items = document.querySelectorAll(`${sel} [role='menuitem']`);
    if (items[7] instanceof HTMLElement) items[7].click();
  }, PANEL_SELECTOR);
  await page.waitForURL(/\/share$/, { timeout: 5000 }).catch(() => null);
  const shareUrl = page.url();
  record("share-landing-menuitem-navigates-to-share", /\/share$/.test(shareUrl), { url: shareUrl });

  // Import item — third in Share column = 9th overall
  // (Review[7] + Share landing + Export + Import). Was 8th before
  // issue #491 added Archive Inventory to the Review column.
  await page.goto(`${BASE}/calendar`, { waitUntil: "networkidle" });
  await wait(800);
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(200);
  await page.evaluate((sel) => {
    const items = document.querySelectorAll(`${sel} [role='menuitem']`);
    if (items[9] instanceof HTMLElement) items[9].click();
  }, PANEL_SELECTOR);
  await page.waitForURL(/\/share\/imports$/, { timeout: 5000 }).catch(() => null);
  const importUrl = page.url();
  record("import-menuitem-navigates-to-share-imports", /\/share\/imports$/.test(importUrl), { url: importUrl });

  // === Step 8: visible + readable items on /calendar (issue #283) ===
  // Same contrast check as the pre-#380 probe — the post-#380 mega-menu
  // panel is also a navy overlay, so cream text is the same requirement.
  console.log("\nStep 8: items visible + readable on /calendar (issue #283 regression)");
  await page.goto(`${BASE}/calendar`, { waitUntil: "networkidle" });
  await wait(1000);
  await page.evaluate((sel) => document.querySelector(sel)?.click(), TRIGGER_SELECTOR);
  await wait(500);
  await page.evaluate(() => (document.activeElement instanceof HTMLElement) ? document.activeElement.blur() : null);
  await wait(200);
  const contrastCheck = await page.evaluate((sel) => {
    function parseRGB(s) {
      const m = s.match(/rgba?\(([^)]+)\)/);
      if (!m) return null;
      const parts = m[1].split(',').map((x) => parseFloat(x.trim()));
      return parts.length >= 3 ? parts : null;
    }
    function lum(c) {
      const [r,g,b] = c.map((v) => {
        v = v / 255;
        return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
      });
      return 0.2126 * r + 0.7152 * g + 0.0722 * b;
    }
    function ratio(c1, c2) {
      const l1 = lum(c1), l2 = lum(c2);
      const [hi, lo] = l1 > l2 ? [l1, l2] : [l2, l1];
      return (hi + 0.05) / (lo + 0.05);
    }
    const panel = document.querySelector(sel);
    if (!panel) return { error: "no panel" };
    const panelBgRaw = getComputedStyle(panel).backgroundColor;
    const panelBg = parseRGB(panelBgRaw);
    const items = Array.from(panel.querySelectorAll('[role="menuitem"]'));
    return {
      panelBg,
      itemCount: items.length,
      items: items.map((it) => {
        const r = it.getBoundingClientRect();
        const cs = getComputedStyle(it);
        const fg = parseRGB(cs.color);
        const alpha = panelBg && panelBg.length === 4 ? panelBg[3] : 1;
        const composedBg = panelBg ? [
          Math.round(panelBg[0] * alpha + 255 * (1 - alpha)),
          Math.round(panelBg[1] * alpha + 255 * (1 - alpha)),
          Math.round(panelBg[2] * alpha + 255 * (1 - alpha)),
        ] : null;
        return {
          text: it.textContent.trim(),
          inViewport: r.bottom > 0 && r.top < window.innerHeight && r.right > 0 && r.left < window.innerWidth,
          hasSize: r.width > 0 && r.height > 0,
          fg,
          rawColor: cs.color,
          bg: composedBg,
          ratio: fg && composedBg ? ratio(fg, composedBg) : null,
        };
      }),
    };
  }, PANEL_SELECTOR);
  if (contrastCheck && contrastCheck.itemCount) {
    for (const it of contrastCheck.items) {
      record(`item-visible-on-calendar:${it.text}`, it.inViewport === true && it.hasSize === true, { inViewport: it.inViewport, hasSize: it.hasSize });
      record(`item-contrast-passes-AA:${it.text}`, it.ratio !== null && it.ratio >= 4.5, { ratio: it.ratio ? Number(it.ratio.toFixed(2)) : null, fg: it.rawColor });
    }
  } else {
    record("contrast-check-ran", false, contrastCheck);
  }
  await page.keyboard.press("Escape");
  await wait(300);

  // === Step 9: first-click regression (issue #283 follow-up) ===
  console.log("\nStep 9: first-click on /calendar opens + stays open (issue #283 follow-up)");
  await page.goto(`${BASE}/calendar`, { waitUntil: "networkidle" });
  await wait(1000);
  await page.locator(TRIGGER_SELECTOR).first().click({ force: true, timeout: 5000 });
  const afterFirstClick = await page.evaluate(([tSel, pSel]) => {
    const panel = document.querySelector(pSel);
    const trigger = document.querySelector(tSel);
    return {
      panelHidden: panel ? panel.classList.contains("hidden") : null,
      panelDisplay: panel ? getComputedStyle(panel).display : null,
      ariaExpanded: trigger ? trigger.getAttribute("aria-expanded") : null,
      itemCount: panel ? panel.querySelectorAll('[role="menuitem"]').length : 0,
      firstItemInViewport: (() => {
        const item = panel ? panel.querySelector('[role="menuitem"]') : null;
        if (!item) return false;
        const r = item.getBoundingClientRect();
        return r.width > 0 && r.height > 0 && r.bottom > 0 && r.top < window.innerHeight;
      })(),
    };
  }, [TRIGGER_SELECTOR, PANEL_SELECTOR]);
  record("first-click-panel-stays-open", afterFirstClick.panelHidden === false, { state: afterFirstClick });

  console.log(`\n  mega_menu_nav probe: ${pass} passed, ${fail} failed`);
} catch (e) {
  console.error("test harness error:", e.message);
  process.exitCode = 2;
} finally {
  if (browser) {
    await browser.close().catch(() => {});
  }
  if (server && typeof server.kill === "function") server.kill();
  await wait(500);
  process.exit(process.exitCode || (fail > 0 ? 1 : 0));
}
