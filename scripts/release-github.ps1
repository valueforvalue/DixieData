<#
.SYNOPSIS
    Tag, push, and create a draft GitHub release for the current version.

.DESCRIPTION
    Safety gates (all must pass before any mutation):
      1. Working tree clean
      2. versioninfo.go CurrentSchemaVersion is committed (matches HEAD)
      3. release/DixieData-release-v{VERSION}.zip exists
      4. Tag v{VERSION} does not exist locally or on origin
      5. gh CLI is authenticated

    On success: tag + push main + push tag + gh release create --draft.
    Draft means the release is created but NOT published. Review in the
    GitHub UI, then run `gh release edit v{VERSION} --draft=false` to publish.

    Bump version FIRST with scripts/bump-version.ps1 if needed.

.EXAMPLE
    pwsh -File scripts/release-github.ps1
#>

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"
$root = (Get-Location).Path

# --- Determine current version (reuse build-common.ps1 helper) ---
$helperPath = Join-Path $root "scripts\build-common.ps1"
if (-not (Test-Path $helperPath)) {
    throw "scripts\build-common.ps1 not found — run from repo root."
}
. $helperPath
$version = Get-DixieDataAppVersion -Root $root
$tag = "v$version"
$archiveName = "DixieData-release-$version.zip"
$archivePath = Join-Path $root "release\$archiveName"

Write-Host "Preparing release for $tag" -ForegroundColor Cyan

# --- Gate 1: Working tree clean ---
$gitStatus = & git status --porcelain 2>$null
if ($gitStatus) {
    throw "Working tree has uncommitted changes. Commit or stash first:`n$gitStatus"
}

# --- Gate 2: versioninfo.go matches HEAD ---
$versionInfoPath = Join-Path $root "internal\versioninfo\versioninfo.go"
$versionInfoStatus = & git status --porcelain $versionInfoPath 2>$null
if ($versionInfoStatus) {
    throw "internal/versioninfo/versioninfo.go has uncommitted changes. Commit the bump first."
}

# --- Gate 3: Archive exists ---
if (-not (Test-Path $archivePath)) {
    throw "Missing archive: $archivePath`nRun 'make archive' first to build + zip."
}

# --- Gate 4: Tag does not exist ---
$localTag = & git tag -l $tag 2>$null
if ($localTag) {
    throw "Tag $tag already exists locally. Delete with 'git tag -d $tag' or bump version."
}
$remoteTag = & git ls-remote --tags origin "refs/tags/$tag" 2>$null
if ($remoteTag) {
    throw "Tag $tag already exists on origin. Use a new version."
}

# --- Gate 5: gh CLI authenticated ---
$ghPath = Get-Command gh -ErrorAction SilentlyContinue
if (-not $ghPath) {
    throw "gh CLI not found on PATH. Install from https://cli.github.com/."
}
$ghAuthStatus = & gh auth status 2>&1
if ($LASTEXITCODE -ne 0) {
    throw "gh CLI not authenticated. Run 'gh auth login' first.`n$ghAuthStatus"
}

# --- All gates passed. Execute. ---
Write-Host ""
Write-Host "All safety gates passed." -ForegroundColor Green
Write-Host "  version:        $version"
Write-Host "  tag:            $tag"
Write-Host "  archive:        $archivePath"
Write-Host "  archive size:   $("{0:N2} MB" -f ((Get-Item $archivePath).Length / 1MB))"
Write-Host ""

$confirm = Read-Host "Create draft release for $tag? Type 'yes' to continue"
if ($confirm -ne "yes") {
    Write-Host "Aborted." -ForegroundColor Yellow
    exit 1
}

# Tag + push
& git tag -a $tag -m "Release $tag"
if ($LASTEXITCODE -ne 0) { throw "git tag failed" }

& git push origin main
if ($LASTEXITCODE -ne 0) {
    & git tag -d $tag | Out-Null
    throw "git push origin main failed — local tag $tag rolled back"
}

& git push origin $tag
if ($LASTEXITCODE -ne 0) {
    throw "git push origin $tag failed. main was already pushed; tag is now on origin — investigate before retrying."
}

# Draft release
& gh release create $tag $archivePath `
    --draft `
    --title "DixieData $tag" `
    --generate-notes
if ($LASTEXITCODE -ne 0) {
    throw "gh release create failed. Tag is pushed; create release manually at https://github.com/valueforvalue/DixieData/releases"
}

Write-Host ""
Write-Host "Draft release created for $tag." -ForegroundColor Green
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. Review the draft at https://github.com/valueforvalue/DixieData/releases/tag/$tag"
Write-Host "  2. Edit release notes if needed: gh release edit $tag"
Write-Host "  3. Publish: gh release edit $tag --draft=false"
