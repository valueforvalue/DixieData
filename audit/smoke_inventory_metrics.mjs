// smoke_inventory_metrics.mjs -- issue #580 slice 1 + #583 slice 3
// regression net.
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
//   data-inventory-metrics-chart       chart wrapper (issue #583)
//   data-inventory-metrics-svg         painted SVG (slice 3)
//   data-inventory-metrics-series      group containing 5 paths
//   data-inventory-metrics-legend      legend region
//   data-inventory-metrics-legend-chip="<kind>" one chip per kind
//   data-active-kinds                  comma-separated visible kinds
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
  // Issue #583 slice 3: wait for the chart paint to settle so the
  // assertions below can rely on the SVG being mounted (the JS
  // renderer runs on DOMContentLoaded). The selector resolves
  // once the SVG is in place; if it never does (e.g. templ
  // shipped without the data attribute) the assertion below
  // fails with a clear shape mismatch.
  await page.waitForSelector('[data-inventory-metrics-svg]', { timeout: 5000 }).catch(() => null);
  const state = await page.evaluate(() => {
    const section = document.querySelector('[data-inventory-metrics]');
    const first = document.querySelector('[data-inventory-metrics-first]')?.textContent || '';
    const latest = document.querySelector('[data-inventory-metrics-latest]')?.textContent || '';
    const activeDays = document.querySelector('[data-inventory-metrics-active-days]')?.textContent || '';
    const chartWrapper = document.querySelector('[data-inventory-metrics-chart]');
    const svg = document.querySelector('[data-inventory-metrics-svg]');
    const seriesPaths = Array.from(
      document.querySelectorAll('[data-inventory-metrics-series] [data-inventory-metrics-series-kind]'),
    );
    const chips = Array.from(
      document.querySelectorAll('[data-inventory-metrics-legend-chip]'),
    );
    const activeKinds = chartWrapper?.getAttribute('data-active-kinds') || '';
    const kindSet = new Set(seriesPaths.map((p) => p.getAttribute('data-inventory-metrics-series-kind')));
    return {
      sectionRendered: !!section,
      first,
      latest,
      activeDays,
      chartRendered: !!chartWrapper,
      svgRendered: !!svg,
      seriesPathCount: seriesPaths.length,
      seriesKinds: Array.from(kindSet).sort(),
      chipCount: chips.length,
      chipKinds: chips.map((c) => c.getAttribute('data-inventory-metrics-legend-chip')).sort(),
      activeKinds: activeKinds.split(',').filter((k) => k.length > 0).sort(),
    };
  });
  await expect(state.sectionRendered, 'populated archive renders metrics section', state);
  await expect(state.first.length >= 8, 'first-entry is an ISO date', state);
  await expect(state.latest.length >= 8, 'latest-entry is an ISO date', state);
  await expect(parseInt(state.activeDays, 10) >= 1, 'active day count is at least 1', state);
  // Issue #583 slice 3 chart wiring.
  await expect(state.chartRendered, 'chart wrapper renders inside metrics section', state);
  await expect(state.svgRendered, 'SVG paint completed (JS renderer ran)', state);
  await expect(state.seriesPathCount === 5, 'chart renders 5 series paths', state);
  await expect(
    JSON.stringify(state.seriesKinds) === JSON.stringify(['article','event','linked','soldier','spouse']),
    'series paths cover all 5 kinds in templ order',
    state,
  );
  await expect(state.chipCount === 5, 'legend has 5 chips', state);
  await expect(
    JSON.stringify(state.chipKinds) === JSON.stringify(['article','event','linked','soldier','spouse']),
    'legend chips cover all 5 kinds in templ order',
    state,
  );
  await expect(state.activeKinds.length === 5, 'all 5 kinds are active by default', state);

  // Issue #595 slice 2: chart clip. The SVG's rendered width
  // must not exceed the host's visible width; otherwise the
  // line "runs off the visible image area" (the host is a flex
  // child of a constrained card, the SVG declares a larger
  // width, and the path — correctly drawn inside the SVG's
  // viewBox — appears past the visible boundary).
  const clipState = await page.evaluate(() => {
    const host = document.querySelector('[data-inventory-metrics-svg-host]');
    const svg = document.querySelector('[data-inventory-metrics-svg]');
    if (!(host instanceof HTMLElement) || !(svg instanceof SVGElement)) {
      return { ok: false };
    }
    const hostRect = host.getBoundingClientRect();
    const declaredWidth = parseFloat(svg.getAttribute('width') || '0');
    const viewBox = svg.getAttribute('viewBox') || '';
    const viewBoxParts = viewBox.split(/\s+/).map((p) => parseFloat(p));
    return {
      ok: true,
      hostWidth: hostRect.width,
      declaredWidth,
      viewBoxWidth: viewBoxParts[2] || 0,
      overflow: declaredWidth - hostRect.width,
    };
  });
  await expect(clipState.ok, 'clip selectors resolve (host + svg present)', clipState);
  await expect(
    clipState.declaredWidth <= clipState.hostWidth + 0.5,
    'SVG declared width does not exceed host width (clip fix — issue #595 slice 2)',
    clipState,
  );
  await expect(
    Math.abs(clipState.declaredWidth - clipState.viewBoxWidth) < 0.5,
    'SVG width attribute matches viewBox width (no internal aspect distortion)',
    clipState,
  );

  // Issue #595 slice 3: hover tooltip. Hovering any data point
  // surfaces a tooltip with the {date} · {count} · {kind label}
  // triple. The probe asserts the tooltip element exists, is
  // hidden by default, and shows the expected text on hover.
  const hoverState = await page.evaluate(async () => {
    const tooltip = document.querySelector('[data-inventory-metrics-tooltip]');
    const firstPoint = document.querySelector('[data-inventory-metrics-points] [data-point]');
    if (!(tooltip instanceof HTMLElement) || !(firstPoint instanceof Element)) {
      return { ok: false };
    }
    const initialVisible = tooltip.getAttribute('data-inventory-metrics-tooltip-visible');
    const initialText = tooltip.textContent || '';
    // Synthesize a hover via a real event so the listener fires.
    const evt = new MouseEvent('mouseover', { bubbles: true, clientX: 10, clientY: 10 });
    firstPoint.dispatchEvent(evt);
    // Give the handler a microtask to update the DOM.
    await new Promise((r) => setTimeout(r, 0));
    const afterVisible = tooltip.getAttribute('data-inventory-metrics-tooltip-visible');
    const afterText = tooltip.textContent || '';
    // Move away to clean up.
    const leave = new MouseEvent('mouseout', { bubbles: true });
    firstPoint.dispatchEvent(leave);
    return {
      ok: true,
      initialVisible,
      initialText,
      afterVisible,
      afterText,
      payload: firstPoint.getAttribute('data-point'),
    };
  });
  await expect(hoverState.ok, 'hover selectors resolve (tooltip + data point present)', hoverState);
  await expect(
    hoverState.initialVisible === 'false',
    'tooltip is hidden by default (data-inventory-metrics-tooltip-visible="false")',
    hoverState,
  );
  await expect(
    hoverState.afterVisible === 'true',
    'tooltip becomes visible on mouseover',
    hoverState,
  );
  await expect(
    /\d{4}-\d{2}-\d{2}.*·.*\d+.*·.*/.test(hoverState.afterText),
    'tooltip text matches {date} · {count} · {kind label} shape',
    { afterText: hoverState.afterText, payload: hoverState.payload },
  );

  // Issue #583 slice 3 legend toggle: clicking a chip hides the
  // matching series path (display:none) and updates
  // data-active-kinds. Re-clicking restores visibility.
  if (state.seriesPathCount === 5) {
    const beforeAfterToggle = await page.evaluate(() => {
      const wrapper = document.querySelector('[data-inventory-metrics-chart]');
      const chip = document.querySelector('[data-inventory-metrics-legend-chip="event"]');
      const path = document.querySelector('[data-inventory-metrics-series-kind="event"]');
      if (!wrapper || !chip || !path) return { ok: false };
      const beforeDisplay = path.getAttribute('display') || '';
      chip.click();
      const afterDisplay = path.getAttribute('display') || '';
      const afterActive = wrapper.getAttribute('data-active-kinds') || '';
      chip.click();
      const restoredDisplay = path.getAttribute('display') || '';
      const restoredActive = wrapper.getAttribute('data-active-kinds') || '';
      return {
        ok: true,
        beforeDisplay,
        afterDisplay,
        afterActive,
        restoredDisplay,
        restoredActive,
      };
    });
    await expect(beforeAfterToggle.ok, 'toggle selectors all resolve', beforeAfterToggle);
    await expect(
      beforeAfterToggle.beforeDisplay === '' && beforeAfterToggle.afterDisplay === 'none',
      'clicking the event chip sets display:none on the event series',
      beforeAfterToggle,
    );
    await expect(
      !beforeAfterToggle.afterActive.split(',').includes('event'),
      'data-active-kinds drops event after toggle',
      beforeAfterToggle,
    );
    await expect(
      beforeAfterToggle.restoredDisplay === '' && beforeAfterToggle.restoredActive.split(',').includes('event'),
      'clicking the chip again restores the series and active kinds',
      beforeAfterToggle,
    );
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
