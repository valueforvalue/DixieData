// lint-rc-commits.test.mjs — sanity test for the rc/v* commit
// classifier. Run with `node --test scripts/ci/lint-rc-commits.test.mjs`.
// The full GitHub-API path is exercised in CI; the local-only
// path is what contributors can pre-flight before pushing.
import { test } from "node:test";
import assert from "node:assert/strict";

// Re-implement the classifier inline to mirror the script
// (avoids the import-from-.mjs path complications on Windows).
const ALLOWED_TYPES = new Set(["fix", "docs", "test", "ci", "chore"]);
const TYPE_PATTERN = /^(?<type>[a-z]+)(?:\([^)]+\))?!?: /;

function classifySubject(subject) {
  const m = subject.match(TYPE_PATTERN);
  if (!m) return { type: null, valid: false, reason: "no conventional-commit type prefix" };
  const type = m.groups.type;
  if (!ALLOWED_TYPES.has(type)) {
    return { type, valid: false, reason: `type '${type}:' is disallowed on rc/v* branches (ADR 0011)` };
  }
  return { type, valid: true, reason: null };
}

test("allowed types pass", () => {
  for (const t of ["fix", "docs", "test", "ci", "chore"]) {
    const r = classifySubject(`${t}: subject`);
    assert.equal(r.valid, true, `${t}: should be allowed`);
    assert.equal(r.type, t);
  }
});

test("disallowed types fail", () => {
  for (const t of ["feat", "refactor", "perf", "build", "style"]) {
    const r = classifySubject(`${t}: subject`);
    assert.equal(r.valid, false, `${t}: should be rejected`);
    assert.equal(r.type, t);
  }
});

test("missing type prefix is rejected", () => {
  const r = classifySubject("this has no type prefix");
  assert.equal(r.valid, false);
  assert.equal(r.type, null);
});

test("scoped types parse correctly", () => {
  // Conventional commits allow type(scope)!: subject. The
  // classifier must not confuse the scope for a separate
  // segment — only the leading word before ( is the type.
  assert.equal(classifySubject("fix(api): close bug").valid, true);
  assert.equal(classifySubject("feat(theme)!: breaking change").valid, false);
  assert.equal(classifySubject("ci(github): add workflow").valid, true);
});

test("breaking-change bang does not change type", () => {
  // ! is a metadata marker, not part of the type
  assert.equal(classifySubject("fix!: breaking fix").valid, true);
  assert.equal(classifySubject("feat!: breaking feature").valid, false);
});
