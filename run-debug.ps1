param(
    [switch]$Rebuild,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $root "build-common.ps1")

Set-DixieDataBuildLocation -Root $root

$exePath = Join-Path (Get-DixieDataBuildBinDir -Root $root) "DixieData.exe"
if ($Rebuild -or -not (Test-Path $exePath)) {
    & (Join-Path $root "build-debug.ps1")
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
