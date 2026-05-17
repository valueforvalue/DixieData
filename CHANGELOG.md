# Changelog

## v1.1.16 - Gold Master

- Synced the production version line to `v1.1.16` so the schema version, runtime metadata, and Wails title all report the same release.
- Added Smart Back behavior that preserves browse context when returning from record detail and edit surfaces.
- Expanded archive search with FTS5-backed quick search, scratch-pad indexing, recent-record defaults, and advanced filters for entry type and review state.
- Hardened spouse and entry-type workflows across create, edit, detail, export, and review flows.
- Added persistent image rotation controls, native image import, and sharded image storage for better large-archive scaling.
- Added gold-master validation tooling for outputs, stress coverage, and archive portability auditing.
- Converted `.ddshare` archives into merge-ready record packages with referenced-image bundling and receiver-namespace ID regeneration.
- Preserved `.ddbak` as the full replacement backup format with schema-aware manifest metadata.
