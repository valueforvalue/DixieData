// audit/probe-article-body-theme.mjs
//
// Regression net for issue #532 slice 3: the article body
// theme + list-page preview excerpt work end-to-end against
// a live server. Pins three contracts:
//
//   1. A markdown table in the article body renders with
//      non-zero border-collapse + cell padding on the
//      rendered DOM (computed style, not source scan).
//   2. An <img> in the article body has max-width: 100%
//      applied (computed style).
//   3. The /articles list page row carries a
//      [data-article-preview] element with the body excerpt
//      when the article has a body.
//
// Requires a running dixiedata-web server at BASE_URL with
// at least one article whose body contains a markdown table,
// image, and 100+ chars of prose. Run via:
//
//   node audit/probe-article-body-theme.mjs
//
// The probe seeds nothing -- it relies on whatever archive
// the running server is configured for. Returns exit 1 on
// any assertion failure.

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

const results = [];
let pass = 0;
let fail = 0;

function record(name, ok, details = {}) {
  results.push({ name, ok, ...details });
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.log(`  ✗ ${name} — ${JSON.stringify(details)}`);
  }
}

async function main() {
  console.log(`BASE_URL: ${BASE}`);
  const browser = await chromium.launch({ headless: true });
  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  // --- Assertion 1 + 2: article detail page renders table + img
  // with the right computed styles ---
  console.log('\n[articles detail] checking table + img computed styles');
  await page.goto(`${BASE}/articles`, { waitUntil: 'networkidle', timeout: 8000 }).catch(() => {});
  const firstArticleLink = await page.locator('a[data-article-row]').first();
  const firstArticleHref = await firstArticleLink.getAttribute('data-article-id');
  if (!firstArticleHref) {
    record('articles list renders at least one row', false, { reason: 'no [data-article-row] found on /articles' });
    await browser.close();
    process.exit(1);
  }
  record('articles list renders at least one row', true);
  await page.goto(`${BASE}${firstArticleHref}`, { waitUntil: 'networkidle', timeout: 8000 }).catch(() => {});

  const tableStyle = await page.evaluate(() => {
    const t = document.querySelector('[data-article-body] table');
    if (!t) return null;
    const cs = getComputedStyle(t);
    const cellCs = t.querySelector('td, th') ? getComputedStyle(t.querySelector('td, th')) : null;
    return {
      borderCollapse: cs.borderCollapse,
      cellPaddingTop: cellCs ? cellCs.paddingTop : null,
      cellPaddingLeft: cellCs ? cellCs.paddingLeft : null,
    };
  });
  if (tableStyle === null) {
    record('article detail body has a markdown table', false, { reason: 'no <table> in [data-article-body] -- is there a table in the seed article?' });
  } else {
    record('article detail body has a markdown table', true);
    const bordersCollapse = tableStyle.borderCollapse === 'collapse';
    record('article table has border-collapse: collapse', bordersCollapse, { got: tableStyle.borderCollapse });
    const cellHasPadding = tableStyle.cellPaddingTop && tableStyle.cellPaddingTop !== '0px';
    record('article table cell has non-zero padding', cellHasPadding, { got: tableStyle });
  }

  const imgStyle = await page.evaluate(() => {
    const img = document.querySelector('[data-article-body] img');
    if (!img) return null;
    const cs = getComputedStyle(img);
    return {
      maxWidth: cs.maxWidth,
      height: cs.height,
      display: cs.display,
    };
  });
  if (imgStyle === null) {
    record('article detail body has an <img>', false, { reason: 'no <img> in [data-article-body] -- is there an image in the seed article?' });
  } else {
    record('article detail body has an <img>', true);
    const hasMaxWidth = imgStyle.maxWidth === '100%' || imgStyle.maxWidth === imgStyle.width;
    record('article <img> has max-width: 100% (or matches width)', hasMaxWidth, { got: imgStyle });
  }

  // --- Assertion 3: list page row carries [data-article-preview] ---
  console.log('\n[articles list] checking row preview element');
  await page.goto(`${BASE}/articles`, { waitUntil: 'networkidle', timeout: 8000 }).catch(() => {});
  const previewCount = await page.locator('a[data-article-row] p[data-article-preview]').count();
  const rowsWithBodyCount = await page.locator('a[data-article-row]').count();
  // The slice-2 contract: rows whose article has a body render
  // the preview; rows with no body do not. The probe asserts at
  // least one preview rendered (so the seed article has a body).
  record('articles list renders at least one [data-article-preview]', previewCount > 0, {
    previewCount,
    rowCount: rowsWithBodyCount,
  });

  await browser.close();

  console.log(`\nResults: ${pass} pass, ${fail} fail`);
  if (fail > 0) {
    process.exit(1);
  }
}

main().catch((err) => {
  console.error('probe crashed:', err);
  process.exit(2);
});