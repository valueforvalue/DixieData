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
    [switch]$VerifyOnly
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
