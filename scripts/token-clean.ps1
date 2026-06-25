<#
.SYNOPSIS
    Token-saver clean — removes generated artifacts so LLM context stays lean.

.DESCRIPTION
    Deletes:
      - build/bin/, build/release/, build/{darwin,windows,linux}/
      - frontend/dist/, frontend/app.css, frontend/wailsjs/
      - internal/templates/*_templ.go and *_templ.txt
      - DixieData.exe in repo root
      - Stray *.exe from repo root that git does not track
      - Orphan zips in release/ older than the last two tags

    Preserves source files. Does NOT touch .dixiedata/ (user data).
    Safe to re-run.
#>

[CmdletBinding()]
param()

$ErrorActionPreference = "Continue"
$root = (Get-Location).Path

$targets = @(
    'build\bin',
    'build\release',
    'build\darwin',
    'build\windows',
    'build\linux',
    'frontend\dist',
    'frontend\wailsjs',
    'DixieData.exe'
)

Write-Host "Cleaning generated artifacts in $root..." -ForegroundColor Cyan

foreach ($rel in $targets) {
    $abs = Join-Path $root $rel
    if (Test-Path $abs) {
        Remove-Item -Path $abs -Recurse -Force -ErrorAction SilentlyContinue
        Write-Host "  removed $rel" -ForegroundColor DarkGray
    }
}

$frontendCss = Join-Path $root 'frontend\app.css'
if (Test-Path $frontendCss) {
    Remove-Item $frontendCss -Force -ErrorAction SilentlyContinue
    Write-Host "  removed frontend\app.css" -ForegroundColor DarkGray
}

$tplDir = Join-Path $root 'internal\templates'
if (Test-Path $tplDir) {
    Get-ChildItem -Path $tplDir -Filter '*_templ.go' -ErrorAction SilentlyContinue | Remove-Item -Force
    Get-ChildItem -Path $tplDir -Filter '*_templ.txt' -ErrorAction SilentlyContinue | Remove-Item -Force
    Write-Host "  removed internal\templates\*_templ.{go,txt}" -ForegroundColor DarkGray
}

$logDir = Join-Path $root 'build\log'
if (Test-Path $logDir) {
    Get-ChildItem -Path $logDir -Filter '*.log' -ErrorAction SilentlyContinue | Remove-Item -Force
    Write-Host "  cleared build\log\*.log" -ForegroundColor DarkGray
}

# Sweep stray *.exe from repo root (caught by *.exe gitignore, but lint the
# root anyway so future reverts of dropped binaries cannot leave orphans).
$rootExes = Get-ChildItem -Path $root -Filter '*.exe' -File -ErrorAction SilentlyContinue
foreach ($exe in $rootExes) {
    $tracked = git ls-files $exe.Name 2>$null
    if (-not $tracked) {
        Remove-Item $exe.FullName -Force -ErrorAction SilentlyContinue
        Write-Host "  removed stray root binary: $($exe.Name)" -ForegroundColor DarkGray
    }
}

# Sweep orphan release/ zips older than the last two tags.
# Cutoff is the timestamp of the SECOND-newest tag; keep everything from
# the last two releases, drop anything older. Version date is taken from
# the zip filename (DixieData-release-vX.Y.Z.zip) so filesystem mtime
# (which can be the day an agent regenerated release/) cannot cause
# over-deletion.
$releaseDir = Join-Path $root 'release'
if (Test-Path $releaseDir) {
    $tags = @(git tag --sort=-creatordate 2>$null | Select-Object -First 2)
    $keepVersions = @{}
    foreach ($t in $tags) { $keepVersions[$t] = $true }
    Get-ChildItem -Path $releaseDir -Filter '*.zip' -File -ErrorAction SilentlyContinue |
        ForEach-Object {
            if ($_.Name -match 'v(\d+(?:\.\d+){1,2})') {
                $ver = $matches[1]
                if (-not $keepVersions.ContainsKey("v$ver")) {
                    Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
                    Write-Host "  removed orphan release zip: $($_.Name)" -ForegroundColor DarkGray
                }
            }
        }
}

Write-Host "Clean complete." -ForegroundColor Green
