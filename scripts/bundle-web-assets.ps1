# bundle-web-assets.ps1 — copy the Typst binary + templates into
# build/bin/ so the web-mode server (cmd/dixiedata-web) can find
# them when launched in CI / audit probes.
#
# Issue #347 surfaced the gap: the Wails debug build path
# (scripts/build-debug.ps1 → Restore-DixieDataTypstAssets) bundles
# templates/bin/typst-windows.exe + build/bin/templates/*.typ
# next to DixieData.exe. The plain `go build -o build/bin/
# dixiedata-web.exe ./cmd/dixiedata-web` (used by `make web`)
# skips that step, so the web-mode binary boots without the
# event_landscape.typ template it needs for /events/{id}/pdf
# (slot #320.3).
#
# Mirrors Restore-DixieDataTypstAssets (build-common.ps1) but is
# callable standalone from `make web` without going through the
# full debug chain (no wails build, no PDFium download, no
# app.css rebuild). Idempotent: re-running is a no-op except
# for file mtime changes.

param(
    [Parameter(Mandatory = $true)]
    [string]$Root
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "build-common.ps1")

Set-Location $Root
$binDir = Get-DixieDataBuildBinDir -Root $Root
New-Item -ItemType Directory -Path $binDir -Force | Out-Null

$sourceBinary = Get-DixieDataTypstSourceBinaryPath -Root $Root
$targetBinary = Get-DixieDataTypstBinaryBuildPath -Root $Root
if (-not (Test-Path $sourceBinary)) {
    throw "Typst binary not found at $sourceBinary."
}
Copy-Item $sourceBinary $targetBinary -Force
Write-Host "Bundled Typst binary: $targetBinary"

$sourceTemplates = Get-DixieDataTypstTemplatesSourceDir -Root $Root
if (-not (Test-Path $sourceTemplates)) {
    throw "Templates directory not found at $sourceTemplates."
}
$targetTemplatesPath = Join-Path $binDir "templates"
if (Test-Path $targetTemplatesPath) {
    Remove-Item $targetTemplatesPath -Recurse -Force
}
Copy-Item $sourceTemplates $targetTemplatesPath -Recurse -Force
Write-Host "Bundled Typst templates: $targetTemplatesPath"

# Verify all source *.typ files made it across. If a future
# slot adds a new template and forgets to bundle it, this
# guard surfaces that immediately.
$sourceTyp = @(Get-ChildItem -Path $sourceTemplates -Filter '*.typ' -Recurse -File)
$missing = @()
foreach ($f in $sourceTyp) {
    $rel = $f.FullName.Substring($sourceTemplates.Length).TrimStart('\','/')
    $dest = Join-Path $targetTemplatesPath $rel
    if (-not (Test-Path $dest)) {
        $missing += $rel
    }
}
if ($missing.Count -gt 0) {
    throw "Bundled templates dir is missing source files: $($missing -join ', ')"
}
Write-Host "Verified $($sourceTyp.Count) *.typ files in bundle."
