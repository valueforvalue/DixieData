// smoke_about.mjs -- issue #585 slice 6 regression net.
//
// Visits /about against a live dixiedata-web server (started
// by audit/run.mjs) and confirms the page renders every locked-
// decision surface. Tied to the data-* attrs the templ partial
// renders:
//
//   data-page-about                  page anchor
//   data-about-identity              #identity section
//   data-about-license               #license section
//   data-about-history               #history section
//   data-about-in-page-nav           on-this-page nav strip
//   data-about-app / -version /      identity fields
//   -codename / -schema / -branch
//   / -commit / -built
//   data-about-license-summary       license summary block
//   data-about-credits               credits section
//   data-about-credit="<name>"       one credit per linked dep
//   data-about-history-empty         empty-state copy (dev binary)
//   data-about-release="<version>"   one card per release
//   data-about-release-expand="<v>"  per-release expand toggle
//   data-about-history-collapse      show-all toggle (10+ releases)
//
// Read-only against /about. The seed-data fixture populates
// enough primary entries that the metrics section renders too,
// but this probe targets /about specifically; it does not
// exercise /inventory.
//
// Run via: node audit/smoke_about.mjs (assumes the
// dixiedata-web server is up at $BASE_URL, default
// http://127.0.0.1:8901).

import { chromium } from 'playwright';

const BASE = process.env.DIXIEDATA_BASE || 'http://127.0.0.1:8901';

async function expect(cond, msg, details) {
  if (!cond) {
    throw new Error(`probe failed: ${msg}\n  details: ${JSON.stringify(details)}`);
  }
  console.log(`  ok: ${msg}`);
}

async function populatedProbe(page) {
  await page.goto(BASE + '/about');
  await page.waitForSelector('[data-page-about]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const pageRoot = document.querySelector('[data-page-about]');
    const identity = document.querySelector('[data-about-identity]');
    const license = document.querySelector('[data-about-license]');
    const history = document.querySelector('[data-about-history]');
    const inPageNav = document.querySelector('[data-about-in-page-nav]');
    const identityFields = {
      app: document.querySelector('[data-about-app]')?.textContent || '',
      version: document.querySelector('[data-about-version]')?.textContent || '',
      codename: document.querySelector('[data-about-codename]')?.textContent || '',
      schema: document.querySelector('[data-about-schema]')?.textContent || '',
    };
    const credits = Array.from(
      document.querySelectorAll('[data-about-credit]'),
    ).map((el) => el.getAttribute('data-about-credit')).sort();
    const releases = Array.from(
      document.querySelectorAll('[data-about-release]'),
    ).map((el) => el.getAttribute('data-about-release'));
    const collapse = document.querySelector('[data-about-history-collapse]');
    const empty = document.querySelector('[data-about-history-empty]');
    return {
      pageRendered: !!pageRoot,
      identityRendered: !!identity,
      licenseRendered: !!license,
      historyRendered: !!history,
      inPageNavRendered: !!inPageNav,
      identityFields,
      credits,
      releaseCount: releases.length,
      releases,
      collapseRendered: !!collapse,
      emptyRendered: !!empty,
    };
  });
  await expect(state.pageRendered, 'about page renders', state);
  await expect(state.identityRendered, 'identity section renders', state);
  await expect(state.licenseRendered, 'license section renders', state);
  await expect(state.historyRendered, 'history section renders', state);
  await expect(state.inPageNavRendered, 'in-page nav strip renders', state);
  await expect(
    state.identityFields.app === 'DixieData',
    `app name is "DixieData" (got ${state.identityFields.app})`,
    state,
  );
  await expect(
    state.identityFields.version.length > 0,
    `version is non-empty (got ${state.identityFields.version})`,
    state,
  );
  await expect(
    state.identityFields.codename.length > 0,
    `codename is non-empty (got ${state.identityFields.codename})`,
    state,
  );
  await expect(
    state.identityFields.schema.length > 0,
    `schema is non-empty (got ${state.identityFields.schema})`,
    state,
  );
  // Credits: the locked 6 minimal entries.
  const wantCredits = ['go-stdlib', 'htmx', 'pdfium', 'playwright', 'tailwindcss', 'typst'];
  await expect(
    JSON.stringify(state.credits) === JSON.stringify(wantCredits),
    `credits cover the 6 locked dependencies (got ${JSON.stringify(state.credits)})`,
    state,
  );
  // The history section is either populated (release baked) or
  // shows the empty-state copy (no bake). One of the two must
  // be present, never both.
  await expect(
    state.emptyRendered !== state.releaseCount > 0,
    'history is either empty-state or has releases, not both',
    state,
  );
  if (state.releaseCount > 0) {
    console.log(`  ok: ${state.releaseCount} release(s) rendered`);
    if (state.releaseCount > 10) {
      await expect(
        state.collapseRendered,
        'history collapse toggle renders when 10+ releases are baked',
        state,
      );
    }
  }
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    console.log('about: populated probe');
    await populatedProbe(page);
    console.log('about: all probes pass');
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});