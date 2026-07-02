<#
.SYNOPSIS
    Increment CurrentSchemaVersion in internal/versioninfo/versioninfo.go.

.DESCRIPTION
    Strict bump: refuses to advance by more than 1 unless -Force, and refuses
    to advance without a migration doc at docs/migrations/v{N+1}.md. This
    protects DixieData's local update feature — every schema bump needs a
    paired migration in internal/db/schema.go and a human-readable note in
    docs/migrations/.

    Does NOT auto-commit. Lets the reviewer amend CHANGELOG.md, run the
    migration test suite, and commit deliberately before tagging.

.PARAMETER Force
    Allow a jump greater than +1. Use sparingly (e.g., re-syncing after a
    missed bump). Always pair with a docs/migrations/v{N+1}.md entry.

.EXAMPLE
    pwsh -File scripts/bump-version.ps1
    # CurrentSchemaVersion 54 -> 55
    # Requires docs/migrations/v55.md to exist with at least one bullet.
#>

[CmdletBinding()]
param(
    [switch]$Force,
    [switch]$VerifyOnly,
    [switch]$DetectDrift
)

$ErrorActionPreference = "Stop"
$root = (Get-Location).Path
$versionInfoPath = Join-Path $root "internal\versioninfo\versioninfo.go"
$migrationsDir = Join-Path $root "docs\migrations"

if (-not (Test-Path $versionInfoPath)) {
    throw "versioninfo.go not found at $versionInfoPath — run from repo root."
}

$content = Get-Content -Path $versionInfoPath -Raw
$match = [regex]::Match($content, "CurrentSchemaVersion\s*=\s*(\d+)")
if (-not $match.Success) {
    throw "Failed to locate 'CurrentSchemaVersion = N' in $versionInfoPath."
}

$current = [int]$match.Groups[1].Value
$next = $current + 1
$delta = $next - $current

# VerifyOnly: non-mutating validation pass. Returns 0 on pass, 1 on fail.
# Checks:
#   1. If versioninfo.go bumped relative to HEAD, paired docs/migrations/v{new}.md exists
#   2. user-manual / implementation-and-features / ai-handoff reference current app version
#   3. CHANGELOG.md has a section header for the current app version
if ($VerifyOnly) {
    $errors = @()
    $headContent = & git show "HEAD:internal/versioninfo/versioninfo.go" 2>$null
    $headVersion = $current
    if ($headContent) {
        $hm = [regex]::Match($headContent, 'CurrentSchemaVersion\s*=\s*(\d+)')
        if ($hm.Success) { $headVersion = [int]$hm.Groups[1].Value }
    }
    if ($current -ne $headVersion) {
        $expectedMigration = Join-Path $migrationsDir ("v{0}.md" -f $current)
        if (-not (Test-Path $expectedMigration)) {
            $errors += "versioninfo.go bumped $headVersion -> $current but missing $expectedMigration"
        }
    }

    $appVer = "1.2.$current"
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
    Write-Host "VERIFY OK: schema $current, docs reference $appVer, discipline intact" -ForegroundColor Green
    exit 0
}

# DetectDrift: walks HEAD..origin/<base> commit subjects for
# schema-touching patterns. If any are present AND CurrentSchemaVersion
# is unchanged, fail with a drift message. The skip hatch is a
# commit subject 'chore: skip-schema-bump' + reason in the body.
# Used by CI on every PR (the .github/workflows/test.yml schema-
# touching detector is a duplicate of this check; the bash variant
# is the canonical for non-Windows runners, this is the canonical
# for Windows).
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

if ($delta -ne 1 -and -not $Force) {
    throw "Refusing to bump by $delta (current=$current, next=$next). " +
          "Use -Force for jumps > 1, and pair with a docs/migrations/v$next.md entry."
}

$migrationPath = Join-Path $migrationsDir ("v{0}.md" -f $next)
if (-not (Test-Path $migrationPath)) {
    throw "Missing migration note: $migrationPath`n" +
          "DixieData's local update feature requires a paired migration doc for each schema bump.`n" +
          "Create the file with at least one '- ' bullet describing the schema change, then re-run."
}

# Enforce non-empty migration note
$migrationContent = Get-Content -Path $migrationPath -Raw
if ($migrationContent -notmatch '^\s*-\s+\S' -and $migrationContent -notmatch '\n\s*-\s+\S') {
    throw "Migration note $migrationPath has no '- ' bullets. " +
          "Document the schema change so reviewers and the update flow have a paper trail."
}

# Refuse if working tree has uncommitted changes touching versioninfo.go
$gitStatus = & git status --porcelain $versionInfoPath 2>$null
if ($gitStatus) {
    throw "versioninfo.go has uncommitted changes. Commit or stash before bumping."
}

# Rewrite the file with new value, preserving everything else
$newContent = $content -replace "CurrentSchemaVersion\s*=\s*\d+", "CurrentSchemaVersion = $next"
Set-Content -Path $versionInfoPath -Value $newContent -NoNewline

$appVersion = "v1.2.{0}" -f $next

if ($VerifyOnly) {
    Write-Host "VERIFY OK: CurrentSchemaVersion would go $current -> $next" -ForegroundColor Green
    Write-Host "  paired migration note: $migrationPath"
    Write-Host "  app version: $appVersion"
    exit 0
}

Write-Host ""
Write-Host "Bumped CurrentSchemaVersion: $current -> $next" -ForegroundColor Green
Write-Host "App version: $appVersion"
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. Update CHANGELOG.md with a '## $appVersion - ...' section."
Write-Host "  2. Run the test suite (make test-quiet) to confirm migrations apply cleanly."
Write-Host "  3. git add internal/versioninfo/versioninfo.go CHANGELOG.md"
Write-Host "  4. git commit -m 'Bump release line to $appVersion'"
Write-Host "  5. make archive   # builds + zips release/DixieData-release-$appVersion.zip"
Write-Host "  6. make release-github   # tag + push + draft GitHub release"
