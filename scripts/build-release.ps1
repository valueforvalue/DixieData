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

    # Build the staging list: binDir contents first, then the
    # seed-fixture helper script. The script is a thin wrapper
    # around `DixieData.exe --seed`; shipping it in the release
    # zip lets RC1 cohort + QA users populate a fresh archive
    # without installing Go (issue #667 follow-up).
    $stageRoot = Join-Path $env:TEMP "dixiedata-release-stage"
    if (Test-Path $stageRoot) { Remove-Item $stageRoot -Recurse -Force }
    New-Item -ItemType Directory -Path $stageRoot | Out-Null
    Copy-Item -Path (Join-Path $binDir "*") -Destination $stageRoot -Recurse -Force
    $seedScript = Join-Path $root "scripts/seed-fixture.ps1"
    if (Test-Path $seedScript) {
        Copy-Item -Path $seedScript -Destination (Join-Path $stageRoot "seed-fixture.ps1") -Force
    }

    Compress-Archive -Path (Join-Path $stageRoot "*") -DestinationPath $archivePath -Force
    Remove-Item $stageRoot -Recurse -Force
    Write-Host "Release archive ready:" $archivePath
}
