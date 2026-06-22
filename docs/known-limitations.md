# Known Limitations

Operational break points and constraints discovered while building the
DixieData stress suite (`tests/stress/`). This file is documentation, not a
source of truth for tests — the stress suite itself lives in `tests/stress/`.

## Stress-suite break points

1. `soldiers.added_by` does not exist in the current schema.
   - The migration stress pass can verify `prefix` and `suffix`, but `added_by` cannot be preserved because the field is not implemented in the current database model.

2. Backup/import hardening relies on structure and count checks, not cryptographic checksums.
   - Poisoned `.ddbak`/`.ddshare` archives are rejected for bad manifests, missing files, bad archive kinds, future schema versions, and count mismatches, but there is no checksum-based tamper proofing yet.

3. Data-directory disappearance/read-only transitions are still operational break points.
   - The suite verifies that save/search paths fail with surfaced errors instead of panicking, but removing or locking `.dixiedata` during runtime remains a real-world failure mode.

4. Aggressive concurrent writes can surface `database is locked` contention.
   - The first bridge hammer draft found SQLite write-lock pressure immediately, so the stable race-hunter now keeps the 50+ simultaneous load on search traffic and records write contention as a known break point.

5. Large note/record payloads can pressure page-fitting and file-handling paths.
   - The suite now stress-tests 10,000+ byte payloads, malformed HTML, and oversized dummy files because these are the fastest ways to expose layout, import, and parser edge cases.

## See also

- `tests/stress/` — Go integration + stress tests, `analyze_stress_log.py`
- `scripts/run-stress-tests.ps1` — local stress harness entry point
- `docs/audit/` — earlier layout-theming audits; see `layout-theming-findings.md`