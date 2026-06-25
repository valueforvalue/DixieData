# Bundled Typst binary

This directory contains the Typst compiler binary used by the PDF/HTML
export pipeline. The binary is invoked by `internal/render` (via
`go-typst` or direct shell-out) to compile `.typ` templates to PDF.

## Files

| File | Target platform |
|---|---|
| `typst-windows.exe` | Windows x86_64 |
| `typst-macos` | macOS Apple Silicon (aarch64) — *see gap below* |
| `typst-linux` | Linux x86_64 (musl) — *see gap below* |
| `LICENSE`, `NOTICE` | Apache-2.0 license from the Typst upstream |
| `README.md` | This file |

**Current gap:** as of 2026-06-24 only `typst-windows.exe` is present in this directory.
macOS and Linux binaries are tracked in `bin/MANIFEST.md` (added with issue #109) and will
land here once the Typst bootstrap script can run on each platform. Until then, builds for
those targets must install Typst system-wide and the build script will fall back to the
system `typst` binary.

## Version

Typst 0.15.0 (released 2026-06-15). The version is pinned; a Typst
upgrade requires re-downloading and committing the new binary.

## Source

https://github.com/typst/typst/releases/tag/v0.15.0

The binaries are downloaded directly from the upstream release. To
upgrade, replace the files in this directory with the new release's
binaries and update the version in `docs/THIRDPARTY.md`.

## Why commit the binary

The app works out of the box without requiring users to install Typst
themselves. The Typst binary is ~50 MB per platform; the total commit
size for all three platforms is ~150 MB. This is a one-time install
cost paid by the developer.

## License

Typst is licensed under Apache-2.0. The LICENSE and NOTICE files in
this directory are unmodified copies from the upstream release.
