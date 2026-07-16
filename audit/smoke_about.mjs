// smoke_about.mjs -- issue #585 + #586 regression net.
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
//   data-about-activity              #activity section (#586)
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
//   data-about-activity-empty        activity empty-state (#586)
//   data-about-activity-summary      activity summary line (#586)
//   data-about-activity-heatmap      heatmap host (#586)
//   data-about-activity-contributor  one per top contributor (#586)
//   data-about-activity-per-release  one per release activity row (#586)
//   data-about-activity-issues-bar    issues-closed stacked bar (#586)
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
    const historyNav = document.querySelector('[data-about-nav-history]');
    const activity = document.querySelector('[data-about-activity]');
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
    // Activity section (#586) — either populated or empty.
    const activityEmpty = document.querySelector('[data-about-activity-empty]');
    const activitySummary = document.querySelector('[data-about-activity-summary]');
    const activityContributors = document.querySelectorAll('[data-about-activity-contributor]');
    const activityIssuesBar = document.querySelector('[data-about-activity-issues-bar]');
    // Issue #594: Recent commits section + nav link + permalinks.
    const recent = document.querySelector('[data-about-recent]');
    const recentEmpty = document.querySelector('[data-about-recent-empty]');
    const recentList = document.querySelector('[data-about-recent-list]');
    const recentNavLink = document.querySelector('[data-about-nav-recent]');
    const recentRows = Array.from(
      document.querySelectorAll('[data-about-recent-row]'),
    );
    // Issue #603: recent-list wrapper carries the
    // scroll-viewport utility classes.
    const recentListEl = document.querySelector('[data-about-recent-list]');
    const recentListWrapper = recentListEl ? recentListEl.parentElement : null;
    const recentListWrapperClassList = recentListWrapper
      ? Array.from(recentListWrapper.classList)
      : [];
    // Issue #601: Repository activity heatmap. The
    // templ emits `data-about-activity-heatmap` host
    // with a `data-about-activity-heatmap-data` JSON
    // payload + a `data-about-activity-heatmap-loading`
    // placeholder paragraph. JS must paint an SVG
    // inside the host + remove the loading paragraph.
    // RED (pre-#601 fix): placeholder stays + no SVG.
    // GREEN: an SVG is present inside the host + the
    // placeholder is gone.
    const activityHeatmapHost = document.querySelector('[data-about-activity-heatmap]');
    const activityHeatmapSVG = activityHeatmapHost
      ? activityHeatmapHost.querySelector('svg')
      : null;
    const activityHeatmapLoading = document.querySelector('[data-about-activity-heatmap-loading]');
    // Issue #602: each cell carries a <title> child for
    // native hover/focus tooltip + a tabindex for keyboard
    // nav; the grid also has month labels above and
    // weekday labels on the left margin.
    const activityHeatmapCells = activityHeatmapSVG
      ? Array.from(activityHeatmapSVG.querySelectorAll('[data-about-activity-heatmap-cell]'))
      : [];
    const activityHeatmapMonthLabels = activityHeatmapSVG
      ? Array.from(activityHeatmapSVG.querySelectorAll('[data-about-activity-heatmap-month]'))
      : [];
    const activityHeatmapWeekdayLabels = activityHeatmapSVG
      ? Array.from(activityHeatmapSVG.querySelectorAll('[data-about-activity-heatmap-weekday]'))
      : [];
    // Sample the first cell with a non-empty <title>
    // child so the assertion can verify the tooltip
    // text shape ("YYYY-MM-DD · N commit(s)").
    const firstCellWithTitle = activityHeatmapCells.find(
      (c) => c.querySelector('title') && (c.querySelector('title').textContent || '').length > 0,
    );
    const firstCellTitleText = firstCellWithTitle
      ? (firstCellWithTitle.querySelector('title').textContent || '')
      : '';
    const firstCellAriaLabel = firstCellWithTitle
      ? (firstCellWithTitle.getAttribute('aria-label') || '')
      : '';
    const monthLabelTexts = activityHeatmapMonthLabels.map((el) => el.getAttribute('data-about-activity-heatmap-month') || el.textContent || '');
    const weekdayLabelTexts = activityHeatmapWeekdayLabels.map((el) => el.getAttribute('data-about-activity-heatmap-weekday') || el.textContent || '');
    // Issue #564 slice 1: Glossary section + nav link + rows.
    const glossary = document.querySelector('[data-about-glossary]');
    const glossaryNavLink = document.querySelector('[data-about-nav-glossary]');
    const glossaryList = document.querySelector('[data-about-glossary-list]');
    const glossaryTerms = Array.from(
      document.querySelectorAll('[data-about-glossary-term]'),
    ).map((el) => el.getAttribute('data-about-glossary-term'));
    // Issue #604: glossary list wraps in a bounded
    // scroll viewport.
    const glossaryListEl = document.querySelector('[data-about-glossary-list]');
    const glossaryWrapper = glossaryListEl ? glossaryListEl.parentElement : null;
    const glossaryWrapperClassList = glossaryWrapper
      ? Array.from(glossaryWrapper.classList)
      : [];
    // Pull the first row's hash + permalink so the assertion
    // can verify the GitHub URL format.
    const firstRowPermalink = recentRows.length > 0
      ? recentRows[0].querySelector('[data-about-recent-hash]')?.getAttribute('href') || ''
      : '';
    return {
      pageRendered: !!pageRoot,
      identityRendered: !!identity,
      licenseRendered: !!license,
      historyRendered: !!history,
      historyNavRendered: !!historyNav,
      activityRendered: !!activity,
      inPageNavRendered: !!inPageNav,
      identityFields,
      credits,
      releaseCount: releases.length,
      releases,
      collapseRendered: !!collapse,
      emptyRendered: !!empty,
      activityEmptyRendered: !!activityEmpty,
      activitySummaryRendered: !!activitySummary,
      activityContributorCount: activityContributors.length,
      activityIssuesBarRendered: !!activityIssuesBar,
      // Issue #601 fields (RED): placeholders stay +
      // no SVG until slice 2 paints them.
      activityHeatmapHostRendered: !!activityHeatmapHost,
      activityHeatmapSVGRendered: !!activityHeatmapSVG,
      activityHeatmapLoadingRendered: !!activityHeatmapLoading,
      // Issue #602 fields.
      activityHeatmapCellCount: activityHeatmapCells.length,
      activityHeatmapMonthLabelCount: activityHeatmapMonthLabels.length,
      activityHeatmapWeekdayLabelCount: activityHeatmapWeekdayLabels.length,
      activityHeatmapFirstCellTitle: firstCellTitleText,
      activityHeatmapFirstCellAriaLabel: firstCellAriaLabel,
      activityHeatmapMonthLabels: monthLabelTexts,
      activityHeatmapWeekdayLabels: weekdayLabelTexts,
      // Issue #594 fields.
      recentRendered: !!recent,
      recentEmptyRendered: !!recentEmpty,
      recentListRendered: !!recentList,
      recentNavLinkRendered: !!recentNavLink,
      recentRowCount: recentRows.length,
      // Issue #603 fields.
      recentListWrapperClassList: recentListWrapperClassList,
      recentListWrapperIsRecentListParent: recentListEl ? recentListEl.parentElement === recentListWrapper : false,
      firstRowPermalink,
      // Issue #601 fields. RED probe asserts absent
      // until slice 2 paints the SVG + removes the
      // loading placeholder.
      activityHeatmapHostRendered: !!activityHeatmapHost,
      activityHeatmapSVGRendered: !!activityHeatmapSVG,
      activityHeatmapLoadingRendered: !!activityHeatmapLoading,
      // Issue #602 fields.
      activityHeatmapCellCount: activityHeatmapCells.length,
      activityHeatmapMonthLabelCount: activityHeatmapMonthLabels.length,
      activityHeatmapWeekdayLabelCount: activityHeatmapWeekdayLabels.length,
      activityHeatmapFirstCellTitle: firstCellTitleText,
      activityHeatmapFirstCellAriaLabel: firstCellAriaLabel,
      activityHeatmapMonthLabels: monthLabelTexts,
      activityHeatmapWeekdayLabels: weekdayLabelTexts,
      // Issue #564 slice 1: Glossary section + rows.
      glossaryRendered: !!glossary,
      glossaryNavLinkRendered: !!glossaryNavLink,
      glossaryListRendered: !!glossaryList,
      glossaryTermCount: glossaryTerms.length,
      // Issue #604 fields.
      glossaryWrapperClassList: glossaryWrapperClassList,
      glossaryWrapperIsGlossaryListParent: glossaryListEl ? glossaryListEl.parentElement === glossaryWrapper : false,
      glossaryTerms,
    };
  });
  await expect(state.pageRendered, 'about page renders', state);
  await expect(state.identityRendered, 'identity section renders', state);
  await expect(state.licenseRendered, 'license section renders', state);
  await expect(state.historyRendered, 'history section renders (issue #585)', state);
  await expect(state.activityRendered, 'activity section renders (#586)', state);
  await expect(state.inPageNavRendered, 'in-page nav strip renders', state);
  // Issue #598: Release history is dropped — /about has 4
  // sections, not 5. The history section + the nav-strip link
  // to it are gone (the recent commits section, #594, is the
  // replacement for "what just landed").
  await expect(
    !state.historyRendered,
    'history section is absent (issue #598 — release history dropped)',
    state,
  );
  await expect(
    !state.historyNavRendered,
    'in-page nav strip does NOT link to #about.history (issue #598)',
    state,
  );
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
  // Issue #598: release-history surface is entirely gone. The
  // about_handlers_test.go's TestBuildAboutViewReleasesFlatten
  // is removed; the templ no longer renders the section. Probe
  // asserts no release cards + no collapse toggle + no history
  // empty-state are present (the section is absent, period).
  await expect(state.releaseCount === 0, 'no release cards rendered (issue #598)', state);
  await expect(!state.collapseRendered, 'history collapse toggle is absent (issue #598)', state);
  await expect(!state.emptyRendered, 'history empty-state is absent (issue #598)', state);
  // Activity section (#586): either populated or empty-state.
  await expect(
    state.activityEmptyRendered !== state.activitySummaryRendered,
    'activity section is either empty-state or populated, not both',
    state,
  );
  if (state.activitySummaryRendered) {
    await expect(
      state.activityContributorCount > 0,
      `at least one top contributor rendered (got ${state.activityContributorCount})`,
      state,
    );
    await expect(
      state.activityIssuesBarRendered,
      'issues-closed stacked bar renders when baked',
      state,
    );
    // Issue #601: when the activity section is populated
    // (baked build), the heatmap host renders + an SVG
    // is painted inside it + the "Loading heatmap..."
    // placeholder is gone. RED: JS renderer missing;
    // placeholder stays + no SVG.
    await expect(
      state.activityHeatmapHostRendered,
      'activity heatmap host renders when baked (#601)',
      state,
    );
    await expect(
      state.activityHeatmapSVGRendered,
      'activity heatmap paints an SVG inside the host (#601)',
      state,
    );
    await expect(
      !state.activityHeatmapLoadingRendered,
      'activity heatmap loading placeholder is removed after paint (#601)',
      state,
    );
    // Issue #602: each cell carries a native <title>
    // tooltip + the grid axis labels render.
    await expect(
      state.activityHeatmapCellCount === 364,
      `heat map paints 364 cells (52 weeks * 7 days; got ${state.activityHeatmapCellCount})`,
      state,
    );
    // The first cell with a <title> child must have
    // tooltip text matching the "YYYY-MM-DD · N
    // commit(s)" shape. The singular form is "1 commit";
    // the plural form is "N commits".
    await expect(
      /^\d{4}-\d{2}-\d{2} · \d+ commits?$/.test(state.activityHeatmapFirstCellTitle),
      `first cell <title> text matches "YYYY-MM-DD · N commit(s)" (got "${state.activityHeatmapFirstCellTitle}")`,
      state,
    );
    // The same cell's aria-label must mirror the
    // <title> text so screen-reader users hear the
    // same context.
    await expect(
      state.activityHeatmapFirstCellAriaLabel === state.activityHeatmapFirstCellTitle,
      `first cell aria-label matches its <title> text (got aria-label "${state.activityHeatmapFirstCellAriaLabel}")`,
      state,
    );
    // Month labels above the grid: the 52-week window
    // covers at least one month transition (usually 12,
    // since 365 days / 30 ~ 12 month starts). The probe
    // pins at least 3 month labels so the user sees
    // the calendar axis even for short windows.
    await expect(
      state.activityHeatmapMonthLabelCount >= 3,
      `heat map paints >=3 month labels above the grid (got ${state.activityHeatmapMonthLabelCount})`,
      state,
    );
    // Weekday labels in the left margin: Sun / Wed /
    // Fri only (the codebase convention to keep the
    // column visually balanced).
    await expect(
      state.activityHeatmapWeekdayLabelCount === 3,
      `heat map paints exactly 3 weekday labels (Sun / Wed / Fri; got ${state.activityHeatmapWeekdayLabelCount})`,
      state,
    );
    await expect(
      ['S', 'W', 'F'].every((wanted) => state.activityHeatmapWeekdayLabels.includes(wanted)),
      `weekday labels include S, W, F (got ${JSON.stringify(state.activityHeatmapWeekdayLabels)})`,
      state,
    );
  }
  // Issue #594: Recent commits section.
  await expect(state.recentRendered, 'recent commits section renders (#594)', state);
  await expect(
    state.recentNavLinkRendered,
    'in-page nav strip has a Recent commits link (#594)',
    state,
  );
  await expect(
    state.recentEmptyRendered !== state.recentListRendered,
    'recent section is either empty-state or populated, not both',
    state,
  );
  if (state.recentListRendered) {
    await expect(
      state.recentRowCount > 0,
      `at least one recent commit row rendered (got ${state.recentRowCount})`,
      state,
    );
    // Issue #603: recent-list is wrapped in a
    // bounded scroll viewport.
    await expect(
      state.recentListWrapperClassList.includes('max-h-96'),
      `recent-list wrapper carries max-h-96 utility class (got ${state.recentListWrapperClassList.join(' ')})`,
      state,
    );
    await expect(
      state.recentListWrapperClassList.includes('overflow-y-auto'),
      `recent-list wrapper carries overflow-y-auto utility class (got ${state.recentListWrapperClassList.join(' ')})`,
      state,
    );
    await expect(
      state.recentListWrapperClassList.some((c) => c.startsWith('sm:max-h-')),
      `recent-list wrapper carries responsive sm:max-h-* utility class (got ${state.recentListWrapperClassList.join(' ')})`,
      state,
    );
    // The permalink must match the GitHub commit URL shape:
    // https://github.com/valueforvalue/DixieData/commit/<40-char SHA1>.
    await expect(
      /^https:\/\/github\.com\/valueforvalue\/DixieData\/commit\/[0-9a-f]{40}$/.test(state.firstRowPermalink),
      `recent row permalink matches GitHub commit URL shape (got ${state.firstRowPermalink})`,
      state,
    );
  }
  // Issue #564 slice 1: Glossary section + at least one row.
  await expect(state.glossaryRendered, 'glossary section renders (#564 slice 1)', state);
  await expect(
    state.glossaryNavLinkRendered,
    'in-page nav strip has a Glossary link (#564 slice 1)',
    state,
  );
  await expect(state.glossaryListRendered, 'glossary list renders when terms present (#564 slice 1)', state);
  // The full registry has 36 terms; this is a tight pin that
  // catches drift between the registry and the rendered
  // section without forcing the test to enumerate every
  // slug. If a future term removal slips through, the
  // assertion fires. If a future term addition slips
  // through, ditto.
  await expect(
    state.glossaryTermCount === 36,
    `glossary renders the canonical 36-term registry (got ${state.glossaryTermCount})`,
    state,
  );
  // Issue #604: the glossary list is wrapped in a
  // bounded scroll viewport.
  await expect(
    state.glossaryWrapperClassList.includes('max-h-96'),
    `glossary wrapper carries max-h-96 utility class (got ${state.glossaryWrapperClassList.join(' ')})`,
    state,
  );
  await expect(
    state.glossaryWrapperClassList.includes('overflow-y-auto'),
    `glossary wrapper carries overflow-y-auto utility class (got ${state.glossaryWrapperClassList.join(' ')})`,
    state,
  );
  await expect(
    state.glossaryWrapperClassList.some((c) => c.startsWith('sm:max-h-')),
    `glossary wrapper carries responsive sm:max-h-* utility class (got ${state.glossaryWrapperClassList.join(' ')})`,
    state,
  );
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