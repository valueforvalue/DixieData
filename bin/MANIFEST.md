# Bin/Manifest — vendored + downloaded binaries

This manifest tracks every native binary the build pipeline expects in `bin/`.
Each entry pins a version, a source URL, and a SHA256 so a fresh clone can
reproduce the build deterministically (within Windows; macOS/Linux land with
issue #109 follow-up).

## Vendored (committed)

These binaries are checked in to the repo so a fresh clone can build without
network access. Upgrades require a PR.

| Binary | Version | Source | SHA256 |
|---|---|---|---|
| `typst-windows.exe` | Typst v0.15.0 | <https://github.com/typst/typst/releases/tag/v0.15.0> | `b561e8bbcccb0caaa665831d9fe08136eb47761b8ea5c2d8209ad64e76db5963` |

## Downloaded (reproducible via scripts/build-common.ps1)

These binaries are not committed. `Restore-DixieDataPdfiumBinary` (in
`scripts/build-common.ps1`) downloads them on demand into `build/bin/`,
verifies the pinned SHA256, and refuses to install on mismatch.

| Binary | Version | Source | SHA256 |
|---|---|---|---|
| `pdfium.dll` | `chromium/7857` | <https://github.com/bblanchon/pdfium-binaries/releases/tag/chromium/7857> | `ebddbc781afbffb6f76c8e674e5900665a8676e778a91c4130b9afcb4a8a812a` |

If the upstream tag is renamed or replaced, the build script throws a clear
error naming the stale version and pointing at this manifest.

## Upgrade procedure

1. Identify the new upstream version (release notes / changelog).
2. Download the new archive from the upstream source listed above.
3. Extract the binary and compute `certutil -hashfile <binary> SHA256`.
4. Update **all three** of:
   - `scripts/build-common.ps1` (`Get-DixieDataPdfiumVersion`,
     `Get-DixieDataPdfiumExpectedHash`, `Get-DixieDataTypstVersion`,
     `Get-DixieDataTypstExpectedHash` — whichever is changing).
   - This manifest (table row + SHA256).
   - `bin/README.md` if the upgrade changes platform coverage or version
     claims.
5. For Typst, also commit the new `typst-windows.exe` to `bin/`.
6. Run a clean build: `make clean && make archive`.
7. Open a PR with all three files in one commit.

## Adding a new binary

Append a row to the appropriate table, add a `<Get-DixieDataXxxVersion>` and
`<Get-DixieDataXxxExpectedHash>` pair in `scripts/build-common.ps1`, and have
the restore function refuse to install on hash mismatch. Never ship a binary
without pinning its hash.
