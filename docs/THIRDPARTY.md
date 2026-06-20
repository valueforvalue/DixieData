# Third-party dependencies

DixieData uses the following third-party components that are not
vendored as Go modules.

## Typst

- **Version:** 0.15.0 (released 2026-06-15)
- **License:** Apache-2.0
- **Source:** https://github.com/typst/typst/releases/tag/v0.15.0
- **Bundled binaries:**
  - `bin/typst-windows.exe` (Windows x86_64)
  - `bin/typst-macos` (macOS Apple Silicon, aarch64)
  - `bin/typst-linux` (Linux x86_64, musl)
- **Used by:** `internal/render` (via `github.com/Dadido3/go-typst`) to
  compile `.typ` templates to PDF.

The Typst binary is invoked via `go-typst`, which shells out to the
`typst` CLI on PATH or to a configured `ExecutablePath`. The bundled
binary means the app works out of the box without a separate Typst
install.

The license text and notice from the Typst upstream are committed at
`bin/LICENSE` and `bin/NOTICE`.

## Liberation Fonts

- **Version:** (bundled with Typst 0.15)
- **License:** SIL Open Font License 1.1
- **Source:** shipped with Typst as the default font family

The `Libertinus Serif` font is used as the default Typst font for
templates that don't specify a font family. Templates can override
this in their `#set text(font: ...)` call.

## Update procedure

1. Watch the Typst release page for new versions.
2. Download the new binaries from the matching assets.
3. Replace the files in `bin/`.
4. Update the version in this file.
5. Re-run the smoke test (`go test ./pkg/render/...`) to confirm
   the new binary still works.
