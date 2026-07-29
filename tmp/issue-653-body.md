## Symptom

Exporting a single Person Record to PDF from the DixieData desktop app
yields a corrupted PDF. User-reported: "When I export a single soldier
pdf via DD the resulting pdf is corrupted somehow."

## Scope (so far)

- Single-soldier PDF export: `handleSoldierPDF` in `internal/appshell/app.go` (~line 760) and `handleSoldierPDFNoImages` (~line 810), both calling `a.export.ExportSoldierPDF` / `ExportSoldierPDFWithoutImages`.
- Bulk DB export (`/export/database-pdf`) and the Event / Article / Anniversary single-record exports are NOT in the report and stay out of scope until repro confirms the corruption mode.

## Working hypothesis (unverified)

`exportSingleRecordViaRegistry` in `internal/archive/export_service.go` (~line 424) opens the user-chosen output file via `os.Create`, then calls `e.registry.Render(...)` which streams typst output into the writer. If typst fails mid-compile, the file is left partially-written on disk and **not** cleaned up — contrast with the bulk path `exportFullDatabasePDFViaRegistry` (~line 350) which does:

```go
if err := e.registry.Render(ctx, settings, "bulk", data, f); err != nil {
    os.Remove(outputPath)
    return err
}
if err := f.Close(); err != nil {
    os.Remove(outputPath)
    return err
}
```

The single-record path returns the error directly with no `os.Remove` and no explicit `f.Close()` before the deferred close runs. On a typst compile error the user gets a half-written PDF at the path they picked — the symptom they would describe as "corrupted."

This is one hypothesis, not a confirmed root cause. Could also be: a typst template error only triggered by the single-record data shape (vs. the bulk shape that ships all records at once), a content-stream corruption in `templates/soldier_landscape.typ` / `soldier_portrait.typ`, an image-staging race, or a Wails asset-server quirk when writing to a user-chosen path.

## Repro (asks for the reporter)

- Build: `just build` (or `make build`).
- Run: open any Person Record → "Export PDF" button.
- Resulting file: open in a PDF viewer; check if it's truncated, opens with "file is damaged" warning, or just shows blank pages.
- Try both Portrait and Landscape if available.
- Try "Export without images" as well — separates the image-staging hypothesis from the typst-content one.

## Triage asks

- [ ] Exact corruption shape (truncated / won't open / blank / wrong content)?
- [ ] App version (`make version` or About screen) and commit hash?
- [ ] OS + Wails version?
- [ ] Repro: same soldier via bulk DB PDF works? Same soldier via another app (e.g. Preview) opens?
- [ ] Output file size compared to a known-good export?
- [ ] First 4 bytes of the file (should be `%PDF`)?
- [ ] Any entries in Debug Console (slog ring buffer) around the export?

## Suggested next steps (post-triage)

1. Reproduce against a real archive; capture a broken file in `tmp/`.
2. Diff the single-export codepath against the bulk codepath in `internal/archive/export_service.go` for missing cleanup.
3. Run with `TYPST_KEEP_WORKDIR=/tmp/typst-dump` to inspect the intermediate `out.pdf` typst actually produced — that separates "typst wrote garbage" from "Go wrote garbage to the user file."
4. Add a regression test under `internal/archive/export_service_test.go` that drives `ExportSoldierPDF` against a typst-injected error (e.g. via a stubbed Registry) and asserts `outputPath` is removed.

## Out of scope

- Event / Article / Anniversary single-record exports until repro confirms they share the corruption mode.
- Bulk DB export.
- Static Archive PDF / HTML export.
- The deprecated fpdf path (removed per commit `9f339fc8`).
