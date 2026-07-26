<#
.SYNOPSIS
    Bump a DixieData release counter.

.DESCRIPTION
    Three independent counters live in internal/versioninfo/versioninfo.go:

      CurrentSchemaVersion     — SQLite user_version; the data plane
                                 (bump = a docs/migrations/v{N+1}.md is
                                 required; that's the migration note
                                 that ships with every release)
      CurrentUpdateFlowVersion — update-flow shape gate (issue #266).
                                 Bump = the in-place update mechanism
                                 itself changed shape; old binaries
                                 must NOT auto-apply a release with a
                                 higher U (the user must reinstall)
      CurrentAppVersionInt     — release counter N. Every release
                                 bumps N; bug-fix-only releases
                                 bump N without touching the schema

    Pick one of the three switches (-BumpSchema, -BumpUpdateFlow,
    -BumpRelease). They're mutually exclusive. The default (no
    switch) is -BumpSchema to match the historical behavior of
    this script.

    Schema bumps:
      pwsh -File scripts/bump-version.ps1
      # or
      pwsh -File scripts/bump-version.ps1 -BumpSchema
      # CurrentSchemaVersion +1; requires docs/migrations/v{N+1}.md

    Update-flow bumps (rare; reserve for changes that genuinely
    reshape the in-place update mechanism):
      pwsh -File scripts/bump-version.ps1 -BumpUpdateFlow
      # CurrentUpdateFlowVersion +1; CurrentAppVersionInt reset to 0;
      # previous U sequence's last N is archived to
      # .release-state/last-n-for-u{U}.json so a future
      # U=1 -> U=3 transition can be reviewed

    Release counter bumps (bug-fix-only releases):
      pwsh -File scripts/bump-version.ps1 -BumpRelease
      # CurrentAppVersionInt +1; nothing else

    Force / verify / detect-drift switches work as before.

.PARAMETER BumpSchema
    Bump CurrentSchemaVersion (+1; -Force for jumps).

.PARAMETER BumpUpdateFlow
    Bump CurrentUpdateFlowVersion (+1), reset CurrentAppVersionInt
    to 0, archive the previous U sequence's last N to a sidecar
    JSON. Does NOT touch CurrentSchemaVersion.

.PARAMETER BumpRelease
    Bump CurrentAppVersionInt (+1) only. Use for bug-fix-only
    releases that don't change the schema or the update flow.

.PARAMETER Force
    Allow a jump greater than +1. Use sparingly. Pair with a
    docs/migrations/v{N+1}.md entry.

.PARAMETER VerifyOnly
    Non-mutating validation pass. Used by CI. Checks drift on
    all three counters (CurrentSchemaVersion, CurrentUpdateFlowVersion,
    CurrentAppVersionInt).

.PARAMETER DetectDrift
    Walks HEAD..origin/<base> commit subjects for schema-touching
    patterns. If present and CurrentSchemaVersion is unchanged,
    fail with a drift message. CI use.

.PARAMETER SetReleaseTag
    Print the -ldflags + build invocation needed to bake a
    pre-release tag (e.g. "rc1", "rc2") into the chrome. Does
    NOT edit versioninfo.go — the tag is injected at build
    time so the same source tree produces a stable zip OR an
    RC zip. See the RC cohort workflow in docs/RELEASING.md.

.PARAMETER ReleaseTag
    Pre-release tag string (used with -SetReleaseTag). Letters
    and digits only, no leading hyphen (e.g. "rc1", "beta2").
    The script appends the hyphen when composing the chrome
    string ("DixieData v1.1.4-rc1").

.PARAMETER ClearReleaseTag
    Print the stable build invocation (no -ldflags). Companion
    to -SetReleaseTag. Use when promoting an RC cohort's
    tested code to the stable channel.

.EXAMPLE
    pwsh -File scripts/bump-version.ps1
    # CurrentSchemaVersion 54 -> 55
    # Requires docs/migrations/v55.md to exist with at least one bullet.

.EXAMPLE
    pwsh -File scripts/bump-version.ps1 -BumpRelease
    # CurrentAppVersionInt +1; nothing else

.EXAMPLE
    pwsh -File scripts/bump-version.ps1 -SetReleaseTag -ReleaseTag rc1
    # Prints the -ldflags invocation to bake the rc1 tag into the chrome.
    # Does not edit versioninfo.go. The bare AppVersion stays 1.1.4; the
    # chrome renders "DixieData v1.1.4-rc1".
    # Pair with `pwsh -File scripts/build-release.ps1 -LDFlags "..."`.
#>

[CmdletBinding()]
param(
    [switch]$BumpSchema,
    [switch]$BumpUpdateFlow,
    [switch]$BumpRelease,
    [switch]$BumpCodename,
    [string]$Codename,
    [switch]$SetReleaseTag,
    [string]$ReleaseTag,
    [switch]$ClearReleaseTag,
    [switch]$Force,
    [switch]$VerifyOnly,
    [switch]$DetectDrift
)

$ErrorActionPreference = "Stop"
$root = (Get-Location).Path
$versionInfoPath = Join-Path $root "internal\versioninfo\versioninfo.go"
$migrationsDir = Join-Path $root "docs\migrations"
$releaseStateDir = Join-Path $root ".release-state"

# Resolve which bump the caller wants. Default = BumpSchema
# (matches the historical behavior of this script pre-#266).
$explicitBumps = @()
if ($BumpSchema) { $explicitBumps += 'Schema' }
if ($BumpUpdateFlow) { $explicitBumps += 'UpdateFlow' }
if ($BumpRelease) { $explicitBumps += 'Release' }
if ($BumpCodename) { $explicitBumps += 'Codename' }
if ($SetReleaseTag) { $explicitBumps += 'SetReleaseTag' }
if ($ClearReleaseTag) { $explicitBumps += 'ClearReleaseTag' }
if ($explicitBumps.Count -gt 1) {
    throw "Pass only one of -BumpSchema, -BumpUpdateFlow, -BumpRelease, -BumpCodename, -SetReleaseTag, -ClearReleaseTag. Got: $($explicitBumps -join ', ')"
}
if ($explicitBumps.Count -eq 0) {
    $bumpKind = 'Schema'
} else {
    $bumpKind = $explicitBumps[0]
}

if (-not (Test-Path $versionInfoPath)) {
    throw "versioninfo.go not found at $versionInfoPath — run from repo root."
}

$content = Get-Content -Path $versionInfoPath -Raw

function Read-Counter($Source, [string]$Name) {
    $m = [regex]::Match($Source, "$Name\s*=\s*(\d+)")
    if (-not $m.Success) {
        throw "Failed to locate '$Name = N' in $versionInfoPath."
    }
    return [int]$m.Groups[1].Value
}

$currentSchema = Read-Counter $content 'CurrentSchemaVersion'
$currentUpdateFlow = Read-Counter $content 'CurrentUpdateFlowVersion'
$currentRelease = Read-Counter $content 'CurrentAppVersionInt'

# Read-Codename is a string-valued variant of Read-Counter. Uses
# a quoted-string match (no escape decoding; release names
# contain no quotes).
function Read-Codename($Source) {
    $m = [regex]::Match($Source, 'CurrentReleaseName\s*=\s*"([^"]*)"')
    if (-not $m.Success) {
        throw "Failed to locate 'CurrentReleaseName = ...' in $versionInfoPath."
    }
    return $m.Groups[1].Value
}
$currentReleaseName = Read-Codename $content

# VerifyOnly: non-mutating validation pass. Returns 0 on pass, 1 on fail.
# Checks (covers all three counters per issue #294 acceptance):
#   1. If CurrentSchemaVersion bumped relative to HEAD, paired
#      docs/migrations/v{new}.md exists.
#   2. user-manual / implementation-and-features / ai-handoff reference
#      the current v{MAJOR}.{U}.{N} shape.
#   3. CHANGELOG.md has a section header for the current release.
#   4. If CurrentUpdateFlowVersion bumped relative to HEAD, the sidecar
#      JSON for the previous U sequence exists at
#      .release-state/last-n-for-u{prev_U}.json.
if ($VerifyOnly) {
    $errors = @()
    $headContent = & git show "HEAD:internal/versioninfo/versioninfo.go" 2>$null
    $headSchema = $currentSchema
    $headUpdateFlow = $currentUpdateFlow
    $headRelease = $currentRelease
    if ($headContent) {
        $headSchema = Read-Counter $headContent 'CurrentSchemaVersion'
        $headUpdateFlow = Read-Counter $headContent 'CurrentUpdateFlowVersion'
        $headRelease = Read-Counter $headContent 'CurrentAppVersionInt'
    }

    # Schema drift
    if ($currentSchema -ne $headSchema) {
        $expectedMigration = Join-Path $migrationsDir ("v{0}.md" -f $currentSchema)
        if (-not (Test-Path $expectedMigration)) {
            $errors += "CurrentSchemaVersion bumped $headSchema -> $currentSchema but missing $expectedMigration"
        }
    }

    # UpdateFlow drift: a U bump must have archived the previous
    # sequence's last N for review (issue #294 acceptance).
    if ($currentUpdateFlow -ne $headUpdateFlow) {
        $expectedSidecar = Join-Path $releaseStateDir ("last-n-for-u{0}.json" -f $headUpdateFlow)
        if (-not (Test-Path $expectedSidecar)) {
            $errors += "CurrentUpdateFlowVersion bumped $headUpdateFlow -> $currentUpdateFlow but missing sidecar $expectedSidecar"
        }
    }

    # Doc references use the canonical v{MAJOR}.{U}.{N} shape
    # from AppVersion() in versioninfo.go. N is the release
    # counter (CurrentAppVersionInt), NOT the schema version —
    # the two diverged in issue #266. Using $currentSchema here
    # was a bug that produced v1.1.68 (a hybrid that matched no
    # real version string); the correct value is v1.1.4 (U=1,
    # N=4) per AppVersion().
    $appVer = "1.$currentUpdateFlow.$currentRelease"
    $docFiles = @(
        "docs\user-manual.md",
        "docs\implementation-and-features.md",
        "docs\ai-handoff.md"
    )
    foreach ($d in $docFiles) {
        $abs = Join-Path $root $d
        if (-not (Test-Path $abs)) { continue }
        $c = Get-Content -Path $abs -Raw
        if ($c -notmatch [regex]::Escape($appVer)) {
            $errors += "$d does not reference $appVer"
        }
    }

    $changelogPath = Join-Path $root "CHANGELOG.md"
    if (Test-Path $changelogPath) {
        $cl = Get-Content -Path $changelogPath -Raw
        $verHeader = "## \[?${appVer}\]?"
        $hasUnreleased = $cl -match "##\s*\[?Unreleased\]?"
        $hasVersion = $cl -match $verHeader
        if (-not $hasVersion -and -not $hasUnreleased) {
            $errors += "CHANGELOG.md has no '## ${appVer}' and no '[Unreleased]' section"
        }
    }

    if ($errors.Count -gt 0) {
        Write-Host "VERIFY FAIL:" -ForegroundColor Red
        foreach ($e in $errors) { Write-Host "  - $e" -ForegroundColor Red }
        exit 1
    }
    Write-Host "VERIFY OK: schema $currentSchema, update_flow $currentUpdateFlow, release $currentRelease / codename: $currentReleaseName" -ForegroundColor Green
    Write-Host "  app version: 1.$currentUpdateFlow.$currentRelease"
    Write-Host "  doc + changelog references intact"
    exit 0
}

# DetectDrift: walks HEAD..origin/<base> commit subjects for
# schema-touching patterns. If any are present AND
# CurrentSchemaVersion is unchanged, fail with a drift message.
# The skip hatch is a commit subject 'chore: skip-schema-bump'
# + reason in the body. Used by CI on every PR.
if ($DetectDrift) {
    $base = "origin/$env:GITHUB_BASE_REF"
    if (-not $env:GITHUB_BASE_REF) {
        Write-Host "DETECT-DRIFT: skipping (GITHUB_BASE_REF not set; run inside GitHub Actions)"
        exit 0
    }
    $subjects = & git log --format=%s "$base..HEAD" 2>$null
    if (-not $subjects) {
        Write-Host "DETECT-DRIFT: no commits to scan"
        exit 0
    }
    $touches = $subjects | Where-Object { $_ -match '^(feat|fix)\((db|schema)\):' -or $_ -match '^feat\(schema\):' }
    if (-not $touches) {
        Write-Host "DETECT-DRIFT: no schema-touching commits in PR"
        exit 0
    }
    $skipHatch = $subjects | Where-Object { $_ -match '^chore: skip-schema-bump' }
    if ($skipHatch) {
        Write-Host "DETECT-DRIFT: skip-schema-bump hatch found, allowing PR"
        exit 0
    }
    $headContent = & git show "HEAD:internal/versioninfo/versioninfo.go" 2>$null
    $baseContent = & git show "$base:internal/versioninfo/versioninfo.go" 2>$null
    if (-not $headContent -or -not $baseContent) {
        Write-Host "DETECT-DRIFT: could not read versioninfo.go at HEAD or base; skipping"
        exit 0
    }
    $headMatch = [regex]::Match($headContent, 'CurrentSchemaVersion\s*=\s*(\d+)')
    $baseMatch = [regex]::Match($baseContent, 'CurrentSchemaVersion\s*=\s*(\d+)')
    if (-not $headMatch.Success -or -not $baseMatch.Success) {
        Write-Host "DETECT-DRIFT: could not parse CurrentSchemaVersion; skipping"
        exit 0
    }
    $headVer = [int]$headMatch.Groups[1].Value
    $baseVer = [int]$baseMatch.Groups[1].Value
    if ($headVer -eq $baseVer) {
        Write-Host ""
        Write-Host "DRIFT: PR touches schema but did not bump CurrentSchemaVersion ($baseVer -> $headVer)." -ForegroundColor Red
        Write-Host "Touching commits:" -ForegroundColor Yellow
        $touches | ForEach-Object { Write-Host "  $_" }
        Write-Host ""
        Write-Host "Either:" -ForegroundColor Yellow
        Write-Host "  1. Add a 'feat(schema): bump to v$($baseVer + 1)' commit in this PR"
        Write-Host "  2. Add a 'chore: skip-schema-bump' commit + reason in body (rare; for non-shape changes)"
        exit 1
    }
    Write-Host "DETECT-DRIFT: schema bumped $baseVer -> $headVer, OK"
    exit 0
}

# Refuse if working tree has uncommitted changes touching versioninfo.go.
# Print-only modes (SetReleaseTag, ClearReleaseTag) don't mutate the file
# and skip this guard so the operator can preview the build invocation
# while other commits are pending.
if ($bumpKind -in @('Schema', 'UpdateFlow', 'Release', 'Codename')) {
    $gitStatus = & git status --porcelain $versionInfoPath 2>$null
    if ($gitStatus) {
        throw "versioninfo.go has uncommitted changes. Commit or stash before bumping."
    }
}

switch ($bumpKind) {
    'Schema' {
        $next = $currentSchema + 1
        $delta = $next - $currentSchema
        if ($delta -ne 1 -and -not $Force) {
            throw "Refusing to bump schema by $delta (current=$currentSchema, next=$next). " +
                  "Use -Force for jumps > 1, and pair with a docs/migrations/v$next.md entry."
        }

        $migrationPath = Join-Path $migrationsDir ("v{0}.md" -f $next)
        if (-not (Test-Path $migrationPath)) {
            throw "Missing migration note: $migrationPath`n" +
                  "DixieData's local update feature requires a paired migration doc for each schema bump.`n" +
                  "Create the file with at least one '- ' bullet describing the schema change, then re-run."
        }
        $migrationContent = Get-Content -Path $migrationPath -Raw
        if ($migrationContent -notmatch '^\s*-\s+\S' -and $migrationContent -notmatch '\n\s*-\s+\S') {
            throw "Migration note $migrationPath has no '- ' bullets. " +
                  "Document the schema change so reviewers and the update flow have a paper trail."
        }

        # Rewrite the file with new schema value, preserving everything else.
        $newContent = $content -replace "CurrentSchemaVersion\s*=\s*\d+", "CurrentSchemaVersion = $next"
        Set-Content -Path $versionInfoPath -Value $newContent -NoNewline

        $appVersion = "v1.$currentUpdateFlow.$next"

        Write-Host ""
        Write-Host "Bumped CurrentSchemaVersion: $currentSchema -> $next" -ForegroundColor Green
        Write-Host "App version: $appVersion"
        Write-Host ""
        Write-Host "Next steps:" -ForegroundColor Cyan
        Write-Host "  1. Update CHANGELOG.md with a '## $appVersion - ...' section."
        Write-Host "  2. Run the test suite (make test-quiet) to confirm migrations apply cleanly."
        Write-Host "  3. git add internal/versioninfo/versioninfo.go CHANGELOG.md"
        Write-Host "  4. git commit -m 'Bump release line to $appVersion'"
        Write-Host "  5. make archive   # builds + zips release/DixieData-release-$appVersion.zip"
        Write-Host "  6. make release-github   # tag + push + draft GitHub release"
    }
    'UpdateFlow' {
        # U bump: bumps U, resets N to 0, archives the previous U
        # sequence's last N to a sidecar for review.
        $nextU = $currentUpdateFlow + 1

        if (-not (Test-Path $releaseStateDir)) {
            New-Item -ItemType Directory -Path $releaseStateDir -Force | Out-Null
        }
        $sidecarPath = Join-Path $releaseStateDir ("last-n-for-u{0}.json" -f $currentUpdateFlow)
        $sidecar = @{
            previous_update_flow_version = $currentUpdateFlow
            last_release_counter         = $currentRelease
            last_schema_version          = $currentSchema
            archived_at                  = (Get-Date).ToUniversalTime().ToString("o")
            note                         = "Archived by bump-version.ps1 -BumpUpdateFlow. The U=$currentUpdateFlow -> U=$nextU transition resets the release counter; this file preserves the last N from the previous sequence for review."
        } | ConvertTo-Json -Depth 4
        Set-Content -Path $sidecarPath -Value $sidecar

        $newContent = $content -replace "CurrentUpdateFlowVersion\s*=\s*\d+", "CurrentUpdateFlowVersion = $nextU"
        $newContent = $newContent -replace "CurrentAppVersionInt\s*=\s*\d+", "CurrentAppVersionInt = 0"
        Set-Content -Path $versionInfoPath -Value $newContent -NoNewline

        $appVersion = "v1.$nextU.0"

        Write-Host ""
        Write-Host "Bumped CurrentUpdateFlowVersion: $currentUpdateFlow -> $nextU" -ForegroundColor Green
        Write-Host "Reset  CurrentAppVersionInt:    $currentRelease -> 0"
        Write-Host "Archived previous U=$currentUpdateFlow sequence last N=$currentRelease to $sidecarPath" -ForegroundColor Yellow
        Write-Host "App version: $appVersion"
        Write-Host ""
        Write-Host "Next steps:" -ForegroundColor Cyan
        Write-Host "  1. Update CHANGELOG.md with a '## $appVersion - ...' section noting the U bump rationale."
        Write-Host "  2. Update docs/RELEASING.md and ADR 0008 if the U bump changes the install/upgrade contract."
        Write-Host "  3. Run the test suite (make test-quiet) — the in-place update flow's compareVersions will reject U-mismatched releases."
        Write-Host "  4. git add internal/versioninfo/versioninfo.go .release-state/ CHANGELOG.md"
        Write-Host "  5. git commit -m 'Bump update-flow version to $appVersion'"
        Write-Host "  6. make archive && make release-github"
    }
    'Release' {
        # Bug-fix-only release: bump N only. Nothing else changes.
        $nextN = $currentRelease + 1
        if ($nextN -ne $currentRelease + 1 -and -not $Force) {
            throw "Refusing to bump release counter by more than +1 (current=$currentRelease). Use -Force for jumps."
        }
        $newContent = $content -replace "CurrentAppVersionInt\s*=\s*\d+", "CurrentAppVersionInt = $nextN"
        Set-Content -Path $versionInfoPath -Value $newContent -NoNewline

        $appVersion = "v1.$currentUpdateFlow.$nextN"

        Write-Host ""
        Write-Host "Bumped CurrentAppVersionInt: $currentRelease -> $nextN" -ForegroundColor Green
        Write-Host "App version: $appVersion"
        Write-Host ""
        Write-Host "Next steps:" -ForegroundColor Cyan
        Write-Host "  1. Update CHANGELOG.md with a '## $appVersion - ...' section."
        Write-Host "  2. Run the test suite (make test-quiet)."
        Write-Host "  3. git add internal/versioninfo/versioninfo.go CHANGELOG.md"
        Write-Host "  4. git commit -m 'Bump release counter to $appVersion'"
        Write-Host "  5. make archive && make release-github"
    }
    'Codename' {
        # Codename bump: rewrite the CurrentReleaseName var. Does
        # not touch schema / U / N — the codename is a chrome
        # label, not a data-plane version. Pairs naturally with
        # a Release bump (new N + new codename = new release line),
        # but the user can also rename a codename independently
        # if the previous one turns out to be embarrassing (per
        # docs/RELEASING.md deprecation rule).
        $name = $Codename
        if (-not $name) {
            $name = Read-Host 'New codename (single English word, no hyphens/underscores; spaces allowed between words)'
        }
        # Validation: no hyphens, no underscores, no tabs, no
        # special characters. Spaces between words are allowed
        # (e.g. "First Manassas" is two words).
        if ($name -match '[_\-\t!@#$%^&*()+={}[\]|:;<>,.?/\\]') {
            throw "Codename '$name' contains a forbidden character (hyphen, underscore, or special char). Use a single English word or short phrase with spaces between words."
        }
        if ([string]::IsNullOrWhiteSpace($name)) {
            throw "Codename cannot be empty."
        }
        $newContent = $content -replace 'CurrentReleaseName\s*=\s*"[^"]*"', "CurrentReleaseName = `"$name`""
        Set-Content -Path $versionInfoPath -Value $newContent -NoNewline

        Write-Host ""
        Write-Host "Bumped CurrentReleaseName: $currentReleaseName -> $name" -ForegroundColor Green
        Write-Host ""
        Write-Host "Next steps:" -ForegroundColor Cyan
        Write-Host "  1. Update docs/RELEASING.md 'Choosing the codename' section if the naming rationale changed."
        Write-Host "  2. Run the test suite (make test-quiet) — versioninfo tests pin the codename."
        Write-Host "  3. git add internal/versioninfo/versioninfo.go"
        Write-Host "  4. git commit -m 'Bump release codename to $name'"
    }
    'SetReleaseTag' {
        # Print the -ldflags + build invocation the operator
        # needs to bake the pre-release tag into the RC zip.
        # Does NOT edit versioninfo.go — the tag is injected at
        # build time so the same source tree can produce both
        # a stable zip (no tag) and an RC zip (tag baked in).
        # The chrome contract: buildinfo.AppLabel() returns
        # "DixieData v1.1.4-rc1" when CurrentReleaseTag is set;
        # the bare AppVersion stays canonical (used by the
        # updater's numeric comparison + every .ddbak / portable
        # output emit site).
        $tag = $ReleaseTag
        if (-not $tag) {
            $tag = Read-Host 'Release tag (e.g. rc1, rc2, beta1; no leading hyphen)'
        }
        if ($tag -match '^-') {
            throw "Release tag '$tag' must not start with a hyphen — the script appends the hyphen itself."
        }
        if ($tag -match '[^A-Za-z0-9]') {
            throw "Release tag '$tag' contains a forbidden character. Use letters and digits only (e.g. rc1, beta2)."
        }
        if ([string]::IsNullOrWhiteSpace($tag)) {
            throw "Release tag cannot be empty; use -ClearReleaseTag to remove the suffix."
        }
        $ldflag = "-X github.com/valueforvalue/DixieData/internal/versioninfo.CurrentReleaseTag=$tag"
        $appVersion = "1.$currentUpdateFlow.$currentRelease-$tag"
        Write-Host ""
        Write-Host "Release tag: $tag" -ForegroundColor Green
        Write-Host "App version (chrome): DixieData v$appVersion"
        Write-Host "App version (canonical numeric, for manifest + .ddbak): 1.$currentUpdateFlow.$currentRelease"
        Write-Host ""
        Write-Host "Bake it in:" -ForegroundColor Cyan
        Write-Host "  pwsh -File scripts/build-release.ps1 -LDFlags '$ldflag'"
        Write-Host ""
        Write-Host "Or via the justfile (issue #642):"
        Write-Host "  DIXIEDATA_RELEASE_TAG=$tag just release"
        Write-Host "  DIXIEDATA_RELEASE_TAG=$tag just archive"
        Write-Host ""
        Write-Host "Next steps (per the RC cohort workflow):" -ForegroundColor Cyan
        Write-Host "  1. Build the RC zip with the -LDFlags above."
        Write-Host "  2. Tag dev as v$appVersion + push (the tag is the same as the chrome string)."
        Write-Host "  3. Update the dixiedata-rc-manifest repo's manifest.json with the new zip URL + sha256."
        Write-Host "  4. The cohort's updater (pointed at the manifest URL via update_source_url) sees the new RC."
        Write-Host "  5. When ready to ship stable, run with -ClearReleaseTag (no suffix) and tag v1.$currentUpdateFlow.$currentRelease as the stable release."
    }
    'ClearReleaseTag' {
        # Companion to -SetReleaseTag. Prints the stable build
        # invocation (no -ldflags injection) so the operator can
        # rebuild + ship the same source tree as a stable release.
        Write-Host ""
        Write-Host "Release tag: <cleared>" -ForegroundColor Green
        Write-Host "App version: DixieData v1.$currentUpdateFlow.$currentRelease"
        Write-Host ""
        Write-Host "Bake it in:" -ForegroundColor Cyan
        Write-Host "  pwsh -File scripts/build-release.ps1"
        Write-Host "  # (no -LDFlags needed; CurrentReleaseTag defaults to empty)"
        Write-Host ""
        Write-Host "Or via the justfile:"
        Write-Host "  just release"
        Write-Host "  just archive"
    }
}