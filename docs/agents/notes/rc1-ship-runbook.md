# RC1 ship runbook

Runbook for shipping DixieData **v1.1.4-rc1** to the RC cohort. Read
this when you're done patching `dev` and ready to cut.

**Last updated:** 2026-07-26, against commit `0d986df2`. If the
working tree is more than ~10 commits ahead of this, re-read
`docs/RELEASING.md` §"RC cut workflow" in case the steps have
drifted; the high-level shape here is stable but the exact tool
names may have moved.

## What's already done (you don't have to redo this)

- ✅ Versioninfo plumbing: `versioninfo.CurrentReleaseTag` (build-time
  injection) + `buildinfo.AppVersionFull()` so a `-ldflags
  "...CurrentReleaseTag=rc1"` build renders "DixieData v1.1.4-rc1"
  in the chrome. Commit `70465bea`.
- ✅ Operator tooling: `bump-version.ps1 -SetReleaseTag / -ClearReleaseTag`,
  `just rc-publish TAG ZIP`, `DIXIEDATA_RELEASE_TAG=rc1 just archive`.
  Same commit.
- ✅ Manifest repo: `https://github.com/valueforvalue/dixiedata-rc-manifest`
  (public, on `main`). Live raw URL:
  `https://raw.githubusercontent.com/valueforvalue/dixiedata-rc-manifest/main/manifest.json`.
  Currently hosts a placeholder manifest advertising `1.1.4-rc1` with
  a `REPLACE_WITH_SHA256_AFTER_BUILD` placeholder.
- ✅ E2E test: `internal/update/cohort_e2e_test.go` runs on every CI
  build, fetches the live manifest URL, asserts DixieData's parser
  accepts it. Commit `0d986df2`. Catches drift in field names
  + version shape.
- ✅ Cohort = just you. (No external testers opted in yet.)

## What's NOT done — your list

### 1. Land your pending patches on dev

Normal dev flow. Commit to `dev`, push. Each patch becomes another
bullet in the `## [Unreleased]` Maintenance / Fixed sections of
`CHANGELOG.md`. The runbook below assumes you've already done this
and the last patch commit is the one you want RC1 to ship.

### 2. Pick the RC1 commit SHA

RC1 is locked to a single commit. Pick the SHA of the last patch
you want included. Don't cut RC1 from `dev` HEAD with patches still
landing — every push after the cut invalidates the next RC.

Recommended: tag the commit on `dev`, then build from the tagged
commit. Subsequent `dev` commits become RC2 territory.

### 3. Pre-flight (run all, in order)

```bash
cd /d C:\Development\DixieData
git fetch origin
git status                              # tree clean
git log --oneline -5                    # last commit is the RC1 SHA
just contract-test                      # 2/2 GREEN
go test -tags debug -short -count=1 ./internal/...  # GREEN
pwsh -File scripts/bump-version.ps1 -VerifyOnly   # "VERIFY OK"
```

If any of these fail, **stop and fix the drift before shipping**.

### 4. Tag + build the RC zip

```bash
git tag v1.1.4-rc1 <RC1-SHA>            # tag the chosen commit
git push origin v1.1.4-rc1              # push the tag (no force)
DIXIEDATA_RELEASE_TAG=rc1 just archive
```

The `archive` recipe runs `pwsh scripts/build-release.ps1 -Archive
-LDFlags "...CurrentReleaseTag=rc1"`. The output zip is at:

```
build/bin/DixieData-release-v1.1.4-rc1.zip
```

**Sanity check the chrome:** the Windows binary at
`build/bin/DixieData.exe` (built by the same recipe) should report:

```
$ build/bin/DixieData.exe --version
DixieData v1.1.4-rc1 · DixieData First Manassas
```

If the chrome says `v1.1.4` (no `-rc1`), the `-ldflags` didn't
propagate. Check that `DIXIEDATA_RELEASE_TAG=rc1` is in front of
`just archive` (env var only inherits to the same shell; don't
chain with `&&` from a shell that doesn't export it).

### 5. Publish the GitHub release

```bash
gh release create v1.1.4-rc1 \
    build/bin/DixieData-release-v1.1.4-rc1.zip \
    --draft \
    --title "v1.1.4-rc1" \
    --notes-file <(printf "RC1 — pre-release cut from dev. See CHANGELOG.md for commits since v1.1.4.\n")
```

Use `--draft` first. Sanity check the download URL works in a
browser:

```
https://github.com/valueforvalue/DixieData/releases/download/v1.1.4-rc1/DixieData-release-v1.1.4-rc1.zip
```

Then publish:

```bash
gh release edit v1.1.4-rc1 --draft=false
```

### 6. Bump the cohort manifest

```bash
just rc-publish TAG=rc1 ZIP=build/bin/DixieData-release-v1.1.4-rc1.zip
```

Prints the JSON to paste into the manifest. Copy it to
`dixiedata-rc-manifest/manifest.json` in the manifest repo,
commit, push:

```bash
cd <wherever the manifest repo is cloned>
$EDITOR manifest.json     # paste the JSON, save
git diff manifest.json    # eyeball the new version, asset_url, sha256
git add manifest.json
git commit -m "v1.1.4-rc1"
git push origin main
```

**Sanity check** the live raw URL returns the new content:

```bash
curl -s https://raw.githubusercontent.com/valueforvalue/dixiedata-rc-manifest/main/manifest.json
```

### 7. Opt the cohort in (you, the tester)

On your DixieData install (debug or release, your choice):

1. Settings → Updates → **Update source URL**
2. Paste: `https://raw.githubusercontent.com/valueforvalue/dixiedata-rc-manifest/main/manifest.json`
3. Save.
4. Click **Check for updates**.
5. DixieData should report "DixieData v1.1.4-rc1 is available".
6. Click **Apply update**. DixieData downloads the zip, verifies
   the SHA256 from the manifest, applies, restarts.
7. After restart, `DixieData.exe --version` should show
   `DixieData v1.1.4-rc1`.

If step 5 reports "no update available" but you just bumped the
manifest, the updater may be caching. Wait one poll interval
(defaults in `config.json` under `Timing.UpdateCheckTimeoutS`)
or kick it manually.

If step 6 fails the SHA256 verify, the zip you uploaded doesn't
match the hash in the manifest. Re-run `just rc-publish` against
the actual published zip, paste the new JSON, push.

### 8. Patches during the RC window

Patches keep landing on `dev`. Each patch is a candidate for the
next RC. The workflow:

1. Land the patch on `dev` (normal flow).
2. Decide if it goes into the existing RC1 (hot-fix) or becomes
   the basis for RC2.

**Hot-fix into RC1:**
```bash
git tag -d v1.1.4-rc1                     # delete the tag locally
git push origin :refs/tags/v1.1.4-rc1     # delete the tag remotely
git tag v1.1.4-rc1 <new-SHA>              # re-tag the patched commit
git push origin v1.1.4-rc1
# Then rebuild + republish the GitHub release + bump the manifest.
# (The `gh release upload v1.1.4-rc1 <new-zip>` overwrites the asset
#  in place; the tag stays put.)
```

**Ship as RC2:** just create a new tag, build, publish, manifest
bump. Cohort's `update_source_url` doesn't change — the updater
sees a different `version` field in the manifest and offers the
update.

### 9. Promote RC → stable (when ready)

When the cohort is satisfied:

```bash
# Build the stable zip (no -ldflags injection)
just archive
# Upload to a new GitHub release tagged v1.1.4 (no -rc suffix)
gh release create v1.1.4 build/bin/DixieData-release-v1.1.4.zip \
    --title "v1.1.4" \
    --notes-file CHANGELOG.md
# Default users (no custom update_source_url) see the update via
# /releases/latest; the RC cohort can either:
#   (a) leave update_source_url as-is and bump the manifest to
#       point at the stable zip, OR
#   (b) flip update_source_url back to the default
#       (https://api.github.com/repos/valueforvalue/DixieData/releases/latest).
```

### 10. Yanking RC1 (if it goes wrong)

```bash
gh release delete v1.1.4-rc1 --yes       # remove the release + tag + asset
# Edit dixiedata-rc-manifest/manifest.json: point at the previous
# known-good RC (or remove the manifest entry entirely so the
# updater offers nothing). Commit + push.
```

The cohort's updater will stop offering the bad version on the
next poll. If the bad version was already applied, the in-place
update flow's restore point manager (`internal/update/`) keeps
the previous install available for rollback.

## Reference

- `docs/RELEASING.md` §"RC cut workflow" — narrative version of
  the same flow.
- `tmp/manifest-repo/README.md` — the cohort-facing instructions
  that ship with the manifest repo.
- `internal/versioninfo/versioninfo.go` — `CurrentReleaseTag` +
  `AppVersionFull()` (chrome contract).
- `internal/update/updater.go` — `manifestReleaseFromJSON` (the
  parser; the version regex `(?i)v?(\d+)\.(\d+)\.(\d+)` strips the
  `-rc1` suffix before numeric compare).
- `internal/update/cohort_e2e_test.go` — live CI check that the
  cohort manifest is reachable + parseable.
- Issue #654 — the design rationale + the alternatives considered.
