# dixiedata-rc-manifest

The DixieData RC cohort's in-place update manifest. The cohort's
updater polls this file; when a new RC ships, the operator bumps
the JSON here. Default users (no custom `update_source_url`) are
unaffected — they continue to point at GitHub's `/releases/latest`.

## File: `manifest.json`

Shape matches DixieData's `internal/update/updateManifest`:

```json
{
  "version": "1.1.4-rc1",
  "asset_url": "https://github.com/valueforvalue/DixieData/releases/download/v1.1.4-rc1/DixieData-release-v1.1.4-rc1.zip",
  "sha256": "<hex-encoded SHA256 of the zip>",
  "release_notes": "Free-form text shown in the cohort's updater UI.",
  "published_at": "<RFC3339 timestamp>"
}
```

- `version` is parsed by the updater's `versionFromString` regex
  which captures only the 3 numeric segments; the `-rc1` suffix
  is stripped before the numeric comparison. A cohort on
  `1.1.4-rc1` parses to `1.1.4`; a newer manifest advertising
  `1.1.4-rc2` ALSO parses to `1.1.4` — same numeric value, so
  the updater's numeric comparison is a no-op.
- The "newer than current" check uses string comparison on the
  *raw* `version` field BEFORE the regex strips the suffix. See
  the cohort workflow section below for how the cohort actually
  receives new RCs.
- `asset_url` must be the full URL to the zip (GitHub release
  asset URL works).
- `sha256` is optional but recommended; the updater verifies it
  on download.

## Operator workflow (RC cut + cohort updates)

From the DixieData repo root, on `dev`:

1. **Tag the RC** — `git tag v1.1.4-rc1 && git push origin v1.1.4-rc1`
2. **Build the RC zip** — `DIXIEDATA_RELEASE_TAG=rc1 just archive`
   (or `pwsh -File scripts/build-release.ps1 -Archive -LDFlags "-X github.com/valueforvalue/DixieData/internal/versioninfo.CurrentReleaseTag=rc1"`)
3. **Upload the zip** — attach `build/bin/...zip` to the GitHub
   release at `v1.1.4-rc1` (use `gh release create` with
   `--draft` for safety, then publish when ready).
4. **Compute SHA256** — `sha256sum DixieData-release-v1.1.4-rc1.zip`
5. **Edit `manifest.json`** — bump `version` to `1.1.4-rc2`, set
   `asset_url` to the new release's asset URL, set `sha256` to
   the new hash, update `release_notes` + `published_at`.
6. **Commit + push** — `git add manifest.json && git commit -m "RC2" && git push`

The cohort's updater polls the manifest URL on its next
`update_source_url`-driven check (the default poll interval
applies). When the manifest's `version` field is a different
string from the installed version, the cohort sees the update.

## Cohort opt-in (per user machine)

A user opts into the RC channel by setting
`update_source_url` in DixieData's Settings panel (or via the
config file at `<dataDir>/config.json`) to:

```
https://raw.githubusercontent.com/valueforvalue/dixiedata-rc-manifest/main/manifest.json
```

After that one-time change, every DixieData launch polls the
manifest. Bumping the manifest from the operator's side is the
only action needed to ship a new RC.

## Promoting RC → stable

When the RC cohort is satisfied:

1. Cut a stable release from the same commit (no `-ldflags
   ...CurrentReleaseTag=...`): `just archive` then publish as
   a new GitHub release tag (e.g. `v1.1.4`).
2. Update the cohort's manifest to point at the stable zip, or
   flip their `update_source_url` back to the default
   (`https://api.github.com/repos/valueforvalue/DixieData/releases/latest`).

## See also

- [`docs/RELEASING.md`](https://github.com/valueforvalue/DixieData/blob/dev/docs/RELEASING.md) §"RC cut workflow" — operator-facing steps from the DixieData side.
- [`internal/update/updater.go`](https://github.com/valueforvalue/DixieData/blob/dev/internal/update/updater.go) — the updater's manifest parser + version regex.
- Issue #654 (RC workflow) — initial design + the cohort opt-in rationale.
