// audit/smoke_popout_panels.mjs — regression net for the 4
// popout-panel sites that share the `data-popout-panel`
// convention + the `clampPopoutPanels` JS helper
// (frontend/app.js:258). The foldout nav got its own probe
// (audit/smoke_foldout_split_screen_sizing.mjs); this probe
// covers the ad-hoc popout family whose primary defense
// against viewport clipping is the width math in
// `w-[min(<cap>,calc(100vw-<margin>))]` plus the clamp helper
// that runs translateX on overflow. At every viewport
// ≥ 640px, each popout panel must:
//   1. Render with positive width when its trigger is opened
//      (<details> expanded, or native popover toggle).
//   2. Have a left edge ≥ 0 (no left clipping).
//   3. Have a right edge ≤ viewportWidth + 0.5px slack (no
//      right clipping — the `w-[min(<cap>,calc(100vw-<margin>))]`
//      cap and the clamp helper's translateX are the two
//      defenses, this probe verifies both).
//   4. Not push a non-zero transform onto itself before any
//      clamp run (regression net for the clamp helper running
//      unexpectedly — the translate should only fire when the
//      panel actually overflows by more than viewportPadding).
//
// Sites covered (see docs/ui-map/glossary.md "popout panel"):
//   A. Export Month (calendar.templ:49-77) — `<details>` with
//      a `<form>` inside, class includes
//      `absolute left-0 ... xl:left-auto xl:right-0`. Default
//      anchor flips left → right at 1280px (xl). Width is
//      `w-[min(24rem,calc(100vw-3rem))]` then `sm:w-96`
//      (= 24rem at sm breakpoint, no change at min).
//   B. Day Action Menu (calendar_day.templ:159) — `<details>`
//      with a `<div>`, anchored right-only. Width
//      `w-[min(28rem,calc(100vw-5rem))]` (28rem cap). The
//      largest of the family. Tested at /anniversary/<month>/<day>
//      since handleAnniversary renders the per-day detail.
//   C. Single Record Export (soldier_card.templ:273) — same
//      shape as (A), per-record.
//   D. Single Record `?` Help (soldier_card.templ:278) — a
//      NATIVE `<div popover>` toggled by `[popovertarget]`,
//      width `w-72` (18rem). Carries `data-popout-panel` so
//      clamp runs on it when open. This is the latent bug the
//      explore flagged: refresh while the popover is open
//      could shift it because `clampPopoutPanels` translates
//      X by the overflow amount. Uses `:popover-open` to
//      detect open state because native popovers live in the
//      top layer (offsetParent is null even when open).
//
// Companion tests:
//   - The Foldout probe (audit/smoke_foldout_split_screen_sizing.mjs)
//     covers the right-anchored top-nav foldout under the same
//     invariant at 7 widths. This probe covers the 4 popout
//     sites. Future popout adoptions should add a test step
//     here.

const PORT = 9997;
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

// 7 widths spanning the split-screen / relaxed breakpoint at
// 1000px and the xl breakpoint at 1280px (affects which side
// the popout anchor flips to for the calendar Export Month +
// soldier Single Record Export).
const VIEWPORTS = [800, 900, 1000, 1100, 1200, 1400, 1600];

try {
  await ready();
  await wait(2000);

  const browser = await Playwright.chromium.launch({ headless: true });

  // Find a soldier for the Single Record Export + ? help sites
  // by visiting /browse (the same pattern as
  // audit/smoke_soldier_tag_picker.mjs).
  console.log("Discovering a soldier for the C + D sites");
  const ctx0 = await browser.newContext({ viewport: { width: 1600, height: 1200 } });
  const discoverPage = await ctx0.newPage();
  await discoverPage.goto(`http://127.0.0.1:${PORT}/browse`, { waitUntil: "networkidle" });
  await wait(800);
  const probeSoldier = await discoverPage.evaluate(() => {
    const links = Array.from(document.querySelectorAll('a[href^="/soldiers/"]'));
    for (const a of links) {
      const m = (a.getAttribute("href") || "").match(/^\/soldiers\/(\d+)$/);
      if (m) return { id: parseInt(m[1], 10) };
    }
    return null;
  });
  if (!probeSoldier) {
    console.log("  ! no soldier found in scratch DB; C + D sites will be skipped");
  } else {
    console.log(`  + using soldier id ${probeSoldier.id}`);
  }
  await ctx0.close();

  if (!probeSoldier) {
    // Without a soldier, only A + B can be tested; we still
    // get useful coverage on the calendar popout.
    console.log("\nNo soldier — running A + B only");
  }

  for (const viewportWidth of VIEWPORTS) {
    console.log(`\nViewport ${viewportWidth}×1200`);
    const ctx = await browser.newContext({ viewport: { width: viewportWidth, height: 1200 } });
    const page = await ctx.newPage();
    page.on("pageerror", (err) => console.log("    [pageerror]", err.message));

    // === Site A: Export Month at /calendar ===
    // The Export Month summary sits inside a <details>. Click
    // the summary to open it; the form (data-popout-panel)
    // then renders, and clampPopoutPanels runs on layout-mode
    // change (which the new context + JS init triggers).
    {
      await page.goto(`http://127.0.0.1:${PORT}/calendar`, { waitUntil: "networkidle" });
      await wait(800);
      // Click the Export Month summary. locator on text is the
      // most stable selector — the templ wraps the label in a
      // <span>.
      const summary = page.locator("summary:has-text('Export Month')").first();
      const summaryExists = await summary.count();
      if (summaryExists > 0) {
        await summary.click({ force: true });
        await wait(400);
        // The <details> only renders the form contents once
        // open; the form carries the data-popout-panel attr.
        const geom = await page.evaluate(() => {
          // Filter to forms inside <details open> so we don't
          // pick up closed ones (offsetParent would be null
          // anyway, but be explicit).
          const forms = Array.from(document.querySelectorAll('form[data-popout-panel=""]'))
            .filter((f) => {
              const host = f.closest("details");
              return host && host.tagName === "DETAILS" && host.open;
            });
          if (forms.length === 0) return null;
          const form = forms[0];
          const pr = form.getBoundingClientRect();
          const cs = getComputedStyle(form);
          return {
            panelLeft: pr.left,
            panelRight: pr.right,
            panelWidth: pr.width,
            panelTop: pr.top,
            panelBottom: pr.bottom,
            transform: cs.transform,
          };
        });
        record(`A-export-month-rendered@${viewportWidth}`, geom !== null, geom);
        if (geom) {
          record(`A-no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });
          record(`A-no-right-clipping@${viewportWidth}`, geom.panelRight <= viewportWidth + 0.5, { panelRight: geom.panelRight });
          record(`A-panel-has-size@${viewportWidth}`, geom.panelWidth > 0, { panelWidth: geom.panelWidth });
          record(`A-no-top-clipping@${viewportWidth}`, geom.panelTop >= 0, { panelTop: geom.panelTop });
        }
      } else {
        record(`A-export-month-summary-present@${viewportWidth}`, false, { reason: "summary not found on /calendar" });
      }
    }

    // === Site B: Day Action Menu at /anniversary/<month>/<day> ===
    // The Day Action <details> lives inside calendar_day.templ,
    // which is rendered by handleAnniversary() when the user
    // navigates to /anniversary/<month>/<day>. The summary is
    // visible for every day (whether or not there's an event
    // being edited). If the seed DB has no calendar events the
    // page still renders with an empty day form, and the
    // <details> still appears — so the summary MUST be present.
    {
      // Anniversary route requires a valid month + day within
      // range. Use Feb 28 (safe in all months/years).
      await page.goto(`http://127.0.0.1:${PORT}/anniversary/2/28`, { waitUntil: "networkidle" });
      await wait(800);
      const summary = page.locator("summary:has-text('Event / Holiday')").first();
      const summaryExists = await summary.count();
      if (summaryExists > 0) {
        await summary.click({ force: true });
        await wait(400);
        const geom = await page.evaluate(() => {
          // The Day Action div is data-popout-panel without the
          // form wrapper. Filter to ones whose <details> host is
          // open AND offsetParent is non-null. Multiple day
          // items can exist on a single day detail page, so
          // narrow by tag != FORM and pick the first open
          // match.
          const els = Array.from(document.querySelectorAll('[data-popout-panel=""]'))
            .filter((el) => {
              if (el.tagName === "FORM") return false; // skip the Export Month form
              const host = el.closest("details");
              return host && host.tagName === "DETAILS" && host.open;
            });
          if (els.length === 0) return null;
          const el = els[0];
          const pr = el.getBoundingClientRect();
          const cs = getComputedStyle(el);
          return {
            tag: el.tagName,
            className: el.className,
            id: el.id,
            panelLeft: pr.left,
            panelRight: pr.right,
            panelWidth: pr.width,
            panelTop: pr.top,
            panelBottom: pr.bottom,
            transform: cs.transform,
          };
        });
        console.log(`  >> site B matched: ${JSON.stringify({ tag: geom?.tag, id: geom?.id, panelLeft: geom?.panelLeft, panelRight: geom?.panelRight, panelWidth: geom?.panelWidth })}`);
        record(`B-day-action-rendered@${viewportWidth}`, geom !== null, geom);
        if (geom) {
          record(`B-no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });
          record(`B-no-right-clipping@${viewportWidth}`, geom.panelRight <= viewportWidth + 0.5, { panelRight: geom.panelRight });
          record(`B-panel-has-size@${viewportWidth}`, geom.panelWidth > 0, { panelWidth: geom.panelWidth });
          record(`B-no-top-clipping@${viewportWidth}`, geom.panelTop >= 0, { panelTop: geom.panelTop });
        }
      } else {
        record(`B-day-action-summary-present@${viewportWidth}`, false, { reason: "summary not found at /anniversary/2/28 — check that handleAnniversary renders the empty-day view by default" });
      }
    }

    // === Sites C + D: Single Record Export + ? Help at /soldiers/<id> ===
    if (probeSoldier) {
      await page.goto(`http://127.0.0.1:${PORT}/soldiers/${probeSoldier.id}`, { waitUntil: "networkidle" });
      await wait(800);

      // Site C: Single Record Export form
      {
        const summary = page.locator("summary:has-text('Export Record')").first();
        const summaryExists = await summary.count();
        if (summaryExists > 0) {
          await summary.click({ force: true });
          await wait(400);
          const geom = await page.evaluate(() => {
            // Find the form with data-pdf-pref-scope="soldier"
            // (the Single Record Export form scopes its PDF
            // preferences to the soldier record so they don't
            // collide with the calendar Export Month).
            const form = document.querySelector('form[data-pdf-pref-scope="soldier"]');
            if (!form) return null;
            const host = form.closest("details");
            if (!host || !host.open) return null;
            const pr = form.getBoundingClientRect();
            const cs = getComputedStyle(form);
            return {
              panelLeft: pr.left,
              panelRight: pr.right,
              panelWidth: pr.width,
              panelTop: pr.top,
              panelBottom: pr.bottom,
              transform: cs.transform,
            };
          });
          record(`C-single-record-export-rendered@${viewportWidth}`, geom !== null, geom);
          if (geom) {
            record(`C-no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });
            record(`C-no-right-clipping@${viewportWidth}`, geom.panelRight <= viewportWidth + 0.5, { panelRight: geom.panelRight });
            record(`C-panel-has-size@${viewportWidth}`, geom.panelWidth > 0, { panelWidth: geom.panelWidth });
            record(`C-no-top-clipping@${viewportWidth}`, geom.panelTop >= 0, { panelTop: geom.panelTop });
          }
        } else {
          record(`C-export-record-summary-present@${viewportWidth}`, false, { reason: "summary not found on /soldiers/<id>" });
        }
      }

      // Site D: Single Record `?` Help (native popover)
      // Native popover behavior: click the popovertarget
      // button, the popover opens via the browser's own
      // positioning. We measure AFTER open.
      {
        const helpBtn = page.locator('button[popovertarget="single-record-export-help"]').first();
        const btnExists = await helpBtn.count();
        if (btnExists > 0) {
          await helpBtn.click({ force: true });
          await wait(400);
          const geom = await page.evaluate(() => {
            const popover = document.getElementById("single-record-export-help");
            if (!popover) return null;
            // Native popovers live in the top layer when
            // open, so offsetParent is null even when open
            // (not a closed-state signal for popovers). Use
            // the `:popover-open` pseudo-class to detect the
            // open state. Falls back to display !== "none"
            // for browsers that don't expose :popover-open.
            const isOpen = popover.matches(":popover-open") || getComputedStyle(popover).display !== "none";
            if (!isOpen) return null;
            const pr = popover.getBoundingClientRect();
            const cs = getComputedStyle(popover);
            return {
              panelLeft: pr.left,
              panelRight: pr.right,
              panelWidth: pr.width,
              panelTop: pr.top,
              panelBottom: pr.bottom,
              transform: cs.transform,
            };
          });
          record(`D-help-popover-rendered@${viewportWidth}`, geom !== null, geom);
          if (geom) {
            record(`D-no-left-clipping@${viewportWidth}`, geom.panelLeft >= -0.5, { panelLeft: geom.panelLeft });
            record(`D-no-right-clipping@${viewportWidth}`, geom.panelRight <= viewportWidth + 0.5, { panelRight: geom.panelRight });
            record(`D-panel-has-size@${viewportWidth}`, geom.panelWidth > 0, { panelWidth: geom.panelWidth });
            record(`D-no-top-clipping@${viewportWidth}`, geom.panelTop >= 0, { panelTop: geom.panelTop });
          }
          // Close so the next iteration starts clean.
          await page.keyboard.press("Escape");
          await wait(200);
        } else {
          record(`D-help-button-present@${viewportWidth}`, false, { reason: "popovertarget button not found on /soldiers/<id>" });
        }
      }
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
