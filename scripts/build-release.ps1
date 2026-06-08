param(
    [switch]$Archive
)

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot

Set-DixieDataBuildLocation -Root $root
Invoke-DixieDataBuild -Root $root -AllowExampleOAuthDefaults

$binDir = Get-DixieDataBuildBinDir -Root $root
Write-Host "Release build ready:" (Join-Path $binDir "DixieData.exe")

if ($Archive) {
    $releaseDir = Join-Path $root "release"
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
    $appVersion = Get-DixieDataAppVersion -Root $root

    $archivePath = Join-Path $releaseDir ("DixieData-release-{0}.zip" -f $appVersion)
    if (Test-Path $archivePath) {
        Remove-Item $archivePath -Force
    }

    Compress-Archive -Path (Join-Path $binDir "*") -DestinationPath $archivePath -Force
    Write-Host "Release archive ready:" $archivePath
}
