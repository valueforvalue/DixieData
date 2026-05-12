param(
    [switch]$Archive
)

$root = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $root "build-common.ps1")

Set-DixieDataBuildLocation -Root $root
Invoke-DixieDataBuild -Root $root

$binDir = Get-DixieDataBuildBinDir -Root $root
Write-Host "Release build ready:" (Join-Path $binDir "DixieData.exe")

if ($Archive) {
    $releaseDir = Join-Path $root "release"
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null

    $archivePath = Join-Path $releaseDir ("DixieData-release-{0}.zip" -f (Get-Date -Format "yyyy-MM-dd"))
    if (Test-Path $archivePath) {
        Remove-Item $archivePath -Force
    }

    Compress-Archive -Path (Join-Path $binDir "*") -DestinationPath $archivePath -Force
    Write-Host "Release archive ready:" $archivePath
}
