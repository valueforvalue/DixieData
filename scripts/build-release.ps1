param(
    [switch]$Archive,
    [string]$LDFlags = ""
)

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $scriptRoot "build-common.ps1")

$root = Get-DixieDataRoot -StartPath $scriptRoot

Set-DixieDataBuildLocation -Root $root
Invoke-DixieDataBuild -Root $root -AllowExampleOAuthDefaults -ExtraLDFlags $LDFlags

$binDir = Get-DixieDataBuildBinDir -Root $root
Write-Host "Release build ready:" (Join-Path $binDir "DixieData.exe")

if ($Archive) {
    $releaseDir = Join-Path $root "release"
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
    $appVersion = Get-DixieDataAppVersion -Root $root
    $releaseTag = ""
    if (-not [string]::IsNullOrWhiteSpace($LDFlags)) {
        $tagMatch = [regex]::Match($LDFlags, 'CurrentReleaseTag=([^\s"]+)')
        if ($tagMatch.Success) {
            $releaseTag = "-" + $tagMatch.Groups[1].Value
        }
    }

    $archivePath = Join-Path $releaseDir ("DixieData-release-{0}{1}.zip" -f $appVersion, $releaseTag)
    if (Test-Path $archivePath) {
        Remove-Item $archivePath -Force
    }

    Compress-Archive -Path (Join-Path $binDir "*") -DestinationPath $archivePath -Force
    Write-Host "Release archive ready:" $archivePath
}
