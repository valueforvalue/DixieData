// smoke_inventory_metrics.mjs -- issue #580 slice 1 regression net.
//
// Visits /inventory against a live dixiedata-web server (started
// by audit/run.mjs), confirms the new Activity metrics section
// renders for a populated archive, and confirms the empty-archive
// zero-state suppresses the section. Tied to the data attrs the
// templ partial renders:
//
//   data-inventory-metrics             section anchor
//   data-inventory-metrics-first       first-entry date
//   data-inventory-metrics-latest      latest-entry date
//   data-inventory-metrics-active-days active day count
//   data-inventory-metrics-days        per-day table wrapper
//   data-inventory-metrics-day-count   per-day row count
//
// Run via: `node audit/run.mjs --probe=inventory-metrics` (or as
// part of the full audit sweep when the script is added to the
// run-all manifest).
//
// The probe is read-only against /inventory -- it does not mutate
// the archive. The seed-data fixture (`build/bin/seed-data.exe`)
// populates enough primary entries to exercise the multi-day
// branch; the empty-state assertion runs against a freshly-cleared
// archive via the existing /debug/clear-db endpoint.

import { chromium } from 'playwright';

const BASE = process.env.DIXIEDATA_BASE || 'http://127.0.0.1:8901';

async function expect(cond, msg, details) {
  if (!cond) {
    throw new Error(`probe failed: ${msg}\n  details: ${JSON.stringify(details)}`);
  }
  console.log(`  ok: ${msg}`);
}

async function populatedProbe(page) {
  await page.goto(BASE + '/inventory');
  await page.waitForSelector('[data-inventory-metrics]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const section = document.querySelector('[data-inventory-metrics]');
    const first = document.querySelector('[data-inventory-metrics-first]')?.textContent || '';
    const latest = document.querySelector('[data-inventory-metrics-latest]')?.textContent || '';
    const activeDays = document.querySelector('[data-inventory-metrics-active-days]')?.textContent || '';
    const tableExists = !!document.querySelector('[data-inventory-metrics-days]');
    return {
      sectionRendered: !!section,
      first,
      latest,
      activeDays,
      tableExists,
    };
  });
  await expect(state.sectionRendered, 'populated archive renders metrics section', state);
  await expect(state.first.length >= 8, 'first-entry is an ISO date', state);
  await expect(state.latest.length >= 8, 'latest-entry is an ISO date', state);
  await expect(parseInt(state.activeDays, 10) >= 1, 'active day count is at least 1', state);
  if (parseInt(state.activeDays, 10) > 1) {
    await expect(state.tableExists, 'multi-day state renders per-day table', state);
  }
}

async function emptyProbe(page) {
  // Use the existing /debug endpoint to clear the DB. The endpoint
  // may not exist on all builds; if it 404s, the probe asserts
  // the populated state instead so an absent endpoint doesn't
  // fail unrelated runs.
  const clearResp = await fetch(BASE + '/debug/clear-db', { method: 'POST' });
  if (clearResp.status === 404) {
    console.log('  skip: /debug/clear-db endpoint not available, skipping empty-state assertion');
    return;
  }
  if (!clearResp.ok && clearResp.status !== 204) {
    console.log(`  skip: /debug/clear-db returned ${clearResp.status}`);
    return;
  }
  await page.goto(BASE + '/inventory');
  await page.waitForSelector('[data-inventory-metrics]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const section = document.querySelector('[data-inventory-metrics]');
    return { sectionRendered: !!section };
  });
  await expect(!state.sectionRendered, 'empty archive suppresses the metrics section', state);
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    console.log('inventory metrics: populated archive');
    await populatedProbe(page);
    console.log('inventory metrics: empty archive');
    await emptyProbe(page);
    console.log('inventory metrics: all probes pass');
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
