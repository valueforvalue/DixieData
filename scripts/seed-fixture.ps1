<#
.SYNOPSIS
    Seeds a fresh DixieData Local Archive with a deterministic fixture that
    exercises every feature surface (Person Records, Source Records, Tags,
    Events, Articles — the latter with the markdown render path so the
    goldmark + bluemonday pipeline is exercised end-to-end).

.DESCRIPTION
    The fixture is designed for:
      - Manual smoke testing of new features
      - RC1 cohort exercise of the v58-v65 surface
      - Ad-hoc UI surface verification (the Articles list/detail/revisions
        panel all have content)
      - Verifying the markdown article render path (issue #523)

    The script is a thin wrapper around `DixieData.exe --seed`. It:
      1. Backs up the current data dir if --backup is passed.
      2. Resolves the data dir (default: appdata.DefaultDir()).
      3. Invokes DixieData.exe --seed with the v58-v65 surface flags.
      4. Prints a one-line summary of what landed where.

    The seed target is the same `internal/seed.Generate` function the
    standalone cmd/seed-data binary calls, so the output is byte-for-byte
    identical between the two entry points.

.PARAMETER DataDir
    The DixieData Local Archive directory to seed. Defaults to the
    standard per-OS appdata path (the same one DixieData.exe uses by
    default). Pass an explicit path when running against a non-default
    install (e.g. the OneDrive release layout).

.PARAMETER Soldiers
    Number of soldiers to generate. Default 250.

.PARAMETER Reset
    Wipe the existing DB + images before seeding. Without this flag the
    seed is additive: it adds tags/events/articles to an existing archive
    without touching existing soldier rows (uses --skip-soldiers
    internally). With --reset it wipes the archive and starts clean.

.PARAMETER ArticlesFormat
    'markdown' (default) or 'plain'. Markdown showcases the goldmark
    render path and is the recommended choice for surfacing the new
    Article rendering. Plain is the legacy <p>-wrapped prose form.

.PARAMETER Backup
    If set, copy the current data dir to <data-dir>.backup-<timestamp>
    before any destructive operation. Always run with --Backup --Reset on
    a production archive.

.PARAMETER JSON
    Emit a JSON summary line instead of human-readable text. Useful for
    CI scripts.

.EXAMPLE
    .\seed-fixture.ps1
    # Seeds the default data dir with 250 soldiers + full v58-v65
    # surface (tags/events/articles, markdown format). Does NOT wipe
    # the existing archive.

.EXAMPLE
    .\seed-fixture.ps1 -DataDir "C:\Users\value\OneDrive\Desktop\DixieData-test" -Reset -Backup
    # Backs up the test archive, wipes it, seeds a fresh fixture.

.EXAMPLE
    .\seed-fixture.ps1 -DataDir "C:\Users\value\OneDrive\Desktop\DixieData-test" -SkipSoldiers -Tags 30 -Events 50 -Articles 10
    # Adds 30 tags, 50 events, 10 markdown articles to the existing
    # archive without touching the soldier table. Good for verifying
    # the v58-v65 surface on a real production archive.

.NOTES
    Issue #667 (future): this script will be retired once the in-app
    `dixiedata seed` positional subcommand lands. For now the flag form
    is the most discoverable surface.
#>
[CmdletBinding()]
param(
    [string]$DataDir = "",
    [int]$Soldiers = 250,
    [int]$Seed = 1865,
    [switch]$Reset,
    [switch]$SkipSoldiers,
    [int]$Tags = 0,
    [int]$Articles = 0,
    [ValidateSet("markdown", "plain")]
    [string]$ArticlesFormat = "markdown",
    [int]$Events = 0,
    [switch]$Backup,
    [switch]$JSON
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$ExePath = Join-Path $ScriptDir "DixieData.exe"

if (-not (Test-Path $ExePath)) {
    throw "DixieData.exe not found at $ExePath. Run this script from the root of an extracted release archive."
}

# Resolve default data dir when none passed. Mirrors appdata.DefaultDir()
# on Windows: %APPDATA%\DixieData
if ([string]::IsNullOrWhiteSpace($DataDir)) {
    $appdata = [Environment]::GetFolderPath("ApplicationData")
    $DataDir = Join-Path $appdata "DixieData"
}

Write-Host "DixieData fixture seeder" -ForegroundColor Cyan
Write-Host "  data dir: $DataDir"
Write-Host "  soldiers: $Soldiers (skip=$($SkipSoldiers.IsPresent))"
Write-Host "  reset:    $($Reset.IsPresent)"
Write-Host "  surface:  tags=$Tags events=$Events articles=$Articles format=$ArticlesFormat"
Write-Host ""

# Optional: back up the existing archive before any destructive op.
if ($Backup -and (Test-Path $DataDir)) {
    $ts = Get-Date -Format "yyyyMMdd-HHmmss"
    $backupDir = "$DataDir.backup-$ts"
    Write-Host "Backing up $DataDir -> $backupDir ..." -ForegroundColor Yellow
    Copy-Item -Path $DataDir -Destination $backupDir -Recurse -Force
    Write-Host "  done." -ForegroundColor Yellow
    Write-Host ""
}

# Build the argument vector for DixieData.exe.
$args = @("--seed", "--data-dir", $DataDir, "--rng-seed", "$Seed")
if ($Reset) { $args += "--reset" }
if ($SkipSoldiers) { $args += "--skip-soldiers" }
if ($Tags -gt 0) { $args += @("--tags", "$Tags") }
if ($Articles -gt 0) { $args += @("--articles", "$Articles") }
$args += @("--articles-format", $ArticlesFormat)
if ($Events -gt 0) { $args += @("--events", "$Events") }
if ($JSON) { $args += "--seed-json" }

# For the soldiers count, only pass when skip-soldiers is OFF. When
# --skip-soldiers is set, --soldiers is ignored by the seeder anyway.
if (-not $SkipSoldiers) {
    # Insert --soldiers right after --seed so ParseSeedOptions sees it.
    $args = @("--seed", "--soldiers", "$Soldiers") + ($args | Select-Object -Skip 1)
}

Write-Host "Running: DixieData.exe $($args -join ' ')" -ForegroundColor DarkGray
Write-Host ""

$proc = Start-Process -FilePath $ExePath -ArgumentList $args -NoNewWindow -Wait -PassThru
$exitCode = $proc.ExitCode

if ($exitCode -ne 0) {
    Write-Host "Seed failed with exit code $exitCode." -ForegroundColor Red
    exit $exitCode
}

Write-Host ""
Write-Host "Seed complete." -ForegroundColor Green
Write-Host "  Open the Wails app against $DataDir to browse the fixture."
Write-Host "  Articles use the $ArticlesFormat body format. Markdown showcases the goldmark render path."
