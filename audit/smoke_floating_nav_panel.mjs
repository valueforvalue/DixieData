// audit/smoke_floating_nav_panel.mjs — regression net for
// the floating nav panel (.floating-nav-panel) that the
// floating-dock Menu button toggles. Sibling probe to
// audit/smoke_foldout_split_screen_sizing.mjs (top-nav foldout)
// and audit/smoke_popout_panels.mjs (popout-panel family).
//
// The floating nav panel is the "menu" button in the
// floating dock at the bottom of every page
// (internal/templates/layout.templ:131). When open it
// renders as a fixed-position panel anchored to
// viewport bottom-right with:
//   right-4 sm:right-6                  (small right margin)
//   w-[24rem] max-w-[calc(100vw-2rem)] min-w-[15rem]
// bottom-[5.5rem]                       (above the floating dock)
// z-50
//
// The panel has no ARIA decorations (`role="dialog"`,
// `aria-modal`, `aria-labelledby`, `aria-haspopup`) and is
// toggled via an inline `onclick` string in the templ —
// when the dock body is re-rendered by htmx (jobs poll,
// calendar refresh) the inline handler can be lost if the
// dock itself is re-rendered. The `initializeFloatingNav`
// helper in frontend/app.js installs a document-level
// outside-click handler that closes the panel. The
// contract under test at every viewport ≥ 640px is:
//   (a) panel-left  ≥ 0
//   (b) panel-right ≤ viewportWidth
//   (c) panel-width stays at 24rem (the templ default; the
//       max-w-[calc(100vw-2rem)] cap is meant to bound, not
//       shrink).
//   (d) panel-bottom stays inside the viewport (above the
//       dock).
//   (e) All 9 nav links + the layout-mode picker are
//       rendered inside the panel and reachable.
//
// At < 640px the templ default still holds — the min-w
// 15rem + right-4 means the panel could clip left if
// the viewport drops below ~256px, which is below the
// app's actual minimum (Wails desktop). Documented but
// not asserted.
//
// Companion:
//   - audit/smoke_foldout_split_screen_sizing.mjs
//     (top-nav foldout, the more-trafficked popout)
//   - audit/smoke_popout_panels.mjs
//     (the 4 sites sharing `data-popout-panel`)

const PORT = 9999;
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

// 7 widths; skip below 640 since the app's actual minimum is
// the Wails desktop frame which always exceeds 800.
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

    // Land on /calendar; the floating-dock + nav panel live in
    // the Layout shell, so any page works. Click the Menu
    // button (data-floating-nav-toggle) to open the panel.
    await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
    await wait(800);
    await page.locator('[data-floating-nav-toggle]').first().click({ force: true });
    await wait(400);

    const geom = await page.evaluate(() => {
      const panel = document.querySelector('[data-floating-nav-panel]');
      if (!(panel instanceof HTMLElement)) return null;
      if (panel.classList.contains("hidden")) return null;
      const pr = panel.getBoundingClientRect();
      const cs = getComputedStyle(panel);
      // Count nav links (the layout-mode picker is also inside).
      const links = panel.querySelectorAll('a');
      return {
        panelLeft: pr.left,
        panelRight: pr.right,
        panelWidth: pr.width,
        panelTop: pr.top,
        panelBottom: pr.bottom,
        computedWidth: cs.width,
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
        navLinkCount: links.length,
      };
    });
    record(`panel-rendered@${viewportWidth}`, geom !== null, geom);
    if (!geom) { await ctx.close(); continue; }

    // (a) panel-left ≥ 0
    record(`no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });
    // (b) panel-right ≤ viewportWidth
    record(`no-right-clipping@${viewportWidth}`, geom.panelRight <= geom.viewportWidth + 0.5, { panelRight: geom.panelRight, viewportWidth: geom.viewportWidth });
    // (c) panel-width is the templ default 24rem (384px) at
    // every viewport ≥ 640 (the max-w cap only bites if the
    // viewport itself is narrower than the cap, which is not
    // the case here).
    record(`panel-width-is-24rem@${viewportWidth}`, Math.abs(geom.panelWidth - 384) < 1, { panelWidth: geom.panelWidth, computedWidth: geom.computedWidth });
    // (d) panel-bottom is inside viewport (above the dock).
    record(`panel-bottom-inside-viewport@${viewportWidth}`, geom.panelBottom <= geom.viewportHeight, { panelBottom: geom.panelBottom, viewportHeight: geom.viewportHeight });
    // (e) all 9 nav links reachable. The layout-mode picker is
    // a <div> wrapper, not a link, so 9 is the right count.
    record(`nav-links-rendered@${viewportWidth}`, geom.navLinkCount === 9, { navLinkCount: geom.navLinkCount });

    // Close so the next iteration starts clean.
    await page.evaluate(() => document.querySelector('[data-floating-nav-panel]')?.classList.add('hidden'));
    await wait(200);
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
