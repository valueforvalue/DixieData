param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$ErrorActionPreference = "Continue"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root     = Get-DixieDataRoot -StartPath $scriptRoot
$binDir   = Get-DixieDataBuildBinDir -Root $root
$exePath  = Join-Path $binDir "DixieData.exe"

if (-not (Test-Path $exePath)) {
    throw "DixieData.exe not found at $exePath — run 'make debug' first."
}

$stamp    = Get-Date -Format "yyyyMMdd-HHmmss"
$logDir   = Join-Path $binDir "crashlogs"
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$logPath  = Join-Path $logDir  ("crash-$stamp.log")
$corePath = Join-Path $logDir  ("core-$stamp.dmp")

# Capture stderr + stdout. Tee to console AND file.
$env:GOTRACEBACK     = "crash"     # full goroutine stacks on panic
$env:GOTRACEBACKALL  = "1"         # include all goroutines
$env:GOTMPDIR        = $logDir     # write minidump-style core if supported

Write-Host "Running $exePath"
Write-Host "Log:    $logPath"
Write-Host "Core:   $corePath"
Write-Host "---"

& $exePath @AppArgs 2>&1 | Tee-Object -FilePath $logPath
$exit = $LASTEXITCODE

Write-Host "---"
Write-Host "Exit code: $exit"

if ($exit -ne 0) {
    Write-Host "CRASH detected — writing core file..."
    # Windows: trigger a manual dump via procdump if installed; else rely on stderr log.
    $procdump = Get-Command procdump.exe -ErrorAction SilentlyContinue
    if ($procdump) {
        & procdump -ma -e 1 $exePath $corePath | Out-Null
        Write-Host "Core: $corePath"
    } else {
        Write-Host "procdump not installed; stderr log already captured at $logPath"
        Write-Host "Install with: choco install procdump  (or download Sysinternals procdump.exe into PATH)"
    }
    exit $exit
}