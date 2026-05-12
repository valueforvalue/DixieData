# Changelog

## Unreleased

- Added persistent image rotation controls in the image viewer overlay.
- Added native image import access in the soldier form flow and improved image-management behavior.
- Added soldier fields for middle name, rank in, rank out, and pension state across schema, forms, detail views, search, and exports.
- Hardened soldier loading so legacy rows with `NULL` values in new columns do not crash the app.
- Added an explicit `None` option for pension state selection.
