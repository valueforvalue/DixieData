// audit/smoke_foldout_split_screen_sizing.mjs — regression net
// for issue #288 (viewport-cap fix) AND its followup (the
// slice-2 regression that swapped "no overflow on any size" for
// "panel hugs right edge of trigger at 14rem, but the slice-2
// layout-mode rule stretched the panel to calc(100vw-2rem) and
// pushed the left edge off-screen at split-screen viewports).
//
// The unified contract the foldout panel honors at every
// viewport ≥ 640px:
//   1. The panel is anchored to the trigger's right edge
//      (class: absolute right-0 top-full).
//   2. The panel width collapses to the templ class's
//      min-w-[14rem] floor (224px on a 16px root) because the
//      4-item menu fits comfortably in 14rem.
//   3. The slice-1 max-w-[calc(100vw-2rem)] cap bounds the
//      width from above so a future menu item with a long
//      label can never overflow the viewport.
//   4. The panel's left edge sits inside the viewport at
//      (triggerRight - 224px). The trigger is never close to
//      the left edge of the page (top-nav lives inside
//      .app-shell with padding); the panel always lands
//      safely inside the viewport.
//
// At ≤ 640px the legacy media query re-centers the panel
// (left: 0; right: 0). Tested separately by the screenshot
// diff harness; not asserted in this probe.
//
// The probe sweeps 7 viewport widths spanning the split-screen
// / relaxed boundary (1000px) and asserts:
//   (a) data-layout-mode flips correctly at the 1000px
//       breakpoint (regression net for the JS auto-detect);
//   (b) the panel's left edge is ≥ 0 (no left clipping);
//   (c) the panel's right edge is ≤ viewportWidth (no right
//       clipping — the slice-1 cap firing);
//   (d) the panel width stays at the 14rem floor (regression
//       net against the reverted slice-2 rule that stretched
//       it to calc(100vw-2rem));
//   (e) all 4 menuitems render + remain in-viewport.
//
// Companion tests:
//   - internal/templates/components/foldout_test.go
//     (TestFoldout_PanelResponsiveSizing) locks slice 1's
//     templ-rendered class tokens (min-w-[14rem] +
//     max-w-[calc(100vw-2rem)]).
//   - This probe locks the user-visible behavior end-to-end.

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

// 7 widths spanning the split-screen / relaxed boundary at
// 1000px. Drop the 640-equivalent — the legacy @media query
// re-centers the panel there and is orthogonal to this probe.
const VIEWPORTS = [800, 900, 1000, 1100, 1200, 1400, 1600];

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });

  for (const viewportWidth of VIEWPORTS) {
    console.log(`\nViewport ${viewportWidth}×1200`);
    const ctx = await browser.newContext({ viewport: { width: viewportWidth, height: 1200 } });
    const page = await ctx.newPage();
    page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

    await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
    await wait(800);

    // (a) layout-mode auto-detect at this width.
    const expectedMode = viewportWidth <= 1000 ? "split-screen" : "relaxed";
    const mode = await page.evaluate(() => document.documentElement.getAttribute("data-layout-mode"));
    record(`layout-mode-correct@${viewportWidth}`, mode === expectedMode, { mode, expected: expectedMode });

    // Open the panel via the real click path (page.locator).
    // Synthetic dispatchEvent would bypass the bubble-phase
    // ordering that first-click regressions (#283) were
    // bitten by; we want the same code path the user hits.
    await page.locator("[data-foldout-trigger='layout.share.menu']").first().click({ force: true });
    await wait(300);

    // Measured geometry: panel edges, width, panel min-width
    // (computed), mode.
    const geom = await page.evaluate(() => {
      const panel = document.querySelector("[data-foldout-panel='layout.share.menu']");
      if (!(panel instanceof HTMLElement)) return null;
      const cs = getComputedStyle(panel);
      const pr = panel.getBoundingClientRect();
      const items = Array.from(panel.querySelectorAll('[role="menuitem"]'));
      return {
        panelLeft: pr.left,
        panelRight: pr.right,
        panelWidth: pr.width,
        panelMinWidth: cs.minWidth,
        panelMaxWidth: cs.maxWidth,
        viewportWidth: window.innerWidth,
        itemCount: items.length,
        items: items.map((it) => {
          const r = it.getBoundingClientRect();
          return {
            text: it.textContent.trim(),
            inViewport: r.bottom > 0 && r.top < window.innerHeight && r.right > 0 && r.left < window.innerWidth,
            hasSize: r.width > 0 && r.height > 0,
          };
        }),
      };
    });
    record(`panel-rendered@${viewportWidth}`, geom !== null, geom);
    if (!geom) { await ctx.close(); continue; }

    // (b) no left clipping. The panel-left-edge ≥ 0
    // invariant — this is the regression net for the slice-2
    // followup where panelLeft went to -240 at 900px.
    record(`no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });

    // (c) no right clipping. The slice-1 max-w-[calc(100vw-2rem)]
    // cap must keep panelRight inside the viewport. 0.5px
    // slack for sub-pixel rounding.
    record(`no-right-clipping@${viewportWidth}`, geom.panelRight <= geom.viewportWidth + 0.5, { panelRight: geom.panelRight, viewportWidth: geom.viewportWidth });

    // (d) panel width stays at the 14rem floor (regression
    // net against the reverted slice-2 layout-mode rule that
    // stretched it to calc(100vw-2rem) at split-screen).
    // 14rem on a 16px root = 224px. panelMinWidth is the
    // computed min-width (e.g. "224px"); panelWidth is the
    // actual rendered width. Both should equal 224px.
    const minPx = parseFloat(geom.panelMinWidth);
    const widPx = geom.panelWidth;
    record(`panel-width-collapses-to-floor@${viewportWidth}`, Math.abs(minPx - 224) < 1 && Math.abs(widPx - 224) < 1, { panelMinWidth: geom.panelMinWidth, panelWidth: widPx });

    // (e) all 4 menuitems render + remain in-viewport.
    record(`4-items-rendered@${viewportWidth}`, geom.itemCount === 4, { itemCount: geom.itemCount });
    for (const it of geom.items) {
      record(`item-visible@${viewportWidth}:${it.text}`, it.inViewport && it.hasSize, it);
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
