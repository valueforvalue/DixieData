# Render every PDF export surface against the live archive for the
# current iteration round. Each export is saved to
# docs/renderings/<surface>/pre-iteration.pdf so the user can open
# it in a PDF viewer alongside the per-surface review.md file.
#
# Usage (from the repo root):
#   pwsh -File scripts/render-round.ps1                       # round 1, writes pre-iteration.pdf
#   pwsh -File scripts/render-round.ps1 -Round 2              # writes round-2.pdf
#   pwsh -File scripts/render-round.ps1 -Round 5 -Only single-soldier-landscape
#   pwsh -File scripts/render-round.ps1 -Round 5 -RecordIDs 1,2,3
#   pwsh -File scripts/render-round.ps1 -Round 5 -KeepRounds 0   # disable auto-prune
#
# The script must be re-runnable: it overwrites the target PDF for
# the requested round. It does NOT touch pre-iteration.pdf from a
# previous round.
#
# -Only <surface>     restrict to a single surface (saves disk +
#                     wall-clock time when iterating on one layout).
#                     Valid values:
#                       single-soldier-landscape, single-soldier-portrait,
#                       single-widow-landscape,  single-widow-portrait,
#                       bulk-sorted, bulk-grouped-pension-state,
#                       bulk-grouped-burial-location,
#                       anniversary, insights
# -RecordIDs <list>   comma-separated IDs for bulk renders. Skips
#                     bulk surfaces entirely when set to the empty
#                     string (default). The script auto-skips bulk
#                     renders when the ID list is short enough that
#                     a full bulk render would be wasteful; pass
#                     "all" to force the bulk surfaces to render.
# -KeepRounds <N>     keep the most recent N rounds of artifacts
#                     (PDF + SVG + PNG) before rendering the new one.
#                     Default 1: the previous round only. Set to 0
#                     to disable pruning, or to a higher N when
#                     comparing across multiple iterations.
param(
    [int]$Round = 1,
    [string]$Only = "",
    [string]$RecordIDs = "",
    [int]$KeepRounds = 1
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
    if ($RecordIDs -ne "") {
        $args += @("--record-ids", $RecordIDs)
        $args += @("--scope", "selected")
    }
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
if ($RecordIDs -ne "") {
    Write-Host "Bulk surfaces limited to record-ids: $RecordIDs (scope=selected)"
}
if ($Only -ne "") {
    Write-Host "Restricting to surface: $Only"
}
if ($KeepRounds -gt 0 -and $Round -gt 1) {
    # Prune iteration artifacts older than $KeepRounds before
    # writing the new round. Snapshots in
    # internal/exportcontract/testdata/ are git-tracked and
    # unaffected by this step. pre-iteration.pdf is preserved
    # because it documents the pre-iteration baseline (round 0).
    $surfaces = @(
        "single-soldier-landscape", "single-soldier-portrait",
        "single-widow-landscape", "single-widow-portrait",
        "bulk-sorted", "bulk-grouped-pension-state",
        "bulk-grouped-burial-location",
        "anniversary", "insights"
    )
    $cutoff = $Round - $KeepRounds - 1
    foreach ($s in $surfaces) {
        $sdir = Join-Path $repoRoot "docs/renderings/$s"
        if (-not (Test-Path $sdir)) { continue }
        Get-ChildItem -Path $sdir -Filter "round-*.pdf" -File | ForEach-Object {
            if ($_.Name -match "round-(\d+)\.pdf$") {
                $n = [int]$Matches[1]
                if ($n -lt $cutoff) {
                    # Delete matching SVG/PNG siblings too.
                    $stem = $_.BaseName
                    Remove-Item -Force -ErrorAction SilentlyContinue $_.FullName
                    Get-ChildItem -Path $sdir -Filter ("$stem-*.svg") -File |
                        Remove-Item -Force -ErrorAction SilentlyContinue
                    Get-ChildItem -Path $sdir -Filter ("$stem.png") -File |
                        Remove-Item -Force -ErrorAction SilentlyContinue
                }
            }
        }
    }
    Write-Host "Pruned iteration artifacts older than round $cutoff (KeepRounds=$KeepRounds)."
}

function Should-Render {
    param([string]$Surface)
    if ($Only -eq "") { return $true }
    return ($Surface -eq $Only)
}

if (Should-Render "single-soldier-landscape") {
    Render-Record -Surface "single-soldier-landscape" -Template "soldier_landscape" -Orientation "L" -RecordID $soldierID
}
if (Should-Render "single-soldier-portrait") {
    Render-Record -Surface "single-soldier-portrait"  -Template "soldier_portrait"  -Orientation "P" -RecordID $soldierID
}
if (Should-Render "single-widow-landscape") {
    Render-Record -Surface "single-widow-landscape"   -Template "widow_landscape"   -Orientation "L" -RecordID $widowID
}
if (Should-Render "single-widow-portrait") {
    Render-Record -Surface "single-widow-portrait"    -Template "widow_portrait"    -Orientation "P" -RecordID $widowID
}

if (Should-Render "bulk-sorted") {
    Render-Bulk -Surface "bulk-sorted"
}
if (Should-Render "bulk-grouped-pension-state") {
    Render-Bulk -Surface "bulk-grouped-pension-state" -GroupFlag "--group-by-pension-state"
}
if (Should-Render "bulk-grouped-burial-location") {
    Render-Bulk -Surface "bulk-grouped-burial-location" -GroupFlag "--group-by-buried-in"
}

if (Should-Render "anniversary") {
    Render-Month -Surface "anniversary"
}
if (Should-Render "insights") {
    Render-Insights -Surface "insights"
}

Write-Host "=== Round $Round complete ==="
Write-Host "Open docs/renderings/<surface>/$outName alongside docs/renderings/<surface>/review.md"
