// audit/ci_race_scope.test.mjs — regression net for issue #479.
//
// Pins the CI race-detector gate shape so a future refactor cannot
// silently re-widen the suite (which timed out at ~10min on the
// appshell package under -race due to seed.Generate's 6400-row
// fixture insert) or re-add the temporary `continue-on-error: true`
// advisory that GitHub Actions emailed on every PR.
//
// What we pin:
//   - .github/workflows/test.yml has exactly ONE race-detector step
//   - the step scopes to ./pkg/... and ./internal/dates/... (not
//     the unscoped ./... that times out)
//   - the step does NOT carry `continue-on-error: true`
//   - the step has a timeout-minutes bound (currently 5)
//
// Issue #479 is closed by this gate shape; the durable fix is
// "keep the suite narrow + skip the slow tests under -short" per
// the issue's Path A guidance.
import { readFileSync } from 'node:fs';
import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { join } from 'node:path';

const repoRoot = join(import.meta.dirname, '..');
const workflowPath = join(repoRoot, '.github', 'workflows', 'test.yml');
const yaml = readFileSync(workflowPath, 'utf8');

test('ci: race-detector gate scopes to ./pkg/... + ./internal/dates/...', () => {
  // Pull the single race-detector step block out of the YAML by
  // anchoring on the step name so a future duplicate step stays
  // detectable. We don't parse YAML — a string-search is enough
  // for a regression net that owns the gate shape.
  const stepIdx = yaml.indexOf('name: Go test (short, race detector');
  assert.ok(stepIdx >= 0, 'race-detector step not found in test.yml');
  // Pull the run block that follows the step header. We grep for
  // both 'go test -race' lines so both pkg/... and internal/dates/...
  // surfaces are pinned.
  const stepEnd = yaml.indexOf('\n      - name:', stepIdx + 1);
  const stepBlock = stepEnd > 0 ? yaml.slice(stepIdx, stepEnd) : yaml.slice(stepIdx);
  assert.match(stepBlock, /go test -race \.\/pkg\/\.\.\. -short -count=1/,
    'race-detector step must scope to ./pkg/... -short -count=1');
  assert.match(stepBlock, /go test -race \.\/internal\/dates\/\.\.\. -short/,
    'race-detector step must also cover ./internal/dates/... (the rapid property-test gate)');
});

test('ci: race-detector gate does not carry continue-on-error: true', () => {
  const stepIdx = yaml.indexOf('name: Go test (short, race detector');
  assert.ok(stepIdx >= 0, 'race-detector step not found in test.yml');
  const stepEnd = yaml.indexOf('\n      - name:', stepIdx + 1);
  const stepBlock = stepEnd > 0 ? yaml.slice(stepIdx, stepEnd) : yaml.slice(stepIdx);
  // Strip YAML comments (lines whose first non-whitespace char is
  // '#') before scanning — the step block can mention
  // `continue-on-error: true` in a breadcrumb comment ("we used to
  // have this advisory; the narrow scope replaced it") without
  // actually carrying the directive.
  const stripped = stepBlock
    .split('\n')
    .filter((line) => !/^\s*#/.test(line))
    .join('\n');
  // Per issue #479, the temporary advisory was reverted once the
  // narrow scope landed. Future authors must not re-add it — the
  // gate is now reliable (pkg/... + dates/... finish in <30s on CI).
  assert.ok(
    !/continue-on-error:\s*true/.test(stripped),
    'race-detector step must not carry continue-on-error: true — the narrow scope is now reliable per issue #479',
  );
});

test('ci: race-detector gate has a timeout-minutes bound', () => {
  // Match the step's indent + the timeout-minutes: line. Pulls the
  // nearest timeout-minutes after the race-detector step header.
  const stepIdx = yaml.indexOf('name: Go test (short, race detector');
  assert.ok(stepIdx >= 0, 'race-detector step not found in test.yml');
  const stepEnd = yaml.indexOf('\n      - name:', stepIdx + 1);
  const stepBlock = stepEnd > 0 ? yaml.slice(stepIdx, stepEnd) : yaml.slice(stepIdx);
  assert.match(stepBlock, /timeout-minutes:\s*\d+/,
    'race-detector step must carry a timeout-minutes bound so a future regression surfaces as a workflow timeout, not a silent run');
});

test('ci: race-detector step appears exactly once', () => {
  const matches = yaml.match(/name: Go test \(short, race detector/g) || [];
  assert.equal(matches.length, 1,
    `expected exactly one race-detector step; found ${matches.length}. A second unscoped step will reintroduce the appshell seed.Generate timeout per issue #479.`);
});
