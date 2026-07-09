<#
.SYNOPSIS
    Archive CHANGELOG.md entries older than the current year.

.DESCRIPTION
    Issue #442: DixieData's CHANGELOG.md has grown past 6,000 lines as
    in-year volume mounts. The original year-chunking proposal
    (commit that introduces this script) keeps the active file lean
    by splitting entries with a year older than the current year
    into archive/CHANGELOG-{year}.md files.

    This script is idempotent — re-running it on an already-archived
    file is a no-op. The v1.1.x entries (which have no date in the
    header per the pre-2026 header format) get bucketed into
    `archive/CHANGELOG-legacy.md` so the bucket boundary stays
    clean.

    Run from the repo root:

        pwsh scripts/archive-changelog.ps1

    Wired into `make changelog-archive`.

.OUTPUTS
    0  success
    1  CHANGELOG.md missing or unreadable
    2  archive/ directory could not be created
    3  parsed zero version sections (likely a format regression)

.NOTES
    The script does NOT touch the active [Unreleased] section.
    The active [Unreleased] block always stays in CHANGELOG.md
    regardless of date (it has no `## v...` header so the
    `Split on "## ["` step skips it).
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path "$PSScriptRoot/..").Path
$changelog = Join-Path $repoRoot 'CHANGELOG.md'
$archiveDir = Join-Path $repoRoot 'archive'

if (-not (Test-Path $changelog)) {
    Write-Error "CHANGELOG.md not found at $changelog"
    exit 1
}

$currentYear = (Get-Date).Year
Write-Host "Current year: $currentYear"

if (-not (Test-Path $archiveDir)) {
    New-Item -ItemType Directory -Path $archiveDir -Force | Out-Null
}

# Read the file once; split into the [Unreleased] header (always
# preserved) + an ordered list of version sections.
$content = Get-Content -Raw -Path $changelog -Encoding UTF8

# A version section starts at "## v" and continues until the next
# "## [" (i.e. the next version header) or end-of-file. We use a
# regex split that keeps the delimiters.
$sectionRegex = [regex]'(?ms)^## v[^\n]*\n(?:.*?)(?=^## v|\z)'
$matches = [regex]::Matches($content, $sectionRegex)

if ($matches.Count -eq 0) {
    Write-Error "Parsed zero ## v... sections — format regression?"
    exit 3
}

Write-Host "Parsed $($matches.Count) version sections"

# Bucket the sections by year. Year extraction:
#   "## v1.2.55 - 2026-06-25"                  -> 2026
#   "## v1.2.22 - 2025 (date not captured ...)"-> 2025
#   "## v1.1.16 - Gold Master"                  -> "legacy" (no date)
$buckets = @{}
foreach ($m in $matches) {
    $header = ($m.Value -split "`n")[0]
    $yearMatch = [regex]::Match($header, '\b(20\d{2})\b')
    if ($yearMatch.Success) {
        $year = $yearMatch.Groups[1].Value
    } else {
        $year = 'legacy'
    }
    if (-not $buckets.ContainsKey($year)) {
        $buckets[$year] = New-Object System.Collections.Generic.List[string]
    }
    $buckets[$year].Add($m.Value)
}

# Find the pre-sections prologue (everything before the first
# "## v..." header). This is the file lead-in + the [Unreleased]
# block; we preserve it verbatim in the active file.
$prologue = $content.Substring(0, $matches[0].Index)

# Write per-year archive files.
foreach ($year in ($buckets.Keys | Sort-Object)) {
    if ($year -eq $currentYear.ToString()) {
        # Current year stays in CHANGELOG.md — skip the archive
        # write for this bucket.
        continue
    }
    $archivePath = Join-Path $archiveDir "CHANGELOG-$year.md"
    $body = @()
    $body += "# CHANGELOG (archive: $year)"
    $body += ""
    $body += "Archived from CHANGELOG.md by `scripts/archive-changelog.ps1`."
    $body += "Historical entries only — the active file is `CHANGELOG.md`."
    $body += ""
    foreach ($section in $buckets[$year]) {
        $body += $section.TrimEnd()
        $body += ""
    }
    $archiveContent = ($body -join "`n").TrimEnd() + "`n"
    Set-Content -Path $archivePath -Value $archiveContent -Encoding UTF8 -NoNewline
    Write-Host "Wrote $archivePath ($($buckets[$year].Count) sections)"
}

# Rewrite CHANGELOG.md: prologue + current-year sections only.
$activeBody = New-Object System.Collections.Generic.List[string]
$activeBody.Add($prologue.TrimEnd())
$activeBody.Add("")
$currentYearKey = $currentYear.ToString()
if ($buckets.ContainsKey($currentYearKey)) {
    foreach ($section in $buckets[$currentYearKey]) {
        $activeBody.Add($section.TrimEnd())
        $activeBody.Add("")
    }
}
$activeContent = ($activeBody -join "`n").TrimEnd() + "`n"
Set-Content -Path $changelog -Value $activeContent -Encoding UTF8 -NoNewline

$activeLineCount = (Get-Content $changelog).Count
Write-Host "CHANGELOG.md now has $activeLineCount lines"

exit 0
