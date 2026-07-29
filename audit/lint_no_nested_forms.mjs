// lint_no_nested_forms.mjs -- issue #682.
//
// Walks every `.templ` source under `internal/templates/` and asserts
// that no `<form>` is opened while another `<form>` is still on the
// element stack. The HTML5 parser forbids `<form>` inside `<form>`:
// when the parser hits the inner `<form>` while a form is in scope,
// it acts as if an end tag for `form` was seen (WHATWG HTML §13.2.6.4.3,
// "in body" insertion mode). The result: the outer form is auto-closed
// at the inner form's open tag, anything between the inner form's open
// and the outer form's close is reparented out of the outer form, and
// submit buttons that lived in that span end up with no `<form>`
// ancestor in the live DOM. The dispatcher's `button.closest("form")`
// returns null and the submit event never fires.
//
// The canonical historical instance: the inner image-upload form at
// `entry_form.templ:392` (and `soldier_card.templ:574`) silently closed
// the outer form, breaking the Save Changes / Download Selected Images
// buttons respectively. The structural fix lands in the templ sources
// (#682); this lint is the gate that fails CI if a future change
// re-introduces the nesting.
//
// Templ files commonly contain multiple `templ Name(...)` blocks at
// the top level. Each block is an independent rendering function —
// a `<form>` in one block is NOT nested under a `<form>` in another
// block. The lint splits the file by `^templ\b` markers and scans
// each block as an independent stream. This is the same convention
// `tidy` and `gofmt` use for templ files.
//
// The output is the canonical row-only pipe-delimited format:
// `file:line | rule-id | note`. --strict flips to exit 1 so CI can
// use it as a gate. Informational by default; --strict flips to CI
// failure. Sibling to `lint-bake-bootstrap` and the `verify-embed-tree`
// gate added by #686.

import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs';
import { join, relative, sep } from 'node:path';

const ROOT = process.env.DIXIE_ROOT || new URL('..', import.meta.url).pathname.replace(/^\/([A-Z]:)/, '$1');
const TEMPL_DIR = join(ROOT, 'internal/templates');
const STRICT = process.argv.includes('--strict');

// ---- Templ walker ----

// Recursively walk a directory and return every `.templ` file path.
function listTemplFiles(dir) {
  const out = [];
  if (!existsSync(dir)) return out;
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    const s = statSync(p);
    if (s.isDirectory()) {
      out.push(...listTemplFiles(p));
    } else if (s.isFile() && p.endsWith('.templ')) {
      out.push(p);
    }
  }
  return out;
}

// Strip Go expressions inside `{` `}` blocks and `//` line comments
// so the regex doesn't match `<form` inside a templ directive or
// inside a comment. Conservative: we replace the matched segments
// with whitespace of the same length so line numbers and column
// positions are preserved. Templ `if`/`for` directives can contain
// `<` literals, so we recurse until no more simple-substitution
// matches happen.
function stripGoExpressions(line) {
  let prev = '';
  let cur = line;
  while (cur !== prev) {
    prev = cur;
    // Strip Go expressions inside `{ ... }` (templ directives).
    cur = cur.replace(/\{[^{}]*\}/g, (m) => ' '.repeat(m.length));
  }
  // Strip `//` line comments (Go-style). These can appear anywhere
  // in the line and contain `<form>` references that aren't real
  // HTML. We only strip after the `//` itself.
  const commentIdx = cur.indexOf('//');
  if (commentIdx >= 0) {
    cur = cur.slice(0, commentIdx) + ' '.repeat(cur.length - commentIdx);
  }
  return cur;
}

// Split a templ file into its component blocks. Each block is a
// `templ Name(...) { ... }` definition. We split on the line that
// starts with `templ ` at column 0 (the keyword is the first token
// on the line). The first block (before any `templ`) is package-level
// content (imports, package declaration, etc.) — we still scan it
// because stray `<form>` tags at package level would be a real bug.
function splitTemplBlocks(text) {
  const lines = text.split('\n');
  const blocks = [];
  let current = { startLine: 0, body: '' };
  for (let i = 0; i < lines.length; i++) {
    if (/^templ\b/.test(lines[i])) {
      if (current.body !== '' || i > 0) {
        blocks.push(current);
      }
      current = { startLine: i + 1, body: '' };
    }
    current.body += (current.body === '' ? '' : '\n') + lines[i];
  }
  if (current.body !== '') {
    blocks.push(current);
  }
  return blocks;
}

// Find nested `<form>` tags within a single templ block. Returns
// an array of violations. The block is a single templ rendering
// function, so `<form>` opens/closes within it are real nesting.
function findNestedForms(block, file) {
  const lines = block.body.split('\n');
  const stack = [];
  const violations = [];
  const blockStartLine = block.startLine;

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i];
    const line = stripGoExpressions(rawLine);
    const lineNum = blockStartLine + i;

    // Find `<form` occurrences on this line (excluding self-closing).
    let pos = 0;
    while (pos < line.length) {
      const openIdx = line.indexOf('<form', pos);
      if (openIdx < 0) break;
      const after = openIdx + 5;
      const c = after < line.length ? line[after] : '';
      // A real `<form>` is followed by whitespace, `>`, or end-of-line.
      // The end-of-line case is `<form` with attributes on subsequent
      // lines (templ files commonly put the open tag's attributes on
      // separate lines). We also accept `>` as the only-attribute-on-this-line
      // form like `<form ...>`.
      const isRealOpen = c === ' ' || c === '\t' || c === '>' || c === '\n' || c === '\r' || c === '';
      if (!isRealOpen) {
        pos = openIdx + 1;
        continue;
      }
      // Self-closing <form/> check (`<form ... />`).
      const tagEnd = line.indexOf('>', openIdx);
      let isSelfClosing = false;
      if (tagEnd > 0) {
        const tagSlice = line.slice(openIdx, tagEnd + 1);
        if (tagSlice.endsWith('/>')) {
          isSelfClosing = true;
        }
      }
      const indent = openIdx;
      if (isSelfClosing) {
        if (stack.length > 0) {
          violations.push({
            file,
            line: lineNum,
            indent,
            rule: 'nested-form-self-closing',
            note: `<form/> self-closing tag at indent ${indent} is inside an open <form> opened at line ${stack[stack.length - 1].line} (HTML5 forbids <form> inside <form>).`,
          });
        }
        pos = (tagEnd >= 0 ? tagEnd : openIdx) + 1;
        continue;
      }
      if (stack.length > 0) {
        const inner = stack[stack.length - 1];
        violations.push({
          file,
          line: lineNum,
          indent,
          rule: 'nested-form-open',
          note: `<form> at indent ${indent} is inside <form> opened at line ${inner.line} (HTML5 forbids <form> inside <form>; the parser auto-closes the outer form at this tag).`,
        });
      }
      stack.push({ line: lineNum, indent });
      pos = openIdx + 5;
    }

    // Find `</form>` closes on this line.
    let closePos = 0;
    while (closePos < line.length) {
      const closeIdx = line.indexOf('</form>', closePos);
      if (closeIdx < 0) break;
      if (stack.length === 0) {
        violations.push({
          file,
          line: lineNum,
          indent: closeIdx,
          rule: 'form-close-without-open',
          note: `</form> at line ${lineNum} has no matching <form>; the templ source has an unclosed form somewhere.`,
        });
      } else {
        stack.pop();
      }
      closePos = closeIdx + 7;
    }
  }

  // Any leftover open forms are unclosed.
  for (const open of stack) {
    violations.push({
      file,
      line: open.line,
      indent: open.indent,
      rule: 'form-open-without-close',
      note: `<form> at line ${open.line} never closes; check the templ source for a missing </form>.`,
    });
  }

  return violations;
}

// ---- Main ----

function main() {
  console.log('=== lint-no-nested-forms ===');
  console.log('R1: nested-form-open — <form> tag opens while another <form> is still on the stack. HTML5 forbids this; the parser auto-closes the outer form at the inner form\'s open tag.');
  console.log('R2: form-close-without-open — </form> without a matching <form>.');
  console.log('R3: form-open-without-close — <form> never closes.');
  console.log('');

  const files = listTemplFiles(TEMPL_DIR);
  console.log(`Found ${files.length} .templ file(s) under ${relative(ROOT, TEMPL_DIR)}.`);
  const violations = [];

  for (const f of files) {
    const text = readFileSync(f, 'utf8');
    const blocks = splitTemplBlocks(text);
    let blockCount = 0;
    for (const block of blocks) {
      blockCount++;
      const found = findNestedForms(block, f);
      for (const v of found) violations.push(v);
    }
  }

  if (violations.length === 0) {
    console.log('  [ok]   no nested <form> tags in any .templ source');
    console.log('');
    console.log('✓ lint-no-nested-forms clean. No nested forms.');
    process.exit(0);
  }

  console.log('');
  for (const v of violations) {
    const rel = relative(ROOT, v.file).split(sep).join('/');
    console.log(`  ${rel}:${v.line} | ${v.rule} | ${v.note}`);
  }
  console.log('');
  console.log(`Found ${violations.length} violation(s).`);
  if (STRICT) {
    console.log('');
    console.log('--strict: treating as a CI failure.');
    process.exit(1);
  }
  process.exit(0);
}

main();
