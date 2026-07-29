// audit/_lib/smoke_paths.mjs (issue #700, ADR 0011)
//
// Cross-platform resolution of the smoke runner's external
// commands (the embedded Go web binary used by audit smokes
// + the Go seed-data helper that prepares a scratch archive).
//
// Why this file is separate from audit/_lib/paths.mjs:
//   audit/_lib/paths.mjs resolves the *Wails desktop* artifact
//   (DixieData.exe / DixieData); the web binary the smokes
//   spawn lives at a different relative path (build/bin/
//   dixiedata-web vs build/bin/DixieData) and has different
//   .exe-suffix rules.
//
// The 4 existing Playwright smokes (smoke_soldier_images /
// smoke_submit_e2e / smoke_mega_menu_nav / smoke_dialog_guard
// for the static-scan variant) all hardcoded the literal
//   build/bin/dixiedata-web.exe
// and never handled the Linux build that audit.yml produces.
// The runner centralises that here so each smoke stops
// branching on platform.

import { existsSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
// __dirname = audit/_lib. Walk up two to the repo root.
export const REPO_ROOT = join(__dirname, '..', '..');

const BIN_DIR = join(REPO_ROOT, 'build', 'bin');
const WEB_BIN_WIN = join(BIN_DIR, 'dixiedata-web.exe');
const WEB_BIN_NIX = join(BIN_DIR, 'dixiedata-web');
const SEED_BIN_WIN = join(BIN_DIR, 'seed-data.exe');
const SEED_BIN_NIX = join(BIN_DIR, 'seed-data');

// webBin returns the absolute path of the embed-web binary
// for the current OS. The caller is responsible for failing
// with a clear message when the file does not exist; this
// resolver never throws so the calling layer can pick the
// better error message ("run `just debug` first" vs
// "platform not supported").
export function webBin() {
  return process.platform === 'win32' ? WEB_BIN_WIN : WEB_BIN_NIX;
}

export function seedBin() {
  return process.platform === 'win32' ? SEED_BIN_WIN : SEED_BIN_NIX;
}

export function webBinExists() {
  return existsSync(webBin());
}

export function seedBinExists() {
  return existsSync(seedBin());
}
