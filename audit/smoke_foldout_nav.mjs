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

  // === Step 7.5: items visible + readable on /calendar (issue #283) ===
  // The original foldout (issue #264) had a 1.00:1 contrast
  // ratio for menuitem text — dark slate inherited from the
  // parent .pill-link against the dark navy panel. The
  // user perceived "items don't show" because they were
  // functionally invisible (1.00:1 = effectively zero
  // contrast). The fix: .foldout-menuitem color: #f2ede1
  // cream text (11.49:1 ratio, WCAG AAA). This step asserts
  // (a) the items are in the painted viewport (not just
  // present in the DOM) on /calendar, and (b) the contrast
  // ratio is at least WCAG AA (4.5:1).
  console.log("\nStep 7.5: items visible + readable on /calendar (issue #283)");
  await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
  await wait(1000);
  await page.evaluate(() => document.querySelector("[data-foldout-trigger='layout.share.menu']")?.click());
  await wait(500);
  // Blur the focused menuitem so the :focus-visible color
  // (#fff8e7 — also a passing cream) doesn't dominate the
  // contrast check; we want to assert the base state.
  await page.evaluate(() => (document.activeElement instanceof HTMLElement) ? document.activeElement.blur() : null);
  await wait(200);
  const contrastCheck = await page.evaluate(() => {
    function parseRGB(s) {
      // s might be 'rgba(36, 48, 61, 0.96)' or 'rgb(36, 48, 61)'.
      // Returns [r, g, b] for rgb, [r, g, b, a] for rgba, null otherwise.
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
    const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
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
        // Composite the panel's translucent bg over white for
        // the contrast ratio (the panel sits on a light cream
        // page surface per the layout body bg); per WCAG 2.1
        // 1.4.3, translucent fg over a known bg should be
        // composed before measuring. panelBg[3] is the alpha
        // (0..1); default 1.0 if the panel is opaque.
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
  });
  if (contrastCheck && contrastCheck.itemCount) {
    for (const it of contrastCheck.items) {
      record(`item-visible-on-calendar:${it.text}`, it.inViewport === true && it.hasSize === true, { w: it.inViewport, hasSize: it.hasSize });
      record(`item-contrast-passes-AA:${it.text}`, it.ratio !== null && it.ratio >= 4.5, { ratio: it.ratio ? Number(it.ratio.toFixed(2)) : null, fg: it.rawColor });
    }
  } else {
    record("contrast-check-ran", false, contrastCheck);
  }
  // Close the panel so the deep-link test below has a clean state.
  await page.keyboard.press("Escape");
  await wait(300);

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