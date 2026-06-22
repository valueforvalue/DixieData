# Render every PDF export surface against the live archive for the
# current iteration round. Each export is saved to
# docs/renderings/<surface>/pre-iteration.pdf so the user can open
# it in a PDF viewer alongside the per-surface review.md file.
#
# Usage (from the repo root):
#   pwsh -File scripts/render-round.ps1            # round 1, writes pre-iteration.pdf
#   pwsh -File scripts/render-round.ps1 -Round 2   # writes round-2.pdf
#
# The script must be re-runnable: it overwrites the target PDF for
# the requested round. It does NOT touch pre-iteration.pdf from a
# previous round.
param(
    [int]$Round = 1
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$db = Join-Path $repoRoot ".dixiedata"
$typst = Join-Path $repoRoot "bin/typst-windows.exe"
$templates = Join-Path $repoRoot "templates"
$tune = Join-Path $repoRoot "tools/tune/bin/dixiedata-tune.exe"

if (-not (Test-Path $typst)) { throw "typst not found at $typst" }
if (-not (Test-Path $tune)) { throw "dixiedata-tune not built; run 'make tune'" }
if (-not (Test-Path $db)) { throw "no .dixiedata/ archive" }

# Pick representative record IDs from the live DB. Hard-coded so the
# round is reproducible. Use the first soldier, first widow.
$soldierID = 1
$widowID = 61

# Output filename for this round.
$outName = if ($Round -eq 1) { "pre-iteration.pdf" } else { "round-$Round.pdf" }

function Render-Record {
    param(
        [string]$Surface,
        [string]$Template,
        [string]$Orientation,
        [int64]$RecordID
    )
    $dir = Join-Path $repoRoot "docs/renderings/$Surface"
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $out = Join-Path $dir $outName
    $args = @(
        "--db", $db,
        "--typst", $typst,
        "--templates", $templates,
        "render",
        "--template", $Template,
        "--mode", "record",
        "--record", $RecordID,
        "--orientation", $Orientation,
        "--out", $out
    )
    Write-Host "  $Surface -> $out"
    & $tune @args | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "tune failed for $Surface" }
}

function Render-Bulk {
    param(
        [string]$Surface,
        [string]$GroupFlag = ""
    )
    $dir = Join-Path $repoRoot "docs/renderings/$Surface"
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $out = Join-Path $dir $outName
    $args = @(
        "--db", $db,
        "--typst", $typst,
        "--templates", $templates,
        "render",
        "--template", "bulk_soldier",
        "--mode", "bulk",
        "--sort-by", "last_name",
        "--out", $out
    )
    if ($GroupFlag -ne "") { $args += @($GroupFlag) }
    Write-Host "  $Surface -> $out"
    & $tune @args | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "tune failed for $Surface" }
}

function Render-Month {
    param(
        [string]$Surface,
        [int]$Month = 5
    )
    $dir = Join-Path $repoRoot "docs/renderings/$Surface"
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $out = Join-Path $dir $outName
    $args = @(
        "--db", $db,
        "--typst", $typst,
        "--templates", $templates,
        "anniversary",
        "--month", $Month,
        "--orientation", "P",
        "--out", $out
    )
    Write-Host "  $Surface (month $Month) -> $out"
    & $tune @args | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "tune failed for $Surface" }
}

function Render-Insights {
    param(
        [string]$Surface
    )
    $dir = Join-Path $repoRoot "docs/renderings/$Surface"
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $out = Join-Path $dir $outName
    $args = @(
        "--db", $db,
        "--typst", $typst,
        "--templates", $templates,
        "insights",
        "--orientation", "P",
        "--out", $out
    )
    Write-Host "  $Surface -> $out"
    & $tune @args | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "tune failed for $Surface" }
}

Write-Host "=== Rendering round $Round ==="

Render-Record -Surface "single-soldier-landscape" -Template "soldier_landscape" -Orientation "L" -RecordID $soldierID
Render-Record -Surface "single-soldier-portrait"  -Template "soldier_portrait"  -Orientation "P" -RecordID $soldierID
Render-Record -Surface "single-widow-landscape"   -Template "widow_landscape"   -Orientation "L" -RecordID $widowID
Render-Record -Surface "single-widow-portrait"    -Template "widow_portrait"    -Orientation "P" -RecordID $widowID

Render-Bulk -Surface "bulk-sorted"
Render-Bulk -Surface "bulk-grouped-pension-state" -GroupFlag "--group-by-pension-state"
Render-Bulk -Surface "bulk-grouped-burial-location" -GroupFlag "--group-by-buried-in"

Render-Month -Surface "anniversary"
Render-Insights -Surface "insights"

Write-Host "=== Round $Round complete ==="
Write-Host "Open docs/renderings/<surface>/$outName alongside docs/renderings/<surface>/review.md"
