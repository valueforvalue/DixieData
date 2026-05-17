# Changelog

## v1.1.19 - Patch Release

- Fixed the new-record localStorage draft flow so successful creates clear the cached entry instead of repopulating the next record form.
- Added an in-app **Discard local draft** recovery action on new/edit record forms so stuck entry drafts can be cleared without DevTools or a debug build.
- Enabled Confederate Home fields for wife and widow records in the entry form.
- Carried the release line forward to `v1.1.19` so the schema version, runtime metadata, Wails title, and packaged release artifacts stay aligned.

## v1.1.18 - Full Release

- Hardened `.ddbak`, `.ddshare`, diagnostics, and static archive ZIP creation to write through a temp file, verify ZIP finalization, and only then replace the destination file.
- This avoids success-shaped partial archives caused by unverified final ZIP close/flush behavior at the final save path.

## v1.1.17 - Patch Release

- Fixed the static web archive detail view so exported `index.html` and `viewer.html` can open a selected person without leaving the expanded data area blank.
- Carried the release line forward to `v1.1.17` so the runtime metadata, Wails title, exported artifacts, and docs stay aligned.

## v1.1.16 - Gold Master

- Synced the production version line to `v1.1.16` so the schema version, runtime metadata, and Wails title all report the same release.
- Added Smart Back behavior that preserves browse context when returning from record detail and edit surfaces.
- Expanded archive search with FTS5-backed quick search, scratch-pad indexing, recent-record defaults, and advanced filters for entry type and review state.
- Hardened spouse and entry-type workflows across create, edit, detail, export, and review flows.
- Added persistent image rotation controls, native image import, and sharded image storage for better large-archive scaling.
- Added gold-master validation tooling for outputs, stress coverage, and archive portability auditing.
- Converted `.ddshare` archives into merge-ready record packages with referenced-image bundling and receiver-namespace ID regeneration.
- Preserved `.ddbak` as the full replacement backup format with schema-aware manifest metadata.
