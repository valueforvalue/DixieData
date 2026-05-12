param(
    [switch]$Run,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $root "build-common.ps1")

Set-DixieDataBuildLocation -Root $root
Invoke-DixieDataBuild -Root $root

$launcherPath = Write-DixieDataDebugLauncher -Root $root
Write-Host "Debug build ready:" (Join-Path (Get-DixieDataBuildBinDir -Root $root) "DixieData.exe")
Write-Host "Debug launcher ready:" $launcherPath

if ($Run) {
    & $launcherPath @AppArgs
    exit $LASTEXITCODE
}
