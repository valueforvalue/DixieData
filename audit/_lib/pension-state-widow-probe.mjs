// audit/_lib/pension-state-widow-probe.mjs — one-shot probe for #75.
// Loads /soldiers/new, switches entry_type to wife/widow/soldier/linked_person,
// and asserts the pension_state field is visible (or hidden) per the
// per-entry-type policy.
//
// Per PR #75: pension_state should be visible for soldier + wife + widow,
// hidden for linked_person.

import { chromium } from 'playwright';

const BASE = 'http://127.0.0.1:8765';
const EXPECTED = {
  soldier:      { visible: true,  reason: 'soldier files for own pension' },
  wife:         { visible: true,  reason: 'widow/wife files in her own right' },
  widow:        { visible: true,  reason: 'widow/wife files in her own right' },
  linked_person:{ visible: false, reason: 'linked_person is not a pensioner' },
};

const browser = await chromium.launch();
const page = await browser.newPage();

const findings = [];
await page.goto(`${BASE}/soldiers/new`, { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(500);

const pensionStateField = page.locator('input[name="pension_state"]').first();
const fieldExists = (await pensionStateField.count()) > 0;
if (!fieldExists) {
  console.log('FAIL: input[name="pension_state"] not in DOM at all');
  process.exit(1);
}

// For each entry type, switch and measure the closest <div>'s visibility.
for (const [entryType, expected] of Object.entries(EXPECTED)) {
  await page.selectOption('select[name="entry_type"]', entryType);
  await page.waitForTimeout(300);
  // The field's <div> wrapper is data-soldier-only-field or
  // data-soldier-or-widow-field. We just check whether the input is
  // visible to a real user (display !== 'none' on the section or
  // any disabled ancestor).
  const isVisible = await pensionStateField.isVisible().catch(() => false);
  const ok = isVisible === expected.visible;
  findings.push({
    entryType,
    expected: expected.visible,
    actual: isVisible,
    ok,
    reason: expected.reason,
  });
  console.log(`  ${ok ? '✓' : '✗'} entry_type=${entryType} pension_state visible=${isVisible} (expected ${expected.visible})`);
}

await browser.close();

const fails = findings.filter((f) => !f.ok);
console.log(`\nResult: ${findings.length - fails.length} pass / ${fails.length} fail`);
process.exit(fails.length > 0 ? 1 : 0);
