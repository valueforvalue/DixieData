param(
    [switch]$Run,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot

Set-DixieDataBuildLocation -Root $root
Invoke-DixieDataBuild -Root $root

$launcherPath = Write-DixieDataDebugLauncher -Root $root
Write-Host "Debug build ready:" (Join-Path (Get-DixieDataBuildBinDir -Root $root) "DixieData.exe")
Write-Host "Debug launcher ready:" $launcherPath

if ($Run) {
    & $launcherPath @AppArgs
    exit $LASTEXITCODE
}
