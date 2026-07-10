// audit/smoke_research_collections.mjs — live regression net for issue #452.
// Boots chromium against dixiedata-web, walks the Research Collections page,
// asserts the page renders (not the "Could not load" error fragment) and
// that the Create Collection form actually creates a collection.
//
// Run after dixiedata-web is up at $BASE_URL:
//   BASE_URL=http://127.0.0.1:8765 node audit/smoke_research_collections.mjs

import { chromium } from 'playwright';

const BASE = process.env.BASE_URL || 'http://127.0.0.1:8765';

let pass = 0;
let fail = 0;
function record(name, ok, details = {}) {
  if (ok) {
    pass++;
    console.log(`  ✓ ${name}`);
  } else {
    fail++;
    console.error(`  ✗ ${name}`, details);
  }
}

// 1. Bare GET renders the hub panel — no "Could not load research collections." text.
async function testBareGetRendersHub(page) {
  await page.goto(`${BASE}/research-collections`, { waitUntil: 'networkidle' });
  const body = await page.locator('body').innerText();
  if (body.includes('Could not load research collections.')) {
    return record('bare-get-renders-hub', false, { reason: 'shows user-facing error fragment' });
  }
  const hubVisible = await page.locator('#panel\\.research-collections\\.hub').count();
  if (hubVisible === 0) {
    return record('bare-get-renders-hub', false, { reason: 'hub panel not rendered' });
  }
  return record('bare-get-renders-hub', true);
}

// 2. Stale ?from=<missing-id> does NOT show the error fragment.
async function testStaleFromIDFallsBack(page) {
  await page.goto(`${BASE}/research-collections?from=999999`, { waitUntil: 'networkidle' });
  const body = await page.locator('body').innerText();
  if (body.includes('Could not load research collections.')) {
    return record('stale-from-id-falls-back', false, { reason: 'user-facing error fragment visible' });
  }
  const hubVisible = await page.locator('#panel\\.research-collections\\.hub').count();
  if (hubVisible === 0) {
    return record('stale-from-id-falls-back', false, { reason: 'hub panel not rendered' });
  }
  return record('stale-from-id-falls-back', true);
}

// 3. Create Collection form: fill, submit, verify POST returned 200 + redirect header was consumed.
async function testCreateCollectionForm(page) {
  await page.goto(`${BASE}/research-collections`, { waitUntil: 'networkidle' });

  const beforeCount = await page.locator('[data-collection-row], #panel\\.research-collections\\.hub tr').count();
  const stamp = `Audit ${Date.now()}`;

  // Fill form
  await page.fill('input[name="name"]', stamp);
  await page.fill('textarea[name="description"]', 'Smoke probe — create collection');

  // Watch network
  let postStatus = null;
  page.on('response', (resp) => {
    if (resp.url().endsWith('/research-collections') && resp.request().method() === 'POST') {
      postStatus = resp.status();
    }
  });

  // Click Create
  await Promise.all([
    page.waitForResponse((r) => r.url().endsWith('/research-collections') && r.request().method() === 'POST', { timeout: 5000 }),
    page.click('button[type="submit"]'),
  ]);

  // Wait for the page to settle on /research-collections after the redirect.
  await page.waitForLoadState('networkidle');

  if (postStatus === 405) {
    return record('create-collection-form', false, { reason: 'POST returned 405 — route not registered', postStatus });
  }
  if (postStatus !== 200) {
    return record('create-collection-form', false, { reason: 'POST did not return 200', postStatus });
  }
  // Verify the new collection row is visible in the hub
  const bodyAfter = await page.locator('body').innerText();
  if (!bodyAfter.includes(stamp)) {
    return record('create-collection-form', false, { reason: `expected name ${stamp} in DOM after submit` });
  }
  return record('create-collection-form', true, { postStatus, stamped: stamp });
}

// 4. Top-nav → Research & Review → "Research Collections" link navigates to the hub.
async function testTopNavLink(page) {
  await page.goto(`${BASE}/`, { waitUntil: 'networkidle' });
  const link = page.locator('[data-research-menu-research-collections]');
  if ((await link.count()) === 0) {
    return record('topnav-research-collections-link', false, { reason: 'menu link missing' });
  }
  // Foldout is closed by default; open the parent trigger first.
  const trigger = page.locator('[data-foldout-trigger="layout.research.menu"]');
  if (await trigger.count() > 0) {
    await trigger.first().click();
    await page.waitForTimeout(150);
  }
  await link.first().click();
  await page.waitForURL(/\/research-collections/, { timeout: 5000 });
  const body = await page.locator('body').innerText();
  if (body.includes('Could not load research collections.')) {
    return record('topnav-research-collections-link', false, { reason: 'landed on error fragment' });
  }
  const hub = await page.locator('#panel\\.research-collections\\.hub').count();
  if (hub === 0) {
    return record('topnav-research-collections-link', false, { reason: 'hub not rendered after navigation' });
  }
  return record('topnav-research-collections-link', true);
}

(async () => {
  const browser = await chromium.launch();
  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  page.on('pageerror', (err) => console.error('[pageerror]', err.message));
  page.on('console', (msg) => {
    if (msg.type() === 'error') console.error('[console.error]', msg.text());
  });

  try {
    await testBareGetRendersHub(page);
    await testStaleFromIDFallsBack(page);
    await testCreateCollectionForm(page);
    await testTopNavLink(page);
  } finally {
    await ctx.close();
    await browser.close();
  }

  console.log('============================================================');
  console.log(`SMOKE results: ${pass} pass, ${fail} fail`);
  console.log('============================================================');
  process.exit(fail === 0 ? 0 : 1);
})();
