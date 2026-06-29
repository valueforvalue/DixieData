// audit/_lib/pension-state-save-probe.mjs — full end-to-end for #75.
// Creates a wife record with pension_state="Louisiana" via direct POST,
// then loads the edit form and verifies the value persisted.

import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:8765';
const browser = await chromium.launch();
const page = await browser.newPage();

// Direct POST — bypasses the busy state on the new-soldier form.
const formBody = new URLSearchParams();
formBody.set('display_id', '');
formBody.set('entry_type', 'wife');
formBody.set('first_name', 'TestWife');
formBody.set('last_name', 'TestHusband');
formBody.set('pension_state', 'Louisiana');
formBody.set('spouse_soldier_id', '1');  // link to the first seeded soldier
formBody.set('existing_needs_review', '0');
formBody.set('existing_review_reason', '');

const resp = await page.request.post(`${BASE}/soldiers`, {
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  data: formBody.toString(),
});
console.log('POST /soldiers status:', resp.status());
const redirect = resp.headers()['x-dixiedata-redirect'];
console.log('X-DixieData-Redirect:', redirect);
const soldierId = redirect ? redirect.split('/').pop() : null;
if (!soldierId) {
  console.log('✗ no redirect; soldier not created');
  process.exit(1);
}

// Load the edit form
await page.goto(`${BASE}/soldiers/${soldierId}/edit`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);
const persisted = await page.locator('input[name="pension_state"]').inputValue();
const firstName = await page.locator('input[name="first_name"]').inputValue();
const lastName = await page.locator('input[name="last_name"]').inputValue();
const entryType = await page.locator('select[name="entry_type"]').inputValue();

console.log('soldier id:', soldierId);
console.log('entry_type on edit:', entryType);
console.log('first_name on edit:', JSON.stringify(firstName));
console.log('last_name on edit:', JSON.stringify(lastName));
console.log('pension_state on edit:', JSON.stringify(persisted));

await browser.close();

const ok = persisted === 'Louisiana' && entryType === 'wife';
console.log(ok ? '✓ pension_state persisted for wife entry type' : '✗ pension_state did not persist or entry_type wrong');
process.exit(ok ? 0 : 1);
