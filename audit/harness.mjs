// Shared audit helpers for DixieData UI/UX harness.
//
// Extracted from run.mjs and run-round2.mjs so future rounds (run-round3.mjs
// and beyond) inherit the same logic without copy-paste drift. See #86.

import { AxeBuilder } from '@axe-core/playwright';

/**
 * Detect whether a fetched HTML payload is a full document or just an HTMX
 * fragment. Fragments don't have <html>, <head>, or <title> elements, so
 * axe-core produces false-positive document-title / html-has-lang violations
 * against them. Callers should skip axe for fragments and still run visual
 * checks.
 *
 * Detection rules (any of these counts as fragment):
 *   - Content-Type is not text/html
 *   - Body length < 200 bytes
 *   - Body contains no `<html` tag
 *
 * Returns { isFragment, reason }.
 */
export function detectFragment({ contentType, body }) {
  if (!contentType || !contentType.toLowerCase().includes('text/html')) {
    return { isFragment: true, reason: `content-type=${contentType || 'unset'}` };
  }
  if (typeof body !== 'string' || body.length < 200) {
    return { isFragment: true, reason: `body length ${body?.length ?? 0} < 200` };
  }
  if (!/<html[\s>]/i.test(body)) {
    return { isFragment: true, reason: 'no <html> tag in body' };
  }
  return { isFragment: false, reason: null };
}

/**
 * Run axe-core against the current page, or return an empty result if the
 * page is an HTMX fragment (in which case axe would report false positives).
 *
 * Fragment detection works by re-fetching the page URL via page.request and
 * checking the response body for an <html> tag. The browser auto-wraps
 * fragments in a synthetic <html>/<head>, so DOM inspection is unreliable;
 * the response body is the source of truth.
 */
export async function runAxe(page, { skipIfFragment = true } = {}) {
  if (skipIfFragment) {
    try {
      const url = page.url();
      const resp = await page.request.get(url);
      const ct = resp.headers()['content-type'] || '';
      const body = await resp.text();
      const frag = detectFragment({ contentType: ct, body });
      if (frag.isFragment) {
        return { skipped: true, reason: `fragment detected: ${frag.reason}`, violations: [] };
      }
    } catch (e) {
      // Probe failed — fall through and run axe anyway. Worst case we get
      // false positives, which is the same behaviour as before this slice.
    }
  }
  try {
    const builder = new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa']);
    const result = await builder.analyze();
    return { skipped: false, reason: null, violations: result.violations || [] };
  } catch (e) {
    return { skipped: false, reason: `axe error: ${e.message}`, violations: [] };
  }
}

/**
 * Visual / layout heuristic detector. Same logic that used to live inline in
 * run.mjs and run-round2.mjs. See those files' git history for the original
 * per-finding rationale.
 */
export async function detectVisualIssues(page) {
  return await page.evaluate(() => {
    const issues = [];

    if (document.documentElement.scrollWidth > window.innerWidth + 1) {
      issues.push({ id: 'h-scroll', severity: 'high', detail: `scrollWidth=${document.documentElement.scrollWidth} > ${window.innerWidth}` });
    }
    if (document.body.scrollWidth > window.innerWidth + 1) {
      issues.push({ id: 'h-scroll-body', severity: 'high', detail: `body scrollWidth=${document.body.scrollWidth} > ${window.innerWidth}` });
    }

    const overflowers = [];
    document.querySelectorAll('*').forEach((el) => {
      if (el.tagName === 'HTML' || el.tagName === 'BODY') return;
      const r = el.getBoundingClientRect();
      if (r.right > window.innerWidth + 1 && r.width > 0) {
        const cs = window.getComputedStyle(el);
        if (cs.position === 'fixed' || cs.position === 'absolute') {
          let p = el.parentElement, clipped = false;
          while (p && p !== document.body) {
            const pcs = window.getComputedStyle(p);
            if (pcs.overflow === 'hidden' || pcs.overflowX === 'hidden') { clipped = true; break; }
            p = p.parentElement;
          }
          if (clipped) return;
        }
        overflowers.push({
          tag: el.tagName.toLowerCase(),
          id: el.id || null,
          cls: typeof el.className === 'string' ? el.className.slice(0, 80) : null,
          right: Math.round(r.right),
          width: Math.round(r.width),
        });
      }
    });
    if (overflowers.length > 0) {
      const seen = new Set(), sample = [];
      for (const o of overflowers) {
        const k = `${o.tag}.${o.cls || ''}`;
        if (seen.has(k)) continue;
        seen.add(k); sample.push(o);
        if (sample.length >= 5) break;
      }
      issues.push({ id: 'overflow-x', severity: 'high', count: overflowers.length, sample });
    }

    const smallTargets = [];
    document.querySelectorAll('button, a, [role="button"]').forEach((el) => {
      const r = el.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) return;
      if (r.width < 24 || r.height < 24) {
        smallTargets.push({
          tag: el.tagName.toLowerCase(),
          text: (el.textContent || '').trim().slice(0, 40),
          w: Math.round(r.width), h: Math.round(r.height),
        });
      }
    });
    if (smallTargets.length > 0) {
      issues.push({ id: 'small-tap-target', severity: 'medium', count: smallTargets.length, sample: smallTargets.slice(0, 5) });
    }

    const unlabeled = [];
    document.querySelectorAll('input:not([type="hidden"]):not([type="submit"]):not([type="button"]), textarea, select').forEach((el) => {
      const id = el.id;
      const lbl = id ? document.querySelector(`label[for="${id}"]`) : null;
      const wrap = el.closest('label');
      const aria = el.getAttribute('aria-label') || el.getAttribute('aria-labelledby') || el.getAttribute('placeholder');
      if (!lbl && !wrap && !aria) {
        unlabeled.push({ tag: el.tagName.toLowerCase(), name: el.name || null, type: el.type || null });
      }
    });
    if (unlabeled.length > 0) {
      issues.push({ id: 'unlabeled-input', severity: 'medium', count: unlabeled.length, sample: unlabeled.slice(0, 5) });
    }

    const nav = document.querySelector('nav');
    if (nav) {
      const links = nav.querySelectorAll('a, button');
      const navR = nav.getBoundingClientRect();
      issues.push({
        id: 'nav-density',
        severity: 'info',
        item_count: links.length,
        nav_width: Math.round(navR.width),
        item_width_avg: links.length ? Math.round(navR.width / links.length) : 0,
      });
    }

    const emptyState = document.querySelector('[data-empty-state], .empty-state');
    if (emptyState) {
      issues.push({ id: 'empty-state-visible', severity: 'info', tag: emptyState.tagName.toLowerCase(), snippet: (emptyState.textContent || '').trim().slice(0, 80) });
    }

    const table = document.querySelector('table');
    if (table) {
      const tbody = table.querySelector('tbody');
      const rowCount = tbody ? tbody.querySelectorAll('tr').length : 0;
      const pageHeight = document.documentElement.scrollHeight;
      issues.push({ id: 'table-rows', severity: 'info', rows: rowCount, page_height: pageHeight });
    }

    const htmxBusy = document.querySelectorAll('[aria-busy="true"]').length;
    if (htmxBusy > 0) {
      issues.push({ id: 'stuck-busy', severity: 'medium', count: htmxBusy });
    }

    const tables = document.querySelectorAll('table');
    if (tables.length > 1) {
      issues.push({ id: 'nested-tables', severity: 'medium', count: tables.length });
    }

    return issues;
  });
}

/**
 * Render a per-route row as it should appear in the markdown summary.
 */
export function renderSummary({ title, routes, findings }) {
  const byImpact = { critical: 0, serious: 0, moderate: 0, minor: 0 };
  const byId = {};
  for (const f of findings) {
    if (f.impact && byImpact[f.impact] !== undefined) byImpact[f.impact]++;
    const k = f.id || f.kind || 'unknown';
    byId[k] = (byId[k] || 0) + 1;
  }
  const lines = [
    `# ${title}`,
    '',
    `Routes audited: **${routes.length}** (${new Set(routes.map((r) => r.path)).size} unique paths)`,
    `Total findings: **${findings.length}**`,
    '',
    '## A11y violations by severity',
    '',
    `- Critical: ${byImpact.critical}`,
    `- Serious: ${byImpact.serious}`,
    `- Moderate: ${byImpact.moderate}`,
    `- Minor: ${byImpact.minor}`,
    '',
    '## Findings by type',
    '',
    ...Object.entries(byId).sort((a, b) => b[1] - a[1]).map(([k, v]) => `- \`${k}\`: ${v}`),
    '',
    '## Per-route snapshot',
    '',
    '| Route | Viewport | Status | Load (ms) | Fragment | A11y types | Visual issues |',
    '|---|---|---:|---:|---|---:|---:|',
    ...routes.map((r) =>
      `| \`${r.path}\` | ${r.viewport} | ${r.status} | ${r.load_ms} | ${r.isFragment ? '✓' : ''} | ${r.axe_violations.length} | ${r.visual_issues.length} |`
    ),
  ];
  return lines.join('\n');
}