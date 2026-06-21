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
- **Used by:** `pkg/render` (via direct `os/exec` calls) to compile
  `.typ` templates to PDF.

The Typst binary is invoked via `exec.Command`, which shells out to
the bundled `typst-windows.exe` (or `typst` on PATH if the bundled
binary is absent). On Windows the child process is created with
`CREATE_NO_WINDOW` / `HideWindow` so the user does not see a black
console window during PDF export.

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
