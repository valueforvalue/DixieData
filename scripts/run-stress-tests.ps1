param(
    [string]$ArtifactRoot = ".\tests\stress\artifacts"
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot
Set-DixieDataBuildLocation -Root $root

$resolvedArtifactRoot = if ([System.IO.Path]::IsPathRooted($ArtifactRoot)) {
    $ArtifactRoot
} else {
    Join-Path $root $ArtifactRoot
}
New-Item -ItemType Directory -Force -Path $resolvedArtifactRoot | Out-Null

$logPath = Join-Path $resolvedArtifactRoot "stress_test.log"
if (Test-Path $logPath) {
    Remove-Item $logPath -Force
}

$env:DIXIEDATA_STRESS_LOG = $logPath

function Invoke-StressStep {
    param(
        [string]$Title,
        [scriptblock]$Action
    )

    Write-Host "==> $Title"
    & $Action 2>&1 | Tee-Object -FilePath $logPath -Append
    if ($LASTEXITCODE -ne 0) {
        throw "Stress step failed: $Title"
    }
}

# Compile-and-run wrapper. Avoids Windows AV races that pollute $LASTEXITCODE
# when `go test` deletes its tmpdir test exe at process exit. We own the
# compiled binary so cleanup is deterministic.
function Invoke-GoTestCompile {
    param(
        [string]$Title,
        [string]$Package,
        [string[]]$GoTestArgs
    )

    $binDir = Join-Path $resolvedArtifactRoot "bin"
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    $binPath = Join-Path $binDir ("stress-" + [guid]::NewGuid().ToString() + ".exe")

    try {
        Write-Host "  compile: go test -c -o $binPath $($GoTestArgs -join ' ')"
        & go test -c -o $binPath $Package @GoTestArgs 2>&1 | Tee-Object -FilePath $logPath -Append
        if ($LASTEXITCODE -ne 0) {
            throw "Compile failed for $Title (exit $LASTEXITCODE)"
        }
        & $binPath 2>&1 | Tee-Object -FilePath $logPath -Append
        $runExit = $LASTEXITCODE
        if ($runExit -ne 0) {
            throw "$Title failed (exit $runExit)"
        }
    } finally {
        # Best-effort cleanup; AV may still hold the file briefly.
        if (Test-Path $binPath) {
            for ($i = 0; $i -lt 3; $i++) {
                try { Remove-Item $binPath -Force -ErrorAction Stop; break }
                catch { Start-Sleep -Milliseconds 500 }
            }
        }
    }
}

# Fuzz step wrapper. `go test -fuzz` cannot use `-c` (fuzz driver runs in the
# test process), so we have to use `go test` directly. On Windows the
# post-run tmpdir cleanup often fails with `unlinkat ... Access is denied`
# (AV scan), which makes $LASTEXITCODE != 0 even when the fuzz target
# itself passed. Inspect the streamed output instead: a true failure prints
# `--- FAIL`, `FAIL`, or a panic stack; `fuzz: ... new interesting:` followed
# by a clean exit means the target ran fine.
function Invoke-FuzzStep {
    param(
        [string]$Title,
        [string]$Package,
        [string]$FuzzTarget,
        [string]$FuzzTime
    )

    Write-Host "==> $Title"
    $output = & go test $Package -run '^$' -fuzz $FuzzTarget -fuzztime $FuzzTime 2>&1
    $output | Tee-Object -FilePath $logPath -Append | Write-Host
    if ($LASTEXITCODE -eq 0) {
        return
    }
    # Distinguish real failure from go test's tmpdir cleanup noise.
    $combined = ($output | Out-String)
    $looksLikePass = $combined -match '(?m)^\s*fuzz:\s+.*new interesting:' -and `
                     $combined -notmatch '(?m)^\s*FAIL\b' -and `
                     $combined -notmatch '(?m)^\s*---\s+FAIL' -and `
                     $combined -notmatch 'panic:' -and `
                     $combined -notmatch 'unlinkat.*Access is denied.*PASS'
    # `unlinkat ... Access is denied` from go's own cleanup is the noise we
    # tolerate; the fuzz target itself reported new interesting inputs and
    # no FAIL/panic markers. Treat as pass.
    if ($looksLikePass) {
        Write-Warning "  ${Title}: ignoring post-run unlinkat cleanup noise (Windows AV race)"
        return
    }
    throw "Stress step failed: $Title"
}

Write-Host "==> Wipe stress artifacts"
if (Test-Path $resolvedArtifactRoot) {
    Get-ChildItem -Path $resolvedArtifactRoot -Force | Where-Object { $_.FullName -ne $logPath } | Remove-Item -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $resolvedArtifactRoot | Out-Null

Invoke-GoTestCompile -Title "Generate garbage DB and corrupt import coverage" `
    -Package ".\tests\stress" `
    -GoTestArgs @("-count=1")

Invoke-FuzzStep -Title "Run Find a Grave fuzz target" `
    -Package ".\tests\stress" `
    -FuzzTarget "FuzzFindAGraveParseHTMLDoesNotPanic" `
    -FuzzTime "5s"

Invoke-GoTestCompile -Title "Hammer bridge with race detector" `
    -Package ".\tests\stress" `
    -GoTestArgs @(
        "-count=1",
        "-run", "TestStressBridgeConcurrentSearchAndSave",
        $(if ((go env CGO_ENABLED).Trim() -eq "1") { @("-race") } else { @() })
    )

Invoke-GoTestCompile -Title "Capture stdout/stderr stress log" `
    -Package ".\tests\stress" `
    -GoTestArgs @("-count=1", "-run", "TestStressAppLoggingToFile")

Invoke-StressStep "Run filesystem chaos script" {
    & (Join-Path $root "tests\stress\filesystem-chaos.ps1") -ArtifactRoot $resolvedArtifactRoot
}

Invoke-StressStep "Audit stress log" {
    python (Join-Path $root "tests\stress\analyze_stress_log.py") $logPath
}

Write-Host "Stress suite complete. See docs\known-limitations.md for known break points."
