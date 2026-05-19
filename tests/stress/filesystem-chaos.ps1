param(
    [string]$ArtifactRoot = (Join-Path $PSScriptRoot "artifacts")
)

$ErrorActionPreference = "Stop"

$helper = Join-Path $PSScriptRoot "..\..\scripts\build-common.ps1"
. $helper

$root = Get-DixieDataRoot -StartPath $PSScriptRoot
Set-Location $root

$chaosRoot = Join-Path $ArtifactRoot "filesystem-chaos"
New-Item -ItemType Directory -Force -Path $chaosRoot | Out-Null

$dummyFile = Join-Path $chaosRoot "dummy-1gb.bin"
if (Test-Path $dummyFile) {
    Remove-Item $dummyFile -Force
}

try {
    $file = [System.IO.File]::Create($dummyFile)
    $file.SetLength(1GB)
    $file.Close()
} catch {
    if ($file) { $file.Dispose() }
    throw
}

Write-Host "Created sparse 1GB dummy file: $dummyFile"
Write-Host "Running in-process filesystem chaos stress test..."
go test .\internal\appshell -run TestStressFilesystemChaosGracefulFailure -count=1
