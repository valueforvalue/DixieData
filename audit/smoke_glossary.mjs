// smoke_glossary.mjs -- issue #564 slice 2 live regression net.
//
// Drives the in-context term disclosure popover against
// the live Person Record detail page. Pins the slice-2
// contract:
//
//   - The 2 in-scope apply sites render their
//     [data-term-disclosure-trigger="<slug>"] hooks
//     in the PanelSoldierDetailSummary <dl>.
//   - Clicking the trigger opens the corresponding panel
//     (hidden attribute removed, aria-expanded="true",
//     focus moved into the panel's "Read in glossary"
//     link).
//   - The "Read in glossary" link points at the
//     matching /about#about.glossary-<slug> anchor.
//   - Pressing Escape closes the open panel and returns
//     focus to the trigger.
//   - Clicking outside closes the open panel.
//   - Only one panel is open at a time (the second
//     click on a different trigger closes the first).
//
// Read-only against the Person Record detail page. The
// probe assumes a seeded local archive (dixiedata-web
// started by audit/run.mjs against a freshly-imported
// archive with at least one Person Record row).
//
// Run via: node audit/smoke_glossary.mjs (assumes
// dixiedata-web up at $BASE_URL, default
// http://127.0.0.1:8901).

import { chromium } from 'playwright';

const BASE = process.env.DIXIEDATA_BASE || 'http://127.0.0.1:8901';

async function expect(cond, msg, details) {
  if (!cond) {
    throw new Error(`probe failed: ${msg}\n  details: ${JSON.stringify(details)}`);
  }
  console.log(`  ok: ${msg}`);
}

async function discoverFirstSoldierDetailURL(page) {
  // Navigate to the soldiers list, click the first row,
  // return the resulting /soldiers/{id} URL. The list
  // is the canonical discovery surface; if no soldiers
  // exist, fall back to the layout's "View Person
  // Record" affordance.
  await page.goto(BASE + '/soldiers');
  await page.waitForSelector('[data-page-soldiers]', { timeout: 5000 }).catch(() => null);
  const rows = await page.evaluate(() => {
    const links = Array.from(document.querySelectorAll('a[href^="/soldiers/"]'));
    for (const a of links) {
      const href = a.getAttribute('href');
      if (!href) continue;
      // Match detail-page URLs only (not /soldiers/new
      // or /soldiers/{id}/edit or any tab-suffixed
      // variant). The detail page ends with the numeric
      // id; tabs would carry an additional path segment.
      const m = href.match(/^\/soldiers\/(\d+)$/);
      if (m) {
        return m[1];
      }
    }
    return null;
  });
  if (!rows) {
    throw new Error('no /soldiers/{id} detail links found on the list');
  }
  return `/soldiers/${rows}`;
}

async function populatedProbe(page) {
  const detailURL = await discoverFirstSoldierDetailURL(page);
  await page.goto(BASE + detailURL);
  await page.waitForSelector('[data-page-soldier.detail]', { timeout: 5000 }).catch(() => null);
  // Wait for the summary panel + the term-disclosure
  // triggers to be painted (the page is dynamic). The
  // summary dl carries the 2 in-scope dt labels.
  await page.waitForSelector('[data-term-disclosure-trigger="display-id"]', { timeout: 5000 }).catch(() => null);
  await page.waitForSelector('[data-term-disclosure-trigger="person-record"]', { timeout: 5000 }).catch(() => null);

  // Baseline: both disclosures render + both panels are
  // hidden + both triggers declare aria-expanded="false".
  const baseline = await page.evaluate(() => {
    const triggers = Array.from(
      document.querySelectorAll('[data-term-disclosure-trigger]'),
    ).map((el) => ({
      slug: el.getAttribute('data-term-disclosure-trigger'),
      expanded: el.getAttribute('aria-expanded'),
      short: el.getAttribute('data-term-disclosure-short'),
    }));
    const panels = Array.from(
      document.querySelectorAll('[data-term-disclosure-panel]'),
    ).map((el) => ({
      slug: el.getAttribute('data-term-disclosure-panel'),
      hidden: el.hasAttribute('hidden'),
      id: el.id,
    }));
    const readLinks = Array.from(
      document.querySelectorAll('[data-term-disclosure-read-in-glossary]'),
    ).map((el) => ({
      slug: el.getAttribute('data-term-disclosure-read-in-glossary'),
      href: el.getAttribute('href'),
    }));
    return { triggers, panels, readLinks };
  });
  await expect(
    baseline.triggers.length === 3,
    `3 term-disclosure triggers render (got ${baseline.triggers.length})`,
    baseline,
  );
  for (const want of ['display-id', 'person-record', 'source-record']) {
    await expect(
      baseline.triggers.some((t) => t.slug === want),
      `trigger for "${want}" renders`,
      baseline,
    );
  }
  for (const want of ['display-id', 'person-record', 'source-record']) {
    const trig = baseline.triggers.find((t) => t.slug === want);
    await expect(
      trig.expanded === 'false',
      `trigger for "${want}" declares aria-expanded="false" baseline`,
      baseline,
    );
  }
  for (const want of ['display-id', 'person-record', 'source-record']) {
    const panel = baseline.panels.find((p) => p.slug === want);
    await expect(
      panel.hidden,
      `panel for "${want}" is hidden baseline`,
      baseline,
    );
  }
  for (const want of ['display-id', 'person-record', 'source-record']) {
    const link = baseline.readLinks.find((l) => l.slug === want);
    await expect(
      link && link.href === `/about#about.glossary-${want}`,
      `Read in glossary link for "${want}" points at /about#about.glossary-${want} (got ${link ? link.href : '(missing)'})`,
      baseline,
    );
  }

  // Click the display-id trigger: opens the panel,
  // removes the hidden attribute, mirrors
  // aria-expanded="true", moves focus into the panel.
  await page.click('[data-term-disclosure-trigger="display-id"]');
  await page.waitForTimeout(80);
  const opened = await page.evaluate(() => {
    const panel = document.querySelector('[data-term-disclosure-panel="display-id"]');
    const trigger = document.querySelector('[data-term-disclosure-trigger="display-id"]');
    return {
      panelHidden: panel ? panel.hasAttribute('hidden') : null,
      triggerExpanded: trigger ? trigger.getAttribute('aria-expanded') : null,
      activeSlug: (() => {
        const active = document.activeElement;
        if (!active) return null;
        return active.getAttribute('data-term-disclosure-read-in-glossary');
      })(),
    };
  });
  await expect(!opened.panelHidden, 'click on display-id trigger opens the panel', opened);
  await expect(
    opened.triggerExpanded === 'true',
    'click on display-id trigger sets aria-expanded="true"',
    opened,
  );
  await expect(
    opened.activeSlug === 'display-id',
    'focus moved into the panel\'s Read in glossary link after open',
    opened,
  );

  // Click the person-record trigger: opens its panel AND
  // closes the display-id panel (single-panel-open).
  await page.click('[data-term-disclosure-trigger="person-record"]');
  await page.waitForTimeout(80);
  const switchOver = await page.evaluate(() => {
    const personPanel = document.querySelector('[data-term-disclosure-panel="person-record"]');
    const displayPanel = document.querySelector('[data-term-disclosure-panel="display-id"]');
    return {
      personOpen: personPanel ? !personPanel.hasAttribute('hidden') : null,
      displayClosed: displayPanel ? displayPanel.hasAttribute('hidden') : null,
    };
  });
  await expect(switchOver.personOpen, 'click on person-record opens its panel', switchOver);
  await expect(switchOver.displayClosed, 'click on person-record closes the previously-open display-id panel (single-panel-open)', switchOver);

  // Escape closes the open panel + restores focus to the
  // trigger.
  await page.keyboard.press('Escape');
  await page.waitForTimeout(80);
  const afterEscape = await page.evaluate(() => {
    const panel = document.querySelector('[data-term-disclosure-panel="person-record"]');
    const trigger = document.querySelector('[data-term-disclosure-trigger="person-record"]');
    const active = document.activeElement;
    return {
      panelHidden: panel ? panel.hasAttribute('hidden') : null,
      triggerExpanded: trigger ? trigger.getAttribute('aria-expanded') : null,
      focusOnTrigger: active === trigger,
    };
  });
  await expect(afterEscape.panelHidden, 'Escape closes the open panel', afterEscape);
  await expect(
    afterEscape.triggerExpanded === 'false',
    'Escape restores aria-expanded="false"',
    afterEscape,
  );
  await expect(afterEscape.focusOnTrigger, 'Escape returns focus to the trigger', afterEscape);

  // Re-open the display-id disclosure, then click outside
  // (the page footer counts as "outside"). The closed
  // panel goes back to hidden.
  await page.click('[data-term-disclosure-trigger="display-id"]');
  await page.waitForTimeout(80);
  // Click somewhere in the body that is NOT inside the
  // trigger or the panel. The page header's <h1> is a safe
  // anchor: it's neither trigger nor panel.
  await page.evaluate(() => {
    const h1 = document.querySelector('h1');
    if (h1) h1.click();
  });
  await page.waitForTimeout(80);
  const afterOutside = await page.evaluate(() => {
    const panel = document.querySelector('[data-term-disclosure-panel="display-id"]');
    return {
      panelHidden: panel ? panel.hasAttribute('hidden') : null,
    };
  });
  await expect(afterOutside.panelHidden, 'outside click closes the open panel', afterOutside);
}

async function main() {
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    console.log('glossary: populated probe');
    await populatedProbe(page);
    console.log('glossary: tags page probe');
    await tagsPageProbe(page);
    console.log('glossary: all probes pass');
  } finally {
    await browser.close();
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});

async function tagsPageProbe(page) {
  await page.goto(BASE + '/tags');
  await page.waitForSelector('[data-term-disclosure-trigger="tag"]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const trigger = document.querySelector('[data-term-disclosure-trigger="tag"]');
    const panel = document.querySelector('[data-term-disclosure-panel="tag"]');
    const link = document.querySelector('[data-term-disclosure-read-in-glossary="tag"]');
    return {
      triggerRendered: !!trigger,
      panelHidden: panel ? panel.hasAttribute('hidden') : null,
      panelRendered: !!panel,
      linkRendered: !!link,
      linkHref: link ? link.getAttribute('href') : null,
    };
  });
  await expect(state.triggerRendered, 'tags page renders TermDisclosure trigger for "tag" (#564 slice 4)', state);
  await expect(state.panelRendered, 'tags page renders the "tag" panel (#564 slice 4)', state);
  await expect(state.panelHidden, 'tags page "tag" panel is hidden baseline (#564 slice 4)', state);
  await expect(
    state.linkHref === '/about#about.glossary-tag',
    `tags page Read in glossary href is /about#about.glossary-tag (got ${state.linkHref})`,
    state,
  );
}
