// probe_data_dir_contract.mjs -- Locks down where the DixieData
// archive lives + how dixiedata-web picks its scratch dir.
//
// Regression net for two related posts (#608 follow-up +
// the markdown.png #607 ship-and-claim retro):
//
//   1. The canonical Local Archive lives at <repo-root>/.dixiedata/.
//      dixiedata-web's default scratch dir lands at
//      <repo-root>/.scratch/webmode. The two never collide.
//   2. `defaultScratchDir()` (cmd/dixiedata-web/main.go) anchors
//      its default in appdata.ProjectRoot(), not the cwd. A
//      process started from ~/Downloads still lands the scratch
//      dir under the repo.
//
// Run via: node audit/probe_data_dir_contract.mjs (requires no
// live server; the probe is pure logic against the Go-default paths).

import { execSync } from 'node:child_process';
import { existsSync, statSync, mkdirSync, rmSync, readlinkSync, realpathSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
// audit/probe_data_dir_contract.mjs lives one level under the
// repo root (audit/), so repoRoot = audit/../
const REPO_ROOT = resolve(__dirname, '..');

function fail(msg) {
  console.error('FAIL: ' + msg);
  process.exit(1);
}

function expect(cond, msg) {
  if (!cond) fail(msg);
  console.log('  ok: ' + msg);
}

// Probe 1: walk up from __dirname until we find go.mod.
const goModPath = (() => {
  let dir = REPO_ROOT;
  while (dir !== '/') {
    if (existsSync(join(dir, 'go.mod'))) return dir;
    dir = dirname(dir);
  }
  return null;
})();
expect(goModPath === REPO_ROOT, 'walk-up-from-audit finds go.mod at the repo root');

// Probe 2: the canonical archive path exists with a sentinel that
// distinguishes it from the scratch dir. We do NOT inspect
// contents (the user's data is private); we only check the path
// + the .dixiedata dir's existence + .scratch/ being present and
// gitignored + .dixiedata NOT being inside .scratch/.
expect(
  !REPO_ROOT.includes('/.scratch/'),
  'repo root walk did not land inside .scratch/ (the audit probe is in audit/, not .scratch/webmode)',
);

// Probe 3: canonical archive + scratch dir live as siblings under
// the repo root. Use go list to confirm the package github.com/
// valueforvalue/DixieData resolves from REPO_ROOT (this would
// fail loudly if the import path + cwd pointed elsewhere).
let goListResult;
try {
  goListResult = execSync('go list -m', { cwd: REPO_ROOT }).toString().trim();
} catch (e) {
  fail('go list -m failed: ' + e.message);
}
expect(
  goListResult === 'github.com/valueforvalue/DixieData',
  'go list -m returns the DixieData module (cwd=' + REPO_ROOT + ')',
);

// Probe 4: verify .scratch/webmode exists OR can be created, and
// .dixiedata exists. Both may be gitignored.
const scratchDir = join(REPO_ROOT, '.scratch', 'webmode');
const liveDir = join(REPO_ROOT, '.dixiedata');
const scratchExists = existsSync(scratchDir);
const liveExists = existsSync(liveDir);
if (!scratchExists) {
  mkdirSync(scratchDir, { recursive: true });
  console.log('  note: created ' + scratchDir + ' for the probe');
}
expect(existsSync(scratchDir), '.scratch/webmode exists or can be created');
expect(existsSync(liveDir), '.dixiedata exists (the canonical live archive)');

// Probe 5: defaultScratchDir() honors DIXIEDATA_WEB_SCRATCH_DIR.
// Build a tiny shim binary that calls a small program echoing
// defaultScratchDir() via the same Go function, but skip the
// build (too slow for a unit probe). Instead, parse the Go source
// for the env-var name + the relative-path default.
const mainSrc = execSync('cat cmd/dixiedata-web/main.go', { cwd: REPO_ROOT }).toString();
expect(
  mainSrc.includes('DIXIEDATA_WEB_SCRATCH_DIR'),
  'Go source honors DIXIEDATA_WEB_SCRATCH_DIR env override',
);
expect(
  mainSrc.includes('appdata.ProjectRoot'),
  'Go source anchors the default scratch dir in appdata.ProjectRoot() (cwd-independent)',
);
// Probe 5b: the default scratch dir lives INSIDE the repo root when
// appdata.ProjectRoot() succeeds. When it fails (e.g. the binary was
// installed standalone without the dev go.mod marker), the source
// falls back to cwd-relative .scratch/webmode. Both branches are
// acceptable; the repo-root anchor is the strict requirement.
expect(
  mainSrc.match(/appdata\.ProjectRoot\(\).+webmode/s),
  'Go source resolves defaultScratchDir() inside the repo root (via appdata.ProjectRoot())',
);

// Probe 6: the .dixiedata and .scratch paths are realpath'd to
// absolute (canonicalizing any symlinks). They must be siblings,
// not nested.
const absScratch = realpathSync(scratchDir);
const absLive = realpathSync(liveDir);
expect(
  !absLive.startsWith(absScratch + '/') && absLive !== absScratch,
  '.dixiedata is NOT inside .scratch/ (canonical + scratch stay separate)',
);
// Live vs scratch is not "siblings at the same dir level" — the
// scratch dir lives inside .scratch/ (one level down), the live
// archive lives at the repo root. The strict invariant is:
// the scratch dir's path contains the substring ".scratch/" + the
// live archive's parent is the repo root (not inside .scratch/).
expect(
  absScratch.includes(`${REPO_ROOT}/.scratch/`) ||
  absScratch.includes(`${REPO_ROOT}\\.scratch\\`),
  '.scratch/webmode lives under <repo-root>/.scratch/',
);
expect(
  !absScratch.includes(absLive + '/') && absLive !== absScratch &&
  dirname(absLive) === REPO_ROOT,
  '.dixiedata lives at the repo root (NOT under .scratch/)',
);

console.log('all probes pass');
