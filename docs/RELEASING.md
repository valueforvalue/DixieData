# Releasing DixieData

DixieData's release line is schema-driven: `CurrentSchemaVersion` in `internal/versioninfo/versioninfo.go` is the single source of truth. The app version is computed as `v1.2.{CurrentSchemaVersion}` and is embedded in the binary, packaged in the release zip, and stamped on every GitHub release.

The local update feature (`internal/appdata/`) downloads release packages and applies schema migrations on top of an existing `.dixiedata` database. **Every schema bump must be paired with a migration in `internal/db/schema.go` AND a human-readable note in `docs/migrations/`.** Without the migration, users on older DBs cannot upgrade via the update flow.

## Versioning rules

- **App version**: `v{MAJOR}.{MINOR}.{SCHEMA}` where `SCHEMA = CurrentSchemaVersion`. Currently `v1.2.{N}`.
- **Bump increment**: always `+1` per release. `bump-version.ps1` refuses jumps greater than `+1` unless `-Force` is passed.
- **Migration note**: `docs/migrations/v{N+1}.md` must exist before `make bump` will run. Must contain at least one `- ` bullet.

## Release workflow

### 1. Write the migration note

Before bumping, describe the schema change in `docs/migrations/v{N+1}.md`. This note:

- documents the schema change for reviewers and the update flow
- serves as the audit trail when users apply the update
- is required by `make bump` — the script will refuse to run without it

```markdown
# Schema v55

- Added `merge_review_conflicts.notes` column for reviewer context.
```

### 2. Update CHANGELOG.md

Add a new section at the top:

```markdown
## v1.2.55 - Patch Release

- Added merge-review conflict notes column.
- Carried the release line forward to `v1.2.55` so the schema version,
  runtime metadata, Wails title, and packaged release artifacts stay aligned.
```

### 3. Bump the schema version

```bash
make bump
```

This calls `scripts/bump-version.ps1`, which:

- verifies `docs/migrations/v{N+1}.md` exists and has at least one bullet
- refuses bumps greater than `+1` (use `-Force` to override)
- rewrites `CurrentSchemaVersion` in `internal/versioninfo/versioninfo.go`
- prints the new app version and next-step instructions

`make bump` does NOT auto-commit. The reviewer must:

- edit CHANGELOG.md (already done in step 2)
- run `make test-quiet` to confirm migrations apply cleanly
- commit deliberately: `git add internal/versioninfo/versioninfo.go CHANGELOG.md docs/migrations/v55.md && git commit -m "Bump release line to v1.2.55"`

### 4. Build and archive

```bash
make archive
```

Produces `release/DixieData-release-v1.2.55.zip` containing the contents of `build\bin\` (`DixieData.exe`, `google-oauth-defaults.json`, `pdfium.dll`, `pdfium.version`).

### 5. Tag and publish (draft)

```bash
make release-github
```

This calls `scripts/release-github.ps1`, which enforces five safety gates before any mutation:

1. Working tree is clean.
2. `internal/versioninfo/versioninfo.go` is committed (matches HEAD).
3. `release/DixieData-release-v{VERSION}.zip` exists.
4. Tag `v{VERSION}` does not exist locally or on origin.
5. `gh` CLI is authenticated.

On success:

- `git tag -a v{VERSION} -m "Release v{VERSION}"`
- `git push origin main`
- `git push origin v{VERSION}`
- `gh release create v{VERSION} release/DixieData-release-v{VERSION}.zip --draft --title "DixieData v{VERSION}" --generate-notes`

The release is **DRAFT** — not publicly visible. Review notes in the GitHub UI, then publish:

```bash
gh release edit v1.2.55 --draft=false
```

### 6. Rollback

If `git push origin main` succeeds but `git push origin v{VERSION}` fails:

- the local tag is automatically deleted
- main was already pushed (no rollback on remote)
- re-run after fixing the remote issue; if a partial state persists, investigate before retrying

If `gh release create` fails after a successful tag push:

- the tag exists on origin but no release does
- create the release manually at `https://github.com/valueforvalue/DixieData/releases/new`
- select the existing tag and upload the zip

## Demo packages

`make demo` produces a seeded demo release under `release/DixieData-demo-{date}.zip` via `scripts/build-demo-release.ps1`. This is independent of the schema-driven release line — demo versions do not get GitHub releases.

## Manual override

If the Makefile pipeline breaks (e.g., `gh` unavailable, network issues), fall back to the PowerShell scripts directly:

```bash
# Build + archive
pwsh -File scripts/build-release.ps1 -Archive

# Tag + push
git tag -a v1.2.55 -m "Release v1.2.55"
git push origin main
git push origin v1.2.55

# Upload release zip manually at https://github.com/valueforvalue/DixieData/releases/new
```

The Makefile and scripts are convenience wrappers; the underlying convention is the schema-driven version + GitHub release zip.
