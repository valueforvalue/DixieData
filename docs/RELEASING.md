# Releasing DixieData

DixieData's release line has three independent counters, all declared in `internal/versioninfo/versioninfo.go` (issue #266):

- **`CurrentSchemaVersion`** — SQLite `user_version`; the data plane. Bump = the migration runner applies forward-only migrations. Most common bump.
- **`CurrentUpdateFlowVersion`** — the update-flow shape gate (U). Bump = the auto-update mechanism itself changed shape (restore-point storage moves, eligibility rules change, in-place codepaths removed). Releases with a higher U cannot be auto-applied by a binary with the current U; the user must reinstall.
- **`CurrentAppVersionInt`** — the release counter (N). Every release bumps N; bug-fix-only releases bump N without a schema change.

The app version is the composite `v{MAJOR}.{U}.{N}` and is embedded in the binary, packaged in the release zip, stamped on every GitHub release, and parsed by the in-place update flow's `compareVersions` to decide eligibility.

The local update feature (`internal/appdata/`) downloads release packages and applies schema migrations on top of an existing `.dixiedata` database. **Every schema bump must be paired with a migration in `internal/db/schema.go` AND a human-readable note in `docs/migrations/`.** Without the migration, users on older DBs cannot upgrade via the update flow.

## Versioning rules

- **App version**: `v{MAJOR}.{U}.{N}` where `MAJOR` is fixed at 1 today (TBD bump policy), `U` is the update-flow version, `N` is the release counter. Initial release is `v1.1.1` (U=1, N=1). Legacy releases (v1.2.52, v1.2.55, ...) parse to **U=1** by default per issue #266 decision 1, so they slot in cleanly alongside the new shape.
- **U bump semantics**: U mismatch in either direction (release.U > installed.U OR release.U < installed.U) forces the user to reinstall. The installed binary's update-flow shape can't safely apply a release whose flow changed. U bumps are rare — reserve them for changes that genuinely reshape the in-place update mechanism.
- **N bump semantics**: every release bumps N. Bug-fix-only releases bump N without touching schema or U. The release counter is independent of the data plane.
- **Schema bump semantics**: `-BumpSchema` bumps `CurrentSchemaVersion`; requires a paired `docs/migrations/v{N+1}.md` with at least one `- ` bullet.
- **Bump increment**: always `+1` per release for each counter. `bump-version.ps1` refuses jumps greater than `+1` unless `-Force` is passed.
- **Migration note**: `docs/migrations/v{N+1}.md` must exist before `-BumpSchema` will run.

## Release workflow

### 1. Pick the right bump

Decide which counter is changing:

- **Schema changed** (new table, new column, drop, rename, index, modified JSON shape): `-BumpSchema`.
- **Update flow changed** (new restore-point storage location, new eligibility rule, new migration runner mechanics, removal of an in-place codepath): `-BumpUpdateFlow`. Bumps U, resets N to 0, archives the previous U sequence's last N to `.release-state/last-n-for-u{prev_U}.json`.
- **Bug-fix only** (no schema change, no update-flow change): `-BumpRelease`. Bumps N only.

### 2. Write the migration note (schema bumps only)

Before bumping, describe the schema change in `docs/migrations/v{N+1}.md`. This note:

- documents the schema change for reviewers and the update flow
- serves as the audit trail when users apply the update
- is required by `-BumpSchema` — the script will refuse to run without it

```markdown
# Schema v55

- Added `merge_review_conflicts.notes` column for reviewer context.
```

### 3. Update CHANGELOG.md

Add a new section at the top:

```markdown
## v1.1.55 - Patch Release

- Added merge-review conflict notes column.
- Carried the release line forward to `v1.1.55` so the schema version,
  runtime metadata, Wails title, and packaged release artifacts stay aligned.
```

### 4. Bump the counter

```bash
# Schema bump (default; matches historical behavior)
make bump

# Or explicitly:
pwsh -File scripts/bump-version.ps1 -BumpSchema
pwsh -File scripts/bump-version.ps1 -BumpUpdateFlow
pwsh -File scripts/bump-version.ps1 -BumpRelease
```

The script:

- verifies the paired migration note exists for `-BumpSchema`
- refuses bumps greater than `+1` (use `-Force` to override)
- rewrites the appropriate constant in `internal/versioninfo/versioninfo.go`
- prints the new app version (`v1.{U}.{N}`) and next-step instructions

`make bump` does NOT auto-commit. The reviewer must:

- edit CHANGELOG.md (already done in step 3)
- run `make test-quiet` to confirm migrations apply cleanly
- commit deliberately: `git add internal/versioninfo/versioninfo.go CHANGELOG.md docs/migrations/v55.md && git commit -m "Bump release line to v1.1.55"`

### 5. Build and archive

```bash
make archive
```

Produces `release/DixieData-release-v1.1.55.zip` containing the contents of `build\bin\` (`DixieData.exe`, `google-oauth-defaults.json`, `pdfium.dll`, `pdfium.version`).

### 6. Promote dev → stable + tag and publish (draft)

Per ADR 0009, releases ship from `stable` (the released-code
branch), not `main`. The chain is:

```bash
# 6a. Dry-run the gate chain (no push, no PR)
make promote-dry-run

# 6b. Open the promotion PR dev → stable
make promote

# 6c. Review + merge the PR via the GitHub UI
#     (operator is the merge authority per ADR 0009)

# 6d. Post-merge sanity: stable HEAD matches the merge SHA
make promote-confirm

# 6e. Tag + push + draft gh release
make release-github
```

`make promote` calls `scripts/promote-open-pr.sh`, which
embeds the gate-chain output + commit log + diff stat in the
PR body. The PR title is `promote: dev → stable (v{VERSION})`.

After the PR is merged, `make release-github` calls
`scripts/release-github.ps1`, which enforces five safety gates
before any mutation:

1. Working tree is clean.
2. `internal/versioninfo/versioninfo.go` is committed (matches HEAD).
3. `release/DixieData-release-v{VERSION}.zip` exists.
4. Tag `v{VERSION}` does not exist locally or on origin.
5. `gh` CLI is authenticated.

On success:

- `git tag -a v{VERSION} -m "Release v{VERSION}"`
- `git push origin HEAD` (the current branch, which is `stable`
  after the promotion PR has been merged)
- `git push origin v{VERSION}`
- `gh release create v{VERSION} release/DixieData-release-v{VERSION}.zip --draft --title "DixieData v{VERSION}" --generate-notes`

The release is **DRAFT** — not publicly visible. Review notes in the GitHub UI, then publish:

```bash
gh release edit v1.1.55 --draft=false
```

### 7. Rollback

If `git push origin main` succeeds but `git push origin v{VERSION}` fails:

- the local tag is automatically deleted
- main was already pushed (no rollback on remote)
- re-run after fixing the remote issue; if a partial state persists, investigate before retrying

If `gh release create` fails after a successful tag push:

- the tag exists on origin but no release does
- create the release manually at `https://github.com/valueforvalue/DixieData/releases/new`
- select the existing tag and upload the zip

## Demo packages

`make demo` produces a seeded demo release under `release/DixieData-demo-{date}.zip` via `scripts/build-demo-release.ps1`. This is independent of the release line — demo versions do not get GitHub releases.

## Manual override

If the Makefile pipeline breaks (e.g., `gh` unavailable, network issues), fall back to the PowerShell scripts directly:

```bash
# Build + archive
pwsh -File scripts/build-release.ps1 -Archive

# Tag + push
git tag -a v1.1.55 -m "Release v1.1.55"
git push origin main
git push origin v1.1.55

# Upload release zip manually at https://github.com/valueforvalue/DixieData/releases/new
```

The Makefile and scripts are convenience wrappers; the underlying convention is the three-counter version + GitHub release zip.

## See also

- [ADR 0008 — Promotion protocol](adr/0008-promotion-protocol.md) — the `make promote` gate chain for `dev → main`, including the cadence-driven promotion story that the v{MAJOR}.{U}.{N} split enables.
- [ADR 0007 — In-place update safety](adr/0007-in-place-update-safety.md) — the four rules that gate the in-place update flow.
- [`internal/versioninfo/versioninfo.go`](../internal/versioninfo/versioninfo.go) — the source-of-truth for all three counters.
