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

Write-Host "==> Wipe stress artifacts"
if (Test-Path $resolvedArtifactRoot) {
    Get-ChildItem -Path $resolvedArtifactRoot -Force | Where-Object { $_.FullName -ne $logPath } | Remove-Item -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $resolvedArtifactRoot | Out-Null

Invoke-StressStep "Generate garbage DB and corrupt import coverage" {
    go test .\tests\stress -count=1
}

Invoke-StressStep "Run Find a Grave fuzz target" {
    go test .\tests\stress -run '^$' -fuzz FuzzFindAGraveParseHTMLDoesNotPanic -fuzztime 5s
}

Invoke-StressStep "Hammer bridge with race detector" {
    if ((go env CGO_ENABLED).Trim() -eq "1") {
        go test -race . -run TestStressBridgeConcurrentSearchAndSave -count=1
    } else {
        Write-Host "CGO is disabled; falling back to non-race bridge hammer run."
        go test . -run TestStressBridgeConcurrentSearchAndSave -count=1
    }
}

Invoke-StressStep "Capture stdout/stderr stress log" {
    go test . -run TestStressAppLoggingToFile -count=1
}

Invoke-StressStep "Run filesystem chaos script" {
    & (Join-Path $root "tests\stress\filesystem-chaos.ps1") -ArtifactRoot $resolvedArtifactRoot
}

Invoke-StressStep "Audit stress log" {
    python (Join-Path $root "tests\stress\analyze_stress_log.py") $logPath
}

Write-Host "Stress suite complete. Report: tests\stress\top-break-points.txt"
