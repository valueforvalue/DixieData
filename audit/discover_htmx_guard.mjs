import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const ROOT = new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const APPSHELL_DIR = join(ROOT, 'internal/appshell');
const JS_FILE = join(ROOT, 'frontend/app.js');

const TOAST_MARKER = 'setInfoToastHeader(';
const REDIRECT_MARKERS = [
  'X-DixieData-Redirect',
  'writeExportRedirect(',
  'enqueueExport(',
  'respondDuplicateInFlight(',
];

const STRICT = process.argv.includes('--strict');

function readText(p) {
  return readFileSync(p, 'utf8');
}

function listGo(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) continue;
    if (!name.endsWith('.go')) continue;
    if (name.endsWith('_test.go')) continue; // Test files legitimately call setInfoToastHeader without a redirect.
    out.push(p);
  }
  return out;
}

// Walk `func NAME(...) ... {` decls in source. For each function body,
// return { name, lineStart, body } where body is the text between
// matching braces. Skip the function's own body when its name appears
// in the substring we're scanning for (definitions, not use sites).
function parseFuncs(text) {
  const lines = text.split('\n');
  const decls = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    const m = line.match(/^func(\s+\([^)]+\))?\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)[^{]*\{/);
    if (!m) {
      i++;
      continue;
    }
    const name = m[2];
    const lineStart = i + 1; // 1-indexed
    // Count braces from this line forward; balance opens minus closes
    // starting from the line that contains the opening brace.
    let depth = 0;
    let started = false;
    let j = i;
    for (; j < lines.length; j++) {
      for (const ch of lines[j]) {
        if (ch === '{') {
          depth++;
          started = true;
        } else if (ch === '}') {
          depth--;
        }
      }
      if (started && depth === 0) break;
    }
    const bodyStartLine = i;
    const bodyEndLine = j;
    const body = lines.slice(bodyStartLine, bodyEndLine + 1).join('\n');
    decls.push({ name, lineStart, body });
    i = j + 1;
  }
  return decls;
}

// Toast walker: flag any Go function whose body contains
// `setInfoToastHeader(` AND lacks ALL redirect markers. Skip the
// definition of setInfoToastHeader itself (and any helper that
// contains the literal but is the standard definition).
function findToastViolations(file) {
  const text = readText(file);
  const funcs = parseFuncs(text);
  const violations = [];
  for (const f of funcs) {
    if (f.name === 'setInfoToastHeader' || f.name === 'writeExportRedirect' ||
        f.name === 'respondDuplicateInFlight' || f.name === 'enqueueExport' ||
        f.name === 'enqueueExportWithResult') {
      // These are definition sites; the body is expected to use the
      // substring we're scanning for. Skip.
      continue;
    }
    if (!f.body.includes(TOAST_MARKER)) continue;
    const hasRedirect = REDIRECT_MARKERS.some((m) => f.body.includes(m));
    if (!hasRedirect) {
      violations.push({ name: f.name, line: f.lineStart });
    }
  }
  return violations;
}

// JS submit coexistence walker. Walks frontend/app.js line by line. Each
// `addEventListener("submit"` site is one of:
//
//   A. doc-level delegate that branches on `data-dixie-submit` inside
//      the listener body and routes to `dispatchDixieDataForm`.
//      Legitimate, no marker needed.
//
//   B. panel-scoped listener with `// htmx-guard: utility-submit`
//      on the line immediately preceding. Legitimate.
//
//   C. anything else → violation.
//
// We detect (A) by brace-walking the listener and checking for either
// `data-dixie-submit` or `dispatchDixieDataForm` anywhere in the body.
// We detect (B) by reading the line above.
function findJsSubmitViolations(file) {
  const text = readText(file);
  const lines = text.split('\n');
  const violations = [];

  const siteRe = /addEventListener\(\s*["']submit["']\s*,/;
  for (let i = 0; i < lines.length; i++) {
    if (!siteRe.test(lines[i])) continue;
    // Find the body of this listener via brace walk starting at the
    // first `{` on this line OR forward up to the next `=> {`.
    const openerIdx = lines[i].indexOf('{');
    if (openerIdx === -1) {
      // arrow style: `=> () => { ... }` — line continues onto next line.
      // For our purposes, find first `{` within the next 4 lines.
      let found = -1;
      for (let k = i; k < Math.min(i + 6, lines.length); k++) {
        const idx = lines[k].indexOf('{');
        if (idx !== -1) { found = k; break; }
      }
      if (found === -1) continue;
      // Brace walk from `found` line.
      const { body, endLine } = collectBody(lines, found);
      // Preceding line for marker check.
      const prev = i > 0 ? lines[i - 1] : '';
      classify(lines, i, body, endLine, prev, violations);
    } else {
      const { body, endLine } = collectBody(lines, i);
      const prev = i > 0 ? lines[i - 1] : '';
      classify(lines, i, body, endLine, prev, violations);
    }
  }
  return violations;
}

function collectBody(lines, fromLine) {
  let depth = 0;
  let started = false;
  let j = fromLine;
  for (; j < lines.length; j++) {
    for (const ch of lines[j]) {
      if (ch === '{') { depth++; started = true; }
      else if (ch === '}') { depth--; }
    }
    if (started && depth === 0) break;
  }
  return { body: lines.slice(fromLine, j + 1).join('\n'), endLine: j };
}

function classify(lines, i, body, endLine, prev, violations) {
  // Case A: branches on data-dixie-submit (delegate to dispatchDixieDataForm).
  if (body.includes('data-dixie-submit') && body.includes('dispatchDixieDataForm')) {
    return;
  }
  // Case A2: branches on data-dixie-submit alone, even without explicit
  // dispatch symbol (it's a future-doc-only listener). Conservative:
  // require the form-matches check to be present in the body.
  if (body.includes("matches(\"[data-dixie-submit]\")") || body.includes("matches('[data-dixie-submit]'")) {
    return;
  }
  // Case B: marker on preceding line.
  if (/^\s*\/\/\s*htmx-guard:\s*utility-submit\b/.test(prev)) return;
  // Otherwise: violation.
  violations.push({
    line: i + 1, // 1-indexed for human consumption
    excerpt: lines[i].trim().slice(0, 120),
    endLine: endLine + 1,
  });
}

function main() {
  console.log('=== htmx-guard lint ===');
  console.log('Probes: toast-no-redirect (Go) + JS submit coexistence (frontend/app.js)');
  console.log('');

  // Allow overriding paths via env for test fixtures. CI/default uses
  // the canonical files.
  const goDir = process.env.HTMX_GUARD_GO_DIR || APPSHELL_DIR;
  const jsFile = process.env.HTMX_GUARD_JS_FILE || JS_FILE;

  // Toast walker.
  const goFiles = listGo(goDir);
  const toastViolations = [];
  for (const f of goFiles) {
    const v = findToastViolations(f);
    if (v.length > 0) {
      toastViolations.push(...v.map((vv) => ({ file: f, ...vv })));
    }
  }

  // JS submit walker.
  const jsViolations = findJsSubmitViolations(jsFile).map((v) => ({
    file: jsFile, ...v,
  }));

  const total = toastViolations.length + jsViolations.length;

  console.log(`Toast-no-redirect violations: ${toastViolations.length}`);
  console.log(`JS submit coexistence violations: ${jsViolations.length}`);
  console.log('');

  if (toastViolations.length > 0) {
    console.log('=== TOAST-NO-REDIRECT VIOLATIONS ===');
    console.log('Each entry is a Go function that calls setInfoToastHeader(w, ...)');
    console.log('but lacks every redirect marker. The 70878ac → 3612dab cycle is');
    console.log('the canonical failure: user sees the toast but stays on the page.');
    console.log('');
    for (const v of toastViolations) {
      const rel = v.file.split(/[/\\]/).slice(-2).join('/');
      console.log(`  ${rel}:${v.line}  func ${v.name}()`);
    }
    console.log('');
  }

  if (jsViolations.length > 0) {
    console.log('=== JS SUBMIT COEXISTENCE VIOLATIONS ===');
    console.log('Each entry is an addEventListener("submit", ...) site on a form');
    console.log('that is NOT a navigation/data submit (data-dixie-submit handler)');
    console.log('and does NOT carry the // htmx-guard: utility-submit marker.');
    console.log('See docs/agents/htmx-guard-conventions.md for the contract.');
    console.log('');
    for (const v of jsViolations) {
      const rel = v.file.split(/[/\\]/).slice(-1)[0];
      console.log(`  ${rel}:${v.line}-${v.endLine}`);
      console.log(`    ${v.excerpt}`);
    }
    console.log('');
  }

  if (total === 0) {
    console.log('✓ htmx-guard clean. No drift detected.');
    process.exit(0);
  } else {
    console.log(`Found ${total} violation(s).`);
    if (STRICT) {
      console.log('--strict: treating as a CI failure.');
      process.exit(1);
    }
    process.exit(0);
  }
}

main();
