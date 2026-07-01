// Real probe: click the JSON export button and watch what happens.
import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:8765';
const browser = await chromium.launch();
const ctx = await browser.newContext();
const page = await ctx.newPage();

const events = [];
page.on('request', (r) => events.push({ kind: 'req', method: r.method(), url: r.url() }));
page.on('response', (r) => events.push({ kind: 'res', status: r.status(), url: r.url(), headers: r.headers() }));
page.on('framenavigated', (f) => events.push({ kind: 'nav', url: f.url() }));

await page.goto(`${BASE}/share`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(800);

const jsonBtn = page.locator('button[data-action="/export/json"]').first();
const csvBtn = page.locator('button[data-action="/export/csv"]').first();
const icalBtn = page.locator('button[data-action="/export/ical"]').first();
const databaseBtn = page.locator('button[data-action="/export/database-pdf?async=1"]').first();

for (const [label, locator] of [['JSON', jsonBtn], ['CSV', csvBtn], ['iCal', icalBtn], ['Database PDF', databaseBtn]]) {
  console.log(`\n=== Testing ${label} ===`);
  console.log(`  Count: ${await locator.count()}`);
  if ((await locator.count()) > 0) {
    events.length = 0;
    await locator.click({ timeout: 2000 }).catch((e) => console.log('  click error:', e.message));
    await page.waitForTimeout(2500);
    console.log('  URL after click:', page.url());
    console.log('  Events:');
    for (const e of events) {
      if (e.kind === 'res') {
        const redir = e.headers['x-dixiedata-redirect'];
        console.log(`    RES ${e.status} ${e.url}${redir ? ' (X-DixieData-Redirect: ' + redir + ')' : ''}`);
      } else {
        console.log(`    ${e.kind === 'req' ? 'REQ' : 'NAV'} ${e.method || ''} ${e.url}`);
      }
    }
  }
}

await browser.close();
