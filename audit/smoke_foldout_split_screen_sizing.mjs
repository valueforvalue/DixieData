// audit/smoke_foldout_split_screen_sizing.mjs — regression net
// for issue #288 slice 3: the Share foldout panel must respond
// to the split-screen layout-mode CSS rule (frontend/tailwind.css,
// `html[data-layout-mode="split-screen"] .foldout-panel`) by
// dropping the 14rem min-width floor and stretching to fit the
// viewport instead of clipping past its right edge.
//
// The auto-layout JS sets `data-layout-mode="split-screen"` on
// the <html> root at viewport widths ≤ 1000px
// (splitScreenBreakpointPx in frontend/app.js). Two viewport
// scenarios:
//
//   1. Split-screen (viewport 900×1200): the layout-mode rule
//      wins. Panel min-width drops to 0, width caps to
//      calc(100vw - 2rem). Assert the computed min-width and
//      width reflect that, NOT the templ default min-w-[14rem].
//   2. Relaxed (viewport 1600×1200): the layout-mode rule does
//      NOT match (data-layout-mode="relaxed"). The templ default
//      min-w-[14rem] wins. Assert the computed min-width is
//      14rem (≈224px) and the width is auto.
//
// Both scenarios also assert the viewport-cap
// (max-w-[calc(100vw-2rem)]) is honored — the slice-1 cap is
// always on, the layout-mode rule only governs the min-width
// floor in split-screen.
//
// Companion tests:
//   - Internal: internal/templates/components/foldout_test.go
//     (TestFoldout_PanelResponsiveSizing) locks slice 1's
//     templ-rendered class tokens.
//   - Internal: internal/templates/layout_test.go (the
//     "split-screen foldout-panel override (issue #288 slice 2)"
//     entry) locks slice 2's compiled CSS rule.
//   - This probe locks the user-visible behavior end-to-end:
//     a real Chromium viewport, a real layout-mode auto-detect,
//     a real panel-open click, computed style after layout.

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

  // === Scenario 1: split-screen (viewport 900×1200) ===
  // splitScreenBreakpointPx = 1000. 900 < 1000, so the JS
  // auto-detect sets data-layout-mode="split-screen" on <html>.
  console.log("Scenario 1: split-screen viewport (900×1200)");
  {
    const ctx = await browser.newContext({ viewport: { width: 900, height: 1200 } });
    const page = await ctx.newPage();
    page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

    await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
    await wait(1000);

    const mode = await page.evaluate(() => document.documentElement.getAttribute("data-layout-mode"));
    record("layout-mode-is-split-screen-at-900px", mode === "split-screen", { mode });

    // Open the foldout. Use real click path — synthetic
    // dispatchEvent would bypass the same bubble phase the
    // first-click regression net (#283 followup) was bitten
    // by; we want the same code path the user hits.
    await page.locator("[data-foldout-trigger='layout.share.menu']").first().click({ force: true });
    await wait(300);

    // Computed styles on the panel after layout has run.
    // min-width: 0 + width: calc(100vw - 2rem) should both
    // come out of the split-screen CSS rule, NOT the templ
    // default (min-w-[14rem] = 224px, width: auto).
    const styles = await page.evaluate(() => {
      const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
      if (!(panel instanceof HTMLElement)) return null;
      const cs = getComputedStyle(panel);
      return {
        minWidth: cs.minWidth,
        width: cs.width,
        maxWidth: cs.maxWidth,
        panelClass: panel.className,
      };
    });
    record("split-screen-panel-rendered", styles !== null, styles);
    if (styles) {
      // The split-screen rule sets min-width: 0 — must NOT be
      // 14rem / 224px (which would mean the rule never matched).
      record("split-screen-min-width-dropped", styles.minWidth === "0px", { minWidth: styles.minWidth });
      // The split-screen rule caps width to calc(100vw - 2rem)
      // = 868px at viewport 900. Must NOT be auto and must be
      // within ±1px of (viewport - 2rem) for sub-pixel slack.
      const widthPx = parseFloat(styles.width);
      record("split-screen-width-is-viewport-cap", Math.abs(widthPx - (900 - 32)) < 1, { width: styles.width });
      // Slice-1 cap (max-w-[calc(100vw-2rem)]) always on. This
      // is the user-visible "can never overflow the viewport"
      // invariant — verify even in split-screen where the
      // layout-mode rule is also active. Equals the viewport
      // cap; doesn't shrink the rendered width further because
      // width already equals the cap.
      const maxPx = parseFloat(styles.maxWidth);
      record("split-screen-max-width-capped-to-viewport", Math.abs(maxPx - (900 - 32)) < 1, { maxWidth: styles.maxWidth });
    }

    // Visual confirm: the panel's right edge stays inside
    // the viewport. If the rule were missing (regression to
    // pre-#288), the panel could overflow the right edge.
    const insideViewport = await page.evaluate(() => {
      const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
      if (!(panel instanceof HTMLElement)) return null;
      const r = panel.getBoundingClientRect();
      return {
        right: r.right,
        viewportWidth: window.innerWidth,
        inside: r.right <= window.innerWidth + 1, // 1px slack for sub-pixel rounding
        left: r.left,
        top: r.top,
        bottom: r.bottom,
      };
    });
    record("split-screen-panel-inside-viewport-right-edge", insideViewport && insideViewport.inside === true, insideViewport);

    // Items still readable (1px+ height, in viewport).
    const itemsRendered = await page.evaluate(() => {
      const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
      const items = panel ? Array.from(panel.querySelectorAll('[role="menuitem"]')) : [];
      return items.map((it) => {
        const r = it.getBoundingClientRect();
        return {
          text: it.textContent.trim(),
          inViewport: r.bottom > 0 && r.top < window.innerHeight && r.right > 0 && r.left < window.innerWidth,
          hasSize: r.width > 0 && r.height > 0,
        };
      });
    });
    record("split-screen-4-items-rendered", itemsRendered.length === 4, { count: itemsRendered.length });
    for (const it of itemsRendered) {
      record(`split-screen-item-visible:${it.text}`, it.inViewport === true && it.hasSize === true, it);
    }

    await ctx.close();
  }

  // === Scenario 2: relaxed (viewport 1600×1200) ===
  // 1600 > 1000 → data-layout-mode="relaxed". The split-screen
  // rule does NOT match. The templ default min-w-[14rem] wins.
  // This is the regression-net corollary: slice 2 only changes
  // sizing under split-screen; the wide-screen layout stays
  // exactly as it was pre-#288.
  console.log("\nScenario 2: relaxed viewport (1600×1200)");
  {
    const ctx = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
    const page = await ctx.newPage();
    page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

    await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
    await wait(1000);

    const mode = await page.evaluate(() => document.documentElement.getAttribute("data-layout-mode"));
    record("layout-mode-is-relaxed-at-1600px", mode === "relaxed", { mode });

    await page.locator("[data-foldout-trigger='layout.share.menu']").first().click({ force: true });
    await wait(300);

    const styles = await page.evaluate(() => {
      const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
      if (!(panel instanceof HTMLElement)) return null;
      const cs = getComputedStyle(panel);
      return {
        minWidth: cs.minWidth,
        width: cs.width,
        maxWidth: cs.maxWidth,
      };
    });
    record("relaxed-panel-rendered", styles !== null, styles);
    if (styles) {
      // 14rem floor: the templ default should still apply.
      // 14rem on a 16px root = 224px. Browsers may report
      // min-width in px.
      const minPx = parseFloat(styles.minWidth);
      record("relaxed-min-width-is-14rem-floor", Math.abs(minPx - 224) < 1, { minWidth: styles.minWidth });
      // width: 14rem templ class has no explicit width, so the
      // ul collapses to its min-width when its content fits
      // inside it. Wide viewport means 4 single-word menuitems
      // ("Export" / "Import" / "Share Queue" / "Sync") fit
      // comfortably in 14rem, so computed width == min-width.
      // The contract under test is: the 14rem floor applies, NOT
      // that width is literally "auto" (a no-op assertion since
      // "auto" + min-width with fitting content == min-width).
      const widthPx = parseFloat(styles.width);
      record("relaxed-width-collapses-to-floor", Math.abs(widthPx - 224) < 1, { width: styles.width, minWidth: styles.minWidth });
      // Slice-1 viewport cap still on (1568px at viewport 1600).
      const maxPx = parseFloat(styles.maxWidth);
      record("relaxed-max-width-capped-to-viewport", Math.abs(maxPx - (1600 - 32)) < 1, { maxWidth: styles.maxWidth });
    }

    await ctx.close();
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
