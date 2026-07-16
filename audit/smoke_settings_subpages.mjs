// smoke_settings_subpages.mjs -- issue #584 slice 5.
//
// Visits each /settings/<section> sub-page against a live
// dixiedata-web server and confirms:
//   1. The sub-page returns 200 (no auth/redirect).
//   2. The expected page ID + breadcrumb + panel render.
//   3. The Settings mega-menu trigger in the top nav points
//      at the panel (the menu items carry the data-marker
//      attrs the templ partial renders).
//
// Read-only against /settings. The probe does not POST
// anything; it just confirms every sub-page renders + the
// mega-menu trigger is wired.

import { chromium } from 'playwright';

const BASE = process.env.DIXIEDATA_BASE || 'http://127.0.0.1:8901';

async function expect(cond, msg, details) {
  if (!cond) {
    throw new Error(`probe failed: ${msg}\n  details: ${JSON.stringify(details)}`);
  }
  console.log(`  ok: ${msg}`);
}

const SUBPAGES = [
  { section: 'appearance', pageId: 'page.settings.appearance', marker: 'settings-appearance' },
  { section: 'updates', pageId: 'page.settings.updates', marker: 'settings-updates' },
  { section: 'maintenance', pageId: 'page.settings.maintenance', marker: 'settings-maintenance' },
  { section: 'data', pageId: 'page.settings.data', marker: 'settings-data' },
  // Issue #599: build destination removed; build info now
  // lives on /about (the App identity section).
  { section: 'diagnostics', pageId: 'page.settings.diagnostics', marker: 'settings-diagnostics' },
];

async function subPageProbe(page, sp) {
  await page.goto(`${BASE}/settings/${sp.section}`);
  await page.waitForSelector(`#${sp.pageId}`, { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(({ pageId, marker }) => {
    const pageRoot = document.getElementById(pageId);
    const breadcrumb = document.querySelector('[data-settings-breadcrumb]');
    const menuItem = document.querySelector(`[data-marker="${marker}"]`);
    return {
      pageRendered: !!pageRoot,
      breadcrumbRendered: !!breadcrumb,
      menuItemRendered: !!menuItem,
      menuItemHref: menuItem?.getAttribute('href') || '',
    };
  }, { pageId: sp.pageId, marker: sp.marker });
  await expect(state.pageRendered, `${sp.section}: page renders (${sp.pageId})`, state);
  await expect(state.breadcrumbRendered, `${sp.section}: breadcrumb renders`, state);
  await expect(state.menuItemRendered, `${sp.section}: mega-menu item renders (${sp.marker})`, state);
  await expect(
    state.menuItemHref === `/settings/${sp.section}`,
    `${sp.section}: mega-menu item points at /settings/${sp.section} (got ${state.menuItemHref})`,
    state,
  );
}

async function indexProbe(page) {
  await page.goto(`${BASE}/settings`);
  await page.waitForSelector('[data-settings-index]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const index = document.querySelector('[data-settings-index]');
    const targets = Array.from(
      document.querySelectorAll('[data-settings-index-target]'),
    ).map((el) => el.getAttribute('data-settings-index-target')).sort();
    return {
      indexRendered: !!index,
      targets,
    };
  });
  await expect(state.indexRendered, '/settings index renders the 5-destination grid', state);
  const want = ['appearance', 'data', 'diagnostics', 'maintenance', 'updates'];
  await expect(
    JSON.stringify(state.targets) === JSON.stringify(want),
    `/settings index lists all 6 destinations (got ${JSON.stringify(state.targets)})`,
    state,
  );
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    console.log('settings: index probe');
    await indexProbe(page);
    for (const sp of SUBPAGES) {
      console.log(`settings: ${sp.section} sub-page probe`);
      await subPageProbe(page, sp);
    }
    console.log('settings: all probes pass');
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});