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

# Compile-and-run instead of `go test` directly to avoid Windows AV races
# that pollute $LASTEXITCODE when go's tmpdir cleanup runs at process exit
# (unlinkat ... Access is denied). See scripts/run-stress-tests.ps1 for the
# same pattern in Invoke-GoTestCompile.
$binPath = Join-Path $chaosRoot ("chaos-" + [guid]::NewGuid().ToString() + ".exe")
try {
    go test -c -o $binPath .\internal\appshell -run TestStressFilesystemChaosGracefulFailure -count=1
    if ($LASTEXITCODE -ne 0) { throw "Compile failed (exit $LASTEXITCODE)" }
    & $binPath
    if ($LASTEXITCODE -ne 0) { throw "Chaos test failed (exit $LASTEXITCODE)" }
} finally {
    if (Test-Path $binPath) {
        for ($i = 0; $i -lt 3; $i++) {
            try { Remove-Item $binPath -Force -ErrorAction Stop; break }
            catch { Start-Sleep -Milliseconds 500 }
        }
    }
}
