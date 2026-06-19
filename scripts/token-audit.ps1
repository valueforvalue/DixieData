<#
.SYNOPSIS
    Token-saver audit — scans DixieData workspace for context-flooding artifacts.

.DESCRIPTION
    Reports:
      1. DB files (.db / -wal / -shm / -journal / .sqlite*) outside .dixiedata/ + release/
      2. Generated code tracked in git (*_templ.go, frontend/wailsjs/)
      3. Test files using file-path sql.Open without :memory: fallback
      4. Build commands missing quiet flags or log redirection

    Emits findings grouped by severity. Exits 0 always (advisory).
#>

[CmdletBinding()]
param()

$ErrorActionPreference = "Continue"
$root = (Get-Location).Path
$findings = [ordered]@{
    CRITICAL = @()
    HIGH     = @()
    MEDIUM   = @()
    LOW      = @()
}

function Add-Finding {
    param([string]$Severity, [string]$Message)
    $findings[$Severity] += $Message
}

# --- 1. DB leak scan ---
Write-Host "[1/5] Scanning for database leaks..." -ForegroundColor Cyan
$dbPatterns = @('*.db', '*.db-shm', '*.db-wal', '*.sqlite', '*.sqlite3', '*-wal', '*-shm', '*-journal')
$dbFiles = Get-ChildItem -Path $root -Recurse -File -Include $dbPatterns -ErrorAction SilentlyContinue |
    Where-Object {
        $full = $_.FullName
        -not $full.Contains([IO.Path]::Combine($root, '.git')) -and
        -not $full.Contains([IO.Path]::Combine($root, 'build', 'log')) -and
        -not $full.Contains([IO.Path]::Combine($root, '.dixiedata')) -and
        -not $full.Contains([IO.Path]::Combine($root, 'release')) -and
        -not $full.Contains([IO.Path]::Combine($root, 'tests', 'goldmaster', 'artifacts')) -and
        -not $full.Contains([IO.Path]::Combine($root, 'tests', 'stress', 'artifacts'))
    }
foreach ($f in $dbFiles) {
    $rel = $f.FullName.Substring($root.Length).TrimStart('\', '/')
    Add-Finding HIGH "DB leak: $rel"
}

# --- 2. Generated code tracked in git ---
Write-Host "[2/5] Checking git index for generated code..." -ForegroundColor Cyan
$gitRoot = & git rev-parse --show-toplevel 2>$null
if ($gitRoot) {
    $tracked = & git ls-files 2>$null
    foreach ($file in $tracked) {
        if ($file -match '_templ\.go$') {
            Add-Finding CRITICAL "Tracked generated: $file  (git rm --cached)"
        } elseif ($file -match '^frontend/wailsjs/') {
            Add-Finding CRITICAL "Tracked generated: $file  (git rm -r --cached frontend/wailsjs/)"
        }
    }
} else {
    Add-Finding LOW "Not a git repository — skipped git tracking scan."
}

# --- 3. Test config scan (sql.Open without :memory:) ---
Write-Host "[3/5] Scanning test files for hardcoded DB paths..." -ForegroundColor Cyan
$testFiles = Get-ChildItem -Path $root -Recurse -File -Filter '*_test.go' -ErrorAction SilentlyContinue
foreach ($tf in $testFiles) {
    $rel = $tf.FullName.Substring($root.Length).TrimStart('\', '/')
    $content = Get-Content -Path $tf.FullName -Raw -ErrorAction SilentlyContinue
    if (-not $content) { continue }
    if ($content -match 'sql\.Open\(' -and $content -notmatch ':memory:') {
        Add-Finding MEDIUM "Test uses file-path DB: $rel  (consider :memory: when TEST_DB_DSN unset)"
    }
}

# --- 4. Quiet-flag + log-redirection check ---
Write-Host "[4/5] Checking quiet-flag + log-redirection coverage..." -ForegroundColor Cyan

$makefilePath = Join-Path $root 'Makefile'
if (Test-Path $makefilePath) {
    $mf = Get-Content $makefilePath -Raw
    # Token-saver convention: build recipes use tee to build/log/<target>.log (multiline-safe)
    $hasLogRedirect = [regex]::IsMatch($mf, 'build/log/.*?tee', [System.Text.RegularExpressions.RegexOptions]::Singleline) -or
                      [regex]::IsMatch($mf, 'tee\s+\$\(LOGDIR\)', [System.Text.RegularExpressions.RegexOptions]::Singleline)
    if (-not $hasLogRedirect) {
        Add-Finding HIGH "Makefile: no 'tee build/log/' log redirection. Add to build recipes so verbose output stays off agent context."
    }
    # Go test should use -short
    $goTestLines = Select-String -Path $makefilePath -Pattern 'go test' -SimpleMatch
    foreach ($line in $goTestLines) {
        if ($line.Line -notmatch '\-short') {
            Add-Finding MEDIUM "Makefile line $($line.LineNumber): 'go test' without '-short'. Add to skip integration tests that flood logs."
        }
    }
    # wails build should use -clean + -trimpath (or be inside a script that does)
    $wailsBuildLines = Select-String -Path $makefilePath -Pattern 'wails build' -SimpleMatch
    foreach ($line in $wailsBuildLines) {
        if ($line.Line -notmatch '\-clean' -or $line.Line -notmatch '\-trimpath') {
            Add-Finding LOW "Makefile line $($line.LineNumber): 'wails build' missing -clean/-trimpath quiet flags."
        }
    }
} else {
    Add-Finding HIGH "Makefile missing — run 'make init' or Phase 2 of token-saver."
}

# Script files (inner helpers invoked from Makefile) — flag if raw `go test` or verbose commands leak without redirect
$scriptFiles = Get-ChildItem -Path $root -Recurse -File -Include '*.ps1' -ErrorAction SilentlyContinue
foreach ($sf in $scriptFiles) {
    $rel = $sf.FullName.Substring($root.Length).TrimStart('\', '/')
    if ($rel -match 'token-(audit|clean)\.ps1') { continue }
    if ($rel -match 'build-common\.ps1$') { continue }
    $content = Get-Content -Path $sf.FullName -Raw -ErrorAction SilentlyContinue
    # Inner scripts that bypass Makefile+tee and run `go test` directly flood captured stream
    if ($content -match 'go test') {
        Add-Finding LOW "Raw 'go test' in $rel — call from Makefile target so log capture applies."
    }
}

# --- 5. Report ---
Write-Host ""
Write-Host "===== TOKEN-SAVER AUDIT =====" -ForegroundColor Magenta
Write-Host ""
$sevOrder = @('CRITICAL', 'HIGH', 'MEDIUM', 'LOW')
foreach ($sev in $sevOrder) {
    $items = $findings[$sev]
    if ($items.Count -eq 0) {
        Write-Host "$sev : 0" -ForegroundColor Green
    } else {
        $color = switch ($sev) {
            'CRITICAL' { 'Red' }
            'HIGH'     { 'Yellow' }
            'MEDIUM'   { 'Cyan' }
            'LOW'      { 'Gray' }
        }
        Write-Host "$sev : $($items.Count)" -ForegroundColor $color
        foreach ($item in $items) {
            Write-Host "  - $item" -ForegroundColor $color
        }
    }
}
Write-Host ""
Write-Host "Reminder: route builds through Makefile; verbose streams go to build/log/*.log so agent context stays clean." -ForegroundColor Magenta
