param(
    [switch]$Rebuild,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot

Set-DixieDataBuildLocation -Root $root

$exePath = Join-Path (Get-DixieDataBuildBinDir -Root $root) "DixieData.exe"
if ($Rebuild -or -not (Test-Path $exePath)) {
    & (Join-Path $scriptRoot "build-debug.ps1")
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}

$launcherPath = Join-Path (Get-DixieDataBuildBinDir -Root $root) "Run-DixieData-Debug.ps1"
if (-not (Test-Path $launcherPath)) {
    $launcherPath = Write-DixieDataDebugLauncher -Root $root
}

& $launcherPath @AppArgs
exit $LASTEXITCODE
